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

package guest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStdioHelperProcess(t *testing.T) {
	t.Parallel()

	if os.Getenv("WSLC_BRIDGE_STDIO_HELPER") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if line == "\r\n" {
			break
		}
	}
	_, _ = fmt.Fprint(os.Stdout, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK")
	_, _ = fmt.Fprint(os.Stderr, "helper diagnostic must not enter HTTP output\n")
	_, _ = io.Copy(io.Discard, reader)
}

func TestStdioDialerCarriesRawBytesAndSeparatesStderr(t *testing.T) {
	t.Parallel()

	dialer := newStdioDialer(os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess"},
		[]string{"WSLC_BRIDGE_STDIO_HELPER=1"},
	)
	connection, err := dialer.DialContext(context.Background(), "tcp", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request := "GET /_ping HTTP/1.1\r\nHost: docker\r\nConnection: close\r\n\r\n"
	if _, err := io.WriteString(connection, request); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		if line == "\r\n" {
			break
		}
	}
	body := make([]byte, 2)
	if _, err := io.ReadFull(reader, body); err != nil {
		t.Fatal(err)
	}
	if status != "HTTP/1.1 200 OK\r\n" || string(body) != "OK" {
		t.Fatalf("unexpected response status %q body %q", status, body)
	}
	if strings.Contains(status+string(body), "helper diagnostic") {
		t.Fatal("stderr leaked into the Docker HTTP byte stream")
	}
}
