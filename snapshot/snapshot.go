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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	defaultHashFunction    = "sha256"
	supportedHashFunctions = map[string]func() hash.Hash{
		"sha256": sha256.New,
	}
)

// Hash represents a hash/fingerprint of a blob.
type Hash struct {
	// function is the name of the hash function used (e.g. `sha256`, etc).
	function string

	// hexContents is the hash value serialized as a hexadecimal string.
	hexContents string
}

// NewHash constructs a new hash by calculating the checksum of the provided reader.
//
// The caller is responsible for closing the reader.
func NewHash(reader io.Reader) (*Hash, error) {
	sum := supportedHashFunctions[defaultHashFunction]()
	if _, err := io.Copy(sum, reader); err != nil {
		return nil, fmt.Errorf("failure hashing an object: %v", err)
	}
	return &Hash{
		function:    defaultHashFunction,
		hexContents: fmt.Sprintf("%x", sum.Sum(nil)),
	}, nil
}

// ParseHash parses the string encoding of a hash.
func ParseHash(str string) (*Hash, error) {
	if len(str) == 0 {
		return nil, nil
	}
	if !strings.Contains(str, ":") {
		return nil, fmt.Errorf("malformed hash string %q", str)
	}
	parts := strings.SplitN(str, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("internal programming error in snapshot.ParseHash(%q)", str)
	}
	if _, ok := supportedHashFunctions[parts[0]]; !ok {
		return nil, fmt.Errorf("unsupported hash function %q", parts[0])
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return nil, fmt.Errorf("malformed hash contents %q: %v", parts[1], err)
	}
	return &Hash{
		function:    parts[0],
		hexContents: parts[1],
	}, nil
}

// Function returns the name of the hash function used (e.g. `sha256`, etc).
func (h *Hash) Function() string {
	return h.function
}

// HexContents returns the hash value serialized as a hexadecimal string.
func (h *Hash) HexContents() string {
	return h.hexContents
}

// Equal reports whether or not two hash objects are equal.
func (h *Hash) Equal(other *Hash) bool {
	if h == nil || other == nil {
		return h == nil && other == nil
	}
	return h.function == other.function && h.hexContents == other.hexContents
}

// String implements the `fmt.Stringer` interface.
//
// The resulting value is used when serializing objects holding a hash.
func (h *Hash) String() string {
	if h == nil {
		return ""
	}
	return h.function + ":" + h.hexContents
}

// Path represents the filesystem path of a file.
//
// This can be either an absolute or relative path.
type Path string

// Join returns the path corresponding to joining this path with the supplied child path.
func (p Path) Join(child Path) Path {
	return Path(filepath.Join(string(p), string(child)))
}

func (p Path) encode() string {
	return base64.RawStdEncoding.EncodeToString([]byte(p))
}

func decodePath(encoded string) (Path, error) {
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return Path(""), fmt.Errorf("failure decoding the encoded path string %q: %v", encoded, err)
	}
	return Path(decoded), nil
}

// Tree represents the contents of a directory.
//
// The keys are relative paths of the directory children, and the values
// are the hashes of each child's latest snapshot.
type Tree map[Path]*Hash

// String implements the `fmt.Stringer` interface.
//
// The resulting value is suitable for serialization.
func (t Tree) String() string {
	var lines []string
	for p, h := range t {
		if h != nil {
			line := p.encode() + " " + h.String()
			lines = append(lines, line)
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// ParseTree parses a `Tree` object from its encoded form.
//
// The input string must match the form returned by the `Tree.String` method.
func ParseTree(encoded string) (Tree, error) {
	t := make(Tree)
	lines := strings.Split(encoded, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed entry %q in encoded tree %q", line, encoded)
		}
		p, err := decodePath(parts[0])
		if err != nil {
			return nil, fmt.Errorf("failure parsing encoded path %q: %v", parts[0], err)
		}
		h, err := ParseHash(parts[1])
		if err != nil {
			return nil, fmt.Errorf("failure parsing encoded hash %q: %v", parts[1], err)
		}
		t[p] = h
	}
	return t, nil
}

// File is the top-level object in a snapshot.
//
// File encodes the entire, transitive history of a file. If the file is
// a directory, then this history also includes the histories for all
// of the children of that directory.
type File struct {
	// Mode is the string representation of a Posix-style file mode.
	//
	// It should be of the form <FILE_TYPE><FILE_PERMISSIONS>.
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
	lines := strings.Split(string(encoded), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("malformed file metadata: %q", encoded)
	}
	var hashes []*Hash
	for _, line := range lines[1:] {
		hash, err := ParseHash(line)
		if err != nil {
			return nil, fmt.Errorf("failure parsing the hash %q: %v", line, err)
		}
		hashes = append(hashes, hash)
	}
	f := &File{
		Mode:     lines[0],
		Contents: hashes[0],
		Parents:  hashes[1:],
	}
	return f, nil
}

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
