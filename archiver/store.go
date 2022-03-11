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
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

func (s *Store) StoreObject(ctx context.Context, reader io.Reader) (h *snapshot.Hash, err error) {
	tmp, err := os.CreateTemp("", "archiver")
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

func (s *Store) StoreFile(ctx context.Context, p snapshot.Path, f *snapshot.File) (*snapshot.Hash, error) {
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

func (s *Store) ReadFile(ctx context.Context, p snapshot.Path) (*snapshot.Hash, *snapshot.File, error) {
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
	reader, err := s.ReadObject(ctx, h)
	if err != nil {
		return nil, nil, fmt.Errorf("failure looking up the file snapshot for %q: %v", p, err)
	}
	defer reader.Close()
	contents, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading file metadata from the reader: %v", err)
	}
	f, err := snapshot.ParseFile(string(contents))
	if err != nil {
		return nil, nil, fmt.Errorf("failure parsing the file snapshot for %q: %v", p, err)
	}
	return h, f, nil
}
