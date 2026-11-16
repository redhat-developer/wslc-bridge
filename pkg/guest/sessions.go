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
	"sync"
)

// MaxSessions bounds the guest subprocesses running at once, active and idle.
// It must stay at or above the transport's MaxIdleConns so idle reuse is
// never starved.
const MaxSessions = 32

// ErrSessionsClosed is returned by dials made after, or interrupted by,
// Sessions.Close.
var ErrSessionsClosed = errors.New("guest sessions are closed")

// Sessions tracks the live guest stdio sessions started by a dialer, so the
// bridge process owns their lifetime explicitly.
//
// There are four lifetimes in the guest transport:
//   - The request: it borrows a pooled connection and returns it; it never
//     owns a subprocess.
//   - The pooled connection: one wslc.exe subprocess per connection. It
//     serves many requests and survives the request that created it, so it
//     runs on a background context. Idle connections are reaped by the
//     transport's IdleConnTimeout; MaxIdleConns caps how many subprocesses
//     stay alive for reuse.
//   - The hijacked exec session: an exec stream runs on the client
//     connection; closing that connection ends the stream.
//   - The bridge process: Sessions.Close waits for in-flight dials, then
//     terminates and waits for every live subprocess during shutdown, after
//     the HTTP server has stopped.
//
// Capacity: at most MaxSessions sessions exist at once. A slot is acquired
// before the dial and released exactly once, when the dial fails, the
// process exits or the connection is closed. A dial at capacity blocks until
// a slot frees, its context is cancelled or the registry is closed.
//
// A session is registered before its dial starts, so Close always sees it
// and no subprocess can escape shutdown.
type Sessions struct {
	dialer   StdioDialer
	slots    chan struct{}
	closedCh chan struct{}

	mu       sync.Mutex
	closed   bool
	sessions map[*session]struct{}
}

type session struct {
	done    chan struct{} // closed once the dial finished, successfully or not
	conn    net.Conn      // set before done is closed; nil if the dial failed
	release sync.Once
}

// sessionConn releases the session when the connection is closed.
type sessionConn struct {
	net.Conn
	onClose func()
}

func (c *sessionConn) Close() error {
	err := c.Conn.Close()
	c.onClose()
	return err
}

// NewSessions returns a session registry that dials through dialer.
func NewSessions(dialer StdioDialer) *Sessions {
	return &Sessions{
		dialer:   dialer,
		slots:    make(chan struct{}, MaxSessions),
		closedCh: make(chan struct{}),
		sessions: make(map[*session]struct{}),
	}
}

// DialContext dials the guest Docker API and registers the session. Sessions
// remove themselves once their subprocess has exited or the connection is
// closed.
func (s *Sessions) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.closedCh:
		return nil, ErrSessionsClosed
	}

	sess := &session{done: make(chan struct{})}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.releaseSlot(sess)
		return nil, ErrSessionsClosed
	}
	s.sessions[sess] = struct{}{}
	s.mu.Unlock()

	conn, err := s.dialer.DialContext(ctx, network, address)
	if err != nil {
		s.finish(sess)
		close(sess.done)
		return nil, err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = conn.Close()
		s.finish(sess)
		close(sess.done)
		return nil, ErrSessionsClosed
	}
	sess.conn = conn
	s.mu.Unlock()
	close(sess.done)

	if stdio, ok := conn.(*stdioConn); ok {
		go func() {
			<-stdio.waitDone
			s.finish(sess)
		}()
	}
	return &sessionConn{Conn: conn, onClose: func() { s.finish(sess) }}, nil
}

func (s *Sessions) finish(sess *session) {
	s.mu.Lock()
	delete(s.sessions, sess)
	s.mu.Unlock()
	s.releaseSlot(sess)
}

func (s *Sessions) releaseSlot(sess *session) {
	sess.release.Do(func() { <-s.slots })
}

// tracked returns the number of live sessions; used by tests.
func (s *Sessions) tracked() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// Close rejects new dials, waits for in-flight dials, closes their late
// connections, then terminates every live session and waits for the
// subprocesses to exit. stdioConn.Close cancels the process, waits up to
// five seconds and then kills it, so a stuck subprocess cannot block
// shutdown indefinitely. Close is idempotent.
func (s *Sessions) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.closedCh)
	pending := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		pending = append(pending, sess)
	}
	s.mu.Unlock()

	var wait sync.WaitGroup
	for _, sess := range pending {
		wait.Add(1)
		go func(sess *session) {
			defer wait.Done()
			<-sess.done
			if sess.conn != nil {
				_ = sess.conn.Close()
			}
		}(sess)
	}
	wait.Wait()
}
