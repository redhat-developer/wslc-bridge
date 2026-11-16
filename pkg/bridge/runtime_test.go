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
	"context"
	"encoding/binary"
	"errors"
	"os/exec"
	"runtime"
	"testing"

	"wslc-bridge/pkg/process"
	"wslc-bridge/pkg/wslutil"
)

func TestParseWSLVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
		wantOK bool
	}{
		{name: "minimum prerelease", output: "WSL version: 2.9.3.0\nKernel version: 6.6.87", want: "2.9.3", wantOK: true},
		{name: "newer", output: "WSL version: 3.0.3.0", want: "3.0.3", wantOK: true},
		{name: "no version label", output: "kernel version: 6.6.87", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseWSLVersion(test.output)
			if got != test.want || ok != test.wantOK {
				t.Fatalf("parseWSLVersion(%q) = (%q, %t), want (%q, %t)", test.output, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestCompareVersionUsesNumericComponents(t *testing.T) {
	t.Parallel()

	if compareVersion("2.10.0", "2.9.3") <= 0 {
		t.Fatal("2.10.0 must compare newer than 2.9.3")
	}
	if compareVersion("2.9.2", MinimumWSLVersion) >= 0 {
		t.Fatal("2.9.2 must compare older than the minimum")
	}
}

func TestDecodeWindowsOutputUTF16LE(t *testing.T) {
	t.Parallel()

	value := "WSL version: 2.9.3.0"
	data := make([]byte, 2+len(value)*2)
	data[0], data[1] = 0xff, 0xfe
	for i, character := range value {
		binary.LittleEndian.PutUint16(data[2+i*2:], uint16(character))
	}
	if got := wslutil.DecodeWindowsOutput(data); got != value {
		t.Fatalf("DecodeWindowsOutput() = %q, want %q", got, value)
	}
}

func TestDetectRuntimeRejectsNonWindowsHost(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("checks the default dependencies on a non-Windows host")
	}
	_, err := DetectRuntime(context.Background())
	assertExit(t, err, 4, "only runs on Windows")
}

func TestDetectRuntime(t *testing.T) {
	t.Parallel()

	failure := &process.CommandError{Name: "wslc.exe", ExitCode: 5, Stderr: "wslc exploded"}
	tests := []struct {
		name        string
		goos        string
		missing     string
		runner      func(*fakeRunner)
		wantCode    int
		wantMessage string
	}{
		{name: "rejects non-Windows host", goos: "linux", wantCode: 4, wantMessage: "only runs on Windows"},
		{name: "requires wslc.exe", missing: "wslc.exe", wantCode: 3, wantMessage: "wslc.exe was not found"},
		{name: "requires wsl.exe", missing: "wsl.exe", wantCode: 4, wantMessage: "wsl.exe was not found"},
		{
			name:        "reports wslc.exe version failure",
			runner:      func(r *fakeRunner) { r.wslcErr = failure },
			wantCode:    3,
			wantMessage: "wslc.exe version failed: wslc exploded",
		},
		{
			name:        "rejects empty wslc.exe version",
			runner:      func(r *fakeRunner) { r.wslcResult = process.Result{Stdout: []byte(" \n")} },
			wantCode:    3,
			wantMessage: "returned no output",
		},
		{
			name:        "reports wsl --version failure",
			runner:      func(r *fakeRunner) { r.wslErr = errors.New("wsl broken") },
			wantCode:    4,
			wantMessage: "wsl --version failed: wsl broken",
		},
		{
			name:        "rejects unparseable WSL version",
			runner:      func(r *fakeRunner) { r.wslResult = process.Result{Stdout: []byte("kernel version: 6.6.87")} },
			wantCode:    4,
			wantMessage: "could not parse",
		},
		{
			name:        "rejects outdated WSL",
			runner:      func(r *fakeRunner) { r.wslResult = process.Result{Stdout: []byte("WSL version: 2.0.0.0")} },
			wantCode:    4,
			wantMessage: "WSL 2.0.0 is installed but unsupported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := healthyRunner()
			if test.runner != nil {
				test.runner(runner)
			}
			deps := fakeDependencies(t, runner, nil)
			if test.goos != "" {
				deps.goos = test.goos
			}
			if test.missing != "" {
				lookPath := deps.lookPath
				deps.lookPath = func(file string) (string, error) {
					if file == test.missing {
						return "", exec.ErrNotFound
					}
					return lookPath(file)
				}
			}
			_, err := deps.detectRuntime(context.Background())
			assertExit(t, err, test.wantCode, test.wantMessage)
		})
	}
}

func TestDetectRuntimeReturnsDetectedRuntime(t *testing.T) {
	t.Parallel()

	deps := fakeDependencies(t, healthyRunner(), nil)
	info, err := deps.detectRuntime(context.Background())
	if err != nil {
		t.Fatalf("detectRuntime() = %v, want nil", err)
	}
	if info.WSLCPath != fakeWSLCPath || info.WSLCVersion != "1.2.3" ||
		info.WSLPath != fakeWSLPath || info.WSLVersion != "2.9.3" {
		t.Fatalf("detectRuntime() = %#v", info)
	}
}

func TestExitErrorPreservesItsCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("wslc exploded")
	exitError := &ExitError{Code: 3, Message: "wslc.exe version failed", Err: cause}
	if !errors.Is(exitError, cause) {
		t.Fatal("ExitError does not unwrap to its cause")
	}
	if exitError.Error() != "wslc.exe version failed (exit code 3)" {
		t.Fatalf("ExitError.Error() = %q", exitError.Error())
	}
}
