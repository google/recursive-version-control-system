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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func TestMergeRegularFile(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{ArchiveDir: archive}

	original := filepath.Join(dir, "original.txt")
	originalPath := snapshot.Path(original)

	// Take an initial snapshot
	if err := os.WriteFile(original, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file to snapshot: %v", err)
	}
	h1, f1, err := snapshot.Current(context.Background(), s, originalPath)
	if err != nil {
		t.Fatalf("failure creating the initial snapshot for the file: %v", err)
	} else if h1 == nil {
		t.Fatalf("unexpected nil hash for the file")
	} else if f1 == nil {
		t.Fatalf("unexpected nil snapshot for the file")
	}

	clone := filepath.Join(dir, "clone.txt")
	clonePath := snapshot.Path(clone)

	if err := Merge(context.Background(), s, h1, clonePath); err != nil {
		t.Fatalf("failure checking out the file snapshot %q: %v", h1, err)
	}

	// Validate that the cloned file matches the original...
	verifyFilesMatch(t, original, clone)
	h2, f2, err := snapshot.Current(context.Background(), s, clonePath)
	if err != nil {
		t.Errorf("failure creating the cloned snapshot for the file: %v", err)
	} else if got, want := h2, h1; !got.Equal(want) {
		t.Errorf("unexpected hash for the cloned file; got %q, want %q", got, want)
	} else if got, want := f2.String(), f1.String(); got != want {
		t.Errorf("unexpected contents for the cloned snapshot for the file: got %q, want %q", got, want)
	}
}

func TestMergeSymlink(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{ArchiveDir: archive}

	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file to target: %v", err)
	}

	original := filepath.Join(dir, "original.txt")
	originalPath := snapshot.Path(original)
	if err := os.Symlink(target, original); err != nil {
		t.Fatalf("failure creating the example symlink: %v", err)
	}

	// Take an initial snapshot
	h1, f1, err := snapshot.Current(context.Background(), s, originalPath)
	if err != nil {
		t.Fatalf("failure creating the initial snapshot for the symlink: %v", err)
	} else if h1 == nil {
		t.Fatalf("unexpected nil hash for the symlink")
	} else if f1 == nil {
		t.Fatalf("unexpected nil snapshot for the symlink")
	}

	clone := filepath.Join(dir, "clone.txt")
	clonePath := snapshot.Path(clone)
	if err := Merge(context.Background(), s, h1, clonePath); err != nil {
		t.Fatalf("failure checking out the symlink snapshot %q: %v", h1, err)
	}

	// Validate that the cloned file matches the original...
	if originalTarget, err := os.Readlink(original); err != nil {
		t.Errorf("failure reading the original symlink target: %v", err)
	} else if clonedTarget, err := os.Readlink(clone); err != nil {
		t.Errorf("failure reading the cloned symlink target: %v", err)
	} else if got, want := originalTarget, clonedTarget; got != want {
		t.Errorf("unexpected target for cloned symlink; got %q, want %q", got, want)
	}
	h2, f2, err := snapshot.Current(context.Background(), s, clonePath)
	if err != nil {
		t.Errorf("failure creating the cloned snapshot for the symlink: %v", err)
	} else if got, want := h2, h1; !got.Equal(want) {
		t.Errorf("unexpected hash for the cloned symlink; got %q, want %q", got, want)
	} else if got, want := f2.String(), f1.String(); got != want {
		t.Errorf("unexpected contents for the cloned snapshot for the symlink: got %q, want %q", got, want)
	}
}

func TestMergeExcludedDir(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{ArchiveDir: archive}

	file := filepath.Join(dir, "example.txt")
	if err := os.WriteFile(file, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file: %v", err)
	}

	dirPath := snapshot.Path(dir)
	h1, f1, err := snapshot.Current(context.Background(), s, dirPath)
	if err != nil {
		t.Fatalf("failure creating the initial snapshot for the directory: %v", err)
	} else if h1 == nil {
		t.Fatalf("unexpected nil hash for the directory")
	} else if f1 == nil {
		t.Fatalf("unexpected nil snapshot for the directory")
	}

	// Verify that the snapshot does not include the storage archive...
	if tree, err := s.ListDirectorySnapshotContents(context.Background(), h1, f1); err != nil {
		t.Fatalf("failure reading the contents of the directory snapshot %q: %v", h1, err)
	} else if _, ok := tree[snapshot.Path("archive")]; ok {
		t.Error("unexpectedly included the storage archive in the snapshot")
	}

	if err := Merge(context.Background(), s, h1, dirPath); err != nil {
		t.Fatalf("failure checking out the directory snapshot %q: %v", h1, err)
	}

	// Verify that the storage archive has not been removed...
	if f2, err := s.ReadSnapshot(context.Background(), h1); err != nil {
		t.Errorf("failure reading the snapshot back from storage: %v", err)
	} else if got, want := f2.String(), f1.String(); got != want {
		t.Errorf("unexpected snapshot read back from storage: got %q, want %q", got, want)
	}
}

func TestMergeDir(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{ArchiveDir: archive}

	workingDir := filepath.Join(dir, "working-dir")
	if err := os.Mkdir(workingDir, 0700); err != nil {
		t.Fatalf("failure creating the working directory for the test: %v", err)
	}
	dirPath := snapshot.Path(workingDir)
	file1 := filepath.Join(workingDir, "example1.txt")
	file2 := filepath.Join(workingDir, "example2.txt")
	file3 := filepath.Join(workingDir, "example3.txt")

	if err := os.WriteFile(file1, []byte("Hello, World 1!"), 0700); err != nil {
		t.Fatalf("failure creating the example file 1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("Hello, World 2!"), 0700); err != nil {
		t.Fatalf("failure creating the example file 2: %v", err)
	}
	if err := os.WriteFile(file3, []byte("Hello, World 3!"), 0700); err != nil {
		t.Fatalf("failure creating the example file 3: %v", err)
	}

	h1, f1, err := snapshot.Current(context.Background(), s, dirPath)
	if err != nil {
		t.Fatalf("failure creating the initial snapshot for the directory: %v", err)
	} else if h1 == nil {
		t.Fatalf("unexpected nil hash for the directory")
	} else if f1 == nil {
		t.Fatalf("unexpected nil snapshot for the directory")
	}

	cloneDir := filepath.Join(dir, "clone-dir")
	cloneDirPath := snapshot.Path(cloneDir)
	if err := Merge(context.Background(), s, h1, cloneDirPath); err != nil {
		t.Fatalf("failure checking out the directory snapshot %q: %v", h1, err)
	}
	verifyFilesMatch(t, file1, filepath.Join(cloneDir, "example1.txt"))
	verifyFilesMatch(t, file2, filepath.Join(cloneDir, "example2.txt"))
	verifyFilesMatch(t, file3, filepath.Join(cloneDir, "example3.txt"))
}
