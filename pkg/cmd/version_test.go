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

package cmd

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"wslc-bridge/pkg/buildinfo"
	"wslc-bridge/pkg/cmd/testutil"
)

func TestVersionCmd_PrintsText(t *testing.T) {
	t.Parallel()

	output, err := executeRoot(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output, "wslc-bridge ") || !strings.Contains(output, runtime.GOOS) {
		t.Fatalf("version output = %q", output)
	}
}

func TestVersionCmd_PrintsJSON(t *testing.T) {
	t.Parallel()

	output, err := executeRoot(t, "version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var info buildinfo.VersionInfo
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		t.Fatalf("version JSON %q: %v", output, err)
	}
	if info.OS != runtime.GOOS || info.Arch != runtime.GOARCH || info.GoVersion == "" {
		t.Fatalf("version JSON = %#v", info)
	}
}

func TestVersionCmd_Examples(t *testing.T) {
	t.Parallel()

	cmd := NewVersionCmd()
	if cmd.Example == "" {
		t.Fatal("Example field should not be empty")
	}

	commands, err := testutil.ParseExampleCommands(cmd.Example)
	if err != nil {
		t.Fatalf("Failed to parse examples: %v", err)
	}
	if len(commands) != 2 {
		t.Errorf("Expected 2 example commands, got %d", len(commands))
	}

	if err := testutil.ValidateCommandExamples(NewRootCmd(), cmd.Example); err != nil {
		t.Errorf("Example validation failed: %v", err)
	}
}

func TestVersionCmd_RejectsPositionalArguments(t *testing.T) {
	t.Parallel()

	_, err := executeRoot(t, "version", "stray")
	assertExitCode(t, err, 2, "version does not accept positional arguments")
}
