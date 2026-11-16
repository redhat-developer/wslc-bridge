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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"wslc-bridge/pkg/native"
)

func (h *handler) createContainer(w http.ResponseWriter, r *http.Request) {
	var request dockerCreateRequest
	if err := decodeRequest(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request.Image = strings.TrimSpace(request.Image)
	spec, err := buildContainerSpec(request, r.URL.Query().Get("name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// WSLC create implicitly pulls a missing image. Preflight against the guest
	// image API to preserve Docker's create-without-pull behavior in the common case.
	imageInspect := r.Clone(r.Context())
	imageInspect.Method = http.MethodGet
	imageInspect.URL = cloneURL(r.URL)
	imageInspect.URL.Path = "/images/" + request.Image + "/json"
	imageInspect.URL.RawPath = ""
	imageInspect.Body = nil
	imageInspect.ContentLength = 0
	imageResponse, imageErr := h.doGuest(r.Context(), imageInspect)
	if imageErr != nil {
		writeError(w, http.StatusBadGateway, "could not verify image before create: "+imageErr.Error())
		return
	}
	_ = imageResponse.Body.Close()
	if imageResponse.StatusCode == http.StatusNotFound {
		writeError(w, http.StatusNotFound, "image must be pulled before container create")
		return
	}
	if imageResponse.StatusCode < 200 || imageResponse.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "guest Docker image inspect returned "+imageResponse.Status)
		return
	}
	id, err := h.native.CreateContainer(r.Context(), spec)
	if err != nil {
		writeError(w, commandStatus(err), "wslc container create failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"Id": id, "Warnings": []string{}})
}

// buildContainerSpec validates the Docker request against the feature set
// WSLC supports and translates it into a native.ContainerSpec. Docker
// semantics live here; wslc.exe argument construction lives in pkg/native.
func buildContainerSpec(request dockerCreateRequest, name string) (native.ContainerSpec, error) {
	image := strings.TrimSpace(request.Image)
	if image == "" {
		return native.ContainerSpec{}, errors.New("Image is required")
	}
	if name != "" && !validNamePattern.MatchString(name) {
		return native.ContainerSpec{}, errors.New("container name contains unsupported characters")
	}
	if hasUnsupportedHostConfig(request.HostConfig) {
		return native.ContainerSpec{}, errors.New("requested host configuration is not supported by WSLC v1")
	}
	if request.NetworkDisabled || request.MacAddress != "" || len(request.OnBuild) > 0 || len(request.Shell) > 0 || request.AttachStdin || request.AttachStdout || request.AttachStderr || request.StdinOnce || request.ArgsEscaped || request.StopTimeout != nil {
		return native.ContainerSpec{}, errors.New("requested container configuration is not supported by WSLC v1")
	}
	if len(request.Healthcheck) > 0 && string(request.Healthcheck) != "null" {
		return native.ContainerSpec{}, errors.New("container health checks are not supported by WSLC 2.9.3")
	}
	if len(request.Volumes) > 0 {
		return native.ContainerSpec{}, errors.New("anonymous volume declarations are not supported; specify a Windows bind or named volume")
	}
	if len(request.Entrypoint) > 1 {
		return native.ContainerSpec{}, errors.New("WSLC supports only a single entrypoint value")
	}
	if request.HostConfig.Memory < 0 || request.HostConfig.CPUS < 0 || request.HostConfig.ShmSize < 0 {
		return native.ContainerSpec{}, errors.New("memory, NanoCPUs, and ShmSize must not be negative")
	}
	if err := validateExposedPorts(request.ExposedPorts, request.HostConfig.PortBindings, request.HostConfig.PublishAllPorts); err != nil {
		return native.ContainerSpec{}, err
	}
	networkMode := strings.TrimSpace(request.HostConfig.NetworkMode)
	if networkMode == "host" || strings.HasPrefix(networkMode, "container:") || networkMode == "none" {
		return native.ContainerSpec{}, fmt.Errorf("network mode %q is not supported", networkMode)
	}
	if networkMode == "default" {
		networkMode = "bridge"
	}
	spec := native.ContainerSpec{
		Name:            name,
		Image:           image,
		Hostname:        request.Hostname,
		Domainname:      request.Domainname,
		User:            request.User,
		WorkingDir:      request.WorkingDir,
		StopSignal:      request.StopSignal,
		Env:             append([]string(nil), request.Env...),
		Labels:          request.Labels,
		NetworkAliases:  append([]string(nil), request.HostConfig.NetworkAliases...),
		DNS:             append([]string(nil), request.HostConfig.DNS...),
		DNSSearch:       append([]string(nil), request.HostConfig.DNSSearch...),
		DNSOptions:      append([]string(nil), request.HostConfig.DNSOptions...),
		Memory:          request.HostConfig.Memory,
		CPUS:            request.HostConfig.CPUS / 1e9,
		ShmSize:         request.HostConfig.ShmSize,
		Interactive:     request.OpenStdin,
		TTY:             request.Tty,
		PublishAllPorts: request.HostConfig.PublishAllPorts,
		Cmd:             append([]string(nil), request.Cmd...),
	}
	if len(request.Entrypoint) == 1 {
		spec.Entrypoint = request.Entrypoint[0]
	}
	for _, value := range spec.Env {
		if !strings.Contains(value, "=") {
			return native.ContainerSpec{}, fmt.Errorf("environment entry %q must include an explicit value", value)
		}
	}
	for _, value := range request.HostConfig.Ulimits {
		if value.Name == "" {
			return native.ContainerSpec{}, errors.New("ulimit Name is required")
		}
		spec.Ulimits = append(spec.Ulimits, native.Ulimit{Name: value.Name, Soft: value.Soft, Hard: value.Hard})
	}
	if networkMode != "" && networkMode != "bridge" {
		spec.Network = networkMode
	}
	if len(request.NetworkingConfig.EndpointsConfig) > 1 {
		return native.ContainerSpec{}, errors.New("multiple network attachments are not supported")
	}
	endpointNetworks := make([]string, 0, len(request.NetworkingConfig.EndpointsConfig))
	for network := range request.NetworkingConfig.EndpointsConfig {
		endpointNetworks = append(endpointNetworks, network)
	}
	sort.Strings(endpointNetworks)
	for _, network := range endpointNetworks {
		endpoint := request.NetworkingConfig.EndpointsConfig[network]
		if len(endpoint.Links) > 0 || len(endpoint.DriverOpts) > 0 {
			return native.ContainerSpec{}, errors.New("network endpoint links and driver options are not supported")
		}
		if len(endpoint.IPAMConfig) > 0 && string(endpoint.IPAMConfig) != "null" && string(endpoint.IPAMConfig) != "{}" {
			return native.ContainerSpec{}, errors.New("per-network IPAM settings are not supported")
		}
		if networkMode != "" && networkMode != "bridge" && networkMode != network {
			return native.ContainerSpec{}, errors.New("multiple network attachments are not supported")
		}
		if network != "bridge" && network != networkMode {
			spec.Network = network
		}
		spec.NetworkAliases = append(spec.NetworkAliases, endpoint.Aliases...)
	}
	portKeys := make([]string, 0, len(request.HostConfig.PortBindings))
	for key := range request.HostConfig.PortBindings {
		portKeys = append(portKeys, key)
	}
	sort.Strings(portKeys)
	for _, key := range portKeys {
		for _, binding := range request.HostConfig.PortBindings[key] {
			publish, err := dockerPublishSpec(key, binding)
			if err != nil {
				return native.ContainerSpec{}, err
			}
			spec.Publish = append(spec.Publish, publish)
		}
	}
	for _, bind := range request.HostConfig.Binds {
		mapped, err := mapBind(bind)
		if err != nil {
			return native.ContainerSpec{}, err
		}
		spec.Volumes = append(spec.Volumes, mapped)
	}
	for _, mount := range request.HostConfig.Mounts {
		if mount.Type != "bind" && mount.Type != "volume" {
			return native.ContainerSpec{}, fmt.Errorf("mount type %q is not supported", mount.Type)
		}
		if mount.Source == "" || mount.Target == "" || !strings.HasPrefix(mount.Target, "/") {
			return native.ContainerSpec{}, errors.New("mount Source and Target are required")
		}
		if mount.Consistency != "" || hasMountOptions(mount.BindOptions) || hasMountOptions(mount.VolumeOptions) || hasMountOptions(mount.TmpfsOptions) {
			return native.ContainerSpec{}, errors.New("mount propagation, consistency, volume, and tmpfs options are not supported")
		}
		mode := "rw"
		if mount.ReadOnly {
			mode = "ro"
		}
		spec.Volumes = append(spec.Volumes, mount.Source+":"+mount.Target+":"+mode)
	}
	tmpfsTargets := make([]string, 0, len(request.HostConfig.Tmpfs))
	for target := range request.HostConfig.Tmpfs {
		tmpfsTargets = append(tmpfsTargets, target)
	}
	sort.Strings(tmpfsTargets)
	for _, target := range tmpfsTargets {
		options := request.HostConfig.Tmpfs[target]
		value := target
		if options != "" {
			value += ":" + options
		}
		spec.Tmpfs = append(spec.Tmpfs, value)
	}
	return spec, nil
}

func hasUnsupportedHostConfig(config dockerHostConfig) bool {
	return config.Privileged || config.RestartPolicy.Name != "" || config.RestartPolicy.MaximumRetryCount != 0 ||
		len(config.CapAdd) > 0 || len(config.CapDrop) > 0 || len(config.Devices) > 0 || len(config.DeviceRequests) > 0 ||
		len(config.ExtraHosts) > 0 || config.ReadonlyRootfs || config.AutoRemove || len(config.SecurityOpt) > 0 || config.LogConfig.Type != "" || len(config.LogConfig.Config) > 0 ||
		config.PidMode != "" || config.IpcMode != "" || config.UTSMode != "" || config.UsernsMode != "" || config.CgroupnsMode != "" ||
		config.Runtime != "" || config.VolumeDriver != "" || config.MemorySwap != 0 || config.MemoryReservation != 0 ||
		config.CPUQuota != 0 || config.CPUPeriod != 0 || config.CPUShares != 0 || config.CpusetCpus != "" || config.CpusetMems != "" ||
		config.CgroupParent != "" || len(config.GroupAdd) > 0 || len(config.Links) > 0 || len(config.VolumesFrom) > 0 || len(config.Sysctls) > 0 ||
		config.OomScoreAdj != 0 || config.PidsLimit != 0 || config.CpuRealtimePeriod != 0 || config.CpuRealtimeRuntime != 0 ||
		config.BlkioWeight != 0 || config.MemorySwappiness != 0 || config.Init != nil
}

func hasMountOptions(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
}

func dockerPublishSpec(key string, binding dockerPortBinding) (string, error) {
	port, protocol, ok := parsePublishPortKey(key)
	if !ok {
		return "", fmt.Errorf("published container port %q is invalid", key)
	}
	hostIP := strings.TrimSpace(binding.HostIP)
	if hostIP == "" {
		hostIP = "0.0.0.0"
	}
	parsedIP := net.ParseIP(hostIP)
	if parsedIP == nil {
		return "", fmt.Errorf("published host IP %q is invalid", hostIP)
	}
	if parsedIP.To4() == nil {
		return "", fmt.Errorf("IPv6 published host IP %q is not supported by WSLC v1", hostIP)
	}
	hostPort := strings.TrimSpace(binding.HostPort)
	if hostPort == "0" {
		hostPort = ""
	}
	if hostPort != "" {
		value, err := strconv.Atoi(hostPort)
		if err != nil || value < 1 || value > 65535 {
			return "", fmt.Errorf("published host port %q must be between 1 and 65535 or empty for dynamic allocation", hostPort)
		}
	}
	return fmt.Sprintf("%s:%s:%d/%s", hostIP, hostPort, port, protocol), nil
}

func parsePublishPortKey(value string) (int, string, bool) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || (parts[1] != "tcp" && parts[1] != "udp") {
		return 0, "", false
	}
	port, err := strconv.Atoi(parts[0])
	return port, parts[1], err == nil && port > 0 && port <= 65535
}

func validateExposedPorts(exposed map[string]json.RawMessage, bindings map[string][]dockerPortBinding, publishAll bool) error {
	for port := range exposed {
		if _, published := bindings[port]; published || publishAll {
			continue
		}
		return fmt.Errorf("exposed port %q has no WSLC CLI mapping; publish it explicitly or use PublishAllPorts", port)
	}
	return nil
}

func mapBind(value string) (string, error) {
	separator := strings.Index(value, ":")
	if separator == 1 && len(value) > 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		separator = strings.Index(value[2:], ":")
		if separator >= 0 {
			separator += 2
		}
	}
	if separator <= 0 || separator == len(value)-1 {
		return "", fmt.Errorf("bind %q must be source:container-path[:ro|rw]", value)
	}
	source := value[:separator]
	parts := strings.Split(value[separator+1:], ":")
	if len(parts) < 1 || len(parts) > 2 || !strings.HasPrefix(parts[0], "/") {
		return "", fmt.Errorf("bind %q must use an absolute container path and optional ro/rw mode", value)
	}
	mode := "rw"
	if len(parts) == 2 {
		mode = parts[1]
	}
	if mode != "ro" && mode != "rw" {
		return "", fmt.Errorf("bind mode %q is unsupported; use ro or rw", mode)
	}
	return source + ":" + parts[0] + ":" + mode, nil
}
