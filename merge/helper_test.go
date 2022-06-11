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

func TestMergeWithHelperNoConflict(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{ArchiveDir: archive}

	original := filepath.Join(dir, "original.txt")
	originalPath := snapshot.Path(original)

	// Take an initial snapshot
	if err := os.WriteFile(original, []byte("A\nB\nC\nD\nE\n"), 0700); err != nil {
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

	clone1 := filepath.Join(dir, "clone1.txt")
	clone1Path := snapshot.Path(clone1)
	if err := Checkout(context.Background(), s, h1, clone1Path); err != nil {
		t.Fatalf("failure checking out the file snapshot %q to %q: %v", h1, clone1Path, err)
	}
	if err := os.WriteFile(clone1, []byte("A\nX\nB\nY\nC\nD\nE\n"), 0700); err != nil {
		t.Fatalf("failure updating the first clone of the example file to snapshot: %v", err)
	}
	h2, _, err := snapshot.Current(context.Background(), s, clone1Path)
	if err != nil {
		t.Fatalf("failure creating the first updated snapshot for the file: %v", err)
	} else if h2 == nil {
		t.Fatalf("unexpected nil hash for the file")
	}

	clone2 := filepath.Join(dir, "clone2.txt")
	clone2Path := snapshot.Path(clone2)
	if err := Checkout(context.Background(), s, h1, clone2Path); err != nil {
		t.Fatalf("failure checking out the file snapshot %q to %q: %v", h1, clone2Path, err)
	}
	if err := os.WriteFile(clone2, []byte("A\nB\nC\nZ\nD\nE\n"), 0700); err != nil {
		t.Fatalf("failure updating the second clone of the example file to snapshot: %v", err)
	}
	h3, _, err := snapshot.Current(context.Background(), s, clone2Path)
	if err != nil {
		t.Fatalf("failure creating the second updated snapshot for the file: %v", err)
	} else if h3 == nil {
		t.Fatalf("unexpected nil hash for the file")
	}

	h4, err := mergeWithHelper(context.Background(), s, originalPath, "-rwx------", h1, h2, h3)
	if err != nil {
		t.Fatalf("failure merging non-conflicting changes with the helper: %v", err)
	}

	merged := filepath.Join(dir, "merged.txt")
	mergedPath := snapshot.Path(merged)
	if err := Checkout(context.Background(), s, h4, mergedPath); err != nil {
		t.Fatalf("failure checking out the merged snapshot %q: %v", h4, err)
	}
	mergedBytes, err := os.ReadFile(merged)
	if err != nil {
		t.Fatalf("failure reading the contents of the checked out merged snapshot: %v", err)
	}
	if got, want := string(mergedBytes), "A\nX\nB\nY\nC\nZ\nD\nE\n"; got != want {
		t.Errorf("unexpected results of merging non-conflicting changes with the helper: got %q, want %q", got, want)
	}
}

func TestMergeWithHelperNilBaseNoConflict(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{ArchiveDir: archive}

	original := filepath.Join(dir, "original.txt")
	originalPath := snapshot.Path(original)

	version1 := filepath.Join(dir, "version1.txt")
	version1Path := snapshot.Path(version1)
	if err := os.WriteFile(version1, []byte("A\nB\nC\nD\nE\n"), 0700); err != nil {
		t.Fatalf("failure writing the first version of the example file to snapshot: %v", err)
	}
	v1Hash, _, err := snapshot.Current(context.Background(), s, version1Path)
	if err != nil {
		t.Fatalf("failure creating the first updated snapshot for the file: %v", err)
	} else if v1Hash == nil {
		t.Fatalf("unexpected nil hash for the file")
	}

	version2 := filepath.Join(dir, "version2.txt")
	version2Path := snapshot.Path(version2)
	if err := os.WriteFile(version2, []byte("A\nB\nC\nD\nE\n"), 0700); err != nil {
		t.Fatalf("failure writing the second version of the example file to snapshot: %v", err)
	}
	v2Hash, _, err := snapshot.Current(context.Background(), s, version2Path)
	if err != nil {
		t.Fatalf("failure creating the second updated snapshot for the file: %v", err)
	} else if v2Hash == nil {
		t.Fatalf("unexpected nil hash for the file")
	}

	if mergedHash, err := mergeWithHelper(context.Background(), s, originalPath, "-rwx------", nil, v1Hash, v2Hash); err == nil {
		t.Errorf("unexpected result from merging unrelated files with the default merge helper: %v", mergedHash)
	} else if got, want := err.Error(), "merge helper \"diff3\" failed: exit status 1"; got != want {
		t.Errorf("unexexpected error message from merging unrelated files with the default merge helper: got %q, want %q", got, want)
	}
}
