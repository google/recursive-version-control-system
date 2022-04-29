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

// Package publish defines methods for publishing rvcs snapshots.
package publish

import (
	"context"
	"fmt"

	"github.com/google/recursive-version-control-system/config"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func pushTo(ctx context.Context, m *config.Mirror, s *storage.LocalFiles, id *snapshot.Identity, h *snapshot.Hash) (*snapshot.Hash, error) {
	if m == nil || m.URL == nil {
		return h, nil
	}
	args := m.HelperFlags
	args = append(args, id.String(), h.String())
	h, err := runHelper(ctx, "push", m.URL.Scheme, args)
	if err != nil {
		return nil, fmt.Errorf("failure invoking the push helper for %q: %v", m.URL.Scheme, err)
	}
	return h, nil
}

func Push(ctx context.Context, settings *config.Settings, s *storage.LocalFiles, id *snapshot.Identity, signature *snapshot.Hash) (*snapshot.Hash, error) {
	pushed := signature
	for _, idSetting := range settings.Identities {
		if idSetting.Name == id.String() {
			for _, mirror := range idSetting.PushMirrors {
				pushed, err := pushTo(ctx, mirror, s, id, pushed)
				if !pushed.Equal(signature) {
					if _, err := Verify(ctx, s, id, signature); err != nil {
						return nil, fmt.Errorf("failure verifying the upstream signature for %q at %q: %v", id, mirror, err)
					}
				}
				if err != nil {
					return nil, fmt.Errorf("failure pushing the latest snapshot for %q to %q: %v", id, mirror, err)
				}
			}
		}
	}
	if !pushed.Equal(signature) {
		if err := s.UpdateSignatureForIdentity(ctx, id, pushed); err != nil {
			return nil, fmt.Errorf("failure updating the latest snapshot for %q to %q: %v", id, pushed, err)
		}
	}
	return pushed, nil
}
