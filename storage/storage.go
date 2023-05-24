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

// Package storage defines the persistent storage of snapshots.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"filippo.io/age"

	"github.com/google/recursive-version-control-system/snapshot"
)

const (
	smallObjectStorageDir = "objects"
	largeObjectStorageDir = "largeObjects"
	localIdentityFile     = "x25519Identity"
)

// LocalFiles implementes the `snapshot.Storage` interface using the local file system.
//
// It is used to write and read snapshots to persistent storage.
type LocalFiles struct {
	ArchiveDir string
}

// Exclude reports whether or not the given path should be excluded from snapshotting.
//
// This should return true for any paths that are part of the underlying persistent storage.
func (s *LocalFiles) Exclude(p snapshot.Path) bool {
	return p == snapshot.Path(s.ArchiveDir)
}

func (s *LocalFiles) identity() (*age.X25519Identity, error) {
	if err := os.MkdirAll(s.ArchiveDir, os.FileMode(0700)); err != nil {
		return nil, fmt.Errorf("failure creating the archive dir: %w", err)
	}
	identityFile := filepath.Join(s.ArchiveDir, localIdentityFile)
	contents, err := os.ReadFile(identityFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failure reading the identity file: %w", err)
	}
	if err != nil {
		// The identity does not exist yet... create it.
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			return nil, fmt.Errorf("failure generating an identity: %w", err)
		}
		if err := os.WriteFile(identityFile, []byte(identity.String()), os.FileMode(0700)); err != nil {
			return nil, fmt.Errorf("failure writing the identity file: %w", err)
		}
		contents, err = os.ReadFile(identityFile)
		if err != nil {
			return nil, fmt.Errorf("failure reading back the written identity file: %w", err)
		}
	}
	identityStr := string(contents)
	return age.ParseX25519Identity(identityStr)
}

func (s *LocalFiles) recipient() (*age.X25519Recipient, error) {
	identity, err := s.identity()
	if err != nil {
		return nil, fmt.Errorf("failure retrieving the local identity: %w", err)
	}
	return identity.Recipient(), nil
}

func (s *LocalFiles) tmpFile(ctx context.Context, subpath string) (*os.File, error) {
	tmpDir := filepath.Join(s.ArchiveDir, subpath, "staging-dir")
	if err := os.MkdirAll(tmpDir, os.FileMode(0700)); err != nil {
		return nil, fmt.Errorf("failure creating the tmp dir: %w", err)
	}
	return os.CreateTemp(tmpDir, "archiver")
}

func (s *LocalFiles) objectStoragePath(ctx context.Context, objectsSubDir string, h *snapshot.Hash, encrypted bool) (filePath string, err error) {
	objPath, objName := objectName(h, filepath.Join(s.ArchiveDir, objectsSubDir), encrypted)
	if err := os.MkdirAll(objPath, os.FileMode(0700)); err != nil {
		return "", fmt.Errorf("failure creating the objects dir %q for %q: %w", objPath, h, err)
	}
	return filepath.Join(objPath, objName), nil
}

