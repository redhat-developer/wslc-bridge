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
	"bytes"
	"errors"
	"strings"
	"testing"

	"wslc-bridge/pkg/bridge"
	"wslc-bridge/pkg/cmd/testutil"
)

// executeRoot runs the root command with args and returns its stdout.
func executeRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	rootCmd := NewRootCmd()
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func assertExitCode(t *testing.T, err error, code int, messagePart string) {
	t.Helper()
	var exitError *bridge.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("error = %v, want *bridge.ExitError", err)
	}
	if exitError.Code != code {
		t.Fatalf("ExitError = %q, want code %d", exitError.Error(), code)
	}
	if messagePart != "" && !strings.Contains(exitError.Message, messagePart) {
		t.Fatalf("ExitError.Message = %q, want it to contain %q", exitError.Message, messagePart)
	}
}

func TestRootCmd_Initialization(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCmd()
	if rootCmd.Use != "wslc-bridge" {
		t.Errorf("Use = %q, want %q", rootCmd.Use, "wslc-bridge")
	}
	if rootCmd.Short == "" {
		t.Error("expected Short description to be set")
	}
	for _, name := range []string{"serve", "doctor", "version"} {
		if found, _, err := rootCmd.Find([]string{name}); err != nil || found.Name() != name {
			t.Errorf("Find(%q) = %v, %v", name, found, err)
		}
	}
}

func TestRootCmd_ServeFlags(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCmd()
	pipe := rootCmd.Flags().Lookup("pipe")
	if pipe == nil || pipe.DefValue != bridge.DefaultPipeName {
		t.Fatalf("--pipe flag = %#v, want default %q", pipe, bridge.DefaultPipeName)
	}
	if rootCmd.Flags().Lookup("shutdown-on-stdin-eof") == nil {
		t.Fatal("expected --shutdown-on-stdin-eof flag on root")
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	t.Parallel()

	_, err := executeRoot(t, "bogus")
	assertExitCode(t, err, 2, `unknown command "bogus" (expected serve, doctor, or version)`)
}

func TestExecute_StrayArgumentAfterServeFlags(t *testing.T) {
	t.Parallel()

	_, err := executeRoot(t, "--pipe", `\\.\pipe\custom`, "stray")
	assertExitCode(t, err, 2, `unknown command "stray"`)
}

func TestExecute_RejectsSingleDashLongFlags(t *testing.T) {
	t.Parallel()

	_, err := executeRoot(t, "-pipe", `\\.\pipe\custom`)
	assertExitCode(t, err, 2, "")
}

func TestExecute_UnknownFlagsExitWithCodeTwo(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"--bogus"},
		{"serve", "--bogus"},
		{"version", "--bogus"},
		{"doctor", "--bogus"},
	} {
		_, err := executeRoot(t, args...)
		if err == nil {
			t.Fatalf("Execute(%v) = nil, want exit code 2", args)
		}
		assertExitCode(t, err, 2, "")
	}
}

func TestExecute_WithHelp(t *testing.T) {
	t.Parallel()

	output, err := executeRoot(t, "-h")
	if err != nil {
		t.Fatalf("Execute(-h) = %v, want nil", err)
	}
	if !strings.Contains(output, "Usage:") || !strings.Contains(output, "serve") {
		t.Fatalf("help output = %q", output)
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()

	if got := ExitCode(&bridge.ExitError{Code: 3, Message: "x"}); got != 3 {
		t.Errorf("ExitCode(ExitError{3}) = %d, want 3", got)
	}
	if got := ExitCode(errors.New("plain")); got != 1 {
		t.Errorf("ExitCode(plain) = %d, want 1", got)
	}
}

func TestRootCmd_Examples(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd()
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
