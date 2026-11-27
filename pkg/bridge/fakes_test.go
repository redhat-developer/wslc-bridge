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
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"wslc-bridge/pkg/guest"
	"wslc-bridge/pkg/native"
	"wslc-bridge/pkg/process"
)

const (
	fakeWSLCPath = `C:\fake\wslc.exe`
	fakeWSLPath  = `C:\fake\wsl.exe`
)

// fakeRunner is a process.Runner that answers the runtime probes.
type fakeRunner struct {
	wslcResult process.Result
	wslcErr    error
	wslResult  process.Result
	wslErr     error
}

var _ process.Runner = (*fakeRunner)(nil)

func (f *fakeRunner) RunWSLC(_ context.Context, args ...string) (process.Result, error) {
	if strings.Join(args, " ") != "version" {
		return process.Result{}, errors.New("unexpected wslc.exe arguments: " + strings.Join(args, " "))
	}
	return f.wslcResult, f.wslcErr
}

func (f *fakeRunner) RunWSL(_ context.Context, args ...string) (process.Result, error) {
	if strings.Join(args, " ") != "--version" {
		return process.Result{}, errors.New("unexpected wsl.exe arguments: " + strings.Join(args, " "))
	}
	return f.wslResult, f.wslErr
}

func (f *fakeRunner) RunWSLCStreaming(context.Context, io.Writer, io.Writer, ...string) (int, error) {
	return -1, errors.New("unexpected streaming call")
}

// fakeGuestClient answers every guest request with a function.
type fakeGuestClient func(*http.Request) (*http.Response, error)

// roundTripperFunc adapts a function to an http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// newFakeGuestClient returns a *guest.Client whose transport answers every
// request with the function.
func newFakeGuestClient(do fakeGuestClient) *guest.Client {
	return &guest.Client{Client: &http.Client{Transport: roundTripperFunc(do)}}
}

func guestResponse(status int, body string) fakeGuestClient {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}
}

// healthyRunner reports WSLC 1.2.3 and the minimum supported WSL version.
func healthyRunner() *fakeRunner {
	return &fakeRunner{
		wslcResult: process.Result{Stdout: []byte("1.2.3\n")},
		wslResult:  process.Result{Stdout: []byte("WSL version: 2.9.3.0\nKernel version: 6.6.87\n")},
	}
}

// fakeDependencies returns dependencies for a healthy Windows host whose
// guest Docker API answers with guestClient. Nothing touches the real system.
func fakeDependencies(t *testing.T, runner *fakeRunner, guestClient fakeGuestClient) dependencies {
	t.Helper()
	return dependencies{
		goos: "windows",
		lookPath: func(file string) (string, error) {
			switch file {
			case "wslc.exe":
				return fakeWSLCPath, nil
			case "wsl.exe":
				return fakeWSLPath, nil
			}
			return "", exec.ErrNotFound
		},
		newRunner: func(wslcPath, wslPath string) process.Runner {
			if wslcPath != fakeWSLCPath || wslPath != fakeWSLPath {
				t.Errorf("newRunner(%q, %q), want the detected paths", wslcPath, wslPath)
			}
			return runner
		},
		newGuestClient: func(wslcPath string) *guest.Client {
			if wslcPath != fakeWSLCPath {
				t.Errorf("newGuestClient(%q), want %q", wslcPath, fakeWSLCPath)
			}
			return newFakeGuestClient(guestClient)
		},
		newNativeClient: func(wslcPath, wslPath string) native.Client {
			if wslcPath != fakeWSLCPath || wslPath != fakeWSLPath {
				t.Errorf("newNativeClient(%q, %q), want the detected paths", wslcPath, wslPath)
			}
			return native.NewClient(wslcPath, wslPath)
		},
		listenPipe: func(string) (net.Listener, error) {
			return nil, errors.New("listenPipe not configured")
		},
		stdout: io.Discard,
	}
}

func assertExit(t *testing.T, err error, code int, messagePart string) {
	t.Helper()
	var exitError *ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("error = %v, want *ExitError", err)
	}
	if exitError.Code != code {
		t.Fatalf("ExitError = %q, want code %d", exitError.Error(), code)
	}
	if messagePart != "" && !strings.Contains(exitError.Message, messagePart) {
		t.Fatalf("ExitError.Message = %q, want it to contain %q", exitError.Message, messagePart)
	}
}
