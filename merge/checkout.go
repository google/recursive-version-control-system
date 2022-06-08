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
	"io"
	"os"
	"path/filepath"

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
	if target, err := os.Readlink(string(p)); err == nil && target == string(contents) {
		// The link already exists and points at the correct target
		return nil
	}
	if err := os.RemoveAll(string(p)); err != nil {
		return fmt.Errorf("failure removing the old file at %q: %v", p, err)
	}
	if err := os.Symlink(string(contents), string(p)); err != nil {
		return fmt.Errorf("failure recreating the symling %q: %v", h, err)
	}
	return nil
}

func ensureDirExistsWithPermissions(ctx context.Context, path string, perm os.FileMode) error {
	if err := os.Mkdir(path, perm); err == nil {
		return nil
	} else if !os.IsExist(err) {
		return fmt.Errorf("failure creating the directory %q: %v", path, err)
	}
	if info, err := os.Lstat(path); err != nil {
		return fmt.Errorf("failure reading file metadata for the path %q: %v", path, err)
	} else if info.IsDir() {
		return os.Chmod(path, perm)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failure removing the old file at %q: %v", path, err)
	}
	return os.Mkdir(path, perm)
}

func recreateDir(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, f *snapshot.File, p snapshot.Path) error {
	perm := f.Permissions()
	if err := ensureDirExistsWithPermissions(ctx, string(p), perm); err != nil {
		return fmt.Errorf("failure creating the directory %q: %v", p, err)
	}

	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure reading the contents of the directory snapshot %q: %v", h, err)
	}

	contents, err := os.Open(string(p))
	if err != nil {
		return fmt.Errorf("failure opening the directory %q: %v", p, err)
	}
	entries, err := contents.ReadDir(0)
	if err != nil {
		return fmt.Errorf("failure reading the filesystem contents of the directory %q: %v", p, err)
	}
	for _, entry := range entries {
		child := snapshot.Path(entry.Name())
		if _, ok := tree[child]; ok {
			continue
		}
		// The child does not exist in the snapshot to checkout
		childPath := p.Join(child)
		if s.Exclude(childPath) {
			// The child path is meant to be excluded from
			// snapshotting, so it is expected that it would not
			// be in the snapshot.
			continue
		}
		if err := os.RemoveAll(string(childPath)); err != nil {
			return fmt.Errorf("failure removing the deleted file %q: %v", childPath, err)
		}
	}
	for child, childHash := range tree {
		childPath := p.Join(child)
		if s.Exclude(childPath) {
			// The child path is meant to be excluded from
			// snapshotting, so it should also be excluded from
			// being updated when checking out a snapshot.
			continue
		}
		if err := Checkout(ctx, s, childHash, childPath); err != nil {
			return fmt.Errorf("failure checking out the child path %q: %v", childPath, err)
		}
	}
	return nil
}

func ensureFileExistsWithPermissions(ctx context.Context, path string, perm os.FileMode) (*os.File, error) {
	out, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		if err := os.RemoveAll(path); err != nil {
			return nil, fmt.Errorf("failure removing the old file at %q: %v", path, err)
		}
		out, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	}
	if err != nil {
		return nil, err
	}
	if err := out.Chmod(perm); err != nil {
		return nil, fmt.Errorf("failure changing the permissions of %q: %v", path, err)
	}
	return out, nil
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
	out, err := ensureFileExistsWithPermissions(ctx, string(p), perm)
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
// If any files already exist at the given location, they will be overwritten.
//
// If there are any nested files under the given location that do not exist
// in the checked out snapshot, then they will be removed.
//
// For regular files and directories, the checked out file permissions will
// match what the corresponding permissions are in the snapshot. However,
// for symbolic links, the file permissions from the snapshot are ignored.
//
// If there are any errors during the checkout, then the applied filesystem
// changes are not rolled back and the local file system can be left in an
// inconsistent state.
func Checkout(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, p snapshot.Path) error {
	f, err := s.ReadSnapshot(ctx, h)
	if err != nil {
		return fmt.Errorf("failure reading the file snapshot for %q: %v", h, err)
	}
	if f == nil {
		// The source file does not exist; nothing for us to do.
		return nil
	}
	parent := filepath.Dir(string(p))
	if err := os.MkdirAll(parent, os.FileMode(0700)); err != nil {
		return fmt.Errorf("failure ensuring the parent directory of %q exists: %v", p, err)
	}
	if err := recreateFile(ctx, s, h, f, p); err != nil {
		return fmt.Errorf("failure checking out the snapshot %q to the path %q: %v", h, p, err)
	}
	if _, err := s.StoreSnapshot(ctx, p, f); err != nil {
		return fmt.Errorf("failure updating the snapshot for %q to %q: %v", p, h, err)
	}
	return nil
}
