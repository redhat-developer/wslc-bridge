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

package dockerapi

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wslc-bridge/pkg/execstore"
	"wslc-bridge/pkg/process"
)

// blockingExec returns an execFunc that signals started, then blocks until
// its context is cancelled.
func blockingExec(started chan<- struct{}, cancelled chan<- struct{}) func(context.Context, string, []string, io.Writer, io.Writer) (int, error) {
	return func(ctx context.Context, _ string, _ []string, _, _ io.Writer) (int, error) {
		started <- struct{}{}
		<-ctx.Done()
		cancelled <- struct{}{}
		return -1, &process.CommandError{Name: "wslc.exe", ExitCode: -1}
	}
}

func runningGuest() fakeGuestClient {
	return fakeGuestClient(func(*http.Request) (*http.Response, error) {
		return guestResponse(http.StatusOK, `{"Id":"container-id","State":{"Running":true}}`), nil
	})
}

func createExecInstance(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/containers/container-id/exec", strings.NewReader(`{"AttachStdout":true,"Cmd":["sleep","60"]}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("exec create returned %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	start := strings.Index(body, `"Id":"`) + len(`"Id":"`)
	return body[start : start+strings.Index(body[start:], `"`)]
}

// startHijackedExec serves handler over a real listener and starts the exec,
// returning the client side of the hijacked connection once the exec runs.
func startHijackedExec(t *testing.T, handler http.Handler, id string) net.Conn {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	request := "POST /exec/" + id + "/start HTTP/1.1\r\nHost: docker\r\nContent-Length: 0\r\n\r\n"
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.Contains(status, "101") {
		t.Fatalf("exec start status = %q, %v; want 101", status, err)
	}
	return conn
}

func waitSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestExecIsCancelledWhenTheHijackedConnectionCloses(t *testing.T) {
	t.Parallel()

	started, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	handler := NewHandler(&fakeNativeRuntime{execFunc: blockingExec(started, cancelled)}, runningGuest())
	conn := startHijackedExec(t, handler, createExecInstance(t, handler))
	waitSignal(t, started, "the exec to start")

	_ = conn.Close()
	waitSignal(t, cancelled, "the exec to be cancelled")
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerShutdownCancelsAndWaitsForRunningExecs(t *testing.T) {
	t.Parallel()

	started, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	handler := NewHandler(&fakeNativeRuntime{execFunc: blockingExec(started, cancelled)}, runningGuest())
	startHijackedExec(t, handler, createExecInstance(t, handler))
	waitSignal(t, started, "the exec to start")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := handler.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() = %v, want nil", err)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("Shutdown returned before the exec was cancelled")
	}
	if err := handler.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() = %v, want nil", err)
	}
}

func TestExecStartAfterShutdownIsRejected(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&fakeNativeRuntime{}, runningGuest())
	id := createExecInstance(t, handler)
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/exec/"+id+"/start", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("exec start after shutdown returned %d, want 503", response.Code)
	}
	if record, _ := handler.execs.Get(id); record.State != execstore.Created {
		t.Fatalf("exec record state = %v, want it untouched", record.State)
	}
}

func TestHandlerShutdownTimesOutWhenAnExecIgnoresCancellation(t *testing.T) {
	t.Parallel()

	started, release := make(chan struct{}, 1), make(chan struct{})
	handler := NewHandler(&fakeNativeRuntime{execFunc: func(context.Context, string, []string, io.Writer, io.Writer) (int, error) {
		started <- struct{}{}
		<-release
		return 0, nil
	}}, runningGuest())
	startHijackedExec(t, handler, createExecInstance(t, handler))
	waitSignal(t, started, "the exec to start")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := handler.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown() = %v, want context.DeadlineExceeded", err)
	}
	close(release)
	if err := handler.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() after release = %v, want nil", err)
	}
}
