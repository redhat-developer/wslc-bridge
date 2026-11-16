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
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wslc-bridge/pkg/native"
	"wslc-bridge/pkg/process"
)

// fakeNativeRuntime records the typed operations the handler performs.
type fakeNativeRuntime struct {
	containers      []native.ContainerListItem
	listError       error
	created         []native.ContainerSpec
	started         []string
	startError      error
	stopped         []native.StopSpec
	stopError       error
	removed         []native.RemoveSpec
	removeError     error
	pulled          []string
	createdNetworks []native.NetworkSpec
	removedNetworks []string
}

var _ native.Client = (*fakeNativeRuntime)(nil)

func (f *fakeNativeRuntime) ListContainers(context.Context) ([]native.ContainerListItem, error) {
	return f.containers, f.listError
}

func (f *fakeNativeRuntime) CreateContainer(_ context.Context, spec native.ContainerSpec) (string, error) {
	f.created = append(f.created, spec)
	return "container-id", nil
}

func (f *fakeNativeRuntime) StartContainer(_ context.Context, id string) error {
	f.started = append(f.started, id)
	return f.startError
}

func (f *fakeNativeRuntime) StopContainer(_ context.Context, spec native.StopSpec) error {
	f.stopped = append(f.stopped, spec)
	return f.stopError
}

func (f *fakeNativeRuntime) RemoveContainer(_ context.Context, spec native.RemoveSpec) error {
	f.removed = append(f.removed, spec)
	return f.removeError
}

func (f *fakeNativeRuntime) PullImage(_ context.Context, reference string) error {
	f.pulled = append(f.pulled, reference)
	return nil
}

func (f *fakeNativeRuntime) ExecContainer(context.Context, string, []string, io.Writer, io.Writer) (int, error) {
	return 0, nil
}

func (f *fakeNativeRuntime) CreateNetwork(_ context.Context, spec native.NetworkSpec) (string, error) {
	f.createdNetworks = append(f.createdNetworks, spec)
	return "network-id", nil
}

func (f *fakeNativeRuntime) RemoveNetwork(_ context.Context, name string, _ bool) error {
	f.removedNetworks = append(f.removedNetworks, name)
	return nil
}

// nativeClassError builds a classified native error for handler tests.
func nativeClassError(class native.ErrorClass) error {
	return &native.Error{Class: class, Cause: &process.CommandError{Name: "wslc.exe", ExitCode: 1, Stderr: "command failed"}}
}

func TestInspectContainerUsesGuestBodyAndWSLCListPorts(t *testing.T) {
	t.Parallel()

	guestBody := `{"Id":"container-id","Config":{"Image":"nginx:alpine"},"State":{"Running":true},"NetworkSettings":{"Ports":{"80/tcp":[{"HostIp":"172.18.0.1","HostPort":"8080"}]}}}`
	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/containers/container-id/json" {
			return nil, errors.New("unexpected guest request path: " + request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(guestBody))}, nil
	})
	nativeRuntime := &fakeNativeRuntime{containers: []native.ContainerListItem{{
		ID: "container-id", State: "running",
		Ports: map[string][]native.PortBinding{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "49152"}}},
	}}}
	handler := NewHandler(nativeRuntime, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/containers/container-id/json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("inspect returned status %d: %s", response.Code, response.Body.String())
	}
	var inspect struct {
		Config struct {
			Image string `json:"Image"`
		} `json:"Config"`
		NetworkSettings struct {
			Ports map[string][]native.PortBinding `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &inspect); err != nil {
		t.Fatal(err)
	}
	if inspect.Config.Image != "nginx:alpine" {
		t.Fatalf("guest inspect fields were not preserved: %#v", inspect.Config)
	}
	if binding := inspect.NetworkSettings.Ports["80/tcp"][0]; binding.HostIP != "127.0.0.1" || binding.HostPort != "49152" {
		t.Fatalf("native Windows port was not overlaid: %#v", binding)
	}
}

func TestInspectContainerFallsBackToGuestWhenWSLCListFails(t *testing.T) {
	t.Parallel()

	guestBody := `{"Id":"container-id","NetworkSettings":{"Ports":{"80/tcp":[{"HostIp":"172.18.0.1","HostPort":"8080"}]}}}`
	guestClient := fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(guestBody))}, nil
	})
	handler := NewHandler(&fakeNativeRuntime{listError: errors.New("native list unavailable")}, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/containers/container-id/json", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("guest inspect should still be served, got status %d: %s", response.Code, response.Body.String())
	}
	var inspect struct {
		NetworkSettings struct {
			Ports map[string][]native.PortBinding `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &inspect); err != nil {
		t.Fatal(err)
	}
	if binding := inspect.NetworkSettings.Ports["80/tcp"][0]; binding.HostIP != "" || binding.HostPort != "" {
		t.Fatalf("guest VM port was incorrectly exposed as a Windows host port: %#v", binding)
	}
}

