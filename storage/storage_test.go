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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/recursive-version-control-system/snapshot"
)

func TestSnapshotCurrent(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &LocalFiles{ArchiveDir: archive}

	workingDir := filepath.Join(dir, "working-dir")
	if err := os.Mkdir(workingDir, 0700); err != nil {
		t.Fatalf("failure creating the working directory for the test: %v", err)
	}
	file := filepath.Join(workingDir, "example.txt")
	dirPath := snapshot.Path(workingDir)
	p := snapshot.Path(file)

	// Take an initial snapshot
	if err := os.WriteFile(file, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file to snapshot: %v", err)
	}
	h1, f1, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure creating the initial snapshot for the file: %v", err)
	} else if h1 == nil {
		t.Error("unexpected nil hash for the file")
	} else if f1 == nil {
		t.Error("unexpected nil snapshot for the file")
	}

	// Verify that we can take the snapshot again without it changing
	h2, f2, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure replicating the initial snapshot for the file: %v", err)
	} else if got, want := h2, h1; !got.Equal(want) {
		t.Errorf("unexpected hash for the file; got %q, want %q", got, want)
	} else if got, want := f2.String(), f1.String(); got != want {
		t.Errorf("unexpected snapshot for the file; got %q, want %q", got, want)
	}

	// Modify the file and verify that the snapshot both changes and points to the parent
	if err := os.WriteFile(file, []byte("Goodbye, World!"), 0700); err != nil {
		t.Fatalf("failure updating the example file to snapshot: %v", err)
	}
	h3, f3, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure creating the updated snapshot for the file: %v", err)
	} else if h3 == nil {
		t.Error("unexpected nil hash for the updated file")
	} else if f3 == nil {
		t.Error("unexpected nil snapshot for the updated file")
	} else if h3.Equal(h1) {
		t.Error("failed to update the snapshot")
	} else if !f3.Parents[0].Equal(h1) {
		t.Errorf("updated snapshot did not include the original as its parent: %q", f3)
	}

	// Write a large file (> 1 MB) and verify that we can snapshot it
	// and read it back
	largeObjSize := 2 * 1024 * 1024
	var largeBytes bytes.Buffer
	largeBytes.Grow(largeObjSize)
	for i := 0; i < largeObjSize; i++ {
		largeBytes.WriteString(" ")
	}
	largeFile := filepath.Join(workingDir, "largeFile.txt")
	p2 := snapshot.Path(largeFile)
	if err := os.WriteFile(largeFile, largeBytes.Bytes(), 0700); err != nil {
		t.Fatalf("failure writing a large file: %v", err)
	}
	h4, f4, err := snapshot.Current(context.Background(), s, dirPath)
	if err != nil {
		t.Errorf("failure creating the updated snapshot containing a large file: %v", err)
	} else if h4 == nil {
		t.Error("unexpected nil hash for the working directory")
	} else if f4 == nil {
		t.Error("unexpected nil snapshot for the working directory")
	}

	var readLargeBytes bytes.Buffer
	h5, f5, err := snapshot.Current(context.Background(), s, p2)
	if err != nil {
		t.Errorf("failure getting the current snapshot for a large file: %v", err)
	} else if h5 == nil {
		t.Error("unexpected nil hash for the large file")
	} else if f5 == nil {
		t.Error("unexpected nil snapshot for the large file")
	} else if largeBytesReader, err := s.ReadObject(context.Background(), f5.Contents); err != nil {
		t.Errorf("failure opening the contents reader of a large file: %v", err)
	} else if _, err := readLargeBytes.ReadFrom(largeBytesReader); err != nil {
		t.Errorf("failure reading back the contents of a large file: %v", err)
	} else if diff := cmp.Diff(string(largeBytes.Bytes()), string(readLargeBytes.Bytes())); len(diff) > 0 {
		t.Errorf("wrong contents read back for a large file: diff %s", diff)
	}

	// Confirm that the stored large object contents are encrypted.
	objPath, objName := objectName(f5.Contents, filepath.Join(s.ArchiveDir, largeObjectStorageDir), true)
	if _, err := os.Stat(filepath.Join(objPath, objName)); err != nil {
		t.Errorf("failure finding the stored object contents in the expected location: %v", err)
	} else {
		var readRawBytes bytes.Buffer
		reader, err := os.Open(filepath.Join(objPath, objName))
		if err != nil {
			t.Errorf("failure opening the stored object contents: %v", err)
		} else if _, err := readRawBytes.ReadFrom(reader); err != nil {
			t.Errorf("failure reading the raw stored object contents: %v", err)
		} else if diff := cmp.Diff(string(readRawBytes.Bytes()), string(largeBytes.Bytes())); len(diff) == 0 {
			t.Error("failed to encrypt the large object")
		}
	}
}

