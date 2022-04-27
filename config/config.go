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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Mirror defines the configuration for a single mirror
type Mirror struct {
	// URL is the location of the mirror
	URL *url.URL

	// HelperFlags are command line arguments to pass to the mirror helper tool
	HelperFlags []string
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (m *Mirror) UnmarshalJSON(text []byte) error {
	var rawMap struct {
		URL         string   `json:"url"`
		HelperFlags []string `json:"helperFlags,omitempty"`
	}
	if err := json.Unmarshal(text, &rawMap); err != nil {
		return fmt.Errorf("failure parsing the raw JSON for a mirror config: %v", err)
	}
	u, err := url.Parse(rawMap.URL)
	if err != nil {
		return fmt.Errorf("failure parsing the URL for a mirror config: %v", err)
	}
	m.URL = u
	m.HelperFlags = rawMap.HelperFlags
	return nil
}

// Identity holds the config for a single identity used to sign and/or verify snapshots.
type Identity struct {
	// Name is the name of the identity and must be able to be parsed by the `snapshot.ParseIdentity` method.
	Name string `json:"name"`

	// PullMirrors are a list of mirrors that we pull snapshots from for the given identity.
	PullMirrors []*Mirror `json:"pullMirrors,omitempty"`

	// PushMirrors are a list of mirrors that we push snapshots to for the given identity.
	PushMirrors []*Mirror `json:"pushMirrors,omitempty"`
}

// Settings defines configuration settings for the rvcs tool.
type Settings struct {
	// Identities is a list of configurations for each of the identities we keep track of.
	Identities []*Identity `json:"identities,omitempty"`

	// AdditionalPullMirrors is a list of mirrors from which we will try to pull information for any identities that do not have a matching entry in the `identities` field.
	AdditionalPullMirrors []*Mirror `json:"additionalPullMirrors,omitempty"`
}

// Read reads in the configuration saved in the user's config directory.
func Read() (*Settings, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failure identifying the user config dir: %v", err)
	}
	rvcsCfgDir := filepath.Join(cfgDir, "rvcs")
	if err := os.MkdirAll(rvcsCfgDir, 0700); err != nil {
		return nil, fmt.Errorf("failure ensuring the config dir exists: %v", err)
	}
	cfgFile := filepath.Join(rvcsCfgDir, "config.json")

	var s Settings
	bs, err := os.ReadFile(cfgFile)
	if os.IsNotExist(err) {
		// The config file does not exist, so return an empty config.
		return &s, nil
	}
	if err := json.Unmarshal(bs, &s); err != nil {
		return nil, fmt.Errorf("failure parsing the config file contents: %v", err)
	}
	return &s, nil
}
