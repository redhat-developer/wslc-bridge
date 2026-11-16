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
	"io"
	"os/exec"
	"strings"
	"time"
)

const MaxOutputBytes = 16 << 20

type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type CommandError struct {
	Name     string
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *CommandError) Unwrap() error { return e.Err }

func (e *CommandError) Error() string {
	message := fmt.Sprintf("%s exited with code %d", e.Name, e.ExitCode)
	if detail := strings.TrimSpace(e.Stderr); detail != "" {
		message += ": " + detail
	}
	if e.Err != nil && e.ExitCode < 0 {
		return fmt.Sprintf("run %s: %v", e.Name, e.Err)
	}
	return message
}

// Runner runs WSLC and WSL commands.
type Runner interface {
	RunWSLC(ctx context.Context, args ...string) (Result, error)
	RunWSL(ctx context.Context, args ...string) (Result, error)
	RunWSLCStreaming(ctx context.Context, stdout, stderr io.Writer, args ...string) (int, error)
}

type runner struct {
	wslcPath  string
	wslPath   string
	waitDelay time.Duration
}

var _ Runner = (*runner)(nil)

// NewRunner returns a Runner for the WSLC and WSL executables.
func NewRunner(wslcPath, wslPath string) Runner {
	return &runner{wslcPath: wslcPath, wslPath: wslPath, waitDelay: 3 * time.Second}
}

func (r *runner) RunWSLC(ctx context.Context, args ...string) (Result, error) {
	return r.run(ctx, r.wslcPath, args...)
}

func (r *runner) RunWSL(ctx context.Context, args ...string) (Result, error) {
	return r.run(ctx, r.wslPath, args...)
}

func (r *runner) RunWSLCStreaming(ctx context.Context, stdout, stderr io.Writer, args ...string) (int, error) {
	if r.wslcPath == "" {
		return -1, &CommandError{Name: "wslc.exe", ExitCode: -1, Err: errors.New("not found on PATH")}
	}
	cmd := exec.CommandContext(ctx, r.wslcPath, args...)
	cmd.WaitDelay = r.waitDelay
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	code := -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		code = exitError.ExitCode()
	}
	if ctx.Err() != nil {
		return code, ctx.Err()
	}
	return code, &CommandError{Name: r.wslcPath, Args: append([]string(nil), args...), ExitCode: code, Err: err}
}

func (r *runner) run(ctx context.Context, name string, args ...string) (Result, error) {
	if name == "" {
		return Result{ExitCode: -1}, &CommandError{Name: "executable", ExitCode: -1, Err: errors.New("not found on PATH")}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = r.waitDelay
	var stdout, stderr limitedBuffer
	stdout.limit = MaxOutputBytes
	stderr.limit = MaxOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: 0}
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	result.ExitCode = -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}
	return result, &CommandError{Name: name, Args: append([]string(nil), args...), ExitCode: result.ExitCode, Stderr: string(result.Stderr), Err: err}
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
