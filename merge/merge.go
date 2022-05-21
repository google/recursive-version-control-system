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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/recursive-version-control-system/log"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func recreateLink(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, f *snapshot.File, p snapshot.Path) error {
	contentsReader, err := s.ReadObject(ctx, f.Contents)
	if err != nil {
		return fmt.Errorf("failure opening the contents of the link snapshot %q: %v", h, err)
	}
	contents, err := io.ReadAll(contentsReader)
	if err != nil {
		return fmt.Errorf("failure reading the contents of the link snapshot %q: %v", h, err)
	}
	if err := os.Symlink(string(contents), string(p)); err != nil {
		return fmt.Errorf("failure recreating the symling %q: %v", h, err)
	}
	return nil
}

func recreateDir(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, f *snapshot.File, p snapshot.Path) error {
	perm := f.Permissions()
	if err := os.Mkdir(string(p), perm); err != nil {
		return fmt.Errorf("failure creating the directory %q: %v", p, err)
	}
	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure reading the contents of the directory snapshot %q: %v", h, err)
	}
	for child, childHash := range tree {
		childPath := p.Join(child)
		if err := Checkout(ctx, s, childHash, childPath); err != nil {
			return fmt.Errorf("failure checking out the child path %q: %v", childPath, err)
		}
	}
	return nil
}

func recreateFile(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, f *snapshot.File, p snapshot.Path) error {
	if f.IsLink() {
		return recreateLink(ctx, s, h, f, p)
	}
	if f.IsDir() {
		return recreateDir(ctx, s, h, f, p)
	}
	perm := f.Permissions()
	contentsReader, err := s.ReadObject(ctx, f.Contents)
	if err != nil {
		return fmt.Errorf("failure opening the contents of the link snapshot %q: %v", h, err)
	}
	out, err := os.OpenFile(string(p), os.O_RDWR|os.O_CREATE, perm)
	if err != nil {
		return fmt.Errorf("failure opening the file %q: %v", p, err)
	}
	if _, err := io.Copy(out, contentsReader); err != nil {
		return fmt.Errorf("failure writing the contents of %q: %v", p, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failure closing the file %q: %v", p, err)
	}
	return nil
}

// Checkout "checks out" the given snapshot to a new file location.
//
// If any files already exist at the given location, they will be overwritten,
// however, if we are checking out a directory on top of an existing directory,
// then any existing files that are not in the checked out snapshot will be
// ignored.
func Checkout(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, p snapshot.Path) error {
	f, err := s.ReadSnapshot(ctx, h)
	if err != nil {
		return fmt.Errorf("failure reading the file snapshot for %q: %v", h, err)
	}
	if f == nil {
		// The source file does not exist; nothing for us to do.
		return nil
	}
	if err := recreateFile(ctx, s, h, f, p); err != nil {
		return fmt.Errorf("failure checking out the snapshot %q to the path %q: %v", h, p, err)
	}
	if _, err := s.StoreSnapshot(ctx, p, f); err != nil {
		return fmt.Errorf("failure updating the snapshot for %q to %q: %v", p, h, err)
	}
	return nil
}

// Base identifies the "merge base" between two snapshots; the most recent
// common ancestor of both.
//
// There is always a common ancestor for any two given snapshots because the
// nil hash/snapshot is considered an ancestor for all other snapshots.
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
	// There are no common ancestors
	return nil, nil
}

// Merge attempts to automatically merge the given snapshot into the local
// filesystem at the specified destination path.
//
// If there are any conflicts between the specified snapshot and the local
// filesystem contents, then the `Merge` method retursn an error without
// modifying the local filesystem.
//
// In case there are no conflicts but the local storage is missing some
// referenced snapshots, then it is possible for this method to both modify
// the local filesystem contents *and* to also return an error. In that case
// the previous version of the local filesystem contents will be retrievable
// using the `rvcs log` command.
func Merge(ctx context.Context, s *storage.LocalFiles, src *snapshot.Hash, dest snapshot.Path) error {
	destParent := filepath.Dir(string(dest))
	if err := os.MkdirAll(destParent, os.FileMode(0700)); err != nil {
		return fmt.Errorf("failure ensuring the parent directory of %q exists: %v", dest, err)
	}
	destPrevHash, _, err := snapshot.Current(ctx, s, dest)
	if err != nil {
		return fmt.Errorf("failure generating snapshot of destination %q prior to merging: %v", dest, err)
	}
	if destPrevHash == nil {
		// The destination does not exist; simply check out the source hash there.
		return Checkout(ctx, s, src, dest)
	}
	mergeBase, err := Base(ctx, s, src, destPrevHash)
	if err != nil {
		return fmt.Errorf("failure determining the merge base for %q and %q: %v", src, destPrevHash, err)
	}
	if mergeBase.Equal(src) {
		// The source has already been merged in
		return nil
	}
	if mergeBase.Equal(destPrevHash) {
		// Simply update the destination to point to the target
		if err := os.RemoveAll(string(dest)); err != nil {
			return fmt.Errorf("failure updating %q to point to newer snapshot %q; failure removing old files: %v", dest, src, err)
		}
		return Checkout(ctx, s, src, dest)
	}
	return errors.New("automatic merging into an already existing destination is not yet supported")
}
