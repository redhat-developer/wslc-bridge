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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

const testPipe = `\\.\pipe\wslc_bridge_test`

func TestServeReportsStartupFailures(t *testing.T) {
	t.Parallel()

	healthyGuest := guestResponse(http.StatusOK, `{"ApiVersion":"1.47"}`)
	tests := []struct {
		name        string
		goos        string
		guestClient fakeGuestClient
		listenErr   error
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
			wantMessage: `guest Docker API is unavailable: Get "http://docker/version": dial failed`,
		},
		{
			name:        "guest API error status",
			guestClient: guestResponse(http.StatusServiceUnavailable, ""),
			wantCode:    6,
			wantMessage: "guest Docker API returned Service Unavailable",
		},
		{
			name:        "guest API invalid JSON",
			guestClient: guestResponse(http.StatusOK, "not-json"),
			wantCode:    6,
			wantMessage: "decode guest Docker version",
		},
		{
			name:        "named pipe unavailable",
			guestClient: healthyGuest,
			listenErr:   errors.New("pipe busy"),
			wantCode:    5,
			wantMessage: "pipe busy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deps := fakeDependencies(t, healthyRunner(), test.guestClient)
			if test.goos != "" {
				deps.goos = test.goos
			}
			if test.listenErr != nil {
				deps.listenPipe = func(string) (net.Listener, error) { return nil, test.listenErr }
			}
			err := deps.serve(context.Background(), testPipe)
			assertExit(t, err, test.wantCode, test.wantMessage)
		})
	}
}

func TestServeEmitsReadinessEventProxiesAndStopsCleanly(t *testing.T) {
	t.Parallel()

	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/version":
			return guestResponse(http.StatusOK, `{"ApiVersion":"1.47"}`)(request)
		case "/_ping":
			return guestResponse(http.StatusOK, "OK")(request)
		}
		return guestResponse(http.StatusNotFound, "")(request)
	})
	deps := fakeDependencies(t, healthyRunner(), guestClient)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var listenedPipe string
	deps.listenPipe = func(name string) (net.Listener, error) {
		listenedPipe = name
		return listener, nil
	}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close() })
	deps.stdout = writer

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- deps.serve(ctx, testPipe) }()

	lines := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(reader)
		if scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	var event readinessEvent
	select {
	case line := <-lines:
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("readiness line %q is not JSON: %v", line, err)
		}
	case err := <-result:
		t.Fatalf("serve() returned before the readiness event: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for the readiness event")
	}
	if event.Event != "ready" || event.ProtocolVersion != 1 || event.Pipe != testPipe || event.GuestAPIVersion != "1.47" {
		t.Fatalf("unexpected readiness event: %#v", event)
	}
	if listenedPipe != testPipe {
		t.Fatalf("listenPipe(%q), want %q", listenedPipe, testPipe)
	}

	response, err := http.Get("http://" + listener.Addr().String() + "/_ping")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "OK" {
		t.Fatalf("/_ping = %d %q, want 200 \"OK\"", response.StatusCode, body)
	}

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve() = %v, want nil after shutdown", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for serve to shut down")
	}
}
