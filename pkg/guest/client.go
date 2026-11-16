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
	"net/http"
	"net/url"
	"time"
)

// Client reaches the guest Docker API over WSLC stdio sessions. It embeds
// *http.Client, so it satisfies any consumer that only needs Do; Close adds
// the shutdown path for the bridge process that owns the sessions.
type Client struct {
	*http.Client
	sessions *Sessions
}

// NewClient returns a Client that reaches the guest Docker API over a WSLC
// stdio session. Each pooled connection runs one wslc.exe subprocess; at
// most MaxSessions (32) run at once, at most 16 idle subprocesses are kept
// alive for reuse, and idle ones are reaped after 90 seconds.
func NewClient(wslcPath string) *Client {
	sessions := NewSessions(NewStdioDialer(wslcPath))
	transport := &http.Transport{
		DialContext:           sessions.DialContext,
		DisableCompression:    true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 0,
		ForceAttemptHTTP2:     false,
		Proxy:                 func(*http.Request) (*url.URL, error) { return nil, nil },
	}
	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{Client: httpClient, sessions: sessions}
}

// Close waits for in-flight dials, terminates every live guest session, waits
// for the subprocesses to exit and then closes the idle pooled connections.
// The bridge calls it during shutdown, after the HTTP server has stopped.
func (c *Client) Close() {
	if c.sessions != nil {
		c.sessions.Close()
	}
	c.CloseIdleConnections()
}