func TestLinkSnapshot(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &LocalFiles{ArchiveDir: archive}

	workingDir := filepath.Join(dir, "working-dir")
	if err := os.Mkdir(workingDir, 0700); err != nil {
		t.Fatalf("failure creating the working directory for the test: %v", err)
	}

	file1 := filepath.Join(workingDir, "example1.txt")
	file2 := filepath.Join(workingDir, "example2.txt")
	link := filepath.Join(workingDir, "link")
	p := snapshot.Path(link)

	// Take an initial snapshot
	if err := os.WriteFile(file1, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the first example file: %v", err)
	}
	if err := os.WriteFile(file2, []byte("Also hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the second example file: %v", err)
	}
	if err := os.Symlink(file1, link); err != nil {
		t.Fatalf("failure creating the example symlink to snapshot: %v", err)
	}

	h1, f1, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure creating the initial snapshot for a symlink: %v", err)
	} else if h1 == nil {
		t.Error("unexpected nil hash for the symlink")
	} else if f1 == nil {
		t.Error("unexpected nil snapshot for the symlink")
	} else if !f1.IsLink() {
		t.Error("unexpected snapshot type for the symlink")
	}

	// Verify that we can take the snapshot again without it changing
	h2, f2, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure replicating the initial snapshot for the symlink: %v", err)
	} else if got, want := h2, h1; !got.Equal(want) {
		t.Errorf("unexpected hash for the symlink; got %q, want %q", got, want)
	} else if got, want := f2.String(), f1.String(); got != want {
		t.Errorf("unexpected snapshot for the symlink; got %q, want %q", got, want)
	}

	// Modify the contents of the linked file and verify that the snapshot of the symlink does not change
	if err := os.WriteFile(file1, []byte("Goodbye, World!"), 0700); err != nil {
		t.Fatalf("failure updating the contents of the example symlink target: %v", err)
	}
	h3, f3, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure replicating the initial snapshot for the symlink: %v", err)
	} else if got, want := h3, h1; !got.Equal(want) {
		t.Errorf("unexpected hash for the symlink; got %q, want %q", got, want)
	} else if got, want := f3.String(), f1.String(); got != want {
		t.Errorf("unexpected snapshot for the symlink; got %q, want %q", got, want)
	}

	if err := os.Remove(link); err != nil {
		t.Fatalf("error removing the symlink: %v", err)
	} else if err := os.Symlink(file2, link); err != nil {
		t.Fatalf("error recreating the symlink: %v", err)
	}

	h4, f4, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Errorf("failure creating the updated snapshot for the symlink: %v", err)
	} else if h4 == nil {
		t.Error("unexpected nil hash for the updated symlink")
	} else if f4 == nil {
		t.Error("unexpected nil snapshot for the updated symlink")
	} else if h4.Equal(h1) {
		t.Error("failed to update the snapshot")
	} else if !f4.Parents[0].Equal(h1) {
		t.Errorf("updated snapshot did not include the original as its parent: %q", f3)
	}
}

