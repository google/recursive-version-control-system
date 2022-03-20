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

func logCommand(ctx context.Context, s *archive.Store, cmd string, args []string) (int, error) {
	if len(args) != 1 {
		fmt.Printf("Usage: %q log <HASH>\n", cmd)
		return 1, nil
	}
	h, err := resolveSnapshot(ctx, s, args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the snapshot hash for %q: %v", args[0], err)
	}
	entries, err := archive.ReadLog(ctx, s, h)
	if err != nil {
		return 1, fmt.Errorf("failure reading the log for %q: %v", args[0], err)
	}
	summaries, err := archive.SummarizeLog(ctx, s, entries)
	if err != nil {
		return 1, fmt.Errorf("failure summarizing log entries for %q: %v", args[0], err)
	}
	for i, e := range entries {
		if i > 0 {
			// Separate log entries for each change with a newline to make the output more readable.
			fmt.Println()
		}
		summary, ok := summaries[*e.Hash]
		if !ok {
			return 1, fmt.Errorf("internal error reading log summaries: entry %q is missing", e.Hash)
		}
		for _, line := range summary {
			fmt.Println(line)
		}
	}
	return 0, nil
}
