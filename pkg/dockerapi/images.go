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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type registryAuth struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Auth          string `json:"auth"`
	IdentityToken string `json:"identitytoken"`
}

func (h *Handler) pullImage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("fromSrc") != "" || query.Get("repo") != "" || query.Get("platform") != "" || query.Get("all") == "1" {
		writeError(w, http.StatusNotImplemented, "this image pull option is not supported by WSLC v1")
		return
	}
	if hasRegistryCredentials(r.Header.Get("X-Registry-Auth")) {
		writeError(w, http.StatusNotImplemented, "authenticated private-registry pulls are not supported by WSLC v1")
		return
	}
	reference := strings.TrimSpace(query.Get("fromImage"))
	if reference == "" {
		writeError(w, http.StatusBadRequest, "fromImage query parameter is required")
		return
	}
	if tag := query.Get("tag"); tag != "" && !strings.Contains(reference, "@") {
		lastSlash := strings.LastIndex(reference, "/")
		if strings.LastIndex(reference, ":") <= lastSlash {
			reference += ":" + tag
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	// Use one encoder per record so each Docker progress object is flushed immediately.
	writer := bufio.NewWriter(w)
	if err := writeProgress(writer, flusher, map[string]string{"status": fmt.Sprintf("Pulling %s", reference)}); err != nil {
		return
	}
	err := h.native.PullImage(r.Context(), reference)
	if err != nil {
		message := fmt.Sprintf("wslc image pull failed: %s", err.Error())
		_ = writeProgress(writer, flusher, map[string]any{"error": message, "errorDetail": map[string]string{"message": message}})
		return
	}
	_ = writeProgress(writer, flusher, map[string]any{"status": fmt.Sprintf("Downloaded %s", reference), "progressDetail": map[string]int{}})
}

func writeProgress(writer *bufio.Writer, flusher http.Flusher, record any) error {
	if err := json.NewEncoder(writer).Encode(record); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func hasRegistryCredentials(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return true
	}
	var auth registryAuth
	if err := json.Unmarshal(decoded, &auth); err != nil {
		return true
	}
	return auth.Username != "" || auth.Password != "" || auth.Auth != "" || auth.IdentityToken != ""
}
