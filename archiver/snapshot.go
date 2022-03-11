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

package archiver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/googlestaging/recursive-version-control-system/snapshot"
)

func snapshotFileMetadata(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo, contentsHash *snapshot.Hash) (*snapshot.Hash, error) {
	modeLine := info.Mode().String()
	prevFileHash, prev, err := s.ReadFile(ctx, p)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failure looking up the previous file snapshot: %v", err)
	}
	if prev != nil && prev.Mode == modeLine && prev.Contents.Equal(contentsHash) {
		// The file is unchanged from the last snapshot...
		return prevFileHash, nil
	}
	f := &snapshot.File{
		Contents: contentsHash,
		Mode:     modeLine,
	}
	if prev != nil {
		f.Parents = []*snapshot.Hash{prevFileHash}
	}
	h, err := s.StoreFile(ctx, p, f)
	if err != nil {
		return nil, fmt.Errorf("failure saving the latest file metadata for %q: %v", p, err)
	}
	return h, nil
}

func snapshotRegularFile(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo, contents io.Reader) (*snapshot.Hash, error) {
	h, err := s.StoreObject(ctx, contents)
	if err != nil {
		return nil, fmt.Errorf("failure storing an object: %v", err)
	}
	return snapshotFileMetadata(ctx, s, p, info, h)
}

func snapshotDirectory(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo, contents *os.File) (*snapshot.Hash, error) {
	entries, err := contents.ReadDir(0)
	if err != nil {
		return nil, fmt.Errorf("failure reading the filesystem contents of the directory %q: %v", p, err)
	}
	childHashes := make(snapshot.Tree)
	for _, entry := range entries {
		childPath := snapshot.Path(filepath.Join(string(p), entry.Name()))
		if s.Exclude(childPath) {
			continue
		}
		childHash, err := Snapshot(ctx, s, childPath)
		if err != nil {
			return nil, fmt.Errorf("failure hashing the child dir %q: %v", childPath, err)
		}
		childHashes[snapshot.Path(entry.Name())] = childHash
	}
	contentsJson := []byte(childHashes.String())
	contentsHash, err := s.StoreObject(ctx, bytes.NewReader(contentsJson))
	return snapshotFileMetadata(ctx, s, p, info, contentsHash)
}

func snapshotLink(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo, visitedLinks map[string]string) (*snapshot.Hash, error) {
	parent := filepath.Dir(string(p))
	if _, ok := visitedLinks[parent]; ok {
		// We've found a cycle of links... stop processing them.
		return nil, nil
	}
	target, err := os.Readlink(string(p))
	if err != nil {
		return nil, fmt.Errorf("failure reading the link target for %q: %v", p, err)
	}
	absoluteTarget, err := filepath.Abs(filepath.Join(parent, target))
	if err != nil {
		return nil, fmt.Errorf("failure resolving the absolute file path for target %q: %v", target, err)
	}
	visitedLinks[parent] = absoluteTarget
	targetHash, err := snapshotRec(ctx, s, snapshot.Path(absoluteTarget), visitedLinks)
	if err != nil {
		return nil, fmt.Errorf("failure reading the link target for %q: %v", p, err)
	}
	return snapshotFileMetadata(ctx, s, p, info, targetHash)
}

func snapshotRec(ctx context.Context, s *Store, p snapshot.Path, visitedLinks map[string]string) (*snapshot.Hash, error) {
	stat, err := os.Lstat(string(p))
	if os.IsNotExist(err) {
		// The referenced file does not exist, so the corresponding
		// hash should be nil.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failure reading the file stat for %q: %v", p, err)
	}
	if stat.Mode()&fs.ModeSymlink != 0 {
		return snapshotLink(ctx, s, p, stat, visitedLinks)
	}
	contents, err := os.Open(string(p))
	if os.IsNotExist(err) {
		// The file we tried to open no longer exists.
		//
		// This could happen if there is a race condition where the
		// file was deleted before we could read it. In that case,
		// return an empty contents.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failure reading the file %q: %v", p, err)
	}
	defer contents.Close()

	info, err := contents.Stat()
	if err != nil {
		return nil, fmt.Errorf("failure reading the filesystem metadata for %q: %v", p, err)
	}
	if info.IsDir() {
		return snapshotDirectory(ctx, s, p, info, contents)
	} else {
		return snapshotRegularFile(ctx, s, p, info, contents)
	}
}

// Snapshot generates a snapshot for the given path, stored in the given store.
//
// The passed in path must be an absolute path.
//
// The returned value is the hash of the generated `snapshot.File` object.
func Snapshot(ctx context.Context, s *Store, p snapshot.Path) (*snapshot.Hash, error) {
	return snapshotRec(ctx, s, p, make(map[string]string))
}
