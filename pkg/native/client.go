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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"wslc-bridge/pkg/process"
	"wslc-bridge/pkg/wslutil"
)

// Timeouts for WSLC commands; native owns them because it knows the commands.
const (
	listTimeout      = 15 * time.Second
	networkTimeout   = 15 * time.Second
	containerTimeout = 2 * time.Minute
	pullTimeout      = 30 * time.Minute
	execTimeout      = 30 * time.Minute
)

// Client runs typed operations against the native WSLC CLI. It owns WSLC
// argument construction and output interpretation; callers describe what to
// do through the specification types.
type Client interface {
	ListContainers(ctx context.Context) ([]ContainerListItem, error)
	CreateContainer(ctx context.Context, spec ContainerSpec) (string, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, spec StopSpec) error
	RemoveContainer(ctx context.Context, spec RemoveSpec) error
	PullImage(ctx context.Context, reference string) error
	ExecContainer(ctx context.Context, id string, command []string, stdout, stderr io.Writer) (int, error)
	CreateNetwork(ctx context.Context, spec NetworkSpec) (string, error)
	RemoveNetwork(ctx context.Context, name string, force bool) error
}

type client struct {
	runner process.Runner
}

var _ Client = (*client)(nil)

// NewClient returns a Client backed by the WSLC and WSL executables.
func NewClient(wslcPath, wslPath string) Client {
	return newClient(process.NewRunner(wslcPath, wslPath))
}

func newClient(runner process.Runner) Client {
	return &client{runner: runner}
}

func (c *client) run(ctx context.Context, args ...string) (process.Result, error) {
	return c.runner.RunWSLC(ctx, args...)
}

func (c *client) runWithTimeout(ctx context.Context, timeout time.Duration, args ...string) (process.Result, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.run(commandCtx, args...)
}

type PortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

func (c *client) ListContainers(ctx context.Context) ([]ContainerListItem, error) {
	result, err := c.runWithTimeout(ctx, listTimeout, "container", "list", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	return DecodeContainerList([]byte(wslutil.DecodeWindowsOutput(result.Stdout)))
}

func (c *client) PullImage(ctx context.Context, reference string) error {
	_, err := c.runWithTimeout(ctx, pullTimeout, "image", "pull", reference)
	return classifyCommandError(err)
}

func (c *client) StartContainer(ctx context.Context, id string) error {
	_, err := c.runWithTimeout(ctx, containerTimeout, "container", "start", id)
	if err == nil {
		return nil
	}
	if errorContains(err, "already running", "already started") {
		return &Error{Class: ClassAlreadyRunning, Cause: err}
	}
	return classifyCommandError(err)
}

func (c *client) StopContainer(ctx context.Context, spec StopSpec) error {
	arguments := []string{"container", "stop"}
	if spec.Signal != "" {
		arguments = append(arguments, "--signal", spec.Signal)
	}
	if spec.TimeoutSeconds != nil {
		arguments = append(arguments, "--time", strconv.Itoa(*spec.TimeoutSeconds))
	}
	arguments = append(arguments, spec.ID)
	_, err := c.runWithTimeout(ctx, containerTimeout, arguments...)
	if err == nil {
		return nil
	}
	if errorContains(err, "not running", "already stopped") {
		return &Error{Class: ClassNotRunning, Cause: err}
	}
	return classifyCommandError(err)
}

func (c *client) RemoveContainer(ctx context.Context, spec RemoveSpec) error {
	arguments := []string{"container", "remove"}
	if spec.Force {
		arguments = append(arguments, "--force")
	}
	arguments = append(arguments, spec.ID)
	_, err := c.runWithTimeout(ctx, containerTimeout, arguments...)
	return classifyCommandError(err)
}

func (c *client) ExecContainer(ctx context.Context, id string, command []string, stdout, stderr io.Writer) (int, error) {
	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	arguments := append([]string{"container", "exec", id}, command...)
	return c.runner.RunWSLCStreaming(execCtx, stdout, stderr, arguments...)
}

func (c *client) CreateNetwork(ctx context.Context, spec NetworkSpec) (string, error) {
	// Create and the follow-up inspect share one overall deadline.
	ctx, cancel := context.WithTimeout(ctx, containerTimeout)
	defer cancel()
	arguments := []string{"network", "create"}
	if spec.Driver != "" {
		arguments = append(arguments, "--driver", spec.Driver)
	}
	arguments = appendOptionPairs(arguments, "--opt", spec.Options)
	arguments = appendOptionPairs(arguments, "--label", spec.Labels)
	arguments = append(arguments, spec.Name)
	if _, err := c.runWithTimeout(ctx, containerTimeout, arguments...); err != nil {
		return "", classifyCommandError(err)
	}
	result, err := c.runWithTimeout(ctx, networkTimeout, "network", "inspect", spec.Name)
	if err != nil {
		return "", classifyCommandError(err)
	}
	var entries []struct {
		ID    string `json:"Id"`
		IDAlt string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(wslutil.DecodeWindowsOutput(result.Stdout)), &entries); err != nil {
		return "", fmt.Errorf("decode wslc network inspect: %w", err)
	}
	if len(entries) == 0 {
		return "", errors.New("wslc network inspect returned no network ID")
	}
	if entries[0].ID == "" {
		entries[0].ID = entries[0].IDAlt
	}
	if entries[0].ID == "" {
		return "", errors.New("wslc network inspect returned no network ID")
	}
	return entries[0].ID, nil
}

func (c *client) RemoveNetwork(ctx context.Context, name string, force bool) error {
	arguments := []string{"network", "remove"}
	if force {
		arguments = append(arguments, "--force")
	}
	arguments = append(arguments, name)
	_, err := c.runWithTimeout(ctx, containerTimeout, arguments...)
	return classifyCommandError(err)
}

// appendOptionPairs appends one flag per key=value pair, ordered by key so
// the arguments stay deterministic.
func appendOptionPairs(arguments []string, flag string, pairs map[string]string) []string {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		arguments = append(arguments, flag, key+"="+pairs[key])
	}
	return arguments
}

