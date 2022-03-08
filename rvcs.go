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

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Hash interface {
	Function() string
	HexContents() string
	ObjectName(parentDir string) (dir string, name string)
	Equal(Hash) bool
	fmt.Stringer
}

type sha256Hash struct {
	Contents string
}

func (h *sha256Hash) Function() string {
	return "sha256"
}

func (h *sha256Hash) HexContents() string {
	return h.Contents
}

func (h *sha256Hash) Equal(other Hash) bool {
	return h.Function() == other.Function() && h.HexContents() == other.HexContents()
}

func (h *sha256Hash) String() string {
	return h.Function() + ":" + h.Contents
}

func (h *sha256Hash) ObjectName(parentDir string) (dir string, name string) {
	return filepath.Join(parentDir, h.Function(), h.Contents[0:2], h.Contents[2:4]), h.Contents[4:]
}

type Path string
type Tree map[Path]Hash

func (t Tree) String() string {
	var lines []string
	for p, h := range t {
		encodedFileName := base64.RawStdEncoding.EncodeToString([]byte(p))
		lines = append(lines, encodedFileName+" "+h.String())
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

type File struct {
	Mode     string
	Contents Hash
	Parents  []Hash
}

func (f *File) IsDir() bool {
	return strings.HasPrefix(f.Mode, "d")
}

func (f *File) String() string {
	lines := []string{f.Mode, f.Contents.String()}
	for _, parent := range f.Parents {
		lines = append(lines, parent.String())
	}
	return strings.Join(lines, "\n")
}

func ParseFile(reader io.Reader) (*File, error) {
	encoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failure reading file metadata from the reader: %v", err)
	}
	lines := strings.Split(string(encoded), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("malformed file metadata: %q", encoded)
	}
	var hashes []Hash
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "sha256:") {
			return nil, fmt.Errorf("unsupported hash function for %q", line)
		}
		hashes = append(hashes, &sha256Hash{strings.TrimPrefix(line, "sha256:")})
	}
	f := &File{
		Mode:     lines[0],
		Contents: hashes[0],
		Parents:  hashes[1:],
	}
	return f, nil
}

type Archiver interface {
	Exclude(Path) bool
	StoreObject(context.Context, io.Reader) (Hash, error)
	ReadObject(context.Context, Hash) (io.ReadCloser, error)
	StoreFile(context.Context, Path, *File) (Hash, error)
	ReadFile(context.Context, Path) (Hash, *File, error)
}

func SnapshotFileMetadata(ctx context.Context, a Archiver, p Path, info os.FileInfo, contentsHash Hash) (Hash, error) {
	modeLine := info.Mode().String()
	prevFileHash, prev, err := a.ReadFile(ctx, p)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failure looking up the previous file snapshot: %v", err)
	}
	if prev != nil && prev.Contents.String() == contentsHash.String() && prev.Mode == modeLine {
		// The file is unchanged from the last snapshot...
		return prevFileHash, nil
	}
	f := &File{
		Contents: contentsHash,
		Mode:     modeLine,
	}
	if prev != nil {
		f.Parents = []Hash{prevFileHash}
	}
	h, err := a.StoreFile(ctx, p, f)
	if err != nil {
		return nil, fmt.Errorf("failure saving the latest file metadata for %q: %v", p, err)
	}
	return h, nil
}

func SnapshotRegularFile(ctx context.Context, a Archiver, p Path, info os.FileInfo, contents io.Reader) (Hash, error) {
	h, err := a.StoreObject(ctx, contents)
	if err != nil {
		return nil, fmt.Errorf("failure storing an object: %v", err)
	}
	return SnapshotFileMetadata(ctx, a, p, info, h)
}

func SnapshotDirectory(ctx context.Context, a Archiver, p Path, info os.FileInfo, contents *os.File) (Hash, error) {
	entries, err := contents.ReadDir(0)
	if err != nil {
		return nil, fmt.Errorf("failure reading the filesystem contents of the directory %q: %v", p, err)
	}
	childHashes := make(Tree)
	for _, entry := range entries {
		childPath := Path(filepath.Join(string(p), entry.Name()))
		if a.Exclude(childPath) {
			continue
		}
		childHash, err := Snapshot(ctx, a, childPath)
		if err != nil {
			return nil, fmt.Errorf("failure hashing the child dir %q: %v", childPath, err)
		}
		childHashes[Path(entry.Name())] = childHash
	}
	contentsJson := []byte(childHashes.String())
	contentsHash, err := a.StoreObject(ctx, bytes.NewReader(contentsJson))
	return SnapshotFileMetadata(ctx, a, p, info, contentsHash)
}

