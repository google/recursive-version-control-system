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

	// ReadOnly indicates that the mirror should only be pulled from, and
	// not pushed to.
	ReadOnly bool
}

// MarshalJSON implements the json.Marshaler interface.
func (m *Mirror) MarshalJSON() ([]byte, error) {
	var rawMap struct {
		URL         string   `json:"url"`
		HelperFlags []string `json:"helperFlags,omitempty"`
		ReadOnly    bool     `json:"readOnly,omitempty"`
	}
	rawMap.URL = m.URL.String()
	rawMap.HelperFlags = m.HelperFlags
	rawMap.ReadOnly = m.ReadOnly
	return json.Marshal(rawMap)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (m *Mirror) UnmarshalJSON(text []byte) error {
	var rawMap struct {
		URL         string   `json:"url"`
		HelperFlags []string `json:"helperFlags,omitempty"`
		ReadOnly    bool     `json:"readOnly,omitempty"`
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
	m.ReadOnly = rawMap.ReadOnly
	return nil
}

func removeMirror(ms []*Mirror, u *url.URL) []*Mirror {
	var remaining []*Mirror
	target := u.String()
	for _, m := range ms {
		if m.URL.String() == target {
			continue
		}
		remaining = append(remaining, m)
	}
	return remaining
}

func addOrOverwriteMirror(ms []*Mirror, m *Mirror) []*Mirror {
	return append(removeMirror(ms, m.URL), m)
}

// Identity holds the config for a single identity used to sign and/or verify snapshots.
type Identity struct {
	// Name is the name of the identity and must be able to be parsed by
	// the `snapshot.ParseIdentity` method.
	Name string `json:"name"`

	// Mirrors are a list of mirrors that we pull snapshots from, and
	// potentially push to (if they are not read-only) for the given
	// identity.
	Mirrors []*Mirror `json:"mirrors,omitempty"`
}

// Settings defines configuration settings for the rvcs tool.
type Settings struct {
	// Identities is a list of configurations for each of the identities we keep track of.
	Identities []*Identity `json:"identities,omitempty"`

	// AdditionalMirrors is a list of mirrors which we will try to use for
	// any identities that do not have a matching entry in the
	// `identities` field.
	AdditionalMirrors []*Mirror `json:"additionalMirrors,omitempty"`
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

// Write writes the given settings to the configuration saved in the user's config directory.
func (s *Settings) Write() error {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("failure identifying the user config dir: %v", err)
	}
	rvcsCfgDir := filepath.Join(cfgDir, "rvcs")
	if err := os.MkdirAll(rvcsCfgDir, 0700); err != nil {
		return fmt.Errorf("failure ensuring the config dir exists: %v", err)
	}
	cfgFile := filepath.Join(rvcsCfgDir, "config.json")

	jsonBytes, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgFile, jsonBytes, 0700)
}

// WithAdditionalMirror returns a new Settings instance with the given additional mirror.
//
// If a mirror with the same URL already exists in the settings
// `AdditionalMirrors` field, then it is replaced with the specified mirror.
//
// WithAdditionalMirror always returns a new Settings instance even if it is
// identical to the original instance.
func (s *Settings) WithAdditionalMirror(m *Mirror) *Settings {
	return &Settings{
		Identities:        s.Identities,
		AdditionalMirrors: addOrOverwriteMirror(s.AdditionalMirrors, m),
	}
}

// WithMirrorForIdentity returns a new Settings instance with the given mirror for the named identity.
//
// If the identity already has a mirror with the same URL, then that mirror
// is replaced with the specified one.
//
// WithMirrorForIdentity always returns a new Settings instance even if it is
// identical to the original instance.
func (s *Settings) WithMirrorForIdentity(idName string, m *Mirror) *Settings {
	res := &Settings{
		AdditionalMirrors: s.AdditionalMirrors,
	}
	for i, existingID := range s.Identities {
		if existingID.Name != idName {
			continue
		}
		updatedID := &Identity{
			Name:    idName,
			Mirrors: addOrOverwriteMirror(existingID.Mirrors, m),
		}
		res.Identities = append(append(s.Identities[:i], updatedID), s.Identities[i+1:]...)
		return res
	}
	res.Identities = append(s.Identities, &Identity{
		Name:    idName,
		Mirrors: []*Mirror{m},
	})
	return res
}

// WithoutAdditionalMirror returns a new Settings instance without the given mirror in the `AdditionalMirrors` field.
//
// WithoutAdditionalMirror always returns a new Settings instance even if it is
// identical to the original instance.
func (s *Settings) WithoutAdditionalMirror(u *url.URL) *Settings {
	return &Settings{
		Identities:        s.Identities,
		AdditionalMirrors: removeMirror(s.AdditionalMirrors, u),
	}
}

// WithoutMirrorForIdentity returns a new Settings instance without the given mirror for the named identity.
//
// WithoutMirrorForIdentity always returns a new Settings instance even if it
// is identical to the original instance.
func (s *Settings) WithoutMirrorForIdentity(idName string, u *url.URL) *Settings {
	res := &Settings{
		AdditionalMirrors: s.AdditionalMirrors,
	}
	for i, existingID := range s.Identities {
		if existingID.Name != idName {
			continue
		}
		updatedID := &Identity{
			Name:    idName,
			Mirrors: removeMirror(existingID.Mirrors, u),
		}
		res.Identities = append(append(s.Identities[:i], updatedID), s.Identities[i+1:]...)
		return res
	}
	res.Identities = s.Identities
	return res
}
