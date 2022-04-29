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

	"github.com/google/recursive-version-control-system/bundle"
	"github.com/google/recursive-version-control-system/storage"
)

const importUsage = `Usage: %s import [<FLAGS>]* <PATH>

Where <PATH> is a local filesystem path for the bundle to import, and <FLAGS> are one of:

`

var (
	importFlags = flag.NewFlagSet("import", flag.ContinueOnError)

	importExcludeFlag = importFlags.String(
		"exclude", "",
		"comma separated list of objects to exclude from the import")
	importExcludeFromFileFlag = importFlags.String(
		"exclude-from-file", "",
		"path to a file containing a newline separated list of objects to exclude from the import")

	importVerboseFlag = importFlags.Bool(
		"v", false,
		"verbose output. Print the hash of every object imported")
)

func importCommand(ctx context.Context, s *storage.LocalFiles, cmd string, args []string) (int, error) {
	importFlags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), importUsage, cmd)
		importFlags.PrintDefaults()
	}
	if err := importFlags.Parse(args); err != nil {
		return 1, nil
	}
	args = importFlags.Args()
	if len(args) < 1 {
		fmt.Fprintf(flag.CommandLine.Output(), importUsage, cmd)
		importFlags.PrintDefaults()
		return 1, nil
	}
	exclude, err := hashesFromFileAndFlag(ctx, *importExcludeFromFileFlag, *importExcludeFlag)
	if err != nil {
		return 1, err
	}

	path, err := filepath.Abs(args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the absolute path of %q: %v", args[0], err)
	}

	included, err := bundle.Import(ctx, s, path, exclude)
	if err != nil {
		return 1, fmt.Errorf("failure importing the bundle: %v\n", err)
	}
	if *importVerboseFlag {
		for _, h := range included {
			fmt.Println(h.String())
		}
	}
	return 0, nil
}
