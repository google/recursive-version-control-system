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

func TestParseTreeRoundTrip(t *testing.T) {
	testCases := []struct {
		Description string
		Serialized  string
		Want        string
		WantError   bool
	}{
		{
			Description: "empty tree",
		},
		{
			Description: "missing hash",
			Serialized:  "abcde",
			WantError:   true,
		},
		{
			Description: "malformed hash",
			Serialized:  "abcde sha256:oops",
			WantError:   true,
		},
		{
			Description: "invalid encoded path",
			Serialized:  ":foo:bar:baz sha256:oops",
			WantError:   true,
		},
		{
			Description: "too many hashes",
			Serialized:  "abcd sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245 sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245",
			WantError:   true,
		},
		{
			Description: "valid encoded tree",
			Serialized:  "abcd sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245\nefgh sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245",
			Want:        "abcd sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245\nefgh sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245",
		},
		{
			Description: "valid encoded tree with empty lines",
			Serialized:  "abcd sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245\n\nefgh sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245\n",
			Want:        "abcd sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245\nefgh sha256:d897f1f67a26ce92b59937134d467131537360a63b39316e5c847114a142c245",
		},
	}
	for _, testCase := range testCases {
		parsed, err := ParseTree(testCase.Serialized)
		if testCase.WantError {
			if err == nil {
				t.Errorf("unexpected response for test case %q: %+v", testCase.Description, parsed)
			}
		} else if err != nil {
			t.Errorf("unexpected failure parsing the serialized tree %q for the test case %q: %v", testCase.Serialized, testCase.Description, err)
		} else if got, want := parsed.String(), testCase.Want; got != want {
			t.Errorf("unexpected result for tree parsing roundtrip of %q; got %q, want %q", testCase.Description, got, want)
		}
	}
}
