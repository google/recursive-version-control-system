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

// Package config defines the configuration options for rvcs.
package config

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseSettings(t *testing.T) {
	testCases := []struct {
		Description string
		Serialized  string
		Want        *Settings
	}{
		{
			Description: "Empty settings",
			Serialized:  "{}",
			Want:        &Settings{},
		},
		{
			Description: "Empty top-level fields",
			Serialized:  "{\"Identities\": [], \"AdditionalMirrors\": []}",
			Want: &Settings{
				Identities:        []*Identity{},
				AdditionalMirrors: []*Mirror{},
			},
		},
		{
			Description: "Non-empty mirrors",
			Serialized:  "{\"AdditionalMirrors\": [{\"url\": \"gcs://example.com/some-path\", \"helperFlags\": [\"--foo\", \"--bar\"], \"readOnly\": true}]}",
			Want: &Settings{
				AdditionalMirrors: []*Mirror{
					&Mirror{
						URL: &url.URL{
							Scheme: "gcs",
							Host:   "example.com",
							Path:   "/some-path",
						},
						HelperFlags: []string{
							"--foo",
							"--bar",
						},
						ReadOnly: true,
					},
				},
			},
		},
	}
	for _, testCase := range testCases {
		var s Settings
		if err := json.Unmarshal([]byte(testCase.Serialized), &s); err != nil {
			t.Errorf("Error parsing the settings for %q: %v", testCase.Description, err)
		} else if got, want := &s, testCase.Want; !cmp.Equal(got, want) {
			t.Errorf("Wrong value unmarshalling config settings; got %+v, want %+v, diff %q", got, want, cmp.Diff(got, want))
		}
	}
}
