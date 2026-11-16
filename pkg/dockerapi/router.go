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
	"net/http"
	"strconv"
	"strings"

	"wslc-bridge/pkg/execstore"
	"wslc-bridge/pkg/native"
)

// guestAPI is the subset of the guest Docker client the handler needs.
// Keeping it next to the consumer means fakes only implement Do.
type guestAPI interface {
	Do(request *http.Request) (*http.Response, error)
}

type handler struct {
	native native.Client
	guest  guestAPI
	execs  *execstore.Store
}

// NewHandler returns the Docker API HTTP handler backed by the native WSLC
// client and the guest Docker API client.
func NewHandler(nativeClient native.Client, guestClient guestAPI) http.Handler {
	return &handler{native: nativeClient, guest: guestClient, execs: execstore.NewStore()}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	apiPath, err := stripVersionPrefix(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	path := apiPath
	switch {
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && path == "/_ping":
		h.proxy(w, r)
	case r.Method == http.MethodGet && isDirectReadRoute(path):
		h.proxy(w, r)
	case r.Method == http.MethodGet && path == "/containers/json":
		h.listContainers(w, r)
	case r.Method == http.MethodGet && containerRoute(path, "json"):
		h.inspectContainer(w, r, path)
	case r.Method == http.MethodPost && path == "/containers/create":
		h.createContainer(w, r)
	case r.Method == http.MethodPost && containerActionRoute(path, "start"):
		h.containerStart(w, r, path)
	case r.Method == http.MethodPost && containerActionRoute(path, "stop"):
		h.containerStop(w, r, path)
	case r.Method == http.MethodDelete && containerItemRoute(path):
		h.containerRemove(w, r, path)
	case r.Method == http.MethodPost && path == "/networks/create":
		h.createNetwork(w, r)
	case r.Method == http.MethodDelete && networkItemRoute(path):
		h.removeNetwork(w, r, path)
	case r.Method == http.MethodPost && containerActionRoute(path, "exec"):
		h.createExec(w, r, path)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/start"):
		h.startExec(w, r, path)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/exec/") && strings.HasSuffix(path, "/json"):
		h.inspectExec(w, r, path)
	case r.Method == http.MethodPost && path == "/images/create":
		h.pullImage(w, r)
	case r.Method == http.MethodGet && isStreamReadRoute(path):
		h.proxy(w, r)
	default:
		writeError(w, http.StatusNotImplemented, "Docker API route is not implemented by wslc-bridge")
	}
}

func stripVersionPrefix(path string) (string, error) {
	if !strings.HasPrefix(path, "/v1.") {
		return path, nil
	}
	separator := strings.Index(path[1:], "/")
	if separator < 0 {
		return "", errUnsupportedAPIVersion
	}
	separator++
	version := strings.TrimPrefix(path[1:separator], "v")
	parts := strings.SplitN(version, ".", 2)
	if len(parts) != 2 {
		return "", errUnsupportedAPIVersion
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major != 1 || minor < 24 {
		return "", errUnsupportedAPIVersion
	}
	return path[separator:], nil
}

func isDirectReadRoute(path string) bool {
	switch path {
	case "/version", "/info", "/events", "/images/json", "/networks", "/volumes", "/volumes/":
		return true
	}
	return strings.HasPrefix(path, "/volumes/") || imageInspectRoute(path) || networkInspectRoute(path)
}

func isStreamReadRoute(path string) bool {
	return strings.HasPrefix(path, "/containers/") && (strings.HasSuffix(path, "/logs") || strings.HasSuffix(path, "/stats"))
}

func imageInspectRoute(path string) bool {
	return strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json") && path != "/images/json"
}

func networkInspectRoute(path string) bool {
	return strings.HasPrefix(path, "/networks/") && len(strings.TrimPrefix(path, "/networks/")) > 0
}

func networkItemRoute(path string) bool {
	if !strings.HasPrefix(path, "/networks/") || path == "/networks/create" {
		return false
	}
	resource := strings.TrimPrefix(path, "/networks/")
	return resource != "" && !strings.Contains(resource, "/")
}

func containerRoute(path, suffix string) bool {
	if !strings.HasPrefix(path, "/containers/") || !strings.HasSuffix(path, "/"+suffix) {
		return false
	}
	return strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/"+suffix) != ""
}

func containerActionRoute(path, action string) bool {
	return containerRoute(path, action)
}

func containerItemRoute(path string) bool {
	if !strings.HasPrefix(path, "/containers/") || path == "/containers/json" {
		return false
	}
	resource := strings.TrimPrefix(path, "/containers/")
	return resource != "" && !strings.Contains(resource, "/")
}

func execIDFromPath(path, suffix string) string {
	return strings.TrimSuffix(strings.TrimPrefix(path, "/exec/"), "/"+suffix)
}

var _ http.Handler = (*handler)(nil)
