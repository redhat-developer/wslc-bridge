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

package dockerapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type dockerPortBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type dockerCreateRequest struct {
	Hostname     string                     `json:"Hostname"`
	Domainname   string                     `json:"Domainname"`
	User         string                     `json:"User"`
	Image        string                     `json:"Image"`
	ExposedPorts map[string]json.RawMessage `json:"ExposedPorts"`
	Healthcheck  json.RawMessage            `json:"Healthcheck"`
	Cmd          []string                   `json:"Cmd"`
	Entrypoint   []string                   `json:"Entrypoint"`
	Env          []string                   `json:"Env"`
	Labels       map[string]string          `json:"Labels"`
	WorkingDir   string                     `json:"WorkingDir"`
	StopSignal   string                     `json:"StopSignal"`
	StopTimeout  *int                       `json:"StopTimeout"`
	ArgsEscaped  bool                       `json:"ArgsEscaped"`
	OpenStdin    bool                       `json:"OpenStdin"`
	AttachStdin  bool                       `json:"AttachStdin"`
	AttachStdout bool                       `json:"AttachStdout"`
	AttachStderr bool                       `json:"AttachStderr"`
	Tty          bool                       `json:"Tty"`
	StdinOnce    bool                       `json:"StdinOnce"`
	// Podman Desktop uses this extension option to request a separate start call after create.
	Start            bool                       `json:"start"`
	Volumes          map[string]json.RawMessage `json:"Volumes"`
	MacAddress       string                     `json:"MacAddress"`
	NetworkDisabled  bool                       `json:"NetworkDisabled"`
	OnBuild          []string                   `json:"OnBuild"`
	Shell            []string                   `json:"Shell"`
	HostConfig       dockerHostConfig           `json:"HostConfig"`
	NetworkingConfig dockerNetworkingConfig     `json:"NetworkingConfig"`
}

type dockerMount struct {
	Type          string          `json:"Type"`
	Source        string          `json:"Source"`
	Target        string          `json:"Target"`
	ReadOnly      bool            `json:"ReadOnly"`
	Consistency   string          `json:"Consistency"`
	BindOptions   json.RawMessage `json:"BindOptions"`
	VolumeOptions json.RawMessage `json:"VolumeOptions"`
	TmpfsOptions  json.RawMessage `json:"TmpfsOptions"`
}

type dockerEndpointConfig struct {
	Aliases    []string          `json:"Aliases"`
	IPAMConfig json.RawMessage   `json:"IPAMConfig"`
	Links      []string          `json:"Links"`
	DriverOpts map[string]string `json:"DriverOpts"`
}

type dockerNetworkingConfig struct {
	EndpointsConfig map[string]dockerEndpointConfig `json:"EndpointsConfig"`
}

type dockerLogConfig struct {
	Type   string            `json:"Type"`
	Config map[string]string `json:"Config"`
}

type dockerHostConfig struct {
	Binds              []string                       `json:"Binds"`
	Mounts             []dockerMount                  `json:"Mounts"`
	PortBindings       map[string][]dockerPortBinding `json:"PortBindings"`
	PublishAllPorts    bool                           `json:"PublishAllPorts"`
	NetworkMode        string                         `json:"NetworkMode"`
	NetworkAliases     []string                       `json:"NetworkAliases"`
	DNS                []string                       `json:"Dns"`
	DNSSearch          []string                       `json:"DnsSearch"`
	DNSOptions         []string                       `json:"DnsOptions"`
	Ulimits            []dockerUlimit                 `json:"Ulimits"`
	Tmpfs              map[string]string              `json:"Tmpfs"`
	LogConfig          dockerLogConfig                `json:"LogConfig"`
	RestartPolicy      dockerRestartPolicy            `json:"RestartPolicy"`
	Privileged         bool                           `json:"Privileged"`
	CapAdd             []string                       `json:"CapAdd"`
	CapDrop            []string                       `json:"CapDrop"`
	Devices            []json.RawMessage              `json:"Devices"`
	ExtraHosts         []string                       `json:"ExtraHosts"`
	ReadonlyRootfs     bool                           `json:"ReadonlyRootfs"`
	Memory             int64                          `json:"Memory"`
	CPUS               float64                        `json:"NanoCPUs"`
	ShmSize            int64                          `json:"ShmSize"`
	AutoRemove         bool                           `json:"AutoRemove"`
	DeviceRequests     []json.RawMessage              `json:"DeviceRequests"`
	SecurityOpt        []string                       `json:"SecurityOpt"`
	PidMode            string                         `json:"PidMode"`
	IpcMode            string                         `json:"IpcMode"`
	UTSMode            string                         `json:"UTSMode"`
	UsernsMode         string                         `json:"UsernsMode"`
	CgroupnsMode       string                         `json:"CgroupnsMode"`
	Runtime            string                         `json:"Runtime"`
	VolumeDriver       string                         `json:"VolumeDriver"`
	MemorySwap         int64                          `json:"MemorySwap"`
	MemoryReservation  int64                          `json:"MemoryReservation"`
	CPUQuota           int64                          `json:"CpuQuota"`
	CPUPeriod          int64                          `json:"CpuPeriod"`
	CPUShares          int64                          `json:"CpuShares"`
	CpusetCpus         string                         `json:"CpusetCpus"`
	CpusetMems         string                         `json:"CpusetMems"`
	CgroupParent       string                         `json:"CgroupParent"`
	GroupAdd           []string                       `json:"GroupAdd"`
	Links              []string                       `json:"Links"`
	VolumesFrom        []string                       `json:"VolumesFrom"`
	Sysctls            map[string]string              `json:"Sysctls"`
	OomScoreAdj        int                            `json:"OomScoreAdj"`
	PidsLimit          int64                          `json:"PidsLimit"`
	CpuRealtimePeriod  int64                          `json:"CpuRealtimePeriod"`
	CpuRealtimeRuntime int64                          `json:"CpuRealtimeRuntime"`
	BlkioWeight        uint16                         `json:"BlkioWeight"`
	MemorySwappiness   int64                          `json:"MemorySwappiness"`
	Init               *bool                          `json:"Init"`
}

