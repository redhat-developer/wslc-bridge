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

package wslutil

import (
	"encoding/binary"
	"testing"
)

func TestDecodeWindowsOutputTrimsPlainASCII(t *testing.T) {
	t.Parallel()
	if got := DecodeWindowsOutput([]byte("  wslc 1.2.3 \n")); got != "wslc 1.2.3" {
		t.Fatalf("DecodeWindowsOutput() = %q, want trimmed ASCII", got)
	}
}

func TestDecodeWindowsOutputHandlesUTF16LEBOM(t *testing.T) {
	t.Parallel()
	value := "WSL version: 2.9.3.0"
	data := make([]byte, 2+len(value)*2)
	data[0], data[1] = 0xff, 0xfe
	for i, character := range value {
		binary.LittleEndian.PutUint16(data[2+i*2:], uint16(character))
	}
	if got := DecodeWindowsOutput(data); got != value {
		t.Fatalf("DecodeWindowsOutput() = %q, want %q", got, value)
	}
}

func TestDecodeWindowsOutputHandlesUTF16BEBOM(t *testing.T) {
	t.Parallel()
	value := "wslc 3.0"
	data := make([]byte, 2+len(value)*2)
	data[0], data[1] = 0xfe, 0xff
	for i, character := range value {
		binary.BigEndian.PutUint16(data[2+i*2:], uint16(character))
	}
	if got := DecodeWindowsOutput(data); got != value {
		t.Fatalf("DecodeWindowsOutput() = %q, want %q", got, value)
	}
}

func TestDecodeWindowsOutputDetectsNulSeparatedUTF16(t *testing.T) {
	t.Parallel()
	data := []byte{'w', 0, 's', 0, 'l', 0}
	if got := DecodeWindowsOutput(data); got != "wsl" {
		t.Fatalf("DecodeWindowsOutput() = %q, want \"wsl\"", got)
	}
}

func TestDecodeWindowsOutputKeepsASCIIWithoutNulBytes(t *testing.T) {
	t.Parallel()
	// No BOM and no NUL bytes: the input must stay plain ASCII.
	if got := DecodeWindowsOutput([]byte("ab")); got != "ab" {
		t.Fatalf("DecodeWindowsOutput() = %q, want \"ab\"", got)
	}
}
