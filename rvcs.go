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

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/googlestaging/recursive-version-control-system/archive"
	"github.com/googlestaging/recursive-version-control-system/command"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("failure resolving the user's home dir: %v\n", err)
	}
	s := &archive.Store{filepath.Join(home, ".rvcs/archive")}
	ctx := context.Background()

	ret := command.Run(ctx, s, os.Args)
	os.Exit(ret)
}
