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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"wslc-bridge/pkg/process"
)

// fakeRunner is a process.Runner that records wslc.exe calls and answers
// them with canned results.
type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string
	// run answers RunWSLC calls.
	run func(args []string) (process.Result, error)
	// stream answers RunWSLCStreaming calls.
	stream func(args []string, stdout, stderr io.Writer) (int, error)
}

var _ process.Runner = (*fakeRunner)(nil)

func (f *fakeRunner) record(args []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), args...))
}

func (f *fakeRunner) recorded() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeRunner) RunWSLC(ctx context.Context, args ...string) (process.Result, error) {
	f.record(args)
	if _, ok := ctx.Deadline(); !ok {
		return process.Result{}, errors.New("wslc.exe called without a timeout")
	}
	return f.run(args)
}

func (f *fakeRunner) RunWSL(context.Context, ...string) (process.Result, error) {
	return process.Result{}, errors.New("unexpected wsl.exe call")
}

func (f *fakeRunner) RunWSLCStreaming(ctx context.Context, stdout, stderr io.Writer, args ...string) (int, error) {
	f.record(args)
	if _, ok := ctx.Deadline(); !ok {
		return -1, errors.New("wslc.exe called without a timeout")
	}
	return f.stream(args, stdout, stderr)
}

func respondWith(output string) func([]string) (process.Result, error) {
	return func([]string) (process.Result, error) {
		return process.Result{Stdout: []byte(output)}, nil
	}
}

func commandFailure(code int) error {
	return &process.CommandError{Name: "wslc.exe", ExitCode: code, Stderr: fmt.Sprintf("failed with %d", code)}
}

func commandFailureWithStderr(stderr string) error {
	return &process.CommandError{Name: "wslc.exe", ExitCode: 1, Stderr: stderr}
}

func assertCalls(t *testing.T, runner *fakeRunner, want ...[]string) {
	t.Helper()
	if got := runner.recorded(); !reflect.DeepEqual(got, want) {
		t.Fatalf("wslc.exe calls = %q, want %q", got, want)
	}
}

func assertExitCode(t *testing.T, err error, code int) {
	t.Helper()
	var commandError *process.CommandError
	if !errors.As(err, &commandError) {
		t.Fatalf("error = %v, want *process.CommandError", err)
	}
	if commandError.ExitCode != code {
		t.Fatalf("exit code = %d, want %d", commandError.ExitCode, code)
	}
}

func assertClass(t *testing.T, err error, class ErrorClass) {
	t.Helper()
	var nativeError *Error
	if !errors.As(err, &nativeError) {
		t.Fatalf("error = %v, want *native.Error", err)
	}
	if nativeError.Class != class {
		t.Fatalf("error class = %d, want %d", nativeError.Class, class)
	}
}

func TestDecodeContainerListSupportsArrayAndJSONLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		data             string
		wantFirstPortKey string
		wantHostPort     string
	}{
		{
			name:             "2.9.3 array",
			data:             `[{"Id":"container-a","State":2,"Ports":[{"HostPort":49152,"ContainerPort":80,"Protocol":6,"BindingAddress":"127.0.0.1"}]},{"Id":"container-b","State":3,"Ports":[]}]`,
			wantFirstPortKey: "80/tcp",
			wantHostPort:     "49152",
		},
		{
			name:             "newer JSON lines",
			data:             "{\"ID\":\"container-a\",\"State\":\"running\",\"Ports\":\"127.0.0.1:49152->80/tcp, [::1]:49153->81/udp\"}\n{\"ID\":\"container-b\",\"State\":\"exited\",\"Ports\":\"\"}\n",
			wantFirstPortKey: "80/tcp",
			wantHostPort:     "49152",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			items, err := DecodeContainerList([]byte(test.data))
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 2 || items[0].ID != "container-a" {
				t.Fatalf("DecodeContainerList() = %#v", items)
			}
			binding := items[0].Ports[test.wantFirstPortKey][0]
			if binding.HostPort != test.wantHostPort {
				t.Fatalf("first port binding = %#v, want host port %s", binding, test.wantHostPort)
			}
		})
	}
}

