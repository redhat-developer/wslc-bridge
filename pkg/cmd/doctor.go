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
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"wslc-bridge/pkg/bridge"
)

// doctorCmd contains the configuration for the doctor command
type doctorCmd struct {
	json bool
}

// run executes the doctor command logic
func (d *doctorCmd) run(cmd *cobra.Command, args []string) error {
	result := bridge.Doctor(context.Background())
	out := cmd.OutOrStdout()
	if d.json {
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "Platform: %s/%s\n", result.OS, result.Arch)
		fmt.Fprintf(out, "WSLC: %s\n", valueOrUnavailable(result.WSLCVersion))
		fmt.Fprintf(out, "WSL: %s\n", valueOrUnavailable(result.WSLVersion))
		fmt.Fprintf(out, "Guest Docker API: %s\n", valueOrUnavailable(result.GuestAPIVersion))
		if result.Error != "" {
			fmt.Fprintf(out, "Status: %s\n", result.Error)
		} else {
			fmt.Fprintln(out, "Status: ready")
		}
	}
	if result.Error != "" {
		return &bridge.ExitError{Code: result.ExitCode, Message: result.Error}
	}
	return nil
}

func NewDoctorCmd() *cobra.Command {
	d := &doctorCmd{}

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check WSLC, WSL, and guest Docker API availability",
		Example: `# Check runtime prerequisites
wslc-bridge doctor

# Check runtime prerequisites in JSON format
wslc-bridge doctor --json`,
		Args: noArgs,
		RunE: d.run,
	}

	cmd.Flags().BoolVar(&d.json, "json", false, "print machine-readable diagnostics")

	return cmd
}

func valueOrUnavailable(value string) string {
	if value == "" {
		return "unavailable"
	}
	return value
}
