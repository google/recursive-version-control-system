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
	"fmt"

	"github.com/googlestaging/recursive-version-control-system/archive"
)

type command func(context.Context, *archive.Store, string, []string) (int, error)

var (
	commandMap = map[string]command{
		"log":      logCommand,
		"snapshot": snapshotCommand,
	}

	usage = `Usage: %s <SUBCOMMAND>

Where <SUBCOMMAND> is one of:

	log
	snapshot
`
)

// Run implements the subcommands of the `rvcs` CLI.
//
// The passed in `args` should be the value returned by `os.Args`
//
// The returned value is the exit code of the command; 0 for success
// and non-zero for any form of failure.
func Run(ctx context.Context, s *archive.Store, args []string) (exitCode int) {
	if len(args) < 2 {
		fmt.Printf(usage, args[0])
		return 1
	}
	subcommand, ok := commandMap[args[1]]
	if !ok {
		fmt.Printf("Unknown subcommand %q\n", args[1])
		fmt.Printf(usage, args[0])
		return 1
	}
	retcode, err := subcommand(ctx, s, args[0], args[2:])
	if err != nil {
		fmt.Printf("Failure running the %q subcommand: %v\n", args[1], err)
	}
	return retcode
}
