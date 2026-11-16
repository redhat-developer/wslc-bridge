//go:build windows

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

package platform

import (
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func ListenPipe(name string) (net.Listener, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, fmt.Errorf("open current Windows process token: %w", err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("get current Windows user SID: %w", err)
	}
	sid := user.User.Sid.String()
	securityDescriptor := fmt.Sprintf("D:P(A;;GA;;;%s)", sid)
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: securityDescriptor,
		MessageMode:        false,
	})
	if err != nil {
		return nil, fmt.Errorf("listen on named pipe %q: %w", name, err)
	}
	return listener, nil
}
