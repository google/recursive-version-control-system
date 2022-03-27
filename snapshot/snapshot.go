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

// Package snapshot defines the model for snapshots of a file's history.
package snapshot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Storage interface {
	StoreObject(context.Context, io.Reader) (*Hash, error)

	Exclude(Path) bool
	FindSnapshot(context.Context, Path) (*Hash, *File, error)
	StoreSnapshot(context.Context, Path, *File) (*Hash, error)

	CachePathInfo(context.Context, Path, os.FileInfo) error
	PathInfoMatchesCache(context.Context, Path, os.FileInfo) bool
}

func snapshotFileMetadata(ctx context.Context, s Storage, p Path, info os.FileInfo, contentsHash *Hash) (*Hash, *File, error) {
	modeLine := info.Mode().String()
	prevFileHash, prev, err := s.FindSnapshot(ctx, p)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("failure looking up the previous file snapshot: %v", err)
	}
	if prev != nil && prev.Mode == modeLine && prev.Contents.Equal(contentsHash) {
		// The file is unchanged from the last snapshot...
		return prevFileHash, prev, nil
	}
	f := &File{
		Contents: contentsHash,
		Mode:     modeLine,
	}
	if prev != nil {
		f.Parents = []*Hash{prevFileHash}
	}
	h, err := s.StoreSnapshot(ctx, p, f)
	if err != nil {
		return nil, nil,fmt.Errorf("failure saving the latest file metadata for %q: %v", p, err)
	}
	return h, f, nil
}

func readCached(ctx context.Context, s Storage, p Path, info os.FileInfo) (*Hash, *File, bool) {
	if !s.PathInfoMatchesCache(ctx, p, info) {
		return nil, nil, false
	}
	cachedHash, cachedFile, err := s.FindSnapshot(ctx, p)
	if err != nil {
		return nil, nil, false
	}
	return cachedHash, cachedFile, true
}

func snapshotRegularFile(ctx context.Context, s Storage, p Path, info os.FileInfo, contents io.Reader) (h *Hash, f *File, err error) {
	if cachedHash, cachedFile, ok := readCached(ctx, s, p, info); ok {
		return cachedHash, cachedFile, nil
	}
	defer func() {
		if err == nil && h != nil {
			s.CachePathInfo(ctx, p, info)
		}
	}()
	h, err = s.StoreObject(ctx, contents)
	if err != nil {
		return nil, nil, fmt.Errorf("failure storing an object: %v", err)
	}
	return snapshotFileMetadata(ctx, s, p, info, h)
}

func snapshotDirectory(ctx context.Context, s Storage, p Path, info os.FileInfo, contents *os.File) (*Hash, *File, error) {
	entries, err := contents.ReadDir(0)
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the filesystem contents of the directory %q: %v", p, err)
	}
	childHashes := make(Tree)
	for _, entry := range entries {
		childPath := Path(filepath.Join(string(p), entry.Name()))
		childHash, _, err := Current(ctx, s, childPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failure hashing the child dir %q: %v", childPath, err)
		}
		if childHash != nil {
			childHashes[Path(entry.Name())] = childHash
		}
	}
	contentsJson := []byte(childHashes.String())
	contentsHash, err := s.StoreObject(ctx, bytes.NewReader(contentsJson))
	return snapshotFileMetadata(ctx, s, p, info, contentsHash)
}

func snapshotLink(ctx context.Context, s Storage, p Path, info os.FileInfo) (*Hash, *File, error) {
	target, err := os.Readlink(string(p))
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the link target for %q: %v", p, err)
	}

	h, err := s.StoreObject(ctx, strings.NewReader(target))
	if err != nil {
		return nil, nil, fmt.Errorf("failure storing an object: %v", err)
	}
	return snapshotFileMetadata(ctx, s, p, info, h)
}

// Current generates a snapshot for the given path, stored in the given store.
//
// The passed in path must be an absolute path.
//
// The returned value is the hash of the generated `snapshot.File` object.
func Current(ctx context.Context, s Storage, p Path) (*Hash, *File, error) {
	if s.Exclude(p) {
		// We are not supposed to store snapshots for the given path, so pretend it does not exist.
		return nil, nil, nil
	}
	stat, err := os.Lstat(string(p))
	if os.IsNotExist(err) {
		// The referenced file does not exist, so the corresponding
		// hash should be nil.
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the file stat for %q: %v", p, err)
	}
	if stat.Mode()&fs.ModeSymlink != 0 {
		return snapshotLink(ctx, s, p, stat)
	}
	contents, err := os.Open(string(p))
	if os.IsNotExist(err) {
		// The file we tried to open no longer exists.
		//
		// This could happen if there is a race condition where the
		// file was deleted before we could read it. In that case,
		// return an empty snapshot.
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the file %q: %v", p, err)
	}
	defer contents.Close()

	info, err := contents.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the filesystem metadata for %q: %v", p, err)
	}
	if info.IsDir() {
		return snapshotDirectory(ctx, s, p, info, contents)
	} else {
		return snapshotRegularFile(ctx, s, p, info, contents)
	}
}
