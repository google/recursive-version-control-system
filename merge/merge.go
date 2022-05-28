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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/recursive-version-control-system/log"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func IsAncestor(ctx context.Context, s *storage.LocalFiles, base, h *snapshot.Hash) (bool, error) {
	snapshotLog, err := log.ReadLog(ctx, s, h, -1)
	if err != nil {
		return false, fmt.Errorf("failure reading the log for %q: %v", h, err)
	}
	for _, e := range snapshotLog {
		if e.Hash.Equal(base) {
			return true, nil
		}
	}
	return false, nil
}

func mergeWithBase(ctx context.Context, s *storage.LocalFiles, subPath snapshot.Path, base, src, dest *snapshot.Hash, forceKeepMode bool) (*snapshot.Hash, error) {
	// First we handle the trivial cases where the merge result should
	// just be one of the two provided snapshots.
	if src.Equal(dest) {
		return src, nil
	}
	if src.Equal(base) {
		return dest, nil
	}
	if dest.Equal(base) {
		return src, nil
	}

	// If either the source or destination do not have the base as an
	// ancestor, then that means the changes in the base were rolled back
	// in that version. In that case, we have to ask the user to manually
	// merge the two versions.
	if src == nil || dest == nil {
		return nil, fmt.Errorf("the nested snapshot under the path %q was deleted in either the source or destination snapshot, so the two snapshots have to be manually merged", subPath)
	}
	if isAncestor, err := IsAncestor(ctx, s, base, src); err != nil {
		return nil, err
	} else if !isAncestor {
		// The changes from the base snapshot were rolled back in
		// the source...
		return nil, fmt.Errorf("nested changes under the path %q were rolled back in the source snapshot, so the two snapshots have to be manually merged", subPath)
	}
	if isAncestor, err := IsAncestor(ctx, s, base, dest); err != nil {
		return nil, err
	} else if !isAncestor {
		// The changes from the base snapshot were rolled back in
		// the destination...
		return nil, fmt.Errorf("nested changes under the path %q were rolled back in the destination snapshot, so the two snapshots have to be manually merged", subPath)
	}

	// For everything else we have to compare the actual snapshots, so
	// we first have to read both snapshots.
	srcFile, err := s.ReadSnapshot(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("failure reading the file snapshot for %q: %v", src, err)
	}
	destFile, err := s.ReadSnapshot(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("failure reading the file snapshot for %q: %v", dest, err)
	}
	var baseFile *snapshot.File
	if base != nil {
		baseFile, err = s.ReadSnapshot(ctx, base)
		if err != nil {
			return nil, fmt.Errorf("failure reading the file snapshot for %q: %v", base, err)
		}
	}

	// If either the source or the destination are symbolic links, then
	// the user has to manually merge them.
	if srcFile.IsLink() || destFile.IsLink() {
		return nil, fmt.Errorf("one or both versions of the snapshot at %q represent a symlink, so the two snapshots for that path have to be manually merged", subPath)
	}

	if !(srcFile.IsDir() && destFile.IsDir()) {
		// TODO: Add support for diff3-style merges of regular files
		return nil, fmt.Errorf("automatic merging of regular files is not yet implemented, so the two snapshots at %q have to be manually merged", subPath)
	}

	// Both source and destination are directories, so we recursively
	// merge every nested path under either of them using the corresponding
	// nested path from the base as a reference point.
	srcTree, err := s.ListDirectorySnapshotContents(ctx, src, srcFile)
	if err != nil {
		return nil, fmt.Errorf("failure reading the tree for the snapshot %q: %v", src, err)
	}
	destTree, err := s.ListDirectorySnapshotContents(ctx, dest, destFile)
	if err != nil {
		return nil, fmt.Errorf("failure reading the tree for the snapshot %q: %v", dest, err)
	}
	var baseTree snapshot.Tree
	if baseFile.IsDir() {
		baseTree, err = s.ListDirectorySnapshotContents(ctx, base, baseFile)
		if err != nil {
			return nil, fmt.Errorf("failure reading the tree for the snapshot %q: %v", base, err)
		}
	} else {
		// The base was a different type, so each subpath of it should
		// just be nil
		baseTree = make(snapshot.Tree)
	}

	mergedTree := make(snapshot.Tree)
	subpaths := make(map[snapshot.Path]struct{})
	for p, _ := range srcTree {
		subpaths[p] = struct{}{}
	}
	for p, _ := range destTree {
		subpaths[p] = struct{}{}
	}
	var nestedErrors []string
	for p, _ := range subpaths {
		childSubPath := subPath.Join(p)
		childBase := baseTree[p]
		childSrc := srcTree[p]
		childDest := destTree[p]
		mergedChild, err := mergeWithBase(ctx, s, childSubPath, childBase, childSrc, childDest, forceKeepMode)
		if err != nil {
			nestedErrors = append(nestedErrors, err.Error())
		}
		if mergedChild != nil {
			mergedTree[p] = mergedChild
		}
	}
	if srcFile.Mode != destFile.Mode && !forceKeepMode {
		nestedErrors = append(nestedErrors, fmt.Sprintf("file permissions for %q do not match between versions; source mode line: %q, destination mode line %q. Manually update the permissions for the source to match what you want for the merge result, and then re-run the merge with the option to force using the source permissions", subPath, srcFile.Mode, destFile.Mode))
	}
	if len(nestedErrors) > 0 {
		return nil, errors.New(strings.Join(nestedErrors, "\n"))
	}

	contentsBytes := []byte(mergedTree.String())
	contentsHash, err := s.StoreObject(ctx, int64(len(contentsBytes)), bytes.NewReader(contentsBytes))
	if err != nil {
		return nil, fmt.Errorf("failure storing the contents of a merged tree: %v", err)
	}
	mergedFile := &snapshot.File{
		Mode:     srcFile.Mode,
		Contents: contentsHash,
		Parents:  []*snapshot.Hash{src, dest},
	}
	fileBytes := []byte(mergedFile.String())
	h, err := s.StoreObject(ctx, int64(len(fileBytes)), bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("failure storing the merged snapshot: %v", err)
	}
	return h, nil
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

	mergedHash, err := mergeWithBase(ctx, s, dest, mergeBase, src, destPrevHash, false)
	if err != nil {
		return fmt.Errorf("unable to automatically merge the two snapshots: %v", err)
	}

	// Update the destination to point to the merged snapshot
	if err := os.RemoveAll(string(dest)); err != nil {
		return fmt.Errorf("failure updating %q to point to newer snapshot %q; failure removing old files: %v", dest, mergedHash, err)
	}
	return Checkout(ctx, s, mergedHash, dest)
}