func TestListContainersDecodesCommandOutput(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: respondWith("{\"ID\":\"container-a\",\"State\":\"running\",\"Ports\":\"127.0.0.1:49152->80/tcp\"}\n{\"ID\":\"container-b\",\"State\":\"exited\",\"Ports\":\"\"}\n")}
	items, err := newClient(runner).ListContainers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"container", "list", "--all", "--format", "json"})
	if len(items) != 2 || items[0].ID != "container-a" || items[0].State != "running" {
		t.Fatalf("ListContainers() = %#v", items)
	}
	binding := items[0].Ports["80/tcp"][0]
	if binding.HostIP != "127.0.0.1" || binding.HostPort != "49152" {
		t.Fatalf("port binding = %#v, want 127.0.0.1:49152", binding)
	}
}

func TestListContainersPropagatesCommandFailure(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func([]string) (process.Result, error) { return process.Result{}, commandFailure(3) }}
	_, err := newClient(runner).ListContainers(context.Background())
	assertExitCode(t, err, 3)
}

func TestCreateContainerReturnsLastNonEmptyOutputLine(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: respondWith("creating container\ncontainer-id\n\n")}
	id, err := newClient(runner).CreateContainer(context.Background(), ContainerSpec{Image: "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"container", "create", "alpine"})
	if id != "container-id" {
		t.Fatalf("CreateContainer() = %q, want \"container-id\"", id)
	}
}

func TestCreateContainerRejectsEmptyOutput(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: respondWith("\n")}
	_, err := newClient(runner).CreateContainer(context.Background(), ContainerSpec{Image: "alpine"})
	if err == nil || !strings.Contains(err.Error(), "empty container ID") {
		t.Fatalf("CreateContainer() error = %v, want an empty container ID error", err)
	}
}

func TestCreateContainerClassifiesCommandFailure(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func([]string) (process.Result, error) {
		return process.Result{}, commandFailureWithStderr("image not found in registry")
	}}
	_, err := newClient(runner).CreateContainer(context.Background(), ContainerSpec{Image: "missing"})
	assertClass(t, err, ClassNotFound)
	assertExitCode(t, err, 1)
}

func TestBuildCreateArgsMapsSpecToWSLCFlags(t *testing.T) {
	t.Parallel()

	spec := ContainerSpec{
		Name:           "bridge-test",
		Image:          "alpine:3.20",
		Hostname:       "box",
		User:           "1000",
		WorkingDir:     "/work",
		StopSignal:     "SIGTERM",
		Env:            []string{"A=1", "B=2"},
		Labels:         map[string]string{"zeta": "26", "alpha": "1"},
		Network:        "testnet",
		NetworkAliases: []string{"alias-a", "alias-b"},
		DNS:            []string{"1.1.1.1"},
		DNSSearch:      []string{"example.com"},
		DNSOptions:     []string{"ndots:2"},
		Ulimits:        []Ulimit{{Name: "nofile", Soft: 1024, Hard: 2048}},
		Publish:        []string{"127.0.0.1::80/tcp"},
		Volumes:        []string{`C:\data:/data:ro`},
		Tmpfs:          []string{"/tmp:size=64m"},
		Memory:         1024,
		CPUS:           1.5,
		ShmSize:        512,
		Cmd:            []string{"sh", "-c", "ls"},
	}
	want := []string{
		"container", "create",
		"--name", "bridge-test",
		"--hostname", "box",
		"--user", "1000",
		"--workdir", "/work",
		"--stop-signal", "SIGTERM",
		"--env", "A=1",
		"--env", "B=2",
		"--label", "alpha=1",
		"--label", "zeta=26",
		"--network", "testnet",
		"--network-alias", "alias-a",
		"--network-alias", "alias-b",
		"--dns", "1.1.1.1",
		"--dns-search", "example.com",
		"--dns-option", "ndots:2",
		"--ulimit", "nofile=1024:2048",
		"--publish", "127.0.0.1::80/tcp",
		"--volume", `C:\data:/data:ro`,
		"--tmpfs", "/tmp:size=64m",
		"--memory", "1024",
		"--cpus", "1.5",
		"--shm-size", "512",
		"alpine:3.20", "sh", "-c", "ls",
	}
	if got := buildCreateArgs(spec); !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateArgs() = %q, want %q", got, want)
	}
}