type dockerRestartPolicy struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

type dockerUlimit struct {
	Name string `json:"Name"`
	Soft int64  `json:"Soft"`
	Hard int64  `json:"Hard"`
}

func (value *dockerCreateRequest) UnmarshalJSON(data []byte) error {
	type plain dockerCreateRequest
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "container config"); err != nil {
		return err
	}
	*value = dockerCreateRequest(decoded)
	return nil
}

func (value *dockerHostConfig) UnmarshalJSON(data []byte) error {
	type plain dockerHostConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "HostConfig"); err != nil {
		return err
	}
	*value = dockerHostConfig(decoded)
	return nil
}

func (value *dockerMount) UnmarshalJSON(data []byte) error {
	type plain dockerMount
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "Mount"); err != nil {
		return err
	}
	*value = dockerMount(decoded)
	return nil
}

func (value *dockerEndpointConfig) UnmarshalJSON(data []byte) error {
	type plain dockerEndpointConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "network endpoint config"); err != nil {
		return err
	}
	*value = dockerEndpointConfig(decoded)
	return nil
}

func (value *dockerNetworkingConfig) UnmarshalJSON(data []byte) error {
	type plain dockerNetworkingConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "NetworkingConfig"); err != nil {
		return err
	}
	*value = dockerNetworkingConfig(decoded)
	return nil
}

func (value *dockerLogConfig) UnmarshalJSON(data []byte) error {
	type plain dockerLogConfig
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "LogConfig"); err != nil {
		return err
	}
	*value = dockerLogConfig(decoded)
	return nil
}

func (value *dockerRestartPolicy) UnmarshalJSON(data []byte) error {
	type plain dockerRestartPolicy
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "RestartPolicy"); err != nil {
		return err
	}
	*value = dockerRestartPolicy(decoded)
	return nil
}

func (value *dockerUlimit) UnmarshalJSON(data []byte) error {
	type plain dockerUlimit
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := rejectUnknownJSONFields(data, reflect.TypeOf(plain{}), "Ulimit"); err != nil {
		return err
	}
	*value = dockerUlimit(decoded)
	return nil
}

func rejectUnknownJSONFields(data []byte, target reflect.Type, label string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	known := make(map[string]struct{}, target.NumField())
	for index := 0; index < target.NumField(); index++ {
		field := target.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name != "-" {
			known[strings.ToLower(name)] = struct{}{}
		}
	}
	unknown := make([]string, 0)
	for name, raw := range fields {
		if _, ok := known[strings.ToLower(name)]; !ok && !isEmptyJSONValue(raw) {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return fmt.Errorf("%s field %q is not supported by WSLC v1", label, unknown[0])
	}
	return nil
}

func isEmptyJSONValue(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	if bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("false")) || bytes.Equal(trimmed, []byte("0")) || bytes.Equal(trimmed, []byte("\"\"")) || bytes.Equal(trimmed, []byte("[]")) || bytes.Equal(trimmed, []byte("{}")) {
		return true
	}
	if len(trimmed) == 0 {
		return true
	}
	if trimmed[0] == '{' {
		var values map[string]json.RawMessage
		return json.Unmarshal(trimmed, &values) == nil && len(values) == 0
	}
	if trimmed[0] == '[' {
		var values []json.RawMessage
		return json.Unmarshal(trimmed, &values) == nil && len(values) == 0
	}
	if trimmed[0] == '"' {
		var text string
		return json.Unmarshal(trimmed, &text) == nil && text == ""
	}
	return false
}
