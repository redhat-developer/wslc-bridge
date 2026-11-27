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
	"errors"
	"runtime"
	"strings"
	"testing"

	"wslc-bridge/pkg/bridge"
	"wslc-bridge/pkg/cmd/testutil"
)

func TestDoctorCmd_PrintsTextDiagnostics(t *testing.T) {
	t.Parallel()

	output, err := executeRoot(t, "doctor")
	if err == nil {
		// A full runtime is installed on this machine; the failure output
		// cannot be asserted there.
		t.Skip("doctor succeeded because a runtime is installed")
	}
	var exitError *bridge.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("doctor error = %v, want *bridge.ExitError", err)
	}
	if !strings.Contains(output, "Platform: ") || !strings.Contains(output, "Status: ") {
		t.Fatalf("doctor output = %q", output)
	}
}

func TestDoctorCmd_PrintsJSONDiagnostics(t *testing.T) {
	t.Parallel()

	output, err := executeRoot(t, "doctor", "--json")
	if err == nil {
		t.Skip("doctor succeeded because a runtime is installed")
	}
	var exitError *bridge.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("doctor error = %v, want *bridge.ExitError", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("doctor JSON %q: %v", output, err)
	}
	if payload["os"] != runtime.GOOS || payload["arch"] != runtime.GOARCH {
		t.Fatalf("doctor JSON = %#v", payload)
	}
}

func TestValueOrUnavailable(t *testing.T) {
	t.Parallel()

	if got := valueOrUnavailable(""); got != "unavailable" {
		t.Errorf("valueOrUnavailable(\"\") = %q", got)
	}
	if got := valueOrUnavailable("1.2"); got != "1.2" {
		t.Errorf("valueOrUnavailable(\"1.2\") = %q", got)
	}
}

func TestDoctorCmd_Examples(t *testing.T) {
	t.Parallel()

	cmd := NewDoctorCmd()
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

func TestDoctorCmd_RejectsPositionalArguments(t *testing.T) {
	t.Parallel()

	_, err := executeRoot(t, "doctor", "stray")
	assertExitCode(t, err, 2, "doctor does not accept positional arguments")
}
