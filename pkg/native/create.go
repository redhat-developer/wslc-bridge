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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"wslc-bridge/pkg/wslutil"
)

func (c *client) CreateContainer(ctx context.Context, spec ContainerSpec) (string, error) {
	result, err := c.runWithTimeout(ctx, containerTimeout, buildCreateArgs(spec)...)
	if err != nil {
		return "", classifyCommandError(err)
	}
	lines := strings.Split(strings.TrimSpace(wslutil.DecodeWindowsOutput(result.Stdout)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(lines[i])
		if candidate != "" {
			return candidate, nil
		}
	}
	return "", errors.New("wslc container create returned an empty container ID")
}

// buildCreateArgs turns a validated ContainerSpec into wslc.exe arguments.
func buildCreateArgs(spec ContainerSpec) []string {
	arguments := []string{"container", "create"}
	if spec.Name != "" {
		arguments = append(arguments, "--name", spec.Name)
	}
	if spec.Hostname != "" {
		arguments = append(arguments, "--hostname", spec.Hostname)
	}
	if spec.Domainname != "" {
		arguments = append(arguments, "--domainname", spec.Domainname)
	}
	if spec.User != "" {
		arguments = append(arguments, "--user", spec.User)
	}
	if spec.WorkingDir != "" {
		arguments = append(arguments, "--workdir", spec.WorkingDir)
	}
	if spec.StopSignal != "" {
		arguments = append(arguments, "--stop-signal", spec.StopSignal)
	}
	if spec.Entrypoint != "" {
		arguments = append(arguments, "--entrypoint", spec.Entrypoint)
	}
	for _, value := range spec.Env {
		arguments = append(arguments, "--env", value)
	}
	arguments = appendOptionPairs(arguments, "--label", spec.Labels)
	if spec.Network != "" {
		arguments = append(arguments, "--network", spec.Network)
	}
	for _, alias := range spec.NetworkAliases {
		arguments = append(arguments, "--network-alias", alias)
	}
	for _, value := range spec.DNS {
		arguments = append(arguments, "--dns", value)
	}
	for _, value := range spec.DNSSearch {
		arguments = append(arguments, "--dns-search", value)
	}
	for _, value := range spec.DNSOptions {
		arguments = append(arguments, "--dns-option", value)
	}
	for _, ulimit := range spec.Ulimits {
		arguments = append(arguments, "--ulimit", fmt.Sprintf("%s=%d:%d", ulimit.Name, ulimit.Soft, ulimit.Hard))
	}
	if spec.PublishAllPorts {
		arguments = append(arguments, "--publish-all")
	}
	for _, publish := range spec.Publish {
		arguments = append(arguments, "--publish", publish)
	}
	for _, volume := range spec.Volumes {
		arguments = append(arguments, "--volume", volume)
	}
	for _, tmpfs := range spec.Tmpfs {
		arguments = append(arguments, "--tmpfs", tmpfs)
	}
	if spec.Memory > 0 {
		arguments = append(arguments, "--memory", strconv.FormatInt(spec.Memory, 10))
	}
	if spec.CPUS > 0 {
		arguments = append(arguments, "--cpus", strconv.FormatFloat(spec.CPUS, 'f', -1, 64))
	}
	if spec.ShmSize > 0 {
		arguments = append(arguments, "--shm-size", strconv.FormatInt(spec.ShmSize, 10))
	}
	if spec.Interactive {
		arguments = append(arguments, "--interactive")
	}
	if spec.TTY {
		arguments = append(arguments, "--tty")
	}
	arguments = append(arguments, spec.Image)
	arguments = append(arguments, spec.Cmd...)
	return arguments
}
