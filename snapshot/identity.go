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
	"fmt"
	"strings"
)

// Identity represents an identity that can sign a hash.
type Identity struct {
	// algorithm is the name of the signing algorithm used (e.g. `ed25519`, etc).
	algorithm string

	// contents is the name of the identity. The semantics of this will vary by signature.
	contents string
}

// ParseIdentity parses the string encoding of an identity.
func ParseIdentity(str string) (*Identity, error) {
	if len(str) == 0 {
		return nil, nil
	}
	if !strings.Contains(str, "::") {
		return nil, fmt.Errorf("malformed identity string %q", str)
	}
	parts := strings.SplitN(str, "::", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("internal programming error in snapshot.ParseIdentity(%q)", str)
	}
	return &Identity{
		algorithm: parts[0],
		contents:  parts[1],
	}, nil
}

// Algorithm returns the name of the signing algorithm used (e.g. `ed25519`, etc).
func (h *Identity) Algorithm() string {
	return h.algorithm
}

// Contents returns the identity contents.
func (h *Identity) Contents() string {
	return h.contents
}

// Equal reports whether or not two hash objects are equal.
func (h *Identity) Equal(other *Identity) bool {
	if h == nil || other == nil {
		return h == nil && other == nil
	}
	return h.algorithm == other.algorithm && h.contents == other.contents
}

// String implements the `fmt.Stringer` interface.
//
// The resulting value is used when serializing objects holding a hash.
func (h *Identity) String() string {
	if h == nil {
		return ""
	}
	return h.algorithm + "::" + h.contents
}