type ContainerListItem struct {
	ID    string
	State string
	Ports map[string][]PortBinding
}

func DecodeContainerList(data []byte) ([]ContainerListItem, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return []ContainerListItem{}, nil
	}
	var array []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &array); err == nil {
		items := make([]ContainerListItem, 0, len(array))
		for _, raw := range array {
			item, err := decodeListItem(raw)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	}
	lines := strings.Split(trimmed, "\n")
	items := make([]ContainerListItem, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		item, err := decodeListItem(json.RawMessage(line))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func decodeListItem(raw json.RawMessage) (ContainerListItem, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return ContainerListItem{}, fmt.Errorf("decode wslc container list item: %w", err)
	}
	for _, key := range []string{"Id", "ID"} {
		if value, ok := values[key]; ok {
			var id string
			if err := json.Unmarshal(value, &id); err == nil && id != "" {
				item := ContainerListItem{ID: id, Ports: make(map[string][]PortBinding)}
				if state, ok := values["State"]; ok {
					item.State = decodeContainerState(state)
				}
				if ports, ok := values["Ports"]; ok {
					if err := decodeContainerPorts(ports, &item); err != nil {
						return ContainerListItem{}, err
					}
				}
				return item, nil
			}
		}
	}
	return ContainerListItem{}, errors.New("wslc container list item has no ID")
}

func decodeContainerState(raw json.RawMessage) string {
	var state string
	if err := json.Unmarshal(raw, &state); err == nil {
		return state
	}
	var stateValue int
	if err := json.Unmarshal(raw, &stateValue); err == nil {
		switch stateValue {
		case 1:
			return "created"
		case 2:
			return "running"
		case 3:
			return "exited"
		case 4:
			return "deleted"
		}
	}
	return ""
}

func decodeContainerPorts(raw json.RawMessage, item *ContainerListItem) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == `""` {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var ports []struct {
			HostPort       uint16 `json:"HostPort"`
			ContainerPort  uint16 `json:"ContainerPort"`
			Protocol       int    `json:"Protocol"`
			BindingAddress string `json:"BindingAddress"`
		}
		if err := json.Unmarshal(raw, &ports); err != nil {
			return fmt.Errorf("decode structured wslc container ports: %w", err)
		}
		for _, port := range ports {
			protocol, ok := dockerProtocol(port.Protocol)
			if !ok {
				return fmt.Errorf("unsupported WSLC port protocol %d", port.Protocol)
			}
			if port.HostPort == 0 || port.ContainerPort == 0 {
				continue
			}
			key := fmt.Sprintf("%d/%s", port.ContainerPort, protocol)
			item.Ports[key] = append(item.Ports[key], PortBinding{
				HostIP: port.BindingAddress, HostPort: strconv.Itoa(int(port.HostPort)),
			})
		}
		return nil
	}

	var formatted string
	if err := json.Unmarshal(raw, &formatted); err != nil {
		return fmt.Errorf("decode formatted wslc container ports: %w", err)
	}
	for _, entry := range strings.Split(formatted, ", ") {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		binding, portKey, err := parseFormattedPort(entry)
		if err != nil {
			return err
		}
		item.Ports[portKey] = append(item.Ports[portKey], binding)
	}
	return nil
}

func parseFormattedPort(value string) (PortBinding, string, error) {
	arrow := strings.Index(value, "->")
	if arrow < 0 {
		return PortBinding{}, "", fmt.Errorf("invalid formatted WSLC port mapping %q", value)
	}
	endpoint := value[:arrow]
	container := value[arrow+2:]
	slash := strings.LastIndex(container, "/")
	if slash < 0 {
		return PortBinding{}, "", fmt.Errorf("invalid formatted WSLC container port %q", value)
	}
	containerPort, err := strconv.Atoi(container[:slash])
	if err != nil || containerPort < 1 || containerPort > 65535 {
		return PortBinding{}, "", fmt.Errorf("invalid formatted WSLC container port %q", value)
	}
	protocol := container[slash+1:]
	if protocol != "tcp" && protocol != "udp" {
		return PortBinding{}, "", fmt.Errorf("unsupported formatted WSLC port protocol %q", protocol)
	}
	var hostIP, hostPortText string
	if strings.HasPrefix(endpoint, "[") {
		closingBracket := strings.Index(endpoint, "]:")
		if closingBracket < 0 {
			return PortBinding{}, "", fmt.Errorf("invalid formatted WSLC host address %q", endpoint)
		}
		hostIP = endpoint[1:closingBracket]
		hostPortText = endpoint[closingBracket+2:]
	} else {
		separator := strings.LastIndex(endpoint, ":")
		if separator < 0 {
			return PortBinding{}, "", fmt.Errorf("invalid formatted WSLC host address %q", endpoint)
		}
		hostIP = endpoint[:separator]
		hostPortText = endpoint[separator+1:]
	}
	hostPort, err := strconv.Atoi(hostPortText)
	if err != nil || hostPort < 1 || hostPort > 65535 {
		return PortBinding{}, "", fmt.Errorf("invalid formatted WSLC host port %q", hostPortText)
	}
	return PortBinding{HostIP: hostIP, HostPort: strconv.Itoa(hostPort)}, fmt.Sprintf("%d/%s", containerPort, protocol), nil
}

func dockerProtocol(protocol int) (string, bool) {
	switch protocol {
	case 6:
		return "tcp", true
	case 17:
		return "udp", true
	default:
		return "", false
	}
}