func (s *LocalFiles) StoreObject(ctx context.Context, size int64, reader io.Reader) (h *snapshot.Hash, err error) {
	var tmp *os.File
	var encrypted bool
	objectsSubDir := smallObjectStorageDir
	if size > 1024*1024 {
		objectsSubDir = largeObjectStorageDir
		encrypted = true
	}
	tmp, err = s.tmpFile(ctx, objectsSubDir)
	if err != nil {
		return nil, fmt.Errorf("failure creating a temp file: %w", err)
	}
	var dest io.WriteCloser = tmp
	if encrypted {
		rvcsLocalRecipient, err := s.recipient()
		if err != nil {
			return nil, fmt.Errorf("failure identifying the local rvcs encryption recipient: %w", err)
		}
		dest, err = age.Encrypt(dest, rvcsLocalRecipient)
		if err != nil {
			return nil, fmt.Errorf("failure creating an encrypted writer: %w", err)
		}
	}
	defer func() {
		dest.Close()
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	reader = io.TeeReader(reader, dest)
	h, err = snapshot.NewHash(reader)
	if err != nil {
		return nil, fmt.Errorf("failure hashing an object: %w", err)
	}
	if h == nil {
		return nil, errors.New("unexpected nil hash for an object")
	}
	storageLocation, err := s.objectStoragePath(ctx, objectsSubDir, h, encrypted)
	if err != nil {
		return nil, fmt.Errorf("failure preparing the storage location for %q: %w", h, err)
	}
	if err := os.Rename(tmp.Name(), storageLocation); err != nil {
		return nil, fmt.Errorf("failure writing the object file for %q: %w", h, err)
	}
	return h, nil
}

func objectName(h *snapshot.Hash, parentDir string, encrypted bool) (dir string, name string) {
	defer func() {
		if encrypted {
			name += ".age"
		}
	}()
	functionDir := filepath.Join(parentDir, h.Function())
	if len(h.HexContents()) > 4 {
		return filepath.Join(functionDir, h.HexContents()[0:2], h.HexContents()[2:4]), h.HexContents()[4:]
	} else if len(h.HexContents()) > 2 {
		return filepath.Join(functionDir, h.HexContents()[0:2]), h.HexContents()[2:]
	}
	return functionDir, h.HexContents()
}

// decryptingReader extends the age-provided reader with the Close method.
type decryptingReader struct {
	originalReader io.ReadCloser
	ageDecrypter   io.Reader
}

func (d *decryptingReader) Read(p []byte) (n int, err error) {
	return d.ageDecrypter.Read(p)
}

func (d *decryptingReader) Close() error {
	return d.originalReader.Close()
}

func (s *LocalFiles) decryptingReader(reader io.ReadCloser) (io.ReadCloser, error) {
	identity, err := s.identity()
	if err != nil {
		return nil, fmt.Errorf("failure reading the local identity: %w", err)
	}
	dr, err := age.Decrypt(reader, identity)
	if err != nil {
		return nil, fmt.Errorf("failure decrypting the underlying object: %w", err)
	}
	return &decryptingReader{
		originalReader: reader,
		ageDecrypter:   dr,
	}, nil
}

func (s *LocalFiles) ReadObject(ctx context.Context, h *snapshot.Hash) (io.ReadCloser, error) {
	if h == nil {
		return nil, errors.New("there is no object associated with the nil hash")
	}
	objPath, objName := objectName(h, filepath.Join(s.ArchiveDir, smallObjectStorageDir), false)
	if r, err := os.Open(filepath.Join(objPath, objName)); err == nil {
		return r, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failure opening the object storage location: %w", err)
	}
	// The object was not found in the small object storage; look in the large object storage instead...
	objPath, objName = objectName(h, filepath.Join(s.ArchiveDir, largeObjectStorageDir), true)
	reader, err := os.Open(filepath.Join(objPath, objName))
	if err != nil {
		return nil, err
	}
	dr, err := s.decryptingReader(reader)
	if err == nil {
		return dr, nil
	}
	reader.Close()
	return nil, err
}

func (s *LocalFiles) mappedPathsDir(p snapshot.Path) string {
	return filepath.Join(s.ArchiveDir, "mappedPaths", string(p))
}

func (s *LocalFiles) pathHashFile(p snapshot.Path) (dir string, name string, err error) {
	pathHash, err := snapshot.NewHash(strings.NewReader(string(p)))
	if err != nil {
		return "", "", fmt.Errorf("failure hashing the path name %q: %w", p, err)
	}
	if pathHash == nil {
		return "", "", fmt.Errorf("unexpected nil hash for the path %q", p)
	}
	dir, name = objectName(pathHash, filepath.Join(s.ArchiveDir, "paths"), false)
	return dir, name, nil
}

func (s *LocalFiles) StoreSnapshot(ctx context.Context, p snapshot.Path, f *snapshot.File) (*snapshot.Hash, error) {
	if err := os.MkdirAll(s.mappedPathsDir(p), 0700); err != nil {
		return nil, fmt.Errorf("failure creating the mapped paths dir entry for %q: %w", p, err)
	}
	bs := []byte(f.String())
	h, err := s.StoreObject(ctx, int64(len(bs)), bytes.NewReader(bs))
	if err != nil {
		return nil, fmt.Errorf("failure saving file metadata for %+v: %w", f, err)
	}
	pathHashDir, pathHashFile, err := s.pathHashFile(p)
	if err != nil {
		return nil, fmt.Errorf("failure calculating the path hash file location for %q: %w", p, err)
	}
	if err := os.MkdirAll(pathHashDir, 0700); err != nil {
		return nil, fmt.Errorf("failure creating the paths dir for %q: %w", p, err)
	}
	if err := os.WriteFile(filepath.Join(pathHashDir, pathHashFile), []byte(h.String()), 0600); err != nil {
		return nil, fmt.Errorf("failure writing the hash for path %q: %w", p, err)
	}
	var currTree snapshot.Tree
	if f.IsDir() {
		currTree, err = s.ListDirectorySnapshotContents(ctx, h, f)
		if err != nil {
			return nil, fmt.Errorf("failure listing the contents of the new snapshot: %w", err)
		}
	}
	mappedSubPaths, err := os.ReadDir(s.mappedPathsDir(p))
	if err != nil {
		return nil, fmt.Errorf("failure reading the mapped subpaths of %q: %w", p, err)
	}
	for _, entry := range mappedSubPaths {
		child := snapshot.Path(entry.Name())
		if currTree != nil {
			if _, ok := currTree[child]; ok {
				continue
			}
		}
		// The previous child entry was removed.
		subpath := p.Join(child)
		if err := s.RemoveMappingForPath(ctx, subpath); err != nil {
			return nil, fmt.Errorf("failure removing path mapping for removed child %q: %w", child, err)
		}
	}
	return h, nil
}

func (s *LocalFiles) ReadSnapshot(ctx context.Context, h *snapshot.Hash) (*snapshot.File, error) {
	reader, err := s.ReadObject(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("failure looking up the file snapshot for %q: %w", h, err)
	}
	defer reader.Close()
	contents, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failure reading file metadata from the reader: %w", err)
	}
	f, err := snapshot.ParseFile(string(contents))
	if err != nil {
		return nil, fmt.Errorf("failure parsing the file snapshot for %q: %w", h, err)
	}
	return f, nil
}

