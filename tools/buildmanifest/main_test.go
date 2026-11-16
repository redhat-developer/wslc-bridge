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

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSHA256(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}
	const want = "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
	if got != want {
		t.Fatalf("fileSHA256 = %q, want %q", got, want)
	}
}

func TestFileSHA256MissingFile(t *testing.T) {
	t.Parallel()

	_, err := fileSHA256(filepath.Join(t.TempDir(), "absent"))
	if err == nil {
		t.Fatal("expected an error for a missing artifact")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Fatalf("error %q does not name the missing file", err)
	}
}

func TestRunWritesChecksumsAndManifest(t *testing.T) {
	t.Parallel()

	dist := t.TempDir()
	for _, arch := range []string{"amd64", "arm64"} {
		name := "wslc-bridge-windows-" + arch + ".exe"
		if err := os.WriteFile(filepath.Join(dist, name), []byte("binary-"+arch), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	args := []string{
		"-dist", dist,
		"-version", "1.2.3",
		"-commit", "abc1234",
		"-date", "2026-01-01T00:00:00Z",
		"-arch", "amd64 arm64",
	}
	if err := run(args); err != nil {
		t.Fatalf("run: %v", err)
	}

	checksums, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatalf("read checksums.txt: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(checksums), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("checksums.txt has %d lines, want 2:\n%s", len(lines), checksums)
	}
	for _, line := range lines {
		if !strings.Contains(line, "  wslc-bridge-windows-") {
			t.Fatalf("unexpected checksum line %q", line)
		}
	}
	if !strings.HasSuffix(string(checksums), "\n") {
		t.Fatal("checksums.txt does not end with a newline")
	}

	raw, err := os.ReadFile(filepath.Join(dist, "bridge-manifest.json"))
	if err != nil {
		t.Fatalf("read bridge-manifest.json: %v", err)
	}
	var got manifest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	switch {
	case got.SchemaVersion != 1:
		t.Errorf("schemaVersion = %d, want 1", got.SchemaVersion)
	case got.Version != "1.2.3":
		t.Errorf("version = %q, want 1.2.3", got.Version)
	case got.Commit != "abc1234":
		t.Errorf("commit = %q, want abc1234", got.Commit)
	case got.BuildDate != "2026-01-01T00:00:00Z":
		t.Errorf("buildDate = %q, want 2026-01-01T00:00:00Z", got.BuildDate)
	case len(got.Artifacts) != 2:
		t.Fatalf("artifacts = %d, want 2", len(got.Artifacts))
	}
	for i, arch := range []string{"amd64", "arm64"} {
		a := got.Artifacts[i]
		if a.OS != "windows" || a.Arch != arch || a.File != "wslc-bridge-windows-"+arch+".exe" || a.SHA256 == "" {
			t.Errorf("artifact %d = %+v", i, a)
		}
	}
}

func TestRunRejectsEmptyVersion(t *testing.T) {
	t.Parallel()

	if err := run([]string{"-dist", t.TempDir(), "-version", ""}); err == nil {
		t.Fatal("expected an error for an empty version")
	}
}
