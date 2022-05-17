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
}
