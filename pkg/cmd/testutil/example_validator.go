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
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ExampleCommand represents a parsed command from an Example string
type ExampleCommand struct {
	Raw         string            // Original command line
	Binary      string            // Binary name (should be "wslc-bridge")
	Args        []string          // Subcommands and positional arguments
	FlagPresent map[string]bool   // Flags that were present in the command
	FlagValues  map[string]string // Values for flags (empty string if no value provided)
}

// ParseExampleCommands extracts wslc-bridge commands from an Example string
// - Ignores empty lines and comment lines (starting with #)
// - Returns error if non-comment lines do not contain a wslc-bridge command
func ParseExampleCommands(example string) ([]ExampleCommand, error) {
	var commands []ExampleCommand
	for i, line := range strings.Split(example, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cmd, err := parseCommandLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		commands = append(commands, cmd)
	}
	return commands, nil
}

// parseCommandLine parses a single command line into ExampleCommand
func parseCommandLine(line string) (ExampleCommand, error) {
	parts := splitCommandLine(line)
	if len(parts) == 0 {
		return ExampleCommand{}, fmt.Errorf("empty command line")
	}
	if parts[0] != "wslc-bridge" {
		return ExampleCommand{}, fmt.Errorf("command must start with 'wslc-bridge', got '%s'", parts[0])
	}

	cmd := ExampleCommand{
		Raw:         line,
		Binary:      parts[0],
		Args:        []string{},
		FlagPresent: make(map[string]bool),
		FlagValues:  make(map[string]string),
	}

	for i := 1; i < len(parts); i++ {
		part := parts[i]
		switch {
		case part == "--":
			cmd.Args = append(cmd.Args, parts[i+1:]...)
			i = len(parts)
		case strings.HasPrefix(part, "--"):
			name, value := parseFlag(strings.TrimPrefix(part, "--"), parts, &i)
			cmd.FlagPresent[name] = true
			cmd.FlagValues[name] = value
		case strings.HasPrefix(part, "-") && len(part) > 1:
			name, value := parseFlag(strings.TrimPrefix(part, "-"), parts, &i)
			cmd.FlagPresent[name] = true
			cmd.FlagValues[name] = value
		default:
			cmd.Args = append(cmd.Args, part)
		}
	}

	return cmd, nil
}

// parseFlag parses --flag=value, --flag value, or --flag (prefix already removed)
func parseFlag(flagPart string, parts []string, i *int) (string, string) {
	if idx := strings.Index(flagPart, "="); idx != -1 {
		return flagPart[:idx], flagPart[idx+1:]
	}
	if *i+1 < len(parts) && !strings.HasPrefix(parts[*i+1], "-") {
		*i++
		return flagPart, parts[*i]
	}
	return flagPart, ""
}

// splitCommandLine splits a command line by whitespace, respecting quotes
func splitCommandLine(line string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, char := range line {
		switch {
		case (char == '"' || char == '\'') && !inQuote:
			inQuote = true
			quoteChar = char
		case char == quoteChar && inQuote:
			inQuote = false
			quoteChar = 0
		case char == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// ValidateExampleCommand verifies the command exists and its flags are valid
func ValidateExampleCommand(rootCmd *cobra.Command, exampleCmd ExampleCommand) error {
	cmd, remainingArgs, err := rootCmd.Find(exampleCmd.Args)
	if err != nil {
		return fmt.Errorf("command not found: %s: %w", strings.Join(exampleCmd.Args, " "), err)
	}

	if cmd.Args != nil {
		if err := cmd.Args(cmd, remainingArgs); err != nil {
			return fmt.Errorf("invalid arguments for %q: %w", cmd.CommandPath(), err)
		}
	}

	for flagName := range exampleCmd.FlagPresent {
		lookup := func(flags *pflag.FlagSet) *pflag.Flag { return flags.Lookup(flagName) }
		prefix := "--"
		if len(flagName) == 1 {
			lookup = func(flags *pflag.FlagSet) *pflag.Flag { return flags.ShorthandLookup(flagName) }
			prefix = "-"
		}

		flag := lookup(cmd.Flags())
		if flag == nil {
			flag = lookup(cmd.PersistentFlags())
		}
		if flag == nil {
			flag = lookup(cmd.InheritedFlags())
		}
		if flag == nil {
			return fmt.Errorf("flag %s%s not found in command: %s", prefix, flagName, cmd.CommandPath())
		}

		if exampleCmd.FlagValues[flagName] == "" && flag.NoOptDefVal == "" && flag.Value.Type() != "bool" {
			return fmt.Errorf("flag %s%s requires a value in command: %s", prefix, flagName, cmd.CommandPath())
		}
	}

	return nil
}

// ValidateCommandExamples combines parsing and validation
func ValidateCommandExamples(rootCmd *cobra.Command, example string) error {
	commands, err := ParseExampleCommands(example)
	if err != nil {
		return fmt.Errorf("failed to parse examples: %w", err)
	}

	for _, cmd := range commands {
		if err := ValidateExampleCommand(rootCmd, cmd); err != nil {
			return fmt.Errorf("invalid example '%s': %w", cmd.Raw, err)
		}
	}

	return nil
}
