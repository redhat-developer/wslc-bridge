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

package guest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

// fakeDialer is a StdioDialer that hands out recorded fake connections.
type fakeDialer struct {
	dialed int
	conns  []*fakeConn
}

func (d *fakeDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	conn := &fakeConn{}
	d.conns = append(d.conns, conn)
	d.dialed++
	return conn, nil
}

type fakeConn struct {
	net.Conn
	mu     sync.Mutex
	closed bool
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func TestSessionsTrackDialledConnections(t *testing.T) {
	t.Parallel()

	dialer := &fakeDialer{}
	sessions := NewSessions(dialer)
	if _, err := sessions.DialContext(context.Background(), "tcp", "ignored"); err != nil {
		t.Fatal(err)
	}
	if dialer.dialed != 1 {
		t.Fatalf("dialer.dialed = %d, want 1", dialer.dialed)
	}
	sessions.Close()
	if !dialer.conns[0].closed {
		t.Fatal("Close did not close the live session")
	}
}

func TestSessionsCloseClosesEveryLiveSession(t *testing.T) {
	t.Parallel()

	dialer := &fakeDialer{}
	sessions := NewSessions(dialer)
	for i := 0; i < 3; i++ {
		if _, err := sessions.DialContext(context.Background(), "tcp", "ignored"); err != nil {
			t.Fatal(err)
		}
	}
	sessions.Close()
	for index, conn := range dialer.conns {
		if !conn.closed {
			t.Fatalf("session %d was not closed", index)
		}
	}
}

func TestSessionsCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	dialer := &fakeDialer{}
	sessions := NewSessions(dialer)
	if _, err := sessions.DialContext(context.Background(), "tcp", "ignored"); err != nil {
		t.Fatal(err)
	}
	sessions.Close()
	sessions.Close()
}

func TestSessionsRemoveFinishedSessions(t *testing.T) {
	t.Parallel()

	dialer := newStdioDialer(os.Args[0],
		[]string{"-test.run=TestStdioHelperProcess"},
		[]string{"WSLC_BRIDGE_STDIO_HELPER=1"},
	)
	sessions := NewSessions(dialer)
	conn, err := sessions.DialContext(context.Background(), "tcp", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if sessions.tracked() != 1 {
		t.Fatalf("tracked = %d, want 1 while the session is live", sessions.tracked())
	}
	stdio := conn.(*sessionConn).Conn.(*stdioConn)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	<-stdio.waitDone
	// The session unregisters itself asynchronously once its process exits.
	deadline := time.Now().Add(5 * time.Second)
	for sessions.tracked() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sessions.tracked() != 0 {
		t.Fatalf("tracked = %d, want 0 after the session exited", sessions.tracked())
	}
}

// blockingDialer blocks every dial until release is closed.
type blockingDialer struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	conns   []*fakeConn
}

func newBlockingDialer() *blockingDialer {
	return &blockingDialer{started: make(chan struct{}, 64), release: make(chan struct{})}
}

func (d *blockingDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	d.started <- struct{}{}
	<-d.release
	conn := &fakeConn{}
	d.mu.Lock()
	d.conns = append(d.conns, conn)
	d.mu.Unlock()
	return conn, nil
}

func (d *blockingDialer) lateConnClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.conns) == 1 && d.conns[0].isClosed()
}

// pipeDialer hands out net.Pipe connections, or fails when err is set.
type pipeDialer struct {
	err error
}

func (d *pipeDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	if d.err != nil {
		return nil, d.err
	}
	client, _ := net.Pipe()
	return client, nil
}

