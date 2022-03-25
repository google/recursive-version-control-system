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

package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
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
