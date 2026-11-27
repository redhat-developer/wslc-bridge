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

package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"wslc-bridge/pkg/buildinfo"
	"wslc-bridge/pkg/dockerapi"
)

const DefaultPipeName = `\\.\pipe\wslc_engine`

type readinessEvent struct {
	Event           string `json:"event"`
	ProtocolVersion int    `json:"protocolVersion"`
	BridgeVersion   string `json:"bridgeVersion"`
	Pipe            string `json:"pipe"`
	GuestAPIVersion string `json:"guestApiVersion"`
}

func Serve(ctx context.Context, pipeName string) error {
	return defaultDependencies().serve(ctx, pipeName)
}

func (d dependencies) serve(ctx context.Context, pipeName string) error {
	runtimeInfo, err := d.detectRuntime(ctx)
	if err != nil {
		return err
	}
	guestClient := d.newGuestClient(runtimeInfo.WSLCPath)
	// The bridge process owns the guest sessions: closing the client
	// terminates and waits for every live wslc.exe session during shutdown,
	// after the HTTP server below has stopped serving requests.
	defer guestClient.Close()
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	versionRequest, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://docker/version", nil)
	if err != nil {
		return &ExitError{Code: 6, Message: err.Error(), Err: err}
	}
	versionResponse, err := guestClient.Do(versionRequest)
	if err != nil {
		return &ExitError{Code: 6, Message: "guest Docker API is unavailable: " + err.Error(), Err: err}
	}
	if versionResponse.StatusCode < 200 || versionResponse.StatusCode >= 300 {
		_ = versionResponse.Body.Close()
		return &ExitError{Code: 6, Message: "guest Docker API returned " + versionResponse.Status}
	}
	var guestVersion struct {
		APIVersion string `json:"ApiVersion"`
	}
	if err := json.NewDecoder(versionResponse.Body).Decode(&guestVersion); err != nil {
		_ = versionResponse.Body.Close()
		return &ExitError{Code: 6, Message: "decode guest Docker version: " + err.Error(), Err: err}
	}
	_ = versionResponse.Body.Close()
	listener, err := d.listenPipe(pipeName)
	if err != nil {
		return &ExitError{Code: 5, Message: err.Error(), Err: err}
	}
	tracker := newConnectionTracker()
	api := dockerapi.NewHandler(d.newNativeClient(runtimeInfo.WSLCPath, runtimeInfo.WSLPath), guestClient)
	server := &http.Server{
		Handler:           api,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ConnState:         tracker.update,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()
	ready := readinessEvent{
		Event: "ready", ProtocolVersion: 1, BridgeVersion: buildinfo.Version,
		Pipe: pipeName, GuestAPIVersion: guestVersion.APIVersion,
	}
	if err := json.NewEncoder(d.stdout).Encode(ready); err != nil {
		_ = server.Close()
		return fmt.Errorf("write bridge readiness event: %w", err)
	}
	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return &ExitError{Code: 5, Message: "named-pipe server stopped: " + err.Error(), Err: err}
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
		tracker.closeHijacked()
		// Hijacked exec streams are no longer tracked by the server: cancel
		// them and wait for their wslc.exe processes to end.
		execCtx, execCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer execCancel()
		_ = api.Shutdown(execCtx)
		_ = server.Close()
		select {
		case <-serveResult:
		case <-time.After(2 * time.Second):
		}
		return nil
	}
}

type stdoutWriter struct{}

func (stdoutWriter) Write(data []byte) (int, error) { return fmt.Print(string(data)) }

type connectionTracker struct {
	mu    sync.Mutex
	state map[net.Conn]http.ConnState
}

func newConnectionTracker() *connectionTracker {
	return &connectionTracker{state: make(map[net.Conn]http.ConnState)}
}

func (t *connectionTracker) update(connection net.Conn, state http.ConnState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if state == http.StateClosed {
		delete(t.state, connection)
		return
	}
	t.state[connection] = state
}

func (t *connectionTracker) closeHijacked() {
	t.mu.Lock()
	connections := make([]net.Conn, 0)
	for connection, state := range t.state {
		if state == http.StateHijacked {
			connections = append(connections, connection)
			delete(t.state, connection)
		}
	}
	t.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
}
