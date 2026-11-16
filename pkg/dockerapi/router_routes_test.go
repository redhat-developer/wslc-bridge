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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wslc-bridge/pkg/native"
)

func guestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestServeHTTPRoutesEverySupportedRoute(t *testing.T) {
	t.Parallel()
	nativeRuntime := &fakeNativeRuntime{containers: []native.ContainerListItem{{
		ID: "container-id", State: "running",
		Ports: map[string][]native.PortBinding{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "49152"}}},
	}}}
	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/containers/json":
			return guestResponse(http.StatusOK, `[{"Id":"container-id","State":"running"}]`), nil
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/images/"):
			return guestResponse(http.StatusOK, `{"Id":"sha256:abc"}`), nil
		case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/containers/"):
			return guestResponse(http.StatusOK, `{"Id":"container-id","State":{"Running":true},"NetworkSettings":{"Ports":{}}}`), nil
		default:
			return guestResponse(http.StatusOK, `{"proxied":true}`), nil
		}
	})
	handler := NewHandler(nativeRuntime, guestClient)

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		wantStatus int
	}{
		{name: "ping", method: http.MethodGet, target: "/_ping", wantStatus: http.StatusOK},
		{name: "version", method: http.MethodGet, target: "/version", wantStatus: http.StatusOK},
		{name: "info", method: http.MethodGet, target: "/info", wantStatus: http.StatusOK},
		{name: "image list", method: http.MethodGet, target: "/images/json", wantStatus: http.StatusOK},
		{name: "network list", method: http.MethodGet, target: "/networks", wantStatus: http.StatusOK},
		{name: "volume list", method: http.MethodGet, target: "/volumes", wantStatus: http.StatusOK},
		{name: "image inspect", method: http.MethodGet, target: "/images/alpine:3.20/json", wantStatus: http.StatusOK},
		{name: "network inspect", method: http.MethodGet, target: "/networks/bridge", wantStatus: http.StatusOK},
		{name: "volume inspect", method: http.MethodGet, target: "/volumes/data", wantStatus: http.StatusOK},
		{name: "container logs", method: http.MethodGet, target: "/containers/container-id/logs", wantStatus: http.StatusOK},
		{name: "container list", method: http.MethodGet, target: "/containers/json", wantStatus: http.StatusOK},
		{name: "container inspect", method: http.MethodGet, target: "/containers/container-id/json", wantStatus: http.StatusOK},
		{name: "container create", method: http.MethodPost, target: "/containers/create?name=created", body: `{"Image":"alpine:3.20","Cmd":["sh"]}`, wantStatus: http.StatusCreated},
		{name: "container start", method: http.MethodPost, target: "/containers/container-id/start", wantStatus: http.StatusNoContent},
		{name: "container stop", method: http.MethodPost, target: "/containers/container-id/stop", wantStatus: http.StatusNoContent},
		{name: "container remove", method: http.MethodDelete, target: "/containers/container-id", wantStatus: http.StatusNoContent},
		{name: "network create", method: http.MethodPost, target: "/networks/create", body: `{"Name":"testnet"}`, wantStatus: http.StatusCreated},
		{name: "network remove", method: http.MethodDelete, target: "/networks/testnet", wantStatus: http.StatusNoContent},
		{name: "exec create", method: http.MethodPost, target: "/containers/container-id/exec", body: `{"AttachStdout":true,"Cmd":["ls"]}`, wantStatus: http.StatusCreated},
		{name: "image pull", method: http.MethodPost, target: "/images/create?fromImage=alpine:3.20", wantStatus: http.StatusOK},
		{name: "unsupported route", method: http.MethodPost, target: "/containers/container-id/restart", wantStatus: http.StatusNotImplemented},
	}
	// The subtests share one stateful fake runtime, so they run sequentially.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var body io.Reader
			if test.body != "" {
				body = strings.NewReader(test.body)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.target, body))
			if response.Code != test.wantStatus {
				t.Fatalf("%s %s returned %d, want %d: %s",
					test.method, test.target, response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestContainerListOverlaysNativePorts(t *testing.T) {
	t.Parallel()
	nativeRuntime := &fakeNativeRuntime{containers: []native.ContainerListItem{{
		ID: "container-id", State: "running",
		Ports: map[string][]native.PortBinding{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "49152"}}},
	}}}
	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		return guestResponse(http.StatusOK, `[{"Id":"container-id","State":"running"}]`), nil
	})
	handler := NewHandler(nativeRuntime, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/containers/json", nil))
	if !strings.Contains(response.Body.String(), "49152") {
		t.Fatalf("container list did not overlay native ports: %s", response.Body.String())
	}
}

