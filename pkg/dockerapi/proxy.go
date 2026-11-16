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
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxJSONResponseBytes = 32 << 20

func (h *handler) proxy(w http.ResponseWriter, r *http.Request) {
	response, err := h.doGuest(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusBadGateway, "guest Docker API request failed: "+err.Error())
		return
	}
	defer response.Body.Close()
	copyResponseHeaders(w.Header(), response.Header)
	if r.Method == http.MethodHead {
		w.WriteHeader(response.StatusCode)
		return
	}
	w.WriteHeader(response.StatusCode)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.Copy(flushWriter{writer: w}, response.Body)
}

func (h *handler) doGuest(ctx context.Context, request *http.Request) (*http.Response, error) {
	upstream, err := http.NewRequestWithContext(ctx, request.Method, "http://docker"+request.URL.RequestURI(), request.Body)
	if err != nil {
		return nil, err
	}
	upstream.Header = request.Header.Clone()
	upstream.Host = "docker"
	upstream.ContentLength = request.ContentLength
	upstream.TransferEncoding = append([]string(nil), request.TransferEncoding...)
	return h.guest.Do(upstream)
}

func copyResponseHeaders(destination, source http.Header) {
	for key, values := range source {
		lower := strings.ToLower(key)
		if lower == "connection" || lower == "keep-alive" || lower == "proxy-authenticate" || lower == "proxy-authorization" || lower == "te" || lower == "trailer" || lower == "transfer-encoding" || lower == "upgrade" {
			continue
		}
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func readBounded(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxJSONResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxJSONResponseBytes {
		return nil, fmt.Errorf("Docker JSON response exceeds %d bytes", maxJSONResponseBytes)
	}
	return data, nil
}

type flushWriter struct {
	writer http.ResponseWriter
}

func (w flushWriter) Write(value []byte) (int, error) {
	n, err := w.writer.Write(value)
	if flusher, ok := w.writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return n, err
}
