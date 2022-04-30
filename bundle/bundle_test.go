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

// Package bundle defines methods for bundling snapshots together so they can be imported and/or exported.
package bundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func TestRoundtrip(t *testing.T) {
	archiveDir := filepath.Join(t.TempDir(), "archive")
	s := &storage.LocalFiles{archiveDir}

	workDir := filepath.Join(t.TempDir(), "workDir")
	if err := os.MkdirAll(workDir, os.FileMode(0700)); err != nil {
		t.Fatalf("failure creating the work dir: %v", err)
	}

	// Take an initial snapshot
	file := filepath.Join(workDir, "hello.txt")
	p := snapshot.Path(file)
	if err := os.WriteFile(file, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file to snapshot: %v", err)
	}
	h1, f1, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Fatalf("failure creating the initial snapshot for the file: %v", err)
	}
	if err := os.WriteFile(file, []byte("Goodbye, World!"), 0700); err != nil {
		t.Fatalf("failure updating the example file to snapshot: %v", err)
	}
	h2, f2, err := snapshot.Current(context.Background(), s, p)
	if err != nil {
		t.Fatalf("failure creating the updated snapshot for the file: %v", err)
	}

	bundleFile := filepath.Join(t.TempDir(), "bundle.zip")
	included, err := Export(context.Background(), s, bundleFile, []*snapshot.Hash{h2}, nil, nil, true)
	if err != nil {
		t.Fatalf("failure creating the bundle %q: %v", bundleFile, err)
	}

	includedMap := make(map[snapshot.Hash]struct{})
	for _, i := range included {
		includedMap[*i] = struct{}{}
	}
	if _, ok := includedMap[*h2]; !ok {
		t.Errorf("bundle export does not include the specified hash %q: got %v", h2, included)
	}
	if _, ok := includedMap[*h1]; !ok {
		t.Errorf("bundle export does not include the parent of the specified hash %q: got %v", h1, included)
	}

	archive2Dir := filepath.Join(t.TempDir(), "archive2")
	s2 := &storage.LocalFiles{archive2Dir}
	imported, err := Import(context.Background(), s2, bundleFile, nil)
	if err != nil {
		t.Fatalf("failure importing the bundle %q: %v", bundleFile, err)
	}
	importedMap := make(map[snapshot.Hash]struct{})
	for _, i := range imported {
		importedMap[*i] = struct{}{}
	}
	if _, ok := importedMap[*h2]; !ok {
		t.Errorf("bundle import does not include the specified hash %q: got %v", h2, imported)
	}
	if _, ok := importedMap[*h1]; !ok {
		t.Errorf("bundle import does not include the parent of the specified hash %q: got %v", h1, imported)
	}
	if f1v2, err := s2.ReadSnapshot(context.Background(), h1); err != nil {
		t.Errorf("error finding imported snapshot %q: %v", h1, err)
	} else if got, want := f1v2.String(), f1.String(); got != want {
		t.Errorf("unexpected contents for snapshot %q: got %q, want %q", h1, got, want)
	}
	if f2v2, err := s2.ReadSnapshot(context.Background(), h2); err != nil {
		t.Errorf("error finding imported snapshot %q: %v", h2, err)
	} else if got, want := f2v2.String(), f2.String(); got != want {
		t.Errorf("unexpected contents for snapshot %q: got %q, want %q", h1, got, want)
	}
}
