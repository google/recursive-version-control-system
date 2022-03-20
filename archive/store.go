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
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/googlestaging/recursive-version-control-system/snapshot"
)

// Store is used to write and read snapshots to persistent storage.
type Store struct {
	ArchiveDir string
}

// Exclude reports whether or not the given path should be excluded from snapshotting.
//
// This should return true for any paths that are part of the underlying persistent storage.
func (s *Store) Exclude(p snapshot.Path) bool {
	return p == snapshot.Path(s.ArchiveDir)
}

func (s *Store) tmpFile(ctx context.Context) (*os.File, error) {
	tmpDir := filepath.Join(s.ArchiveDir, "tmp")
	if err := os.MkdirAll(tmpDir, os.FileMode(0700)); err != nil {
		return nil, fmt.Errorf("failure creating the tmp dir: %v", err)
	}
	return os.CreateTemp(tmpDir, "archiver")
}

func (s *Store) StoreObject(ctx context.Context, reader io.Reader) (h *snapshot.Hash, err error) {
	var tmp *os.File
	tmp, err = s.tmpFile(ctx)
	if err != nil {
		return nil, fmt.Errorf("failure creating a temp file: %v", err)
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	reader = io.TeeReader(reader, tmp)
	sum := sha256.New()
	if _, err := io.Copy(sum, reader); err != nil {
		return nil, fmt.Errorf("failure hashing an object: %v", err)
	}
	tmp.Close()
	h = &snapshot.Hash{
		Function:    "sha256",
		HexContents: fmt.Sprintf("%x", sum.Sum(nil)),
	}
	objPath, objName := objectName(h, filepath.Join(s.ArchiveDir, "objects"))
	if err := os.MkdirAll(objPath, os.FileMode(0700)); err != nil {
		return nil, fmt.Errorf("failure creating the object dir for %q: %v", h, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(objPath, objName)); err != nil {
		return nil, fmt.Errorf("failure writing the object file for %q: %v", h, err)
	}
	return h, nil
}

func objectName(h *snapshot.Hash, parentDir string) (dir string, name string) {
	functionDir := filepath.Join(parentDir, h.Function)
	if len(h.HexContents) > 4 {
		return filepath.Join(functionDir, h.HexContents[0:2], h.HexContents[2:4]), h.HexContents[4:]
	} else if len(h.HexContents) > 2 {
		return filepath.Join(functionDir, h.HexContents[0:2]), h.HexContents[2:]
	}
	return functionDir, h.HexContents
}

func (s *Store) ReadObject(ctx context.Context, h *snapshot.Hash) (io.ReadCloser, error) {
	objPath, objName := objectName(h, filepath.Join(s.ArchiveDir, "objects"))
	return os.Open(filepath.Join(objPath, objName))
}

func (s *Store) pathHashFile(p snapshot.Path) (dir string, name string) {
	pathHash := &snapshot.Hash{
		Function:    "sha256",
		HexContents: fmt.Sprintf("%x", sha256.Sum256([]byte(p))),
	}
	return objectName(pathHash, filepath.Join(s.ArchiveDir, "paths"))
}

func (s *Store) StoreSnapshot(ctx context.Context, p snapshot.Path, f *snapshot.File) (*snapshot.Hash, error) {
	bs := []byte(f.String())
	h, err := s.StoreObject(ctx, bytes.NewReader(bs))
	if err != nil {
		return nil, fmt.Errorf("failure saving file metadata for %+v: %v", f, err)
	}
	pathHashDir, pathHashFile := s.pathHashFile(p)
	if err := os.MkdirAll(pathHashDir, 0700); err != nil {
		return nil, fmt.Errorf("failure creating the paths dir for %q: %v", p, err)
	}
	if err := os.WriteFile(filepath.Join(pathHashDir, pathHashFile), []byte(h.String()), 0600); err != nil {
		return nil, fmt.Errorf("failure writing the hash for path %q: %v", p, err)
	}
	return h, nil
}

func (s *Store) ReadSnapshot(ctx context.Context, h *snapshot.Hash) (*snapshot.File, error) {
	reader, err := s.ReadObject(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failure looking up the file snapshot for %q: %v", h, err)
	}
	defer reader.Close()
	contents, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failure reading file metadata from the reader: %v", err)
	}
	f, err := snapshot.ParseFile(string(contents))
	if err != nil {
		return nil, fmt.Errorf("failure parsing the file snapshot for %q: %v", h, err)
	}
	return f, nil
}

func (s *Store) FindSnapshot(ctx context.Context, p snapshot.Path) (*snapshot.Hash, *snapshot.File, error) {
	pathHashDir, pathHashFile := s.pathHashFile(p)
	bs, err := os.ReadFile(filepath.Join(pathHashDir, pathHashFile))
	if err != nil {
		return nil, nil, err
	}
	fileHashStr := string(bs)
	if !strings.HasPrefix(fileHashStr, "sha256:") {
		return nil, nil, fmt.Errorf("unsupported hash function for %q", fileHashStr)
	}
	h := &snapshot.Hash{
		Function:    "sha256",
		HexContents: strings.TrimPrefix(fileHashStr, "sha256:"),
	}
	f, err := s.ReadSnapshot(ctx, h)
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the file snapshot for %q: %v", h, err)
	}
	return h, f, nil
}

