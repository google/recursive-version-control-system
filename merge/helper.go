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

// Package merge defines methods for merging two snapshots together.
package merge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

const (
	HelperEnvironmentVariable     = "RVCS_MERGE_HELPER_COMMAND"
	HelperArgsEnvironmentVariable = "RVCS_MERGE_HELPER_ARGS"
)

func mergeWithHelper(ctx context.Context, s *storage.LocalFiles, p snapshot.Path, mode string, base, src, dest *snapshot.Hash) (*snapshot.Hash, error) {
	helperCmd := os.Getenv(HelperEnvironmentVariable)
	helperArgs := os.Getenv(HelperArgsEnvironmentVariable)
	if len(helperCmd) == 0 {
		helperCmd = "diff3"
		if len(helperArgs) == 0 {
			helperArgs = "[\"-m\"]"
		}
	}
	var args []string
	if err := json.Unmarshal([]byte(helperArgs), &args); err != nil {
		return nil, fmt.Errorf("failure parsing the helper args %q: %v", helperArgs, err)
	}
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("rvcs-merge-helper-%q", helperCmd))
	if err != nil {
		return nil, fmt.Errorf("failure generating the temporary working directory for the merge helper %q: %v", helperCmd, err)
	}
	defer os.RemoveAll(tmpDir)

	tmpPath := snapshot.Path(tmpDir)
	srcPath := tmpPath.Join(snapshot.Path("src")).Join(p)
	if err := Checkout(ctx, s, src, srcPath); err != nil {
		return nil, fmt.Errorf("failure checking out %q to a temporary path for the merge helper: %v", src, err)
	}
	basePath := tmpPath.Join(snapshot.Path("base")).Join(p)
	if err := Checkout(ctx, s, base, basePath); err != nil {
		return nil, fmt.Errorf("failure checking out %q to a temporary path for the merge helper: %v", base, err)
	}
	destPath := tmpPath.Join(snapshot.Path("dest")).Join(p)
	if err := Checkout(ctx, s, dest, destPath); err != nil {
		return nil, fmt.Errorf("failure checking out %q to a temporary path for the merge helper: %v", dest, err)
	}
	args = append(args, string(srcPath), string(basePath), string(destPath))

	helperCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	out, err := exec.CommandContext(helperCtx, helperCmd, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("merge helper %q failed: %v", helperCmd, err)
	}
	contentsHash, err := s.StoreObject(ctx, int64(len(out)), bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("failure hashing the merged contents: %v", err)
	}
	mergedFile := &snapshot.File{
		Mode:     mode,
		Contents: contentsHash,
		Parents:  []*snapshot.Hash{src, dest},
	}
	fileBytes := []byte(mergedFile.String())
	h, err := s.StoreObject(ctx, int64(len(fileBytes)), bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("failure storing the merged snapshot: %v", err)
	}
	return h, nil
}
