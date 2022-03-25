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

// Package snapshot implements the history model for rvcs.
package snapshot

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// File is the top-level object in a snapshot.
//
// File encodes the entire, transitive history of a file. If the file is
// a directory, then this history also includes the histories for all
// of the children of that directory.
type File struct {
	// Mode is the string representation of a Posix-style file mode.
	//
	// It should be of the form [<FILE_TYPE>]+<FILE_PERMISSIONS>.
	//
	// <FILE_TYPE> is a single character indicating the type of the
	// file, such as `d` for a directory or `L` for a symbolic link, etc.
	//
	// <FILE_PERMISSIONS> is a sequence of 9 characters representing the
	// Unix permission bits.
	Mode string

	// Contents is the hash of the contents for the snapshotted file.
	//
	// If the file is a directory (the mode line starts with `d`), then
	// this will be the hash of a `Tree` object.
	//
	// If the file is a symbolic link (the mode line starts with a `L`),
	// then this will be the hash of another `File` object, unless the
	// link is broken in which case the contents will be nil.
	//
	// In all other cases, the contents is a hash of the sequence of
	// bytes read from the file.
	Contents *Hash

	// Parents stores the hashes for the previous snapshots that
	// immediately preceeded this one.
	Parents []*Hash
}

// IsDir reports whether or not the file is the snapshot of a directory.
func (f *File) IsDir() bool {
	if f == nil {
		return false
	}
	return strings.HasPrefix(f.Mode, "d")
}

// IsLink reports whether or not the file is the snapshot of a symbolic link.
func (f *File) IsLink() bool {
	if f == nil {
		return false
	}
	return strings.HasPrefix(f.Mode, "L")
}

// String implements the `fmt.Stringer` interface.
//
// The resulting value is suitable for serialization.
func (f *File) String() string {
	if f == nil {
		return ""
	}
	var contentsStr string
	if f.Contents != nil {
		contentsStr = f.Contents.String()
	}
	lines := []string{f.Mode, contentsStr}
	for _, parent := range f.Parents {
		if parent != nil {
			lines = append(lines, parent.String())
		}
	}
	return strings.Join(lines, "\n")
}

// ParseFile parses a `File` object from its encoded form.
//
// The input string must match the form returned by the `File.String` method.
func ParseFile(encoded string) (*File, error) {
	if len(encoded) == 0 {
		return nil, nil
	}
	lines := strings.Split(string(encoded), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("malformed file metadata: %q", encoded)
	}
	var hashes []*Hash
	for i, line := range lines[1:] {
		hash, err := ParseHash(line)
		if err != nil {
			return nil, fmt.Errorf("failure parsing the hash %q: %v", line, err)
		}
		if hash != nil {
			hashes = append(hashes, hash)
		} else if i == 0 {
			return nil, fmt.Errorf("missing contents for the encoded file %q", encoded)
		}
	}
	f := &File{
		Mode:     lines[0],
		Contents: hashes[0],
		Parents:  hashes[1:],
	}
	return f, nil
}

// Permissions returns the permission subset of the file mode.
//
// The returned `os.FileMode` object does not include any information
// on the file type (e.g. directory vs. link, etc).
func (f *File) Permissions() os.FileMode {
	if f == nil || len(f.Mode) < 9 {
		// This is not a Posix-style mode line; default to 0700
		return os.FileMode(0700)
	}
	permStr := f.Mode[len(f.Mode)-9:]
	perm := fs.ModePerm
	for i, c := range permStr {
		if c == '-' {
			perm ^= (1 << uint(8-i))
		}
	}
	return perm
}
