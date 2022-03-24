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
	"path/filepath"

	"github.com/google/recursive-version-control-system/archive"
	"github.com/google/recursive-version-control-system/snapshot"
)

const mergeUsage = `Usage: %s merge <SOURCE> <DESTINATION>

Where <DESTINATION> is a local file path, and <SOURCE> is one of:

	The hash of a known snapshot.
	A local file path which has previously been snapshotted.
`

func mergeCommand(ctx context.Context, s *archive.Store, cmd string, args []string) (int, error) {
	if len(args) != 2 {
		fmt.Fprintf(flag.CommandLine.Output(), mergeUsage, cmd)
		return 1, nil
	}
	h, err := resolveSnapshot(ctx, s, args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the snapshot hash for %q: %v", args[0], err)
	}
	abs, err := filepath.Abs(args[1])
	if err != nil {
		return 1, fmt.Errorf("failure determining the absolute path of %q: %v", args[1], err)
	}
	if err := archive.Merge(ctx, s, h, snapshot.Path(abs)); err != nil {
		return 1, fmt.Errorf("failure merging %q into %q: %v", h, abs, err)
	}
	return 0, nil
}