func TestContainerStartUsesNativeStartDirectlyWithoutInspect(t *testing.T) {
	t.Parallel()

	nativeRuntime := &fakeNativeRuntime{}
	guestClient := fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("start must not make a guest inspect request")
	})
	handler := NewHandler(nativeRuntime, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/containers/container-id/start", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("container start returned %d: %s", response.Code, response.Body.String())
	}
	if len(nativeRuntime.started) != 1 || nativeRuntime.started[0] != "container-id" {
		t.Fatalf("native start IDs = %q, want [container-id]", nativeRuntime.started)
	}
}

func TestContainerLifecycleMapsClassifiedErrorsToDockerStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		target     string
		startError error
		stopError  error
		wantStatus int
	}{
		{name: "start already running", method: http.MethodPost, target: "/containers/container-id/start", startError: nativeClassError(native.ClassAlreadyRunning), wantStatus: http.StatusNoContent},
		{name: "start conflict", method: http.MethodPost, target: "/containers/container-id/start", startError: nativeClassError(native.ClassConflict), wantStatus: http.StatusConflict},
		{name: "start unknown", method: http.MethodPost, target: "/containers/container-id/start", startError: nativeClassError(native.ClassUnknown), wantStatus: http.StatusBadGateway},
		{name: "stop not running", method: http.MethodPost, target: "/containers/container-id/stop", stopError: nativeClassError(native.ClassNotRunning), wantStatus: http.StatusNoContent},
		{name: "stop conflict", method: http.MethodPost, target: "/containers/container-id/stop", stopError: nativeClassError(native.ClassConflict), wantStatus: http.StatusConflict},
		{name: "remove not found", method: http.MethodDelete, target: "/containers/container-id", wantStatus: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			nativeRuntime := &fakeNativeRuntime{startError: test.startError, stopError: test.stopError}
			if test.name == "remove not found" {
				nativeRuntime.removeError = nativeClassError(native.ClassNotFound)
			}
			handler := NewHandler(nativeRuntime, fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
				return nil, errors.New("lifecycle must not make a guest request")
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.target, nil))
			if response.Code != test.wantStatus {
				t.Fatalf("%s returned %d, want %d: %s", test.target, response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestContainerStopPassesSignalAndTimeoutSpec(t *testing.T) {
	t.Parallel()

	nativeRuntime := &fakeNativeRuntime{}
	handler := NewHandler(nativeRuntime, fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("stop must not make a guest request")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/containers/container-id/stop?signal=SIGKILL&t=10", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("container stop returned %d: %s", response.Code, response.Body.String())
	}
	if len(nativeRuntime.stopped) != 1 {
		t.Fatalf("native stop calls = %d, want 1", len(nativeRuntime.stopped))
	}
	spec := nativeRuntime.stopped[0]
	if spec.ID != "container-id" || spec.Signal != "SIGKILL" || spec.TimeoutSeconds == nil || *spec.TimeoutSeconds != 10 {
		t.Fatalf("stop spec = %#v, want container-id with SIGKILL and a 10s timeout", spec)
	}
}

func TestContainerRemovePassesForceSpec(t *testing.T) {
	t.Parallel()

	nativeRuntime := &fakeNativeRuntime{}
	handler := NewHandler(nativeRuntime, fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("remove must not make a guest request")
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/containers/container-id?force=1", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("container remove returned %d: %s", response.Code, response.Body.String())
	}
	if len(nativeRuntime.removed) != 1 || nativeRuntime.removed[0].ID != "container-id" || !nativeRuntime.removed[0].Force {
		t.Fatalf("remove specs = %#v, want container-id with force", nativeRuntime.removed)
	}
}
