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

package native

import (
	"errors"
	"strings"

	"wslc-bridge/pkg/process"
)

// ErrorClass classifies why a WSLC command failed.
type ErrorClass int

const (
	ClassUnknown ErrorClass = iota
	ClassNotFound
	ClassConflict
	ClassAlreadyRunning
	ClassNotRunning
)

// Error wraps a failed WSLC command with its classification, so callers can
// react to the failure without interpreting WSLC output themselves. Error and
// Unwrap keep the underlying command error intact for errors.Is and errors.As.
type Error struct {
	Class ErrorClass
	Cause error
}

func (e *Error) Error() string { return e.Cause.Error() }
func (e *Error) Unwrap() error { return e.Cause }

// classifyCommandError inspects a failed WSLC command and wraps it in an
// *Error. WSLC has no structured error contract, so the classification reads
// the command's stderr; all WSLC-specific interpretation lives in this
// package, which knows the command that was executed.
func classifyCommandError(err error) error {
	if err == nil {
		return nil
	}
	var commandError *process.CommandError
	if !errors.As(err, &commandError) {
		return err
	}
	return &Error{Class: classifyStderr(commandError.Stderr), Cause: err}
}

func classifyStderr(stderr string) ErrorClass {
	message := strings.ToLower(stderr)
	switch {
	case strings.Contains(message, "not found"), strings.Contains(message, "no such"):
		return ClassNotFound
	case strings.Contains(message, "already"), strings.Contains(message, "conflict"), strings.Contains(message, "running"):
		return ClassConflict
	}
	return ClassUnknown
}

// errorContains reports whether the failed command's error message contains
// any of the phrases, ignoring case.
func errorContains(err error, phrases ...string) bool {
	message := strings.ToLower(err.Error())
	for _, phrase := range phrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}
