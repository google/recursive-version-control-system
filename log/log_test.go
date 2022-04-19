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

package log

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func TestLog(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("failure resolving the absolute path of %q: %v", dir, err)
	}
	dir = abs
	archiveDir := filepath.Join(dir, "archive")
	s := &storage.LocalFiles{
		ArchiveDir: archiveDir,
	}
	workingDir := filepath.Join(dir, "working-dir")
	if err := os.MkdirAll(workingDir, os.FileMode(0700)); err != nil {
		t.Fatalf("failure creating the temporary working dir: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Take an initial snapshot
	h1, _, err := snapshot.Current(ctx, s, snapshot.Path(workingDir))
	if err != nil {
		t.Fatalf("failure creating the initial (empty) snapshot: %v", err)
	}

	// Write a file and take another snapshot
	file := filepath.Join(workingDir, "example.txt")
	if err := os.WriteFile(file, []byte("Hello, World!"), 0700); err != nil {
		t.Fatalf("failure creating the example file to snapshot: %v", err)
	}
	h2, _, err := snapshot.Current(ctx, s, snapshot.Path(workingDir))
	if err != nil {
		t.Fatalf("failure creating the updated snapshot: %v", err)
	}

	if entries, err := ReadLog(ctx, s, h2, 0); err != nil {
		t.Errorf("failure reading the log with a depth of 0: %v", err)
	} else if len(entries) != 0 {
		t.Errorf("unexpected log entries with a depth of 0: %+v", entries)
	}

	if entries, err := ReadLog(ctx, s, h2, 1); err != nil {
		t.Errorf("failure reading the log with a depth of 1: %v", err)
	} else if len(entries) != 1 {
		t.Errorf("unexpected log entries with a depth of 1: %+v", entries)
	} else if !entries[0].Hash.Equal(h2) {
		t.Errorf("unexpected log entry hash with a depth of 1: %+v", entries[0].Hash)
	}

	if entries, err := ReadLog(ctx, s, h2, -1); err != nil {
		t.Errorf("failure reading the log with a depth of -1: %v", err)
	} else if len(entries) != 2 {
		t.Errorf("unexpected log entries with a depth of -1: %+v", entries)
	} else if !entries[0].Hash.Equal(h2) {
		t.Errorf("unexpected first log entry hash with a depth of -1: %+v", entries[0].Hash)
	} else if !entries[1].Hash.Equal(h1) {
		t.Errorf("unexpected second log entry hash with a depth of -1: %+v", entries[0].Hash)
	}
}