func TestCreateContainerRejectsUnknownImage(t *testing.T) {
	t.Parallel()
	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		return guestResponse(http.StatusNotFound, `{"message":"no such image"}`), nil
	})
	handler := NewHandler(&fakeNativeRuntime{}, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/containers/create", strings.NewReader(`{"Image":"missing:latest","Cmd":["sh"]}`)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("create with a missing image returned %d, want 404: %s", response.Code, response.Body.String())
	}
}

func TestExecLifecycleRoutes(t *testing.T) {
	t.Parallel()
	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		return guestResponse(http.StatusOK, `{"Id":"container-id","State":{"Running":true},"NetworkSettings":{"Ports":{}}}`), nil
	})
	handler := NewHandler(&fakeNativeRuntime{}, guestClient)

	unknownInspect := httptest.NewRecorder()
	handler.ServeHTTP(unknownInspect, httptest.NewRequest(http.MethodGet, "/exec/missing/json", nil))
	if unknownInspect.Code != http.StatusNotFound {
		t.Fatalf("unknown exec inspect returned %d, want 404", unknownInspect.Code)
	}

	unknownStart := httptest.NewRecorder()
	handler.ServeHTTP(unknownStart, httptest.NewRequest(http.MethodPost, "/exec/missing/start", nil))
	if unknownStart.Code != http.StatusNotFound {
		t.Fatalf("unknown exec start returned %d, want 404", unknownStart.Code)
	}

	created := httptest.NewRecorder()
	handler.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/containers/container-id/exec", strings.NewReader(`{"AttachStdout":true,"Cmd":["ls"]}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("exec create returned %d: %s", created.Code, created.Body.String())
	}
	var creation struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &creation); err != nil {
		t.Fatal(err)
	}
	if creation.ID == "" {
		t.Fatalf("exec create returned no ID: %s", created.Body.String())
	}

	inspect := httptest.NewRecorder()
	handler.ServeHTTP(inspect, httptest.NewRequest(http.MethodGet, "/exec/"+creation.ID+"/json", nil))
	if inspect.Code != http.StatusOK {
		t.Fatalf("exec inspect returned %d: %s", inspect.Code, inspect.Body.String())
	}
	var inspection struct {
		ProcessConfig struct {
			Entrypoint string `json:"entrypoint"`
		} `json:"ProcessConfig"`
	}
	if err := json.Unmarshal(inspect.Body.Bytes(), &inspection); err != nil {
		t.Fatal(err)
	}
	if inspection.ProcessConfig.Entrypoint != "ls" {
		t.Fatalf("exec inspect entrypoint = %q, want \"ls\"", inspection.ProcessConfig.Entrypoint)
	}

	// httptest.ResponseRecorder cannot be hijacked, so a valid start reports
	// that the connection does not support Docker exec streaming.
	start := httptest.NewRecorder()
	handler.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/exec/"+creation.ID+"/start", nil))
	if start.Code != http.StatusInternalServerError {
		t.Fatalf("exec start returned %d, want 500 without a hijackable connection", start.Code)
	}

	// The failed start marks the record, so starting again conflicts.
	restart := httptest.NewRecorder()
	handler.ServeHTTP(restart, httptest.NewRequest(http.MethodPost, "/exec/"+creation.ID+"/start", nil))
	if restart.Code != http.StatusConflict {
		t.Fatalf("restarting a started exec returned %d, want 409", restart.Code)
	}
}
