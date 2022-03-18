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

package archive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/googlestaging/recursive-version-control-system/snapshot"
)

func snapshotFileMetadata(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo, contentsHash *snapshot.Hash) (*snapshot.Hash, error) {
	modeLine := info.Mode().String()
	prevFileHash, prev, err := s.FindFile(ctx, p)
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

func readCached(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo) (*snapshot.Hash, bool) {
	if !s.PathInfoMatchesCache(ctx, p, info) {
		return nil, false
	}
	cachedHash, _, err := s.FindFile(ctx, p)
	if err != nil {
		return nil, false
	}
	return cachedHash, true
}

func snapshotRegularFile(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo, contents io.Reader) (h *snapshot.Hash, err error) {
	if cached, ok := readCached(ctx, s, p, info); ok {
		return cached, nil
	}
	defer func() {
		if err == nil && h != nil {
			s.CachePathInfo(ctx, p, info)
		}
	}()
	h, err = s.StoreObject(ctx, contents)
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

func snapshotLink(ctx context.Context, s *Store, p snapshot.Path, info os.FileInfo) (*snapshot.Hash, error) {
	target, err := os.Readlink(string(p))
	if err != nil {
		return nil, fmt.Errorf("failure reading the link target for %q: %v", p, err)
	}

	h, err := s.StoreObject(ctx, strings.NewReader(target))
	if err != nil {
		return nil, fmt.Errorf("failure storing an object: %v", err)
	}
	return snapshotFileMetadata(ctx, s, p, info, h)
}

// Snapshot generates a snapshot for the given path, stored in the given store.
//
// The passed in path must be an absolute path.
//
// The returned value is the hash of the generated `snapshot.File` object.
func Snapshot(ctx context.Context, s *Store, p snapshot.Path) (*snapshot.Hash, error) {
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
		return snapshotLink(ctx, s, p, stat)
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

type LogEntry struct {
	Hash *snapshot.Hash
	File *snapshot.File
}

func ReadLog(ctx context.Context, s *Store, h *snapshot.Hash) ([]*LogEntry, error) {
	visited := make(map[snapshot.Hash]*snapshot.File)
	queue := []*snapshot.Hash{h}
	result := []*LogEntry{}
	for len(queue) > 0 {
		h, queue = queue[0], queue[1:]
		f, err := s.ReadFile(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("failure reading the snapshot for %q: %v", h, err)
		}
		visited[*h] = f
		result = append(result, &LogEntry{
			Hash: h,
			File: f,
		})
		for _, p := range f.Parents {
			if _, ok := visited[*p]; !ok {
				queue = append(queue, p)
			}
		}
	}
	return result, nil
}
