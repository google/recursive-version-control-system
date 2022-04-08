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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/recursive-version-control-system/bundle"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

const exportUsage = `Usage: %s export [<FLAGS>]* <PATH>

Where <PATH> is a local filesystem path for the newly generated bundle, and <FLAGS> are one of:

`

var (
	exportFlags = flag.NewFlagSet("export", flag.ContinueOnError)

	exportSnapshotsFlag = exportFlags.String(
		"snapshots", "",
		"comma separated list of snapshots to include in the exported bundle")
	exportExcludeFlag = exportFlags.String(
		"exclude", "",
		("comma separated list of objects to exclude from the exported bundle." +
			"This takes precedence over the `snapshots` flag, so a hash specified " +
			"in both flags will not be included in the bundle."))
	exportMetadataFlag = exportFlags.String(
		"metadata", "",
		"comma separated list of key=value pairs to include in the exported bundle")
	exportIncludeParentsFlag = exportFlags.Bool(
		"include-parents", false,
		"if true, then the exported bundle will recursively include the parents of selected snapshots")
)

func exportCommand(ctx context.Context, s *storage.LocalFiles, cmd string, args []string) (int, error) {
	exportFlags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), exportUsage, cmd)
		exportFlags.PrintDefaults()
	}
	if err := exportFlags.Parse(args); err != nil {
		return 1, nil
	}
	args = exportFlags.Args()
	if len(args) < 1 {
		fmt.Fprintf(flag.CommandLine.Output(), exportUsage, cmd)
		exportFlags.PrintDefaults()
		return 1, nil
	}

	var snapshots []*snapshot.Hash
	for _, s := range strings.Split(*exportSnapshotsFlag, ",") {
		if len(s) == 0 {
			continue
		}
		h, err := snapshot.ParseHash(s)
		if err != nil {
			return 1, fmt.Errorf("failure parsing snapshot hash %q: %v", s, err)
		}
		if h != nil {
			snapshots = append(snapshots, h)
		}
	}

	var exclude []*snapshot.Hash
	for _, e := range strings.Split(*exportExcludeFlag, ",") {
		if len(e) == 0 {
			continue
		}
		h, err := snapshot.ParseHash(e)
		if err != nil {
			return 1, fmt.Errorf("failure parsing exclude hash %q: %v", e, err)
		}
		if h != nil {
			exclude = append(exclude, h)
		}
	}

	metadata := make(map[string]io.Reader)
	for _, pair := range strings.Split(*exportMetadataFlag, ",") {
		if len(pair) == 0 {
			continue
		}
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			return 1, fmt.Errorf("malformed key=value pair %q", pair)
		}
		metadata[parts[0]] = strings.NewReader(parts[1])
	}

	path, err := filepath.Abs(args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the absolute path of %q: %v", args[0], err)
	}

	out, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return 1, fmt.Errorf("failure opening the file %q: %v", path, err)
	}
	if err := bundle.Export(ctx, s, out, snapshots, exclude, metadata, *exportIncludeParentsFlag); err != nil {
		return 1, fmt.Errorf("failure creating the bundle: %v\n", err)
	}
	return 0, nil
}
