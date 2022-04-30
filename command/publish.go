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

// Package command defines the command line interface for rvcs
package command

import (
	"context"
	"flag"
	"fmt"

	"github.com/google/recursive-version-control-system/config"
	"github.com/google/recursive-version-control-system/publish"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

const publishUsage = `Usage: %s merge <SOURCE> <DESTINATION>

Where <DESTINATION> is an identity, and <SOURCE> is one of:

	The hash of a known snapshot.
	A local file path which has previously been snapshotted.
	A different identity for which a snapshot has already been published.
`

func publishCommand(ctx context.Context, s *storage.LocalFiles, cmd string, args []string) (int, error) {
	settings, err := config.Read()
	if err != nil {
		return 1, fmt.Errorf("failure reading the config settings: %v", err)
	}
	if len(args) != 2 {
		fmt.Fprintf(flag.CommandLine.Output(), publishUsage, cmd)
		return 1, nil
	}
	h, err := resolveSnapshot(ctx, s, args[0])
	if err != nil {
		return 1, fmt.Errorf("failure resolving the snapshot hash for %q: %v", args[0], err)
	}
	id, err := snapshot.ParseIdentity(args[1])
	if err != nil {
		return 1, fmt.Errorf("failure parsing the identity %q: %v", args[1], err)
	}
	signature, signed, err := resolveIdentitySnapshot(ctx, s, id)
	if err != nil {
		return 1, fmt.Errorf("failure resolving the previous signature for %q: %v", id, err)
	}
	if !signed.Equal(h) {
		// The hash has not already been signed for this identity, so
		// we must do that now.
		signature, err = publish.Sign(ctx, s, id, h, signature)
		if err != nil {
			return 1, fmt.Errorf("failure signing %q with %q: %v", h, id, err)
		}
	}
	signature, err = publish.Push(ctx, settings, s, id, signature)
	if err != nil {
		return 1, fmt.Errorf("failure pushing the latest signature for %q: %v", id, err)
	}
	fmt.Printf("%s  %s\n", signature, id)
	return 0, nil
}
