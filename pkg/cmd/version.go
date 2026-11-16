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
	"fmt"

	"github.com/spf13/cobra"

	"wslc-bridge/pkg/buildinfo"
)

// versionCmd contains the configuration for the version command
type versionCmd struct {
	json bool
}

// run executes the version command logic
func (v *versionCmd) run(cmd *cobra.Command, args []string) error {
	info := buildinfo.Info()
	if v.json {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "wslc-bridge %s (%s/%s, Go %s)\n", info.Version, info.OS, info.Arch, info.GoVersion)
	return err
}

func NewVersionCmd() *cobra.Command {
	v := &versionCmd{}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Example: `# Show version
wslc-bridge version

# Show version in JSON format
wslc-bridge version --json`,
		Args: noArgs,
		RunE: v.run,
	}

	cmd.Flags().BoolVar(&v.json, "json", false, "print machine-readable version information")

	return cmd
}
