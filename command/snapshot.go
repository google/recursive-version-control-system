// Copyright 2022 Google LLC
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
	"os"
	"path/filepath"
	"strings"

	"github.com/google/recursive-version-control-system/archive"
	"github.com/google/recursive-version-control-system/snapshot"
)

const snapshotUsage = `Usage: %s snapshot [<FLAGS>]* <PATH>

Where <PATH> is a local filesystem path, and <FLAGS> are one of:

`

var (
	snapshotFlags = flag.NewFlagSet("snapshot", flag.ContinueOnError)

	snapshotAdditionalParentsFlag = snapshotFlags.String(
		"additional-parents", "",
		"comma separated list of additional parents for the generated snapshot")
)

func snapshotCommand(ctx context.Context, s *archive.Store, cmd string, args []string) (int, error) {
	snapshotFlags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), snapshotUsage, cmd)
		snapshotFlags.PrintDefaults()
	}
	if err := snapshotFlags.Parse(args); err != nil {
		return 1, nil
	}
	args = snapshotFlags.Args()

	var additionalParents []*snapshot.Hash
	for _, parent := range strings.Split(*snapshotAdditionalParentsFlag, ",") {
		parentHash, err := resolveSnapshot(ctx, s, parent)
		if err != nil {
			return 1, fmt.Errorf("failure resolving the additional parent %q: %v", parent, err)
		}
		if parentHash != nil {
			additionalParents = append(additionalParents, parentHash)
		}
	}

	var path string
	if len(args) > 0 {
		path = args[0]
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return 1, fmt.Errorf("failure determining the current working directory: %v\n", err)
		}
		path = wd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return 1, fmt.Errorf("failure resolving the absolute path of %q: %v", path, err)
	}
	path = abs

	h, err := archive.Snapshot(ctx, s, snapshot.Path(path), additionalParents...)
	if err != nil {
		return 1, fmt.Errorf("failure snapshotting the directory %q: %v\n", path, err)
	} else if h == nil {
		fmt.Printf("Did not generate a snapshot as %q does not exist\n", path)
		return 1, nil
	}

	fmt.Printf("Snapshotted %q to %q\n", path, h)
	return 0, nil
}
