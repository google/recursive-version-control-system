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

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func addFileToZipWriter(ctx context.Context, s *storage.LocalFiles, w *zip.Writer, h *snapshot.Hash, f *snapshot.File) ([]*snapshot.Hash, error) {
	fw, err := w.Create(fmt.Sprintf("%s/%s", h.Function(), h.HexContents()))
	if err != nil {
		return nil, fmt.Errorf("failure creating the zip file entry for %q: %v", h, err)
	}
	if _, err := fw.Write([]byte(f.String())); err != nil {
		return nil, fmt.Errorf("failure writing the zip file entry for %q: %v", h, err)
	}
	if f.Contents == nil {
		return nil, nil
	}
	if f.IsDir() {
		tree, err := s.ListDirectorySnapshotContents(ctx, h, f)
		if err != nil {
			return nil, fmt.Errorf("failure reading the contents of the directory snapshot %q: %v", h, err)
		}
		var next []*snapshot.Hash
		for _, childHash := range tree {
			next = append(next, childHash)
		}
		return next, nil
	}
	contentsReader, err := s.ReadObject(ctx, f.Contents)
	if err != nil {
		return nil, fmt.Errorf("failure opening the contents of the link snapshot %q: %v", h, err)
	}
	cw, err := w.Create(fmt.Sprintf("%s/%s", f.Contents.Function(), f.Contents.HexContents()))
	if err != nil {
		return nil, fmt.Errorf("failure creating the zip file entry for the contents %q: %v", f.Contents, err)
	}
	if _, err := io.Copy(cw, contentsReader); err != nil {
		return nil, fmt.Errorf("failure writing the zip file entry for the contents %q: %v", f.Contents, err)
	}
	return nil, nil
}

// Export writes a bundle with the specified snapshots to the given writer.
//
// If the returned error is nil, then the written bundle will include the
// specified snapshots, and their contents. For any snapshots of a directory,
// the bundle will also recursively include the snapshots for the children
// of that directory.
func Export(ctx context.Context, s *storage.LocalFiles, w io.Writer, snapshots []*snapshot.Hash) (err error) {
	zw := zip.NewWriter(w)
	defer func() {
		ce := zw.Close()
		if err == nil {
			err = ce
		}
	}()

	visited := make(map[snapshot.Hash]struct{})
	for len(snapshots) > 0 {
		var next []*snapshot.Hash
		for _, h := range snapshots {
			visited[*h] = struct{}{}
			f, err := s.ReadSnapshot(ctx, h)
			if err != nil {
				return fmt.Errorf("failure reading the snapshot %q: %v", h, err)
			}
			children, err := addFileToZipWriter(ctx, s, zw, h, f)
			if err != nil {
				return fmt.Errorf("failure adding %q to the zip file: %v", h, err)
			}
			for _, childHash := range children {
				if _, ok := visited[*childHash]; !ok {
					next = append(next, childHash)
				}
			}
		}
		snapshots = next
	}
	return nil
}
