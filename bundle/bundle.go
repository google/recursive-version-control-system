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
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func bundleEntryPath(h *snapshot.Hash) string {
	if len(h.HexContents()) > 4 {
		return path.Join("objects", h.Function(), h.HexContents()[0:2], h.HexContents()[2:4], h.HexContents()[4:])
	}
	if len(h.HexContents()) > 2 {
		return path.Join("objects", h.Function(), h.HexContents()[0:2], h.HexContents()[2:])
	}
	return path.Join("objects", h.Function(), h.HexContents())
}

type ZipWriter struct {
	nested         *zip.Writer
	visited        map[snapshot.Hash]struct{}
	exclude        map[snapshot.Hash]struct{}
	recurseParents bool
}

func NewZipWriter(w io.Writer, exclude []*snapshot.Hash, metadata map[string]io.Reader, recurseParents bool) (*ZipWriter, error) {
	excludeMap := make(map[snapshot.Hash]struct{})
	for _, h := range exclude {
		excludeMap[*h] = struct{}{}
	}
	nested := zip.NewWriter(w)
	for name, r := range metadata {
		fw, err := nested.Create(path.Join("metadata", name))
		if err != nil {
			return nil, fmt.Errorf("failure creating a zip file entry for metadata key %q: %v", name, err)
		}
		if _, err := io.Copy(fw, r); err != nil {
			return nil, fmt.Errorf("failure writing the zip file entry for metadata key %q: %v", name, err)
		}
	}
	return &ZipWriter{
		nested:         nested,
		visited:        make(map[snapshot.Hash]struct{}),
		exclude:        excludeMap,
		recurseParents: recurseParents,
	}, nil
}

func (w *ZipWriter) Close() error {
	return w.nested.Close()
}

func (w *ZipWriter) AddObject(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash) error {
	if _, ok := w.exclude[*h]; ok {
		// We are explicitly excluding this object.
		return nil
	}
	if _, ok := w.visited[*h]; ok {
		// We already added this to the zip writer.
		return nil
	}
	w.visited[*h] = struct{}{}
	r, err := s.ReadObject(ctx, h)
	if err != nil {
		return fmt.Errorf("failure opening the contents of the object %q: %v", h, err)
	}
	defer r.Close()
	fw, err := w.nested.Create(bundleEntryPath(h))
	if err != nil {
		return fmt.Errorf("failure creating the zip file entry for %q: %v", h, err)
	}
	if _, err := io.Copy(fw, r); err != nil {
		return fmt.Errorf("failure writing the zip file entry for %q: %v", h, err)
	}
	return nil
}

func (w *ZipWriter) AddFile(ctx context.Context, s *storage.LocalFiles, h *snapshot.Hash, f *snapshot.File) error {
	if err := w.AddObject(ctx, s, h); err != nil {
		return fmt.Errorf("failure adding the snapshot %q to the bundle: %v", h, err)
	}
	if f.Contents == nil {
		return nil
	}
	if err := w.AddObject(ctx, s, f.Contents); err != nil {
		return fmt.Errorf("failure adding the contents of the snapshot %q to the bundle: %v", h, err)
	}
	if !f.IsDir() {
		return nil
	}
	tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
	if err != nil {
		return fmt.Errorf("failure reading the contents of the directory snapshot %q: %v", h, err)
	}
	for _, childHash := range tree {
		if _, ok := w.exclude[*childHash]; ok {
			continue
		}
		if _, ok := w.visited[*childHash]; ok {
			continue
		}
		child, err := s.ReadSnapshot(ctx, childHash)
		if err != nil {
			return fmt.Errorf("failure reading the snapshot %q: %v", childHash, err)
		}
		if err := w.AddFile(ctx, s, childHash, child); err != nil {
			return fmt.Errorf("failure adding the child %q to the bundle: %v", childHash, err)
		}
	}
	if !w.recurseParents {
		return nil
	}
	for _, parentHash := range f.Parents {
		parent, err := s.ReadSnapshot(ctx, parentHash)
		if err != nil {
			// The history is incomplete
			continue
		}
		if err := w.AddFile(ctx, s, parentHash, parent); err != nil {
			return fmt.Errorf("failure adding the parent %q to the bundle: %v", parentHash, err)
		}
	}
	return nil
}

// Export writes a bundle with the specified snapshots to the given writer.
//
// If the returned error is nil, then the written bundle will include the
// specified snapshots, and their contents. For any snapshots of a directory,
// the bundle will also recursively include the snapshots for the children
// of that directory.
//
// The `exclude` argument specifies a list of objects (by hash) that will
// not be included in the resulting bundle even if they otherwise would
// have been.
//
// The `metadata` argument specifies an additional map of key/value pairs
// to include in the bundle in a separate subpath from the bundled objects.
func Export(ctx context.Context, s *storage.LocalFiles, w io.Writer, snapshots []*snapshot.Hash, exclude []*snapshot.Hash, metadata map[string]io.Reader, recurseParents bool) (err error) {
	zw, err := NewZipWriter(w, exclude, metadata, recurseParents)
	if err != nil {
		return fmt.Errorf("failure creating the zip writer for the bundle: %v", err)
	}
	defer func() {
		ce := zw.Close()
		if err == nil {
			err = ce
		}
	}()

	for _, h := range snapshots {
		f, err := s.ReadSnapshot(ctx, h)
		if err != nil {
			return fmt.Errorf("failure reading the snapshot %q: %v", h, err)
		}
		if err := zw.AddFile(ctx, s, h, f); err != nil {
			return fmt.Errorf("failure adding %q to the zip file: %v", h, err)
		}
	}
	return nil
}