func (s *LocalFiles) FindSnapshot(ctx context.Context, p snapshot.Path) (*snapshot.Hash, *snapshot.File, error) {
	pathHashDir, pathHashFile, err := s.pathHashFile(p)
	if err != nil {
		return nil, nil, fmt.Errorf("failure calculating the path hash file location for %q: %w", p, err)
	}
	bs, err := os.ReadFile(filepath.Join(pathHashDir, pathHashFile))
	if err != nil {
		return nil, nil, err
	}
	fileHashStr := string(bs)
	h, err := snapshot.ParseHash(fileHashStr)
	if err != nil {
		return nil, nil, fmt.Errorf("failure parsing the hash %q: %w", fileHashStr, err)
	}
	f, err := s.ReadSnapshot(ctx, h)
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the file snapshot for %q: %w", h, err)
	}
	return h, f, nil
}

// ListDirectorySnapshotContents returns the parsed `*snapshot.Tree` object listing the contents of `f`.
//
// The supplied `*snapshot.File` object must correspond to a directory.
func (s *LocalFiles) ListDirectorySnapshotContents(ctx context.Context, h *snapshot.Hash, f *snapshot.File) (snapshot.Tree, error) {
	if !f.IsDir() {
		return nil, fmt.Errorf("%q is not the snapshot of a directory", h)
	}
	contentsReader, err := s.ReadObject(ctx, f.Contents)
	if err != nil {
		return nil, fmt.Errorf("failure opening the contents of %q: %w", h, err)
	}
	contents, err := io.ReadAll(contentsReader)
	if err != nil {
		return nil, fmt.Errorf("failure reading the contents of %q: %w", h, err)
	}
	tree, err := snapshot.ParseTree(string(contents))
	if err != nil {
		return nil, fmt.Errorf("failure parsing the directory contents of the snapshot %q: %w", h, err)
	}
	return tree, nil
}

