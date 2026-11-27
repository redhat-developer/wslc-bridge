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
	"net/http"
	"strings"

	"wslc-bridge/pkg/native"
)

type networkCreateRequest struct {
	Name           string            `json:"Name"`
	CheckDuplicate bool              `json:"CheckDuplicate"`
	Driver         string            `json:"Driver"`
	Internal       bool              `json:"Internal"`
	Attachable     bool              `json:"Attachable"`
	Ingress        bool              `json:"Ingress"`
	EnableIPv6     bool              `json:"EnableIPv6"`
	Options        map[string]string `json:"Options"`
	Labels         map[string]string `json:"Labels"`
	IPAM           struct {
		Driver  string            `json:"Driver"`
		Config  []json.RawMessage `json:"Config"`
		Options map[string]string `json:"Options"`
	} `json:"IPAM"`
}

func (h *Handler) createNetwork(w http.ResponseWriter, r *http.Request) {
	var request networkCreateRequest
	if err := decodeRequest(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(request.Name) == "" || !validNamePattern.MatchString(request.Name) {
		writeError(w, http.StatusBadRequest, "a valid network Name is required")
		return
	}
	if request.Internal || request.Attachable || request.Ingress || request.EnableIPv6 || request.IPAM.Driver != "" || len(request.IPAM.Config) > 0 || len(request.IPAM.Options) > 0 {
		writeError(w, http.StatusNotImplemented, "requested network IPAM or mode options are not supported by WSLC 2.9.3")
		return
	}
	spec := native.NetworkSpec{
		Name:    request.Name,
		Driver:  request.Driver,
		Options: request.Options,
		Labels:  request.Labels,
	}
	id, err := h.native.CreateNetwork(r.Context(), spec)
	if err != nil {
		writeError(w, commandStatus(err), "wslc network create failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"Id": id, "Warning": ""})
}

func (h *Handler) removeNetwork(w http.ResponseWriter, r *http.Request, path string) {
	name := strings.TrimPrefix(path, "/networks/")
	force := r.URL.Query().Get("force") == "1" || strings.EqualFold(r.URL.Query().Get("force"), "true")
	if err := h.native.RemoveNetwork(r.Context(), name, force); err != nil {
		writeError(w, commandStatus(err), "wslc network remove failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
