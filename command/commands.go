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
	"os"
	"path/filepath"
	"strings"

	"github.com/google/recursive-version-control-system/config"
	"github.com/google/recursive-version-control-system/publish"
	"github.com/google/recursive-version-control-system/snapshot"
	"github.com/google/recursive-version-control-system/storage"
)

type command func(context.Context, *storage.LocalFiles, string, []string) (int, error)

var (
	commandMap = map[string]command{
		"add-mirror":    addMirrorCommand,
		"export":        exportCommand,
		"import":        importCommand,
		"inspect":       inspectCommand,
		"log":           logCommand,
		"merge":         mergeCommand,
		"publish":       publishCommand,
		"remove-mirror": removeMirrorCommand,
		"snapshot":      snapshotCommand,
	}

	usage = `Usage: %s <SUBCOMMAND>

Where <SUBCOMMAND> is one of:

	add-mirror
	export
	import
	inspect
	log
	merge
	publish
	remove-mirror
	snapshot
`
)

func resolveIdentitySnapshot(ctx context.Context, s *storage.LocalFiles, id *snapshot.Identity) (signature *snapshot.Hash, signed *snapshot.Hash, err error) {
	settings, err := config.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("failure reading the config settings: %v", err)
	}
	signature, signed, err = publish.Pull(ctx, settings, s, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failure pulling the latest snapshot for %q: %v", id, err)
	}
	return signature, signed, nil
}

func resolveSnapshot(ctx context.Context, s *storage.LocalFiles, name string) (*snapshot.Hash, error) {
	h, err := snapshot.ParseHash(name)
	if err == nil {
		return h, nil
	}
	id, err := snapshot.ParseIdentity(name)
	if err == nil {
		_, signed, err := resolveIdentitySnapshot(ctx, s, id)
		return signed, err
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("failure resolving the absolute path of %q: %v", name, err)
	}
	h, _, err = s.FindSnapshot(ctx, snapshot.Path(abs))
	if err == nil {
		return h, nil
	}
	return nil, fmt.Errorf("unable to resolve the hash corresponding to %q", name)
}

func readHashesFromFile(ctx context.Context, path string) ([]*snapshot.Hash, error) {
	if path == "" {
		return nil, nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failure reading hashes from the file %q: %v", path, err)
	}
	var hashes []*snapshot.Hash
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		h, err := snapshot.ParseHash(line)
		if err != nil {
			return nil, fmt.Errorf("failure parsing file hash entry %q: %v", line, err)
		}
		if h != nil {
			hashes = append(hashes, h)
		}
	}
	return hashes, nil
}

func hashesFromFileAndFlag(ctx context.Context, fromFile, fromFlag string) ([]*snapshot.Hash, error) {
	hashes, err := readHashesFromFile(ctx, fromFile)
	if err != nil {
		return nil, err
	}

	for _, s := range strings.Split(fromFlag, ",") {
		if len(s) == 0 {
			continue
		}
		h, err := snapshot.ParseHash(s)
		if err != nil {
			return nil, fmt.Errorf("failure parsing flag hash entry %q: %v", s, err)
		}
		if h != nil {
			hashes = append(hashes, h)
		}
	}
	return hashes, nil
}

// Run implements the subcommands of the `rvcs` CLI.
//
// The passed in `args` should be the value returned by `os.Args`
//
// The returned value is the exit code of the command; 0 for success
// and non-zero for any form of failure.
func Run(ctx context.Context, s *storage.LocalFiles, args []string) (exitCode int) {
	if len(args) < 2 {
		fmt.Fprintf(flag.CommandLine.Output(), usage, args[0])
		return 1
	}
	subcommand, ok := commandMap[args[1]]
	if !ok {
		fmt.Fprintf(flag.CommandLine.Output(), "Unknown subcommand %q\n", args[1])
		fmt.Fprintf(flag.CommandLine.Output(), usage, args[0])
		return 1
	}
	retcode, err := subcommand(ctx, s, args[0], args[2:])
	if err != nil {
		fmt.Fprintf(flag.CommandLine.Output(), "Failure running the %q subcommand: %v\n", args[1], err)
	}
	return retcode
}
