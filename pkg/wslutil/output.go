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
	"strings"
	"unicode/utf16"
)

func DecodeWindowsOutput(data []byte) string {
	if len(data) >= 2 {
		if data[0] == 0xff && data[1] == 0xfe {
			data = data[2:]
		} else if data[0] == 0xfe && data[1] == 0xff {
			units := make([]uint16, 0, len(data)/2)
			for i := 2; i+1 < len(data); i += 2 {
				units = append(units, uint16(data[i])<<8|uint16(data[i+1]))
			}
			return strings.TrimSpace(string(utf16.Decode(units)))
		}
	}
	if len(data) > 1 {
		for _, value := range data {
			if value == 0 {
				units := make([]uint16, 0, len(data)/2)
				for i := 0; i+1 < len(data); i += 2 {
					units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
				}
				return strings.TrimSpace(string(utf16.Decode(units)))
			}
		}
	}
	return strings.TrimSpace(string(data))
}