// ListDirectorySnapshotContents returns the parsed `*snapshot.Tree` object listing the contents of `f`.
//
// The supplied `*snapshot.File` object must correspond to a directory.
func (s *Store) ListDirectorySnapshotContents(ctx context.Context, h *snapshot.Hash, f *snapshot.File) (snapshot.Tree, error) {
	if !f.IsDir() {
		return nil, fmt.Errorf("%q is not the snapshot of a directory", h)
	}
	contentsReader, err := s.ReadObject(ctx, f.Contents)
	if err != nil {
		return nil, fmt.Errorf("failure opening the contents of %q: %v", h, err)
	}
	contents, err := io.ReadAll(contentsReader)
	if err != nil {
		return nil, fmt.Errorf("failure reading the contents of %q: %v", h, err)
	}
	tree, err := snapshot.ParseTree(string(contents))
	if err != nil {
		return nil, fmt.Errorf("failure parsing the directory contents of the snapshot %q: %v", h, err)
	}
	return tree, nil
}

func (s *Store) RemoveMappingForPath(ctx context.Context, p snapshot.Path) error {
	h, f, err := s.FindSnapshot(ctx, p)
	if os.IsNotExist(err) {
		// There is no file snapshot corresponding to the given path,
		// so we have nothing to do.
		return nil
	}

	pathHashDir, pathHashFile := s.pathHashFile(p)
	mappingPath := filepath.Join(pathHashDir, pathHashFile)
	if err := os.Remove(mappingPath); err != nil {
		return fmt.Errorf("failure removing the mapping from %q to %q: %v", p, h, err)
	}
	if !f.IsDir() {
		return nil
	}
	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure listing the contents of %q: %v", h, err)
	}
	for child, _ := range tree {
		childPath := p.Join(child)
		if err := s.RemoveMappingForPath(ctx, childPath); err != nil {
			return fmt.Errorf("failure removing mapping for the child path %q: %v", child, err)
		}
	}
	return nil
}

type cachedInfo struct {
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	Ino     uint64
}

func (s *Store) pathCacheFile(p snapshot.Path) (dir string, name string) {
	pathHash := &snapshot.Hash{
		Function:    "sha256",
		HexContents: fmt.Sprintf("%x", sha256.Sum256([]byte(p))),
	}
	return objectName(pathHash, filepath.Join(s.ArchiveDir, "cache"))
}

func (s *Store) CachePathInfo(ctx context.Context, p snapshot.Path, info os.FileInfo) error {
	sysInfo := info.Sys()
	if sysInfo == nil {
		return nil
	}
	unix_info, ok := sysInfo.(*syscall.Stat_t)
	if !ok || unix_info == nil {
		return nil
	}
	ino := unix_info.Ino

	cacheDir, cacheFile := s.pathCacheFile(p)
	cachePath := filepath.Join(cacheDir, cacheFile)
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failure creating the cache dir for %q: %v", p, err)
	}
	if err := os.Remove(cachePath); !os.IsNotExist(err) {
		return fmt.Errorf("failure removing the old cache entry for %q: %v", p, err)
	}

	newInfo := fmt.Sprintf("%+v", &cachedInfo{
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		Ino:     ino,
	})
	return os.WriteFile(cachePath, []byte(newInfo), 0700)
}

func (s *Store) PathInfoMatchesCache(ctx context.Context, p snapshot.Path, info os.FileInfo) bool {
	sysInfo := info.Sys()
	if sysInfo == nil {
		return false
	}
	unix_info, ok := sysInfo.(*syscall.Stat_t)
	if !ok || unix_info == nil {
		return false
	}
	ino := unix_info.Ino
	cacheDir, cacheFile := s.pathCacheFile(p)
	bs, err := os.ReadFile(filepath.Join(cacheDir, cacheFile))
	if err != nil {
		return false
	}
	cachedInfoStr := string(bs)

	newInfo := fmt.Sprintf("%+v", &cachedInfo{
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		Ino:     ino,
	})
	return cachedInfoStr == newInfo
}
