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

// ContainerSpec describes a container to create with WSLC. Callers validate
// Docker-specific semantics and build a spec; native turns it into wslc.exe
// arguments. The spec only covers the feature set the bridge supports.
type ContainerSpec struct {
	Name            string
	Image           string
	Hostname        string
	Domainname      string
	User            string
	WorkingDir      string
	StopSignal      string
	Entrypoint      string
	Env             []string
	Labels          map[string]string
	Network         string
	NetworkAliases  []string
	DNS             []string
	DNSSearch       []string
	DNSOptions      []string
	Ulimits         []Ulimit
	Publish         []string
	Volumes         []string
	Tmpfs           []string
	Memory          int64
	CPUS            float64
	ShmSize         int64
	Interactive     bool
	TTY             bool
	PublishAllPorts bool
	Cmd             []string
}

// Ulimit is a container resource limit.
type Ulimit struct {
	Name string
	Soft int64
	Hard int64
}

// StopSpec describes how to stop a container.
type StopSpec struct {
	ID             string
	Signal         string
	TimeoutSeconds *int
}

// RemoveSpec describes how to remove a container.
type RemoveSpec struct {
	ID    string
	Force bool
}

// NetworkSpec describes a network to create.
type NetworkSpec struct {
	Name    string
	Driver  string
	Options map[string]string
	Labels  map[string]string
}
