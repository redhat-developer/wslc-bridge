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

package bridge

import (
	"io"
	"net"
	"os/exec"
	"runtime"

	"wslc-bridge/pkg/guest"
	"wslc-bridge/pkg/native"
	"wslc-bridge/pkg/platform"
	"wslc-bridge/pkg/process"
)

// dependencies are the external services used by DetectRuntime, Doctor and
// Serve. Tests replace them with fakes.
type dependencies struct {
	goos            string
	lookPath        func(file string) (string, error)
	newRunner       func(wslcPath, wslPath string) process.Runner
	newGuestClient  func(wslcPath string) *guest.Client
	newNativeClient func(wslcPath, wslPath string) native.Client
	listenPipe      func(name string) (net.Listener, error)
	stdout          io.Writer
}

func defaultDependencies() dependencies {
	return dependencies{
		goos:            runtime.GOOS,
		lookPath:        exec.LookPath,
		newRunner:       process.NewRunner,
		newGuestClient:  guest.NewClient,
		newNativeClient: native.NewClient,
		listenPipe:      platform.ListenPipe,
		stdout:          stdoutWriter{},
	}
}