func Snapshot(ctx context.Context, a Archiver, p Path) (Hash, error) {
	contents, err := os.Open(string(p))
	if err != nil {
		return nil, fmt.Errorf("failure reading the file %q: %v", p, err)
	}
	defer contents.Close()

	info, err := contents.Stat()
	if err != nil {
		return nil, fmt.Errorf("failure reading the filesystem metadata for %q: %v", p, err)
	}
	if info.IsDir() {
		return SnapshotDirectory(ctx, a, p, info, contents)
	} else {
		return SnapshotRegularFile(ctx, a, p, info, contents)
	}
}

type archiver struct {
	ArchiveDir string
}

func (a *archiver) Exclude(p Path) bool {
	return p == Path(a.ArchiveDir)
}

func (a *archiver) StoreObject(ctx context.Context, reader io.Reader) (h Hash, err error) {
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
	h = &sha256Hash{fmt.Sprintf("%x", sum.Sum(nil))}
	objPath, objName := h.ObjectName(filepath.Join(a.ArchiveDir, "objects"))
	if err := os.MkdirAll(objPath, os.FileMode(0700)); err != nil {
		return nil, fmt.Errorf("failure creating the object dir for %q: %v", h, err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(objPath, objName)); err != nil {
		return nil, fmt.Errorf("failure writing the object file for %q: %v", h, err)
	}
	return h, nil
}

func (a *archiver) ReadObject(ctx context.Context, h Hash) (io.ReadCloser, error) {
	objPath, objName := h.ObjectName(filepath.Join(a.ArchiveDir, "objects"))
	return os.Open(filepath.Join(objPath, objName))
}

func (a *archiver) pathHashFile(p Path) (dir string, name string) {
	pathHash := &sha256Hash{fmt.Sprintf("%x", sha256.Sum256([]byte(p)))}
	return pathHash.ObjectName(filepath.Join(a.ArchiveDir, "paths"))
}

func (a *archiver) StoreFile(ctx context.Context, p Path, f *File) (Hash, error) {
	bs := []byte(f.String())
	h, err := a.StoreObject(ctx, bytes.NewReader(bs))
	if err != nil {
		return nil, fmt.Errorf("failure saving file metadata for %+v: %v", f, err)
	}
	pathHashDir, pathHashFile := a.pathHashFile(p)
	if err := os.MkdirAll(pathHashDir, 0700); err != nil {
		return nil, fmt.Errorf("failure creating the paths dir for %q: %v", p, err)
	}
	if err := os.WriteFile(filepath.Join(pathHashDir, pathHashFile), []byte(h.String()), 0600); err != nil {
		return nil, fmt.Errorf("failure writing the hash for path %q: %v", p, err)
	}
	return h, nil
}

func (a *archiver) ReadFile(ctx context.Context, p Path) (Hash, *File, error) {
	pathHashDir, pathHashFile := a.pathHashFile(p)
	bs, err := os.ReadFile(filepath.Join(pathHashDir, pathHashFile))
	if err != nil {
		return nil, nil, err
	}
	fileHashStr := string(bs)
	if !strings.HasPrefix(fileHashStr, "sha256:") {
		return nil, nil, fmt.Errorf("unsupported hash function for %q", fileHashStr)
	}
	h := &sha256Hash{strings.TrimPrefix(fileHashStr, "sha256:")}
	reader, err := a.ReadObject(ctx, h)
	if err != nil {
		return nil, nil, fmt.Errorf("failure looking up the file snapshot for %q: %v", p, err)
	}
	defer reader.Close()
	f, err := ParseFile(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failure parsing the file snapshot for %q: %v", p, err)
	}
	return h, f, nil
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "snapshot" {
		fmt.Printf("Usage: %s snapshot [<PATH>]\n", os.Args[0])
		os.Exit(1)
	}
	var path string
	if len(os.Args) > 2 {
		path = os.Args[2]
	} else {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Error determining the current working directory: %v\n", err)
		}
		path = wd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("Failure resolving the absolute path of %q: %v", path, err)
	}
	path = abs

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failure resolving the user's home dir: %v\n", err)
	}
	a := &archiver{filepath.Join(home, ".archive")}
	ctx := context.Background()
	if h, err := Snapshot(ctx, a, Path(path)); err != nil {
		log.Fatalf("Failure snapshotting the directory %q: %v\n", path, err)
	} else {
		fmt.Printf("Snapshotted %q to %q\n", path, h)
	}
}