func TestBuildCreateArgsAddsInteractiveTTYAndPublishAll(t *testing.T) {
	t.Parallel()

	got := buildCreateArgs(ContainerSpec{Image: "alpine", Interactive: true, TTY: true, PublishAllPorts: true})
	want := []string{"container", "create", "--publish-all", "--interactive", "--tty", "alpine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateArgs() = %q, want %q", got, want)
	}
}

func TestStartContainerRunsNativeStart(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: respondWith("")}
	if err := newClient(runner).StartContainer(context.Background(), "container-id"); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"container", "start", "container-id"})
}

func TestStartContainerClassifiesAlreadyRunning(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: func([]string) (process.Result, error) {
		return process.Result{}, commandFailureWithStderr("container is already running")
	}}
	err := newClient(runner).StartContainer(context.Background(), "container-id")
	assertClass(t, err, ClassAlreadyRunning)
}

func TestStopContainerBuildsSignalAndTimeoutArguments(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: respondWith("")}
	timeout := 10
	err := newClient(runner).StopContainer(context.Background(), StopSpec{ID: "container-id", Signal: "SIGKILL", TimeoutSeconds: &timeout})
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"container", "stop", "--signal", "SIGKILL", "--time", "10", "container-id"})
}

func TestStopContainerClassifiesNotRunning(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: func([]string) (process.Result, error) {
		return process.Result{}, commandFailureWithStderr("container is not running")
	}}
	err := newClient(runner).StopContainer(context.Background(), StopSpec{ID: "container-id"})
	assertClass(t, err, ClassNotRunning)
}

func TestRemoveContainerBuildsForceArgument(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: respondWith("")}
	if err := newClient(runner).RemoveContainer(context.Background(), RemoveSpec{ID: "container-id", Force: true}); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"container", "remove", "--force", "container-id"})
}

func TestRemoveContainerClassifiesConflict(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: func([]string) (process.Result, error) {
		return process.Result{}, commandFailureWithStderr("container is running: cannot remove")
	}}
	err := newClient(runner).RemoveContainer(context.Background(), RemoveSpec{ID: "container-id"})
	assertClass(t, err, ClassConflict)
}

func TestPullImageSucceeds(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: respondWith("")}
	if err := newClient(runner).PullImage(context.Background(), "alpine:3.20"); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"image", "pull", "alpine:3.20"})
}

func TestPullImagePropagatesCommandFailure(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{run: func([]string) (process.Result, error) { return process.Result{}, commandFailure(9) }}
	err := newClient(runner).PullImage(context.Background(), "fail:latest")
	assertExitCode(t, err, 9)
}

func TestExecContainerStreamsCommandOutput(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{stream: func(_ []string, stdout, _ io.Writer) (int, error) {
		_, _ = io.WriteString(stdout, "exec-output\n")
		return 0, nil
	}}
	var stdout, stderr bytes.Buffer
	code, err := newClient(runner).ExecContainer(context.Background(), "container-id", []string{"ls", "-l"}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"container", "exec", "container-id", "ls", "-l"})
	if code != 0 || stdout.String() != "exec-output\n" || stderr.Len() != 0 {
		t.Fatalf("ExecContainer() = (%d, %q, %q), want streamed output", code, stdout.String(), stderr.String())
	}
}

func TestExecContainerPropagatesExitCode(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{stream: func([]string, io.Writer, io.Writer) (int, error) { return 8, commandFailure(8) }}
	var stdout, stderr bytes.Buffer
	code, err := newClient(runner).ExecContainer(context.Background(), "failing-container", []string{"ls"}, &stdout, &stderr)
	assertExitCode(t, err, 8)
	if code != 8 {
		t.Fatalf("ExecContainer() exit code = %d, want 8", code)
	}
}

