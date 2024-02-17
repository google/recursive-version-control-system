// Copyright 2024 Google LLC
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
	"archive/zip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/recursive-version-control-system/bundle"
	"github.com/google/recursive-version-control-system/storage"
)

func readMetadata(ctx context.Context, bundlePath string) (signature string, previousBundles []string, err error) {
	r, err := zip.OpenReader(bundlePath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, err
	}
	if err != nil {
		return "", nil, fmt.Errorf("failure opening the zip file %q: %w", bundlePath, err)
	}
	defer r.Close()

	sigFile, err := r.Open("metadata/signature")
	if err != nil {
		return "", nil, fmt.Errorf("failure finding the signature in the bundle: %q, %w", bundlePath, err)
	}
	defer sigFile.Close()
	sigBytes, err := io.ReadAll(sigFile)
	if err != nil {
		return "", nil, fmt.Errorf("failure reading signature contents from the bundle: %q, %w", bundlePath, err)
	}
	signature = strings.TrimSpace(string(sigBytes))

	prevFile, err := r.Open("metadata/previous")
	if errors.Is(err, fs.ErrNotExist) {
		// There are no previous bundles
		return signature, nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("failure finding the previous bundles from the bundle: %q, %w", bundlePath, err)
	}
	defer prevFile.Close()

	prevBytes, err := io.ReadAll(prevFile)
	if err != nil {
		return "", nil, fmt.Errorf("failure reading previous bundles from the bundle: %q, %w", bundlePath, err)
	}

	previousBundles = strings.Split(strings.TrimSpace(string(prevBytes)), "\n")
	return signature, previousBundles, nil
}

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  rvcs-pull-file file://<PATH> <IDENTITY> <PREVIOUS_HASH> <OUTPUT_FILE>")
		os.Exit(1)
	}

	path := strings.TrimPrefix(os.Args[1], "file://")
	identity := os.Args[2]
	// N.B. `os.Args[3]` holds the previous hash for the signature, which we don't use.
	outFile := os.Args[4]

	bundleName := sha256.Sum256([]byte(fmt.Sprintf("%s\n", identity)))
	bundlePath := fmt.Sprintf("%s/%x-bundle.zip", path, bundleName)
	
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failure resolving the user's home dir: %v\n", err)
		os.Exit(1)
	}
	s := &storage.LocalFiles{filepath.Join(home, ".rvcs/archive")}
	ctx := context.Background()

	signature, previousBundles, err := readMetadata(ctx, bundlePath)
	if errors.Is(err, fs.ErrNotExist) {
		// The bundle does not exist, so there is nothing for us to import.
		fmt.Fprintf(os.Stderr, "File %q does not exist...\n", bundlePath)
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failure reading the bundle metadata: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outFile, []byte(signature), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "failure writing the bundle signature: %v\n", err)
		os.Exit(1)
	}

	imported, err := bundle.Import(ctx, s, bundlePath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failure importing the bundle: %v\n", err)
		os.Exit(1)
	}
	for _, previousBundle := range previousBundles {
		if len(imported) == 0 {
			return
		}
		imported, err = bundle.Import(ctx, s, previousBundle, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failure importing from the previous bundle %q: %v\n", previousBundle, err)
			os.Exit(1)
		}
	}
}
