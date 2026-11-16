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
	"testing"

	"wslc-bridge/pkg/bridge"
	"wslc-bridge/pkg/cmd/testutil"
)

func TestServeCmd_Flags(t *testing.T) {
	t.Parallel()

	serveCmd := NewServeCmd()
	pipe := serveCmd.Flags().Lookup("pipe")
	if pipe == nil || pipe.DefValue != bridge.DefaultPipeName {
		t.Fatalf("--pipe flag = %#v, want default %q", pipe, bridge.DefaultPipeName)
	}
	if serveCmd.Flags().Lookup("shutdown-on-stdin-eof") == nil {
		t.Fatal("expected --shutdown-on-stdin-eof flag")
	}
}

func TestServeCmd_RejectsPositionalArguments(t *testing.T) {
	t.Parallel()

	_, err := executeRoot(t, "serve", "stray")
	assertExitCode(t, err, 2, "serve does not accept positional arguments")
}

func TestServeCmd_Examples(t *testing.T) {
	t.Parallel()

	cmd := NewServeCmd()
	if cmd.Example == "" {
		t.Fatal("Example field should not be empty")
	}

	commands, err := testutil.ParseExampleCommands(cmd.Example)
	if err != nil {
		t.Fatalf("Failed to parse examples: %v", err)
	}
	if len(commands) != 3 {
		t.Errorf("Expected 3 example commands, got %d", len(commands))
	}

	if err := testutil.ValidateCommandExamples(NewRootCmd(), cmd.Example); err != nil {
		t.Errorf("Example validation failed: %v", err)
	}
}
