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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/recursive-version-control-system/snapshot"
)

// runHelper invokes the external helper tool for the given command/namespace.
//
// The stdin and stderr are connected to the corresponding stdin/stderr of
// the rvcs tool, while the stdout is captured.
//
// If the external helper tool exits with a 0 status and outputs the hash
// of a snapshot, then this method returns that hash. Otherwise, this returns
// an error.
func runHelper(ctx context.Context, cmd, namespace string, args []string) (*snapshot.Hash, error) {
	helperCommand := fmt.Sprintf("rvcs-%s-%s", cmd, namespace)
	helper := exec.CommandContext(ctx, helperCommand, args...)
	var out bytes.Buffer
	helper.Stdin = os.Stdin
	helper.Stdout = &out
	helper.Stderr = os.Stderr
	if err := helper.Run(); err != nil {
		return nil, fmt.Errorf("failure running the helper command %q: %v", helperCommand, err)
	}
	h, err := snapshot.ParseHash(out.String())
	if err != nil {
		return nil, fmt.Errorf("failure parsing the stdout of the helper %q: %v", helperCommand, err)
	}
	return h, nil
}
