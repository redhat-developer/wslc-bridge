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
	"net/http"

	"wslc-bridge/pkg/native"
)

var errUnsupportedAPIVersion = errors.New("Docker API versions older than 1.24 are not supported")

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// commandStatus maps a failed native operation to an HTTP status. WSLC error
// interpretation happens in pkg/native; this only translates the resulting
// classification.
func commandStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	var nativeError *native.Error
	if errors.As(err, &nativeError) {
		switch nativeError.Class {
		case native.ClassNotFound:
			return http.StatusNotFound
		case native.ClassConflict:
			return http.StatusConflict
		}
	}
	return http.StatusBadGateway
}
