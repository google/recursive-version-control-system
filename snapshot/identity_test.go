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

import "testing"

func TestParseIdentityRoundTrip(t *testing.T) {
	testCases := []struct {
		Description string
		Serialized  string
		WantError   bool
	}{
		{
			Description: "empty identity string",
		},
		{
			Description: "missing colon",
			Serialized:  "a",
			WantError:   true,
		},
		{
			Description: "not enough colons",
			Serialized:  "a:b",
			WantError:   true,
		},
		{
			Description: "valid format",
			Serialized:  "ed25519::0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
		},
	}
	for _, testCase := range testCases {
		parsed, err := ParseIdentity(testCase.Serialized)
		if testCase.WantError {
			if err == nil {
				t.Errorf("unexpected response for test case %q: %+v", testCase.Description, parsed)
			}
		} else if err != nil {
			t.Errorf("unexpected failure parsing the serialized identity %q for the test case %q: %v", testCase.Serialized, testCase.Description, err)
		} else if got, want := parsed.String(), testCase.Serialized; got != want {
			t.Errorf("unexpected result for identity parsing roundtrip of %q; got %q, want %q", testCase.Description, got, want)
		}
	}
}