func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func TestSessionsCloseWaitsForPendingDialAndClosesTheLateConnection(t *testing.T) {
	t.Parallel()

	dialer := newBlockingDialer()
	sessions := NewSessions(dialer)
	dialErr := make(chan error, 1)
	go func() {
		_, err := sessions.DialContext(context.Background(), "tcp", "ignored")
		dialErr <- err
	}()
	<-dialer.started

	closed := make(chan struct{})
	go func() {
		sessions.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while a dial was pending")
	case <-time.After(50 * time.Millisecond):
	}

	close(dialer.release)
	if err := <-dialErr; !errors.Is(err, ErrSessionsClosed) {
		t.Fatalf("dial error = %v, want ErrSessionsClosed", err)
	}
	<-closed
	if !dialer.lateConnClosed() {
		t.Fatal("the late connection was not closed")
	}
	if sessions.tracked() != 0 {
		t.Fatalf("tracked = %d, want 0", sessions.tracked())
	}
}

func TestSessionsDialAfterCloseFailsFast(t *testing.T) {
	t.Parallel()

	dialer := &fakeDialer{}
	sessions := NewSessions(dialer)
	sessions.Close()
	if _, err := sessions.DialContext(context.Background(), "tcp", "ignored"); !errors.Is(err, ErrSessionsClosed) {
		t.Fatalf("dial error = %v, want ErrSessionsClosed", err)
	}
	if dialer.dialed != 0 {
		t.Fatalf("dialer.dialed = %d, want 0", dialer.dialed)
	}
}

func fillSessions(t *testing.T, sessions *Sessions) []net.Conn {
	t.Helper()
	conns := make([]net.Conn, 0, MaxSessions)
	for i := 0; i < MaxSessions; i++ {
		conn, err := sessions.DialContext(context.Background(), "tcp", "ignored")
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, conn)
	}
	return conns
}

func TestSessionsCapacityBlocksUntilASlotFrees(t *testing.T) {
	t.Parallel()

	sessions := NewSessions(&pipeDialer{})
	conns := fillSessions(t, sessions)

	result := make(chan error, 1)
	go func() {
		_, err := sessions.DialContext(context.Background(), "tcp", "ignored")
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("dial returned at capacity: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	_ = conns[0].Close()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dial did not proceed after a slot freed")
	}
	sessions.Close()
}

func TestSessionsCapacityHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	sessions := NewSessions(&pipeDialer{})
	conns := fillSessions(t, sessions)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := sessions.DialContext(ctx, "tcp", "ignored")
		result <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("dial error = %v, want context.Canceled", err)
	}

	_ = conns[0].Close()
	conn, err := sessions.DialContext(context.Background(), "tcp", "ignored")
	if err != nil {
		t.Fatalf("slot leaked after cancellation: %v", err)
	}
	_ = conn.Close()
	sessions.Close()
}

func TestSessionsDialFailureReleasesSlot(t *testing.T) {
	t.Parallel()

	dialErr := errors.New("boom")
	sessions := NewSessions(&pipeDialer{err: dialErr})
	for i := 0; i < MaxSessions+1; i++ {
		if _, err := sessions.DialContext(context.Background(), "tcp", "ignored"); !errors.Is(err, dialErr) {
			t.Fatalf("dial %d error = %v, want %v", i, err, dialErr)
		}
	}
	if sessions.tracked() != 0 {
		t.Fatalf("tracked = %d, want 0", sessions.tracked())
	}
}

func TestClientCloseDuringInFlightRequest(t *testing.T) {
	t.Parallel()

	dialer := newBlockingDialer()
	sessions := NewSessions(dialer)
	client := &http.Client{Transport: &http.Transport{DialContext: sessions.DialContext}}

	requestErr := make(chan error, 1)
	go func() {
		resp, err := client.Get("http://docker/version")
		if err == nil {
			_ = resp.Body.Close()
		}
		requestErr <- err
	}()
	<-dialer.started

	closed := make(chan struct{})
	go func() {
		sessions.Close()
		close(closed)
	}()
	time.Sleep(20 * time.Millisecond)
	close(dialer.release)

	if err := <-requestErr; err == nil {
		t.Fatal("request succeeded during shutdown")
	}
	<-closed
	if !dialer.lateConnClosed() {
		t.Fatal("connection left open after shutdown")
	}
	if sessions.tracked() != 0 {
		t.Fatalf("tracked = %d, want 0", sessions.tracked())
	}
}
