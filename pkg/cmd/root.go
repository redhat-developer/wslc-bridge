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
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"wslc-bridge/pkg/bridge"
)

func NewRootCmd() *cobra.Command {
	// A bare invocation (or one starting with a flag) means serve.
	s := &serveCmd{}

	rootCmd := &cobra.Command{
		Use:   "wslc-bridge",
		Short: "Bridge Docker API clients to WSL Containers (WSLC)",
		Example: `# Serve the Docker API on the default named pipe
wslc-bridge

# Serve on a custom named pipe
wslc-bridge --pipe '\\.\pipe\wslc_engine_test'`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return &bridge.ExitError{Code: 2, Message: fmt.Sprintf("unknown command %q (expected serve, doctor, or version)", args[0])}
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          s.run,
	}
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &bridge.ExitError{Code: 2, Message: err.Error(), Err: err}
	})
	s.addFlags(rootCmd)

	rootCmd.AddCommand(NewServeCmd())
	rootCmd.AddCommand(NewDoctorCmd())
	rootCmd.AddCommand(NewVersionCmd())

	return rootCmd
}

// noArgs rejects positional arguments with the usage exit code.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return &bridge.ExitError{Code: 2, Message: fmt.Sprintf("%s does not accept positional arguments", cmd.Name())}
	}
	return nil
}

// ExitCode maps an error returned by the root command to a process exit code.
func ExitCode(err error) int {
	var exitErr *bridge.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
