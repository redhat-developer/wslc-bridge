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
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wslc-bridge/pkg/native"
)

func (h *Handler) containerStart(w http.ResponseWriter, r *http.Request, path string) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/start")
	if err := h.native.StartContainer(r.Context(), id); err != nil {
		var nativeError *native.Error
		if errors.As(err, &nativeError) && nativeError.Class == native.ClassAlreadyRunning {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, commandStatus(err), "wslc container start failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) containerStop(w http.ResponseWriter, r *http.Request, path string) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/stop")
	query := r.URL.Query()
	spec := native.StopSpec{ID: id}
	if signal := query.Get("signal"); signal != "" {
		spec.Signal = signal
	}
	if timeout := query.Get("t"); timeout != "" {
		seconds, err := strconv.Atoi(timeout)
		if err != nil || seconds < -1 {
			writeError(w, http.StatusBadRequest, "stop timeout must be -1 or a non-negative number of seconds")
			return
		}
		spec.TimeoutSeconds = &seconds
	}
	if err := h.native.StopContainer(r.Context(), spec); err != nil {
		var nativeError *native.Error
		if errors.As(err, &nativeError) && nativeError.Class == native.ClassNotRunning {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, commandStatus(err), "wslc container stop failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) containerRemove(w http.ResponseWriter, r *http.Request, path string) {
	id := strings.TrimPrefix(path, "/containers/")
	query := r.URL.Query()
	if query.Get("v") == "1" || strings.EqualFold(query.Get("v"), "true") {
		writeError(w, http.StatusNotImplemented, "removing associated volumes is not supported by WSLC v1")
		return
	}
	force := query.Get("force") == "1" || strings.EqualFold(query.Get("force"), "true")
	if err := h.native.RemoveContainer(r.Context(), native.RemoveSpec{ID: id, Force: force}); err != nil {
		writeError(w, commandStatus(err), "wslc container remove failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
