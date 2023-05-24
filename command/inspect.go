// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package command defines the command line interface for rvcs
package command

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"os"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

const inspectUsage = `Usage: %s inspect [<FLAGS>]* <SOURCE>

Where <SOURCE> is one of:

	The hash of a known snapshot.
	A local file path which has previously been snapshotted.

`

var (
	inspectShortTemplate = template.Must(template.New("short").Parse(
		`{{.LogSize}} snapshots including {{.TotalObjects}} total objects
`))

	inspectLongTemplate = template.Must(template.New("long").Parse(`Hash: {{.Hash}}
	Log entries:		{{.LogSize}}
	Missing log entries:	{{.MissingLogEntries}}
	Total objects:		{{.TotalObjects}}
	Missing objects:	{{.MissingObjects}}
`))
)

var (
	inspectFlags = flag.NewFlagSet("inspect", flag.ContinueOnError)

	inspectShort     bool
	inspectDepthFlag = inspectFlags.Int(
		"depth", -1,
		"maximum depth of the history to traverse. If less than 0, then there is no limit.")
)

func init() {
	inspectFlags.BoolVar(&inspectShort, "short", false,
		"print short output, consisting of just the hash for each snapshot")
	inspectFlags.BoolVar(&inspectShort, "s", false,
		"print short output, consisting of just the hash for each snapshot")
}

type summary struct {
	Hash              *snapshot.Hash
	LogSize           int
	MissingLogEntries int
	TotalObjects      int
	MissingObjects    int
}

func inspectDir(ctx context.Context, s *storage.LocalFiles, summ *summary, h *snapshot.Hash, f *snapshot.File, maxDepth int, visited map[snapshot.Hash]*summary) error {
	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure listing the directory contents of the snapshot %q: %w", h, err)
	}
	// We were able to read the tree, so count that in the total objects...
	summ.TotalObjects += 1
	for _, ch := range tree {
		childInspect, isNew, err := inspect(ctx, s, ch, maxDepth, visited)
		if err != nil {
			return fmt.Errorf("failure inspecting child snapshot %q: %w", ch, err)
		}
		if isNew {
			summ.TotalObjects += childInspect.TotalObjects
			summ.MissingObjects += childInspect.MissingObjects
		}
	}
	return nil
}

func inspectContents(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, f *snapshot.File, maxDepth int, visited map[snapshot.Hash]*summary) (*summary, bool, error) {
	if summ, ok := visited[*f.Contents]; ok {
		return summ, false, nil
	}
	var summ summary
	summ.Hash = f.Contents
	visited[*f.Contents] = &summ
	if f.IsDir() {
		if err := inspectDir(ctx, s, &summ, h, f, maxDepth, visited); err != nil {
			return nil, false, err
		}
		return &summ, true, nil
	}
	r, err := s.ReadObject(ctx, f.Contents)
	if r != nil {
		r.Close()
	}
	if err != nil && os.IsNotExist(err) {
		summ.MissingObjects += 1
	} else if err != nil {
		return nil, false, fmt.Errorf("failure reading a nested object %q: %w", f.Contents, err)
	}
	summ.TotalObjects = 1
	return &summ, true, nil
}

func inspect(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, maxDepth int, visited map[snapshot.Hash]*summary) (*summary, bool, error) {
	if summ, ok := visited[*h]; ok {
		return summ, false, nil
	}
	var summ summary
	summ.Hash = h
	visited[*h] = &summ
	f, err := s.ReadSnapshot(ctx, h)
	if err != nil && os.IsNotExist(err) {
		summ.MissingLogEntries = 1
		summ.MissingObjects = 1
		return &summ, true, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failure reading the snapshot %q: %w", h, err)
	}
	summ.LogSize = 1
	summ.TotalObjects = 1
	if contentsSummary, isNew, err := inspectContents(ctx, s, h, f, maxDepth, visited); err != nil {
		return nil, false, err
	} else if isNew {
		summ.TotalObjects += contentsSummary.TotalObjects
		summ.MissingObjects += contentsSummary.MissingObjects
	}
	if maxDepth == 1 {
		return &summ, true, nil
	}
	for _, parent := range f.Parents {
		if parentSummary, isNew, err := inspect(ctx, s, parent, maxDepth-1, visited); err != nil {
			return nil, false, err
		} else if isNew {
			summ.LogSize += parentSummary.LogSize
			summ.MissingLogEntries += parentSummary.MissingLogEntries
			summ.TotalObjects += parentSummary.TotalObjects
			summ.MissingObjects += parentSummary.MissingObjects
		}
	}
	return &summ, true, nil
}

func inspectCommand(ctx context.Context, s *storage.LocalFiles, cmd string, args []string) (int, error) {
	inspectFlags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), inspectUsage, cmd)
		inspectFlags.PrintDefaults()
	}
	if err := inspectFlags.Parse(args); err != nil {
		return 1, nil
	}
	args = inspectFlags.Args()
	if len(args) != 1 {
		fmt.Fprintf(flag.CommandLine.Output(), inspectUsage, cmd)
		inspectFlags.PrintDefaults()
		return 1, nil
	}
	h, err := resolveSnapshot(ctx, s, args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the snapshot hash for %q: %w", args[0], err)
	}
	visited := make(map[snapshot.Hash]*summary)
	sum, _, err := inspect(ctx, s, h, *inspectDepthFlag, visited)
	if err != nil {
		return 1, fmt.Errorf("failure reading the log for %q: %w", args[0], err)
	}
	tmpl := inspectLongTemplate
	if inspectShort {
		tmpl = inspectShortTemplate
	}
	if err := tmpl.Execute(os.Stdout, sum); err != nil {
		return 1, fmt.Errorf("failure executing the inspect output template: %w", err)
	}
	return 0, nil
}
