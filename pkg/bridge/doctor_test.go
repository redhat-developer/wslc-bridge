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
	"net/http"
	"strings"
	"testing"
)

func TestDoctorReportsHealthyRuntime(t *testing.T) {
	t.Parallel()

	var requested string
	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		requested = request.Method + " " + request.URL.String()
		return guestResponse(http.StatusOK, `{"Version":"27.0.0","ApiVersion":"1.47"}`)(request)
	})
	result := fakeDependencies(t, healthyRunner(), guestClient).doctor(context.Background())
	if result.Error != "" || result.ExitCode != 0 {
		t.Fatalf("doctor() = %#v, want a healthy result", result)
	}
	if requested != "GET http://docker/version" {
		t.Fatalf("guest request = %q, want GET http://docker/version", requested)
	}
	if result.WSLCVersion != "1.2.3" || result.WSLVersion != "2.9.3" ||
		result.GuestVersion != "27.0.0" || result.GuestAPIVersion != "1.47" {
		t.Fatalf("doctor() versions = %#v", result)
	}
}

func TestDoctorReportsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		goos        string
		guestClient fakeGuestClient
		wantCode    int
		wantMessage string
	}{
		{name: "runtime unavailable", goos: "linux", wantCode: 4, wantMessage: "only runs on Windows"},
		{
			name: "guest API unreachable",
			guestClient: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			},
			wantCode:    6,
			wantMessage: `unavailable: Get "http://docker/version": dial failed`,
		},
		{
			name:        "guest API error status",
			guestClient: guestResponse(http.StatusInternalServerError, ""),
			wantCode:    6,
			wantMessage: "guest Docker API returned Internal Server Error",
		},
		{
			name:        "guest API invalid JSON",
			guestClient: guestResponse(http.StatusOK, "not-json"),
			wantCode:    6,
			wantMessage: "could not decode guest Docker version response",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deps := fakeDependencies(t, healthyRunner(), test.guestClient)
			if test.goos != "" {
				deps.goos = test.goos
			}
			result := deps.doctor(context.Background())
			if result.ExitCode != test.wantCode || !strings.Contains(result.Error, test.wantMessage) {
				t.Fatalf("doctor() = %#v, want exit code %d and an error containing %q", result, test.wantCode, test.wantMessage)
			}
		})
	}
}
