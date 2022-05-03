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
	"errors"
	"fmt"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

func Sign(ctx context.Context, s *storage.LocalFiles, id *snapshot.Identity, h *snapshot.Hash, prevSignature *snapshot.Hash) (*snapshot.Hash, error) {
	if id == nil {
		return nil, errors.New("identity must not be nil")
	}
	if h == nil {
		// Signing a nil hash is a no-op
		return nil, nil
	}
	args := []string{id.String(), h.String()}
	if prevSignature != nil {
		args = append(args, prevSignature.String())
	}
	h, err := runHelper(ctx, "sign", id.Algorithm(), args)
	if err != nil {
		return nil, fmt.Errorf("failure invoking the sign helper for %q: %v", id.Algorithm(), err)
	}
	if err := s.UpdateSignatureForIdentity(ctx, id, h); err != nil {
		return nil, fmt.Errorf("failure updating the latest snapshot for %q to %q: %v", id, h, err)
	}
	return h, nil
}
