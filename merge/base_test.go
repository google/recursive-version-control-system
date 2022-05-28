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

func setupSnapshots(t *testing.T) (s *storage.LocalFiles, parent *snapshot.Hash, child1 *snapshot.Hash, child2 *snapshot.Hash) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "archive")
	s = &storage.LocalFiles{ArchiveDir: archive}

	filename := filepath.Join(dir, "example.txt")
	p := snapshot.Path(filename)

	// Take an initial snapshot
	if err := os.WriteFile(filename, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file to snapshot: %v", err)
	}
	h1, f1, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Fatalf("failure creating the initial snapshot for the file: %v", err)
	} else if h1 == nil {
		t.Fatalf("unexpected nil hash for the file")
	} else if f1 == nil {
		t.Fatalf("unexpected nil snapshot for the file")
	}

	if err := os.WriteFile(filename, []byte("Goodbye, World!"), 0700); err != nil {
		t.Fatalf("failure updating the example file to snapshot: %v", err)
	}
	h2, f2, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Fatalf("failure creating the updated snapshot for the file: %v", err)
	} else if h2 == nil {
		t.Fatalf("unexpected nil hash for the file")
	} else if f2 == nil {
		t.Fatalf("unexpected nil snapshot for the file")
	}

	if err := os.RemoveAll(filename); err != nil {
		t.Fatalf("failure removing the example file: %v", err)
	}
	if err := Checkout(context.Background(), s, h1, snapshot.Path(filename)); err != nil {
		t.Fatalf("failure checking out the initial snapshot: %v", err)
	}
	if err := os.WriteFile(filename, []byte("Hello again, World!"), 0700); err != nil {
		t.Fatalf("failure updating the example file to a second snapshot: %v", err)
	}
	h3, f3, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Fatalf("failure creating the second updated snapshot for the file: %v", err)
	} else if h3 == nil {
		t.Fatalf("unexpected nil hash for the file")
	} else if f3 == nil {
		t.Fatalf("unexpected nil snapshot for the file")
	}
	return s, h1, h2, h3
}

func TestNilBase(t *testing.T) {
	s, h1, h2, _ := setupSnapshots(t)
	if base, err := Base(context.Background(), s, nil, nil); err != nil {
		t.Errorf("failure computing the mergebase of nil and itself: %v", err)
	} else if base != nil {
		t.Errorf("unexpected mergebase for nil and itself: %q", base)
	}
	if base, err := Base(context.Background(), s, nil, h1); err != nil {
		t.Errorf("failure computing the mergebase of nil and an initial snapshot: %v", err)
	} else if base != nil {
		t.Errorf("unexpected mergebase for nil and an initial snapshot: %q", base)
	}
	if base, err := Base(context.Background(), s, h1, nil); err != nil {
		t.Errorf("failure computing the mergebase of an initial snapshot and nil: %v", err)
	} else if base != nil {
		t.Errorf("unexpected mergebase for an initial snapshot and nil: %q", base)
	}
	if base, err := Base(context.Background(), s, nil, h2); err != nil {
		t.Errorf("failure computing the mergebase of nil and an updated snapshot: %v", err)
	} else if base != nil {
		t.Errorf("unexpected mergebase for nil and an updated snapshot: %q", base)
	}
	if base, err := Base(context.Background(), s, h2, nil); err != nil {
		t.Errorf("failure computing the mergebase of an updated snapshot and nil: %v", err)
	} else if base != nil {
		t.Errorf("unexpected mergebase for an updated snapshot and nil: %q", base)
	}
}

func TestTrivialBase(t *testing.T) {
	s, h1, h2, _ := setupSnapshots(t)
	if base, err := Base(context.Background(), s, h1, h1); err != nil {
		t.Errorf("failure computing the mergebase of an initial snapshot and itself: %v", err)
	} else if !base.Equal(h1) {
		t.Errorf("unexpected mergebase for an initial snapshot and itself: %q", base)
	}
	if base, err := Base(context.Background(), s, h2, h2); err != nil {
		t.Errorf("failure computing the mergebase of an updated snapshot and itself: %v", err)
	} else if !base.Equal(h2) {
		t.Errorf("unexpected mergebase for an updated snapshot and itself: %q", base)
	}
}

func TestDirectBase(t *testing.T) {
	s, h1, h2, _ := setupSnapshots(t)
	if base, err := Base(context.Background(), s, h1, h2); err != nil {
		t.Errorf("failure computing the mergebase of an initial snapshot and its child: %v", err)
	} else if !base.Equal(h1) {
		t.Errorf("unexpected mergebase for an initial snapshot and its child: %q", base)
	}
	if base, err := Base(context.Background(), s, h2, h1); err != nil {
		t.Errorf("failure computing the mergebase of a child and its parent: %v", err)
	} else if !base.Equal(h1) {
		t.Errorf("unexpected mergebase for a child and its parent: %q", base)
	}
}

func TestMutualParentBase(t *testing.T) {
	s, h1, h2, h3 := setupSnapshots(t)
	if base, err := Base(context.Background(), s, h2, h3); err != nil {
		t.Errorf("failure computing the mergebase of two sibling snapshots: %v", err)
	} else if !base.Equal(h1) {
		t.Errorf("unexpected mergebase for two sibling snapshots: %v", base)
	}
}
