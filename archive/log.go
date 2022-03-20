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

package archive

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/googlestaging/recursive-version-control-system/snapshot"
	"golang.org/x/term"
)

type LogEntry struct {
	// Hash is the hash of the file snapshot
	Hash *snapshot.Hash

	// File is the file snapshot
	File *snapshot.File

	// summary is a list of strings that describe what changed
	// between the file snapshot and its first parent.
	//
	// This is empty until the first time the `SummarizeLog` method
	// has been successfully called with this LogEntry.
	summary []string

	// nestedPaths is an ordered slice of all subpaths of the file.
	//
	// This is only ever populated for snapshots of directories,
	// and only if the `SummarizeLog` method has been called.
	nestedPaths []string

	// nestedContents is a map from all subpaths of the file to
	// the corresponding nested file snapshots.
	//
	// This is only ever populated for snapshots of directories,
	// and only if the `SummarizeLog` method has been called.
	nestedContents map[string]*snapshot.Hash
}

func dirContents(ctx context.Context, s *Store, h *snapshot.Hash, f *snapshot.File, subpath string, includeDirectories bool, contentsMap map[string]*snapshot.Hash) error {
	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure listing the directory contents of the snapshot %q: %v", h, err)
	}
	for p, ph := range tree {
		child, err := s.ReadSnapshot(ctx, ph)
		if err != nil {
			return fmt.Errorf("failure reading the file snapshot for %q: %v", p, err)
		}
		childPath := filepath.Join(subpath, string(p))
		if child.IsDir() {
			if includeDirectories {
				contentsMap[childPath] = ph
			}
			if err := dirContents(ctx, s, ph, child, childPath, includeDirectories, contentsMap); err != nil {
				return fmt.Errorf("failure enumerating the contents of %q: %v", p, err)
			}
		} else {
			contentsMap[childPath] = ph
		}
	}
	return nil
}

// NestedContents returns a map from subpaths of the log entry's file to
// the corresponding (hashes of the) file snapshots for the nested files.
//
// This is only defined for snapshots of directories, and for all other
// cases the return value will be nil.
func (e *LogEntry) NestedContents(ctx context.Context, s *Store, includeDirectories bool) ([]string, map[string]*snapshot.Hash, error) {
	if e.nestedPaths != nil && e.nestedContents != nil {
		return e.nestedPaths, e.nestedContents, nil
	}
	if !e.File.IsDir() {
		return nil, nil, nil
	}
	paths := []string{}
	contentsMap := make(map[string]*snapshot.Hash)
	if err := dirContents(ctx, s, e.Hash, e.File, "", includeDirectories, contentsMap); err != nil {
		return nil, nil, fmt.Errorf("failure reading the nested contents for %q: %v", e.Hash, err)
	}
	for path, _ := range contentsMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	e.nestedPaths = paths
	e.nestedContents = contentsMap
	return e.nestedPaths, e.nestedContents, nil
}

func deleteLine(deletedPath string, deletedHash *snapshot.Hash) string {
	coreText := fmt.Sprintf("  -%s(%s)", deletedPath, deletedHash)
	if !term.IsTerminal(syscall.Stdout) {
		return coreText
	}
	// Add ascii color escape codes if running in a terminal
	return fmt.Sprintf("\033[31m%s\033[0m", coreText)
}

func insertLine(insertedPath string, insertedHash *snapshot.Hash) string {
	coreText := fmt.Sprintf("  +%s(%s)", insertedPath, insertedHash)
	if !term.IsTerminal(syscall.Stdout) {
		return coreText
	}
	// Add ascii color escape codes if running in a terminal
	return fmt.Sprintf("\033[32m%s\033[0m", coreText)
}

func describeChanged(paths, previousPaths []string, contents, previousContents map[string]*snapshot.Hash) []string {
	changes := []string{}
	for _, p := range paths {
		h := contents[p]
		for len(previousPaths) > 0 && previousPaths[0] < p {
			deletedPath := previousPaths[0]
			previousPaths = previousPaths[1:]
			changes = append(changes, deleteLine(deletedPath, previousContents[deletedPath]))
		}
		var previousHash *snapshot.Hash
		if len(previousPaths) > 0 && previousPaths[0] == p {
			previousHash = previousContents[p]
			previousPaths = previousPaths[1:]
		}
		if previousHash.Equal(h) {
			continue
		}
		if previousHash != nil {
			changes = append(changes, deleteLine(p, previousHash))
		}
		changes = append(changes, insertLine(p, h))
	}
	for _, deletedPath := range previousPaths {
		previousHash := previousContents[deletedPath]
		changes = append(changes, deleteLine(deletedPath, previousHash))
	}
	return changes
}

func SummarizeLog(ctx context.Context, s *Store, entries []*LogEntry) (map[snapshot.Hash][]string, error) {
	pathsMap := make(map[snapshot.Hash][]string)
	contentsMap := make(map[snapshot.Hash]map[string]*snapshot.Hash)
	for _, e := range entries {
		paths, contents, err := e.NestedContents(ctx, s, false)
		if err != nil {
			return nil, fmt.Errorf("failure reading the nested contents of snapshot %q: %v", e.Hash, err)
		}
		if paths != nil && contents != nil {
			pathsMap[*e.Hash] = paths
			contentsMap[*e.Hash] = contents
		}
	}
	result := make(map[snapshot.Hash][]string)
	for _, e := range entries {
		var prevPaths []string
		var prevContents map[string]*snapshot.Hash
		if len(e.File.Parents) > 0 {
			firstParent := e.File.Parents[0]
			prevPaths = pathsMap[*firstParent]
			prevContents = contentsMap[*firstParent]
		}
		summary := []string{e.Hash.String()}
		contents, contentsOk := contentsMap[*e.Hash]
		paths, pathsOk := pathsMap[*e.Hash]
		if contentsOk && pathsOk {
			summary = append(summary, describeChanged(paths, prevPaths, contents, prevContents)...)
		}
		result[*e.Hash] = summary
	}
	return result, nil
}

func ReadLog(ctx context.Context, s *Store, h *snapshot.Hash) ([]*LogEntry, error) {
	visited := make(map[snapshot.Hash]*snapshot.File)
	queue := []*snapshot.Hash{h}
	result := []*LogEntry{}
	for len(queue) > 0 {
		h, queue = queue[0], queue[1:]
		f, err := s.ReadSnapshot(ctx, h)
		if err != nil {
			return nil, fmt.Errorf("failure reading the snapshot for %q: %v", h, err)
		}
		visited[*h] = f
		result = append(result, &LogEntry{
			Hash: h,
			File: f,
		})
		for _, p := range f.Parents {
			if _, ok := visited[*p]; !ok {
				queue = append(queue, p)
			}
		}
	}
	return result, nil
}
