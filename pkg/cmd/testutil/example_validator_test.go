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

package testutil

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "wslc-bridge"}
	sub := &cobra.Command{Use: "sub", Args: cobra.NoArgs, Run: func(*cobra.Command, []string) {}}
	sub.Flags().String("name", "", "a value flag")
	sub.Flags().BoolP("json", "j", false, "a bool flag")
	root.AddCommand(sub)
	return root
}

func TestParseExampleCommands(t *testing.T) {
	t.Parallel()

	commands, err := ParseExampleCommands(`# comment

wslc-bridge sub --name 'a b' -j
wslc-bridge sub --name=x`)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("got %d commands, want 2", len(commands))
	}
	first := commands[0]
	if strings.Join(first.Args, " ") != "sub" || first.FlagValues["name"] != "a b" || !first.FlagPresent["j"] {
		t.Fatalf("first command = %#v", first)
	}
	if commands[1].FlagValues["name"] != "x" {
		t.Fatalf("second command = %#v", commands[1])
	}
}

func TestParseExampleCommandsRejectsOtherBinary(t *testing.T) {
	t.Parallel()

	if _, err := ParseExampleCommands("docker ps"); err == nil {
		t.Fatal("expected error for a non wslc-bridge command")
	}
}

func TestValidateCommandExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		example string
		wantErr string
	}{
		{example: "wslc-bridge sub --name x --json"},
		{example: "wslc-bridge sub -j"},
		{example: "wslc-bridge sub --bogus", wantErr: "flag --bogus not found"},
		{example: "wslc-bridge sub --name", wantErr: "requires a value"},
		{example: "wslc-bridge sub stray", wantErr: "invalid arguments"},
	}
	for _, test := range tests {
		err := ValidateCommandExamples(newTestRoot(), test.example)
		if test.wantErr == "" && err != nil {
			t.Errorf("%q: unexpected error %v", test.example, err)
		}
		if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
			t.Errorf("%q: error = %v, want it to contain %q", test.example, err, test.wantErr)
		}
	}
}
