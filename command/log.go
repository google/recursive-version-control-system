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
	"github.com/googlestaging/recursive-version-control-system/snapshot"
)

func logCommand(ctx context.Context, s *archive.Store, cmd string, args []string) (int, error) {
	if len(args) != 1 {
		fmt.Printf("Usage: %q log <HASH>\n", cmd)
		return 1, nil
	}
	h, err := snapshot.ParseHash(args[0])
	if err != nil {
		return 1, fmt.Errorf("failure parsing the hash %q: %v", args[0], err)
	}
	entries, err := archive.ReadLog(ctx, s, h)
	if err != nil {
		return 1, fmt.Errorf("failure reading the log for %q: %v", args[0], err)
	}
	for _, e := range entries[1:] {
		fmt.Printf("%s\n", e.Hash)
	}
	return 0, nil
}
