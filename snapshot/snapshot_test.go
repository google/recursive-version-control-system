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

import "testing"

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
