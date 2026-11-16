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
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type orderedResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	events *[]string
}

func (w *orderedResponseWriter) Header() http.Header { return w.header }

func (w *orderedResponseWriter) WriteHeader(status int) {
	*w.events = append(*w.events, "status")
}

func (w *orderedResponseWriter) Write(body []byte) (int, error) {
	*w.events = append(*w.events, "write")
	return w.body.Write(body)
}

func (w *orderedResponseWriter) Flush() {
	*w.events = append(*w.events, "flush")
}

type orderedBody struct {
	readOnce bool
	events   *[]string
}

func (b *orderedBody) Read(target []byte) (int, error) {
	*b.events = append(*b.events, "read")
	if b.readOnce {
		return 0, io.EOF
	}
	b.readOnce = true
	return copy(target, []byte("event")), nil
}

func (b *orderedBody) Close() error { return nil }

// fakeGuestClient answers every guest request with a function; it only
// implements the Do method the handler depends on.
type fakeGuestClient func(*http.Request) (*http.Response, error)

var _ guestAPI = fakeGuestClient(nil)

func (f fakeGuestClient) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestPingIsProxiedOverGuestTransport(t *testing.T) {
	t.Parallel()

	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/_ping" || request.Method != http.MethodGet {
			t.Fatalf("unexpected guest request: %s %s", request.Method, request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader("OK"))}, nil
	})
	handler := NewHandler(&fakeNativeRuntime{}, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/_ping", nil))
	if response.Code != http.StatusOK || response.Body.String() != "OK" {
		t.Fatalf("unexpected ping response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestVersionedPingPreservesSupportedAPIPrefixForGuest(t *testing.T) {
	t.Parallel()

	guestClient := fakeGuestClient(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1.44/_ping" {
			t.Fatalf("version prefix was not forwarded: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("OK"))}, nil
	})
	handler := NewHandler(&fakeNativeRuntime{}, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1.44/_ping", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("versioned ping returned %d", response.Code)
	}
}

func TestUnsupportedAPIVersionDoesNotReachGuest(t *testing.T) {
	t.Parallel()

	guestClient := fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("unsupported API version reached guest")
		return nil, nil
	})
	handler := NewHandler(&fakeNativeRuntime{}, guestClient)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1.23/_ping", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsupported version returned %d, want 400", response.Code)
	}
}

func TestStreamingReadFlushesHeadersBeforeWaitingForFirstPayload(t *testing.T) {
	t.Parallel()

	events := make([]string, 0, 8)
	guestClient := fakeGuestClient(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       &orderedBody{events: &events},
		}, nil
	})
	handler := NewHandler(&fakeNativeRuntime{}, guestClient)
	writer := &orderedResponseWriter{header: make(http.Header), events: &events}
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/events", nil))

	flushIndex := -1
	readIndex := -1
	for index, event := range events {
		if event == "flush" && flushIndex == -1 {
			flushIndex = index
		}
		if event == "read" {
			readIndex = index
			break
		}
	}
	if flushIndex == -1 || readIndex == -1 || flushIndex > readIndex {
		t.Fatalf("stream response did not flush headers before reading first payload: %v", events)
	}
}
