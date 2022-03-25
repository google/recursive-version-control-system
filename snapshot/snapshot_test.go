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

func TestParseFileRoundTrip(t *testing.T) {
	testCases := []struct {
		Description string
		Serialized  string
		Want        string
		WantError   bool
	}{
		{
			Description: "empty file string",
		},
		{
			Description: "missing contents",
			Serialized:  "drwxr-x---",
			WantError:   true,
		},
		{
			Description: "empty contents",
			Serialized:  "drwxr-x---\n",
			WantError:   true,
		},
		{
			Description: "empty directory",
			Serialized:  "drwxr-x---\nsha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Want:        "drwxr-x---\nsha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			Description: "empty lines in parents",
			Serialized:  "drwxr-x---\nsha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n\n",
			Want:        "drwxr-x---\nsha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}
	for _, testCase := range testCases {
		parsed, err := ParseFile(testCase.Serialized)
		if testCase.WantError {
			if err == nil {
				t.Errorf("unexpected response for test case %q: %+v", testCase.Description, parsed)
			}
		} else if err != nil {
			t.Errorf("unexpected failure parsing the serialized file %q for the test case %q: %v", testCase.Serialized, testCase.Description, err)
		} else if got, want := parsed.String(), testCase.Want; got != want {
			t.Errorf("unexpected result for file parsing roundtrip of %q; got %q, want %q", testCase.Description, got, want)
		}
	}
}

func TestFilePermissions(t *testing.T) {
	testCases := []struct {
		Description string
		File        *File
		Want        string
	}{
		{
			Description: "nil file",
			Want:        "-rwx------",
		},
		{
			Description: "empty mode",
			File: &File{
				Mode: "",
			},
			Want: "-rwx------",
		},
		{
			Description: "permissions only",
			File: &File{
				Mode: "rw-rw-rw-",
			},
			Want: "-rw-rw-rw-",
		},
		{
			Description: "regular file",
			File: &File{
				Mode: "-r-xr--r--",
			},
			Want: "-r-xr--r--",
		},
		{
			Description: "directory",
			File: &File{
				Mode: "dr-xr-xr--",
			},
			Want: "-r-xr-xr--",
		},
		{
			Description: "multiple file type descriptors",
			File: &File{
				Mode: "dLTrwxr-xr-x",
			},
			Want: "-rwxr-xr-x",
		},
	}
	for _, testCase := range testCases {
		if got, want := testCase.File.Permissions().String(), testCase.Want; got != want {
			t.Errorf("unexpected permissions for %q: got %q, want %q", testCase.Description, got, want)
		}
	}
}
