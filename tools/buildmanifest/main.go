/**********************************************************************
 * Copyright (C) 2026 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

// Command buildmanifest writes checksums.txt and bridge-manifest.json for the
// Windows executables produced by `make build`.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	SchemaVersion int        `json:"schemaVersion"`
	Version       string     `json:"version"`
	Commit        string     `json:"commit"`
	BuildDate     string     `json:"buildDate"`
	Artifacts     []artifact `json:"artifacts"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "buildmanifest: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("buildmanifest", flag.ContinueOnError)
	dist := fs.String("dist", "dist", "directory containing the built artifacts")
	version := fs.String("version", "0.0.1-dev", "version recorded in the manifest")
	commit := fs.String("commit", "unknown", "git commit recorded in the manifest")
	date := fs.String("date", "", "build date in RFC 3339 format (default: current UTC time)")
	archs := fs.String("arch", "amd64 arm64", "space separated GOARCH values to hash")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *version == "" {
		return fmt.Errorf("-version must not be empty")
	}
	if *date == "" {
		*date = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}

	m := manifest{SchemaVersion: 1, Version: *version, Commit: *commit, BuildDate: *date}
	checksums := make([]string, 0, 2)

	for _, arch := range strings.Fields(*archs) {
		name := fmt.Sprintf("wslc-bridge-windows-%s.exe", arch)
		sum, err := fileSHA256(filepath.Join(*dist, name))
		if err != nil {
			return err
		}
		m.Artifacts = append(m.Artifacts, artifact{OS: "windows", Arch: arch, File: name, SHA256: sum})
		checksums = append(checksums, fmt.Sprintf("%s  %s", sum, name))
	}

	if err := os.WriteFile(
		filepath.Join(*dist, "checksums.txt"),
		[]byte(strings.Join(checksums, "\n")+"\n"),
		0o644,
	); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*dist, "bridge-manifest.json"), append(data, '\n'), 0o644)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
