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
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

type DoctorResult struct {
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	WSLCVersion     string `json:"wslcVersion,omitempty"`
	WSLVersion      string `json:"wslVersion,omitempty"`
	GuestVersion    string `json:"guestVersion,omitempty"`
	GuestAPIVersion string `json:"guestApiVersion,omitempty"`
	Error           string `json:"error,omitempty"`
	ExitCode        int    `json:"-"`
}

func Doctor(ctx context.Context) DoctorResult {
	return defaultDependencies().doctor(ctx)
}

func (d dependencies) doctor(ctx context.Context) DoctorResult {
	result := DoctorResult{OS: runtime.GOOS, Arch: runtime.GOARCH}
	info, err := d.detectRuntime(ctx)
	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 4
		if exitError, ok := err.(*ExitError); ok {
			result.ExitCode = exitError.Code
		}
		return result
	}
	result.WSLCVersion = info.WSLCVersion
	result.WSLVersion = info.WSLVersion
	client := d.newGuestClient(info.WSLCPath)
	defer client.Close()
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://docker/version", nil)
	if err != nil {
		result.Error = err.Error()
		result.ExitCode = 6
		return result
	}
	response, err := client.Do(request)
	if err != nil {
		result.Error = "guest Docker API unavailable: " + err.Error()
		result.ExitCode = 6
		return result
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.Error = "guest Docker API returned " + response.Status
		result.ExitCode = 6
		return result
	}
	var version struct {
		Version    string `json:"Version"`
		APIVersion string `json:"ApiVersion"`
	}
	if err := json.NewDecoder(response.Body).Decode(&version); err != nil {
		result.Error = "could not decode guest Docker version response: " + err.Error()
		result.ExitCode = 6
		return result
	}
	result.GuestVersion = version.Version
	result.GuestAPIVersion = version.APIVersion
	return result
}
