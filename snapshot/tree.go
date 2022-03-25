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
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

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
