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
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/googlestaging/recursive-version-control-system/archiver"
	"github.com/googlestaging/recursive-version-control-system/snapshot"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "snapshot" {
		fmt.Printf("Usage: %s snapshot [<PATH>]\n", os.Args[0])
		os.Exit(1)
	}
	var path string
	if len(os.Args) > 2 {
		path = os.Args[2]
	} else {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("Error determining the current working directory: %v\n", err)
		}
		path = wd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("Failure resolving the absolute path of %q: %v", path, err)
	}
	path = abs

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failure resolving the user's home dir: %v\n", err)
	}
	s := &archiver.Store{filepath.Join(home, ".archive")}
	ctx := context.Background()
	if h, err := archiver.Snapshot(ctx, s, snapshot.Path(path)); err != nil {
		log.Fatalf("Failure snapshotting the directory %q: %v\n", path, err)
	} else if h == nil {
		fmt.Printf("Did not generate a snapshot as %q does not exist\n", path)
	} else {
		fmt.Printf("Snapshotted %q to %q\n", path, h)
	}
}
