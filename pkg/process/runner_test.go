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

package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if code, handled := runHelperProcess(os.Args[1:]); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// runHelperProcess emulates an external command for tests that re-execute the
// test binary as a subprocess. Helper mode requires a positional argument and
// the absence of -test.* flags, so regular `go test` runs never trigger it.
func runHelperProcess(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-test.") {
			return 0, false
		}
	}
	switch args[0] {
	case "helper-ok":
		fmt.Println("hello")
	case "helper-fail":
		fmt.Fprintln(os.Stderr, "boom")
		return 7, true
	case "helper-silent":
	default:
		return 0, false
	}
	return 0, true
}

func TestRunWSLCReturnsOutputAndZeroExitCode(t *testing.T) {
	t.Parallel()
	runner := NewRunner(os.Args[0], "")
	result, err := runner.RunWSLC(context.Background(), "helper-ok")
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Stdout) != "hello\n" || result.ExitCode != 0 {
		t.Fatalf("RunWSLC() = %#v, want stdout \"hello\\n\" and exit code 0", result)
	}
}

func TestRunWSLUsesWSLExecutable(t *testing.T) {
	t.Parallel()
	runner := NewRunner("", os.Args[0])
	result, err := runner.RunWSL(context.Background(), "helper-ok")
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Stdout) != "hello\n" {
		t.Fatalf("RunWSL() stdout = %q, want \"hello\\n\"", result.Stdout)
	}
}

func TestRunWSLCReportsCommandError(t *testing.T) {
	t.Parallel()
	runner := NewRunner(os.Args[0], "")
	result, err := runner.RunWSLC(context.Background(), "helper-fail")
	var commandError *CommandError
	if !errors.As(err, &commandError) {
		t.Fatalf("RunWSLC() error = %v, want *CommandError", err)
	}
	if commandError.ExitCode != 7 {
		t.Fatalf("CommandError.ExitCode = %d, want 7", commandError.ExitCode)
	}
	if strings.TrimSpace(commandError.Stderr) != "boom" {
		t.Fatalf("CommandError.Stderr = %q, want \"boom\"", commandError.Stderr)
	}
	if !strings.Contains(commandError.Error(), "exited with code 7") {
		t.Fatalf("CommandError.Error() = %q, want it to mention the exit code", commandError.Error())
	}
	if result.ExitCode != 7 {
		t.Fatalf("Result.ExitCode = %d, want 7", result.ExitCode)
	}
}

func TestRunWSLCRequiresExecutablePath(t *testing.T) {
	t.Parallel()
	runner := NewRunner("", "")
	_, err := runner.RunWSLC(context.Background(), "version")
	var commandError *CommandError
	if !errors.As(err, &commandError) {
		t.Fatalf("RunWSLC() error = %v, want *CommandError", err)
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("RunWSLC() error = %v, want a missing PATH error", err)
	}
}

func TestRunWSLCReturnsContextErrorWhenCancelled(t *testing.T) {
	t.Parallel()
	runner := NewRunner(os.Args[0], "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runner.RunWSLC(ctx, "helper-ok")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWSLC() error = %v, want context.Canceled", err)
	}
}

func TestRunWSLCStreamingWritesOutput(t *testing.T) {
	t.Parallel()
	runner := NewRunner(os.Args[0], "")
	var stdout, stderr bytes.Buffer
	code, err := runner.RunWSLCStreaming(context.Background(), &stdout, &stderr, "helper-ok")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || stdout.String() != "hello\n" || stderr.Len() != 0 {
		t.Fatalf("RunWSLCStreaming() = (%d, %q, %q), want (0, \"hello\\n\", \"\")", code, stdout.String(), stderr.String())
	}
}

func TestRunWSLCStreamingReportsExitCode(t *testing.T) {
	t.Parallel()
	runner := NewRunner(os.Args[0], "")
	var stdout, stderr bytes.Buffer
	code, err := runner.RunWSLCStreaming(context.Background(), &stdout, &stderr, "helper-fail")
	var commandError *CommandError
	if !errors.As(err, &commandError) {
		t.Fatalf("RunWSLCStreaming() error = %v, want *CommandError", err)
	}
	if code != 7 || strings.TrimSpace(stderr.String()) != "boom" {
		t.Fatalf("RunWSLCStreaming() = (%d, %q), want exit code 7 and \"boom\" on stderr", code, stderr.String())
	}
}

func TestCommandErrorMessageVariants(t *testing.T) {
	t.Parallel()
	withDetail := &CommandError{Name: "wslc.exe", ExitCode: 3, Stderr: "  not enough arguments  "}
	if withDetail.Error() != "wslc.exe exited with code 3: not enough arguments" {
		t.Fatalf("Error() = %q, want the trimmed stderr detail", withDetail.Error())
	}
	withoutDetail := &CommandError{Name: "wslc.exe", ExitCode: 3}
	if withoutDetail.Error() != "wslc.exe exited with code 3" {
		t.Fatalf("Error() = %q", withoutDetail.Error())
	}
	startFailure := &CommandError{Name: "wslc.exe", ExitCode: -1, Err: errors.New("file does not exist")}
	if startFailure.Error() != "run wslc.exe: file does not exist" {
		t.Fatalf("Error() = %q, want the start failure variant", startFailure.Error())
	}
}

func TestLimitedBufferTruncatesAtLimit(t *testing.T) {
	t.Parallel()
	buffer := limitedBuffer{limit: 4}
	written, err := buffer.Write([]byte("hello world"))
	if err != nil {
		t.Fatal(err)
	}
	if written != len("hello world") {
		t.Fatalf("Write() accepted %d bytes, want %d", written, len("hello world"))
	}
	if buffer.Len() != 4 {
		t.Fatalf("buffer kept %d bytes, want 4", buffer.Len())
	}
}