func (s *LocalFiles) RemoveMappingForPath(ctx context.Context, p snapshot.Path) error {
	if err := os.RemoveAll(s.mappedPathsDir(p)); err != nil {
		return fmt.Errorf("failure removing the mapped paths entry for %q: %w", p, err)
	}
	h, f, err := s.FindSnapshot(ctx, p)
	if os.IsNotExist(err) {
		// There is no file snapshot corresponding to the given path,
		// so we have nothing to do.
		return nil
	}
	pathHashDir, pathHashFile, err := s.pathHashFile(p)
	if err != nil {
		return fmt.Errorf("failure calculating the path hash file location for %q: %w", p, err)
	}
	mappingPath := filepath.Join(pathHashDir, pathHashFile)
	if err := os.Remove(mappingPath); err != nil {
		return fmt.Errorf("failure removing the mapping from %q to %q: %w", p, h, err)
	}
	if !f.IsDir() {
		return nil
	}
	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure listing the contents of %q: %w", h, err)
	}
	for child, _ := range tree {
		childPath := p.Join(child)
		if err := s.RemoveMappingForPath(ctx, childPath); err != nil {
			return fmt.Errorf("failure removing mapping for the child path %q: %w", child, err)
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

func (s *LocalFiles) pathCacheFile(p snapshot.Path) (dir string, name string, err error) {
	pathHash, err := snapshot.NewHash(strings.NewReader(string(p)))
	if err != nil {
		return "", "", fmt.Errorf("failure hashing the path name %q: %w", p, err)
	}
	if pathHash == nil {
		return "", "", fmt.Errorf("unexpected nil hash for the path %q", p)
	}
	dir, name = objectName(pathHash, filepath.Join(s.ArchiveDir, "cache"), false)
	return dir, name, nil
}

func (s *LocalFiles) CachePathInfo(ctx context.Context, p snapshot.Path, info os.FileInfo) error {
	sysInfo := info.Sys()
	if sysInfo == nil {
		return nil
	}
	unix_info, ok := sysInfo.(*syscall.Stat_t)
	if !ok || unix_info == nil {
		return nil
	}
	ino := unix_info.Ino

	cacheDir, cacheFile, err := s.pathCacheFile(p)
	if err != nil {
		return fmt.Errorf("failure constructing the cache dir path for %q: %w", p, err)
	}
	cachePath := filepath.Join(cacheDir, cacheFile)
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failure creating the cache dir for %q: %w", p, err)
	}
	if err := os.Remove(cachePath); !os.IsNotExist(err) {
		return fmt.Errorf("failure removing the old cache entry for %q: %w", p, err)
	}

	newInfo := fmt.Sprintf("%+v", &cachedInfo{
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		Ino:     ino,
	})
	return os.WriteFile(cachePath, []byte(newInfo), 0700)
}

func (s *LocalFiles) PathInfoMatchesCache(ctx context.Context, p snapshot.Path, info os.FileInfo) bool {
	sysInfo := info.Sys()
	if sysInfo == nil {
		return false
	}
	unix_info, ok := sysInfo.(*syscall.Stat_t)
	if !ok || unix_info == nil {
		return false
	}
	ino := unix_info.Ino
	cacheDir, cacheFile, err := s.pathCacheFile(p)
	if err != nil {
		return false
	}
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

func (s *LocalFiles) idFile(id *snapshot.Identity) (dir string, name string, err error) {
	idHash, err := snapshot.NewHash(strings.NewReader(id.String()))
	if err != nil {
		return "", "", fmt.Errorf("failure hashing the identity %q: %w", id, err)
	}
	if idHash == nil {
		return "", "", fmt.Errorf("unexpected nil hash for the identity %q", id)
	}
	dir, name = objectName(idHash, filepath.Join(s.ArchiveDir, "identities"), false)
	return dir, name, nil
}

func (s *LocalFiles) LatestSignatureForIdentity(ctx context.Context, id *snapshot.Identity) (*snapshot.Hash, error) {
	idDir, idFile, err := s.idFile(id)
	if err != nil {
		return nil, fmt.Errorf("failure constructing the id dir path for %q: %w", id, err)
	}
	idPath := filepath.Join(idDir, idFile)
	bs, err := os.ReadFile(idPath)
	if os.IsNotExist(err) {
		// The given identity is not known
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failure reading the identity file for %q: %w", id, err)
	}
	h, err := snapshot.ParseHash(string(bs))
	if err != nil {
		return nil, fmt.Errorf("failure parsing the hash for identity %q: %w", id, err)
	}
	return h, nil
}

func (s *LocalFiles) UpdateSignatureForIdentity(ctx context.Context, id *snapshot.Identity, h *snapshot.Hash) error {
	idDir, idFile, err := s.idFile(id)
	if err != nil {
		return fmt.Errorf("failure constructing the id dir path for %q: %w", id, err)
	}
	idPath := filepath.Join(idDir, idFile)
	if h == nil {
		if err := os.Remove(idPath); !os.IsNotExist(err) {
			return fmt.Errorf("failure removing the identity entry for %q: %w", id, err)
		}
	}
	if err := os.MkdirAll(idDir, 0700); err != nil {
		return fmt.Errorf("failure creating the id dir for %q: %w", id, err)
	}
	return os.WriteFile(idPath, []byte(h.String()), 0700)
}