func TestCreateNetwork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		createErr   error
		inspect     string
		inspectErr  error
		wantID      string
		wantCode    int
		wantMessage string
		wantCalls   [][]string
	}{
		{name: "returns inspected ID", inspect: `[{"ID":"net-42"}]`, wantID: "net-42"},
		{name: "accepts legacy Id field", inspect: `[{"Id":"net-legacy"}]`, wantID: "net-legacy"},
		{name: "propagates create failure", createErr: commandFailure(6), wantCode: 6, wantCalls: [][]string{{"network", "create", "testnet"}}},
		{name: "propagates inspect failure", inspectErr: commandFailure(5), wantCode: 5},
		{name: "rejects undecodable inspect output", inspect: "not-json", wantMessage: "decode wslc network inspect"},
		{name: "rejects empty inspect output", inspect: "[]", wantMessage: "no network ID"},
		{name: "rejects inspect output without ID", inspect: `[{}]`, wantMessage: "no network ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := &fakeRunner{run: func(args []string) (process.Result, error) {
				if args[1] == "create" {
					return process.Result{}, test.createErr
				}
				return process.Result{Stdout: []byte(test.inspect)}, test.inspectErr
			}}
			spec := NetworkSpec{Name: "testnet"}
			id, err := newClient(runner).CreateNetwork(context.Background(), spec)
			wantCalls := test.wantCalls
			if wantCalls == nil {
				wantCalls = [][]string{{"network", "create", "testnet"}, {"network", "inspect", "testnet"}}
			}
			assertCalls(t, runner, wantCalls...)
			switch {
			case test.wantCode != 0:
				assertExitCode(t, err, test.wantCode)
			case test.wantMessage != "":
				if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
					t.Fatalf("CreateNetwork() error = %v, want it to contain %q", err, test.wantMessage)
				}
			case err != nil:
				t.Fatal(err)
			case id != test.wantID:
				t.Fatalf("CreateNetwork() = %q, want %q", id, test.wantID)
			}
		})
	}
}

func TestCreateNetworkMapsSpecToWSLCFlags(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: func(args []string) (process.Result, error) {
		if args[1] == "inspect" {
			return process.Result{Stdout: []byte(`[{"ID":"net-42"}]`)}, nil
		}
		return process.Result{}, nil
	}}
	spec := NetworkSpec{
		Name:    "testnet",
		Driver:  "bridge",
		Options: map[string]string{"b": "2", "a": "1"},
		Labels:  map[string]string{"y": "2", "x": "1"},
	}
	if _, err := newClient(runner).CreateNetwork(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner,
		[]string{"network", "create", "--driver", "bridge", "--opt", "a=1", "--opt", "b=2", "--label", "x=1", "--label", "y=2", "testnet"},
		[]string{"network", "inspect", "testnet"},
	)
}

func TestRemoveNetworkBuildsForceArgument(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{run: respondWith("")}
	if err := newClient(runner).RemoveNetwork(context.Background(), "testnet", true); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, runner, []string{"network", "remove", "--force", "testnet"})
}

func TestCommandErrorClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stderr string
		class  ErrorClass
	}{
		{name: "not found", stderr: "Error: container not found", class: ClassNotFound},
		{name: "no such", stderr: "no such container", class: ClassNotFound},
		{name: "conflict", stderr: "name conflict detected", class: ClassConflict},
		{name: "already", stderr: "already exists", class: ClassConflict},
		{name: "running", stderr: "container is running", class: ClassConflict},
		{name: "unknown", stderr: "something else went wrong", class: ClassUnknown},
		{name: "empty", stderr: "", class: ClassUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			class := classifyStderr(test.stderr)
			if class != test.class {
				t.Fatalf("classifyStderr(%q) = %d, want %d", test.stderr, class, test.class)
			}
		})
	}
}

func TestClassifyCommandErrorPreservesTheCause(t *testing.T) {
	t.Parallel()

	cause := commandFailureWithStderr("container not found")
	err := classifyCommandError(cause)
	var nativeError *Error
	if !errors.As(err, &nativeError) || nativeError.Class != ClassNotFound {
		t.Fatalf("classifyCommandError() = %v, want a not-found *Error", err)
	}
	if !errors.Is(err, cause) || err.Error() != cause.Error() {
		t.Fatalf("classifyCommandError() lost the cause: %v", err)
	}
}

func TestClassifyCommandErrorLeavesOtherErrorsAlone(t *testing.T) {
	t.Parallel()

	cause := errors.New("plain failure")
	if err := classifyCommandError(cause); err != cause {
		t.Fatalf("classifyCommandError() = %v, want the original error", err)
	}
}