func TestDirSnapshot(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &LocalFiles{ArchiveDir: archive}

	workingDir := filepath.Join(dir, "working-dir")
	if err := os.Mkdir(workingDir, 0700); err != nil {
		t.Fatalf("failure creating the working directory for the test: %v", err)
	}

	// Setup multiple levels of directory that we will snapshot
	containerDir := filepath.Join(workingDir, "container")
	containerPath := snapshot.Path(containerDir)
	nestedDir := filepath.Join(containerDir, "nested")
	nestedPath := snapshot.Path(nestedDir)
	if err := os.MkdirAll(nestedDir, 0700); err != nil {
		t.Fatalf("failure creating the test directories: %v", err)
	}
	file1 := filepath.Join(nestedDir, "example1.txt")
	file1Path := snapshot.Path(file1)
	file2 := filepath.Join(nestedDir, "example2.txt")
	file2Path := snapshot.Path(file2)
	// Take an initial snapshot
	if err := os.WriteFile(file1, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the first example file to snapshot: %v", err)
	}
	if err := os.WriteFile(file2, []byte("Also... hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the second example file to snapshot: %v", err)
	}
	link := filepath.Join(nestedDir, "link")
	linkPath := snapshot.Path(link)
	if err := os.Symlink(file1, link); err != nil {
		t.Fatalf("failure creating the example symlink to snapshot: %v", err)
	}
	containerHash, containerFile, err := snapshot.Current(context.Background(), s, containerPath)
	if err != nil {
		t.Errorf("failure creating the initial snapshot for the dir: %v", err)
	} else if containerHash == nil {
		t.Error("unexpected nil hash for the dir")
	} else if containerFile == nil {
		t.Error("unexpected nil snapshot for the dir")
	} else if !containerFile.IsDir() {
		t.Errorf("unexpected type for the dir snapshot: %q", containerFile.Mode)
	}

	nestedHash, nestedFile, err := snapshot.Current(context.Background(), s, nestedPath)
	if err != nil {
		t.Errorf("failure creating the initial snapshot for the nested dir: %v", err)
	} else if nestedHash == nil {
		t.Error("unexpected nil hash for the nested dir")
	} else if nestedFile == nil {
		t.Error("unexpected nil snapshot for the nested dir")
	} else if !nestedFile.IsDir() {
		t.Errorf("unexpected type for the nested dir snapshot: %q", nestedFile.Mode)
	}

	expectedTree := make(snapshot.Tree)
	expectedTree[snapshot.Path("nested")] = nestedHash
	expectedHash, err := snapshot.NewHash(strings.NewReader(expectedTree.String()))
	if err != nil {
		t.Errorf("failure hashing the expected tree: %v", err)
	} else if got, want := containerFile.Contents, expectedHash; !got.Equal(want) {
		t.Errorf("unexpected contents hash for the containing dir: got %q, want %q", got, want)
	}

	// Take a second snapshot and verify that it remains unchanged...
	containerHash2, containerFile2, err := snapshot.Current(context.Background(), s, containerPath)
	if err != nil {
		t.Errorf("failure creating the initial snapshot for the dir: %v", err)
	} else if got, want := containerHash2, containerHash; !got.Equal(want) {
		t.Errorf("unexpected hash for an unchanged directory; got %q, want %q", got, want)
	} else if got, want := containerFile2.String(), containerFile.String(); got != want {
		t.Errorf("unexpected snapshot for an unchanged directory; got %q, want %q", got, want)
	}

	// Look up the initial snapshots for all the nested files for later checks...
	file1Hash, file1Snapshot, err := s.FindSnapshot(context.Background(), file1Path)
	if err != nil {
		t.Errorf("failure looking up the snapshot for the first nested file: %v", err)
	} else if file1Hash == nil || file1Snapshot == nil {
		t.Errorf("missing snapshot for the first nested file: %q, %+v", file1Hash, file1Snapshot)
	}
	file2Hash, file2Snapshot, err := s.FindSnapshot(context.Background(), file2Path)
	if err != nil {
		t.Errorf("failure looking up the snapshot for the second nested file: %v", err)
	} else if file2Hash == nil || file2Snapshot == nil {
		t.Errorf("missing snapshot for the second nested file: %q, %+v", file2Hash, file2Snapshot)
	}
	linkHash, linkSnapshot, err := s.FindSnapshot(context.Background(), linkPath)
	if err != nil {
		t.Errorf("failure looking up the snapshot for the nested symlink: %v", err)
	} else if linkHash == nil || linkSnapshot == nil {
		t.Errorf("missing snapshot for the nested symlink: %q, %+v", linkHash, linkSnapshot)
	}

	// Perform a single nested update, and then re-snapshot...
	if err := os.WriteFile(file2, []byte("Goodbye, World!"), 0700); err != nil {
		t.Fatalf("failure updating the second example file to snapshot: %v", err)
	}
	containerHash3, containerFile3, err := snapshot.Current(context.Background(), s, containerPath)
	if err != nil {
		t.Errorf("failure creating the updated snapshot for the dir: %v", err)
	} else if containerHash3.Equal(containerHash) {
		t.Errorf("failed to update the hash for a nested change; got %q", containerHash3)
	} else if containerFile3.String() == containerFile.String() {
		t.Errorf("failed to update the snapshot for a nested change; got %+v", containerFile3)
	}

	// Compare the nested file snapshots for unchanged files to verify they are the same...
	if file1Hash2, file1Snapshot2, err := s.FindSnapshot(context.Background(), file1Path); err != nil {
		t.Errorf("failure looking up the snapshot for the first nested file: %v", err)
	} else if got, want := file1Hash2, file1Hash; !got.Equal(want) {
		t.Errorf("unexpected hash for an unchanged nested file: got %q, want %q", got, want)
	} else if got, want := file1Snapshot2.String(), file1Snapshot.String(); got != want {
		t.Errorf("unexpected snapshot for an unchanged nested file: got %q, want %q", got, want)
	}
	if linkHash2, linkSnapshot2, err := s.FindSnapshot(context.Background(), linkPath); err != nil {
		t.Errorf("failure looking up the snapshot for the nested link: %v", err)
	} else if got, want := linkHash2, linkHash; !got.Equal(want) {
		t.Errorf("unexpected hash for an unchanged nested symlink: got %q, want %q", got, want)
	} else if got, want := linkSnapshot2.String(), linkSnapshot.String(); got != want {
		t.Errorf("unexpected snapshot for an unchanged nested symlink: got %q, want %q", got, want)
	}

	// Compare the nested file snapshot for the changed file to verify that it has been updated...
	if file2Hash2, file2Snapshot2, err := s.FindSnapshot(context.Background(), file2Path); err != nil {
		t.Errorf("failure looking up the snapshot for the updated nested file: %v", err)
	} else if file2Hash2.Equal(file2Hash) {
		t.Errorf("unexpectedly unchanged hash for a changed nested file: got %q", file2Hash2)
	} else if file2Snapshot2.String() == file2Snapshot.String() {
		t.Errorf("unexpectedly unchanged snapshot for a changed nested file: got %+v", file2Snapshot2)
	}

	// Remove the nested file and verify that updated snapshots are correct...
	if err := os.Remove(file2); err != nil {
		t.Fatalf("failure removing a nested file: %v", err)
	}
	containerHash4, containerFile4, err := snapshot.Current(context.Background(), s, containerPath)
	if err != nil {
		t.Errorf("failure creating the updated snapshot for the dir after removing a nested file: %v", err)
	} else if containerHash4.Equal(containerHash3) {
		t.Errorf("failed to update the hash for a nested file removal; got %q", containerHash4)
	} else if containerFile4.String() == containerFile3.String() {
		t.Errorf("failed to update the snapshot for a nested file removal; got %+v", containerFile4)
	}
	if file2Hash3, file2Snapshot3, err := s.FindSnapshot(context.Background(), file2Path); err == nil {
		t.Errorf("unexpected hash and/or snapshot for a removed file: hash %q, snapshot %+v", file2Hash3, file2Snapshot3)
	}
}
