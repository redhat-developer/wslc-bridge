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

package overlay

import (
	"encoding/json"
	"testing"

	"wslc-bridge/pkg/native"
)

func TestInspectOverlaysWindowsPortsAndPreservesOtherFields(t *testing.T) {
	t.Parallel()

	guest := []byte(`{"Id":"abc","Config":{"Image":"nginx"},"State":{"Running":true},"NetworkSettings":{"Ports":{"80/tcp":[{"HostIp":"172.18.0.1","HostPort":"80"}],"81/tcp":null}}}`)
	ports := map[string][]native.PortBinding{
		"80/tcp": {{HostIP: "127.0.0.1", HostPort: "49152"}},
	}
	data, err := Inspect(guest, ports)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Config struct {
			Image string `json:"Image"`
		} `json:"Config"`
		NetworkSettings struct {
			Ports map[string][]native.PortBinding `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value.Config.Image != "nginx" {
		t.Fatalf("unrelated guest field was lost: %#v", value.Config)
	}
	binding := value.NetworkSettings.Ports["80/tcp"][0]
	if binding.HostIP != "127.0.0.1" || binding.HostPort != "49152" {
		t.Fatalf("Windows binding not overlaid: %#v", binding)
	}
	if value.NetworkSettings.Ports["81/tcp"] != nil {
		t.Fatalf("expected unbound port to remain null, got %#v", value.NetworkSettings.Ports["81/tcp"])
	}
}

func TestInspectRetainsConfiguredWindowsPortsForStoppedContainer(t *testing.T) {
	t.Parallel()

	guest := []byte(`{"Id":"stopped","State":{"Running":false},"NetworkSettings":{"Ports":{"80/tcp":null}}}`)
	ports := map[string][]native.PortBinding{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "8080"}}}
	data, err := Inspect(guest, ports)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		NetworkSettings struct {
			Ports map[string][]native.PortBinding `json:"Ports"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if got := value.NetworkSettings.Ports["80/tcp"]; len(got) != 1 || got[0].HostPort != "8080" {
		t.Fatalf("configured stopped-container mapping was lost: %#v", got)
	}
}

func TestSummaryReplacesGuestPortAndHidesStoppedMapping(t *testing.T) {
	t.Parallel()

	guest := []byte(`[{"Id":"running","Ports":[{"PrivatePort":80,"PublicPort":8080,"Type":"tcp","IP":"172.18.0.1"}]},{"Id":"stopped","Ports":[{"PrivatePort":80,"PublicPort":8081,"Type":"tcp","IP":"172.18.0.1"}]}]`)
	ports := map[string]ContainerPorts{
		"running": {Running: true, Ports: map[string][]native.PortBinding{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "49152"}}}},
		"stopped": {Running: false, Ports: map[string][]native.PortBinding{"80/tcp": {{HostIP: "127.0.0.1", HostPort: "49153"}}}},
	}
	data, err := Summary(guest, ports)
	if err != nil {
		t.Fatal(err)
	}
	var values []struct {
		ID    string `json:"Id"`
		Ports []struct {
			PublicPort int    `json:"PublicPort"`
			IP         string `json:"IP"`
		} `json:"Ports"`
	}
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values[0].Ports[0].PublicPort != 49152 || values[0].Ports[0].IP != "127.0.0.1" {
		t.Fatalf("running container has wrong published port: %#v", values[0].Ports)
	}
	if values[1].Ports[0].PublicPort != 0 {
		t.Fatalf("stopped container exposed a Windows host port: %#v", values[1].Ports)
	}
}
