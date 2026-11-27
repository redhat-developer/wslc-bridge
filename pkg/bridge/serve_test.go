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
	"strings"
	"testing"
	"time"

	"wslc-bridge/pkg/native"
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

// execNative is a native.Client whose only supported call is ExecContainer.
type execNative struct {
	native.Client
	started   chan struct{}
	cancelled chan struct{}
}

func (e *execNative) ExecContainer(ctx context.Context, _ string, _ []string, _, _ io.Writer) (int, error) {
	e.started <- struct{}{}
	<-ctx.Done()
	e.cancelled <- struct{}{}
	return -1, ctx.Err()
}

func TestServeShutdownCancelsAndWaitsForHijackedExecs(t *testing.T) {
	t.Parallel()

	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/version" {
			return guestResponse(http.StatusOK, `{"ApiVersion":"1.47"}`)(request)
		}
		return guestResponse(http.StatusOK, `{"Id":"container-id","State":{"Running":true}}`)(request)
	})
	deps := fakeDependencies(t, healthyRunner(), guestClient)
	exec := &execNative{started: make(chan struct{}, 1), cancelled: make(chan struct{}, 1)}
	deps.newNativeClient = func(string, string) native.Client { return exec }
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deps.listenPipe = func(string) (net.Listener, error) { return listener, nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- deps.serve(ctx, testPipe) }()

	base := "http://" + listener.Addr().String()
	var created *http.Response
	for deadline := time.Now().Add(15 * time.Second); ; {
		created, err = http.Post(base+"/containers/container-id/exec", "application/json",
			strings.NewReader(`{"AttachStdout":true,"Cmd":["sleep","60"]}`))
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	var creation struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&creation); err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("POST /exec/" + creation.ID + "/start HTTP/1.1\r\nHost: docker\r\nContent-Length: 0\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exec.started:
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for the exec to start")
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
	select {
	case <-exec.cancelled:
	default:
		t.Fatal("serve returned before the hijacked exec was cancelled")
	}
}
