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
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wslc-bridge/pkg/process"
	"wslc-bridge/pkg/wslutil"
)

const MinimumWSLVersion = "2.9.3"

var versionPattern = regexp.MustCompile(`(?i)(\d+)\.(\d+)\.(\d+)(?:\.(\d+))?`)

// RuntimeInfo describes the detected WSLC and WSL runtime.
type RuntimeInfo struct {
	WSLCPath    string `json:"wslcPath"`
	WSLCVersion string `json:"wslcVersion"`
	WSLPath     string `json:"wslPath"`
	WSLVersion  string `json:"wslVersion"`
}

func DetectRuntime(ctx context.Context) (RuntimeInfo, error) {
	return defaultDependencies().detectRuntime(ctx)
}

func (d dependencies) detectRuntime(ctx context.Context) (RuntimeInfo, error) {
	if d.goos != "windows" {
		return RuntimeInfo{}, &ExitError{Code: 4, Message: "WSL Containers bridge only runs on Windows"}
	}
	wslcPath, err := d.lookPath("wslc.exe")
	if err != nil {
		return RuntimeInfo{}, &ExitError{Code: 3, Message: "wslc.exe was not found on PATH; install WSLC before starting the bridge", Err: err}
	}
	wslPath, err := d.lookPath("wsl.exe")
	if err != nil {
		return RuntimeInfo{}, &ExitError{Code: 4, Message: "wsl.exe was not found on PATH", Err: err}
	}
	runner := d.newRunner(wslcPath, wslPath)
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	wslcResult, err := runner.RunWSLC(probeCtx, "version")
	if err != nil {
		return RuntimeInfo{}, &ExitError{Code: 3, Message: fmt.Sprintf("wslc.exe version failed: %s", conciseCommandError(err)), Err: err}
	}
	wslcVersion := strings.TrimSpace(wslutil.DecodeWindowsOutput(wslcResult.Stdout))
	if wslcVersion == "" {
		return RuntimeInfo{}, &ExitError{Code: 3, Message: "wslc.exe version returned no output; verify the WSLC installation"}
	}
	wslResult, err := runner.RunWSL(probeCtx, "--version")
	if err != nil {
		return RuntimeInfo{}, &ExitError{Code: 4, Message: fmt.Sprintf("wsl --version failed: %s; update WSL with `wsl --update`", conciseCommandError(err)), Err: err}
	}
	wslVersion, ok := parseWSLVersion(wslutil.DecodeWindowsOutput(wslResult.Stdout))
	if !ok {
		return RuntimeInfo{}, &ExitError{Code: 4, Message: "could not parse `wsl --version`; install WSL 2.9.3 or newer with `wsl --update`"}
	}
	if compareVersion(wslVersion, MinimumWSLVersion) < 0 {
		return RuntimeInfo{}, &ExitError{Code: 4, Message: fmt.Sprintf("WSL %s is installed but unsupported; version %s or newer is required. Run `wsl --update`", wslVersion, MinimumWSLVersion)}
	}
	return RuntimeInfo{
		WSLCPath: wslcPath, WSLCVersion: wslcVersion,
		WSLPath: wslPath, WSLVersion: wslVersion,
	}, nil
}

func parseWSLVersion(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "wsl") && strings.Contains(lower, "version") {
			if match := versionPattern.FindString(line); match != "" {
				return normalizeVersion(match), true
			}
		}
	}
	return "", false
}

func normalizeVersion(value string) string {
	match := versionPattern.FindStringSubmatch(value)
	if len(match) < 4 {
		return value
	}
	return strings.Join(match[1:4], ".")
}

func compareVersion(left, right string) int {
	leftParts := versionParts(left)
	rightParts := versionParts(right)
	for i := range leftParts {
		if leftParts[i] < rightParts[i] {
			return -1
		}
		if leftParts[i] > rightParts[i] {
			return 1
		}
	}
	return 0
}

func versionParts(value string) [4]int {
	match := versionPattern.FindStringSubmatch(value)
	var parts [4]int
	for i := 1; i <= 4 && i < len(match); i++ {
		parts[i-1], _ = strconv.Atoi(match[i])
	}
	return parts
}

func conciseCommandError(err error) string {
	var commandError *process.CommandError
	if errors.As(err, &commandError) {
		stderr := strings.TrimSpace(wslutil.DecodeWindowsOutput([]byte(commandError.Stderr)))
		if len(stderr) > 240 {
			stderr = stderr[:240]
		}
		if stderr != "" {
			return stderr
		}
	}
	return err.Error()
}
