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

// Package merge defines methods for merging two snapshots together.
package merge

import (
	"context"
	"fmt"

	"github.com/google/recursive-version-control-system/log"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

// Base identifies the "merge base" between two snapshots; the most recent
// common ancestor of both.
//
// Ancestry for snapshots is defined as follows:
//
// 1. The nil snapshot is an ancestor of every snapshot
// 2. Every snapshot is an ancestor of itself
// 3. If a snapshot has parents, then every ancestor of one of the parents
//    is also an ancestor of the snapshot.
//
// This means there is always a common ancestor for any two given snapshots,
// because the nil hash/snapshot is considered an ancestor for all snapshots.
//
// Regardless, this method can still return an error in cases where the
// snapshot storage is incomplete and some snapshots are missing.
func Base(ctx context.Context, s *storage.LocalFiles, lhs, rhs *snapshot.Hash) (*snapshot.Hash, error) {
	if lhs.Equal(rhs) {
		return lhs, nil
	}
	if lhs == nil || rhs == nil {
		return nil, nil
	}
	lhsLog, err := log.ReadLog(ctx, s, lhs, -1)
	if err != nil {
		return nil, fmt.Errorf("failure reading the log for %q: %v", lhs, err)
	}
	lhsAncestors := make(map[snapshot.Hash]struct{})
	for _, e := range lhsLog {
		lhsAncestors[*e.Hash] = struct{}{}
	}
	rhsLog, err := log.ReadLog(ctx, s, rhs, -1)
	if err != nil {
		return nil, fmt.Errorf("failure reading the log for %q: %v", rhs, err)
	}
	rhsAncestors := make(map[snapshot.Hash]struct{})
	for _, e := range rhsLog {
		rhsAncestors[*e.Hash] = struct{}{}
	}
	for len(lhsLog) > 0 && len(rhsLog) > 0 {
		if _, ok := rhsAncestors[*lhsLog[0].Hash]; ok {
			return lhsLog[0].Hash, nil
		}
		if _, ok := lhsAncestors[*rhsLog[0].Hash]; ok {
			return rhsLog[0].Hash, nil
		}
		lhsLog = lhsLog[1:]
		rhsLog = rhsLog[1:]
	}
	// There are no common ancestors other than the nil snapshot
	return nil, nil
}
