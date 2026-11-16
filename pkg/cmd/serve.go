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
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"wslc-bridge/pkg/bridge"
)

// serveCmd contains the configuration for the serve command
type serveCmd struct {
	pipe               string
	shutdownOnStdinEOF bool
}

// addFlags registers the serve flags on cmd
func (s *serveCmd) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.pipe, "pipe", bridge.DefaultPipeName, "Windows named pipe path")
	cmd.Flags().BoolVar(&s.shutdownOnStdinEOF, "shutdown-on-stdin-eof", false, "gracefully stop when stdin closes (for extension child-process lifecycle)")
}

// run executes the serve command logic
func (s *serveCmd) run(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if s.shutdownOnStdinEOF {
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}
	return bridge.Serve(ctx, s.pipe)
}

func NewServeCmd() *cobra.Command {
	s := &serveCmd{}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the Docker API over a Windows named pipe",
		Example: `# Serve the Docker API on the default named pipe
wslc-bridge serve

# Serve on a custom named pipe
wslc-bridge serve --pipe '\\.\pipe\wslc_engine_test'

# Stop when stdin closes (extension child-process lifecycle)
wslc-bridge serve --shutdown-on-stdin-eof`,
		Args: noArgs,
		RunE: s.run,
	}

	s.addFlags(cmd)

	return cmd
}
