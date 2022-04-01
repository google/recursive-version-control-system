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
		h, err := snapshot.ParseHash(s)
		if err != nil {
			return 1, fmt.Errorf("failure parsing snapshot hash %q: %v", s, err)
		}
		if h != nil {
			snapshots = append(snapshots, h)
		}
	}

	path, err := filepath.Abs(args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the absolute path of %q: %v", args[0], err)
	}

	out, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0700)
	if err != nil {
		return 1, fmt.Errorf("failure opening the file %q: %v", path, err)
	}
	if err := bundle.Export(ctx, s, out, snapshots); err != nil {
		return 1, fmt.Errorf("failure creating the bundle: %v\n", err)
	}
	return 0, nil
}
