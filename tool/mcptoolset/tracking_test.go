// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mcptoolset

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"google.golang.org/adk/v2/auth"
	"google.golang.org/adk/v2/internal/version"
)

// headerRecorder records the headers of every request a test server receives.
type headerRecorder struct {
	mu      sync.Mutex
	headers []http.Header
}

func (r *headerRecorder) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.headers = append(r.headers, req.Header.Clone())
		r.mu.Unlock()
		next.ServeHTTP(w, req)
	})
}

func (r *headerRecorder) all() []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]http.Header(nil), r.headers...)
}

func TestTrackingHeadersOnMCPRequests(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "ua_server", Version: "v1"}, nil)
	want := version.TrackingHeader()

	tests := []struct {
		name      string
		handler   http.Handler
		transport func(url string) Config
		// wantAuth is the Authorization header every request must carry, if any.
		wantAuth string
	}{
		{
			name:    "endpoint",
			handler: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil),
			transport: func(url string) Config {
				return Config{Endpoint: url}
			},
		},
		{
			name:    "caller streamable transport with its own client",
			handler: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil),
			transport: func(url string) Config {
				return Config{Transport: &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: &http.Client{}}}
			},
		},
		{
			name:    "endpoint with auth",
			handler: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil),
			transport: func(url string) Config {
				return Config{Endpoint: url, Auth: auth.StaticToken("tok")}
			},
			wantAuth: "Bearer tok",
		},
		{
			name:    "caller SSE transport",
			handler: mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil),
			transport: func(url string) Config {
				return Config{Transport: &mcp.SSEClientTransport{Endpoint: url}}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &headerRecorder{}
			srv := httptest.NewServer(rec.wrap(tt.handler))
			defer srv.Close()

			tr, err := buildTransport(tt.transport(srv.URL))
			if err != nil {
				t.Fatalf("buildTransport() error = %v", err)
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "ua_client", Version: "v1"}, nil)
			session, err := client.Connect(t.Context(), tr, nil)
			if err != nil {
				t.Fatalf("Connect() error = %v", err)
			}
			if _, err := session.ListTools(t.Context(), nil); err != nil {
				t.Fatalf("ListTools() error = %v", err)
			}
			if err := session.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			got := rec.all()
			if len(got) == 0 {
				t.Fatal("server received no requests")
			}
			for i, h := range got {
				for _, name := range []string{"User-Agent", "X-Goog-Api-Client"} {
					if v := h.Values(name); len(v) != 1 || v[0] != want {
						t.Errorf("request %d: %s = %q, want [%q]", i, name, v, want)
					}
				}
				if tt.wantAuth != "" && h.Get("Authorization") != tt.wantAuth {
					t.Errorf("request %d: Authorization header missing or wrong", i)
				}
			}
		})
	}
}

func TestTrackingTransportMergesCallerHeaders(t *testing.T) {
	adk := version.TrackingHeader()
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{name: "no caller value", header: http.Header{}, want: adk},
		{name: "caller value is kept after ADK tokens", header: http.Header{"User-Agent": {"my-app/1.0"}}, want: adk + " my-app/1.0"},
		{name: "duplicate tokens are dropped", header: http.Header{"User-Agent": {"my-app/1.0 " + adk}}, want: adk + " my-app/1.0"},
		{name: "repeated header values are joined", header: http.Header{"User-Agent": {"a/1", "b/2"}}, want: adk + " a/1 b/2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://mcp.example/mcp", nil)
			req.Header = tt.header
			orig := tt.header.Clone()
			var sent *http.Request
			rt := &trackingTransport{base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				sent = r
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
			})}
			if _, err := rt.RoundTrip(req); err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}
			if got := sent.Header.Values("User-Agent"); len(got) != 1 || got[0] != tt.want {
				t.Errorf("User-Agent = %q, want [%q]", got, tt.want)
			}
			if got := sent.Header.Values("X-Goog-Api-Client"); len(got) != 1 || got[0] != adk {
				t.Errorf("X-Goog-Api-Client = %q, want [%q]", got, adk)
			}
			if len(req.Header) != len(orig) || req.Header.Get("User-Agent") != orig.Get("User-Agent") {
				t.Errorf("caller request header was mutated: got %v, want %v", req.Header, orig)
			}
		})
	}
}

// A nil HTTPClient means http.DefaultClient to the SDK, so the tracking client
// must start from it rather than from a zero client. Not parallel: it swaps a
// package-level variable.
func TestTrackingClientStartsFromDefaultClient(t *testing.T) {
	orig := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = orig })
	http.DefaultClient = &http.Client{Timeout: 42 * time.Second}

	for _, cfg := range []Config{
		{Endpoint: testEndpoint},
		{Endpoint: testEndpoint, Auth: auth.StaticToken("tok")},
		{Transport: &mcp.SSEClientTransport{Endpoint: testEndpoint}},
	} {
		tr, err := buildTransport(cfg)
		if err != nil {
			t.Fatalf("buildTransport(%+v) error = %v", cfg, err)
		}
		var c *http.Client
		switch tr := tr.(type) {
		case *mcp.StreamableClientTransport:
			c = tr.HTTPClient
		case *mcp.SSEClientTransport:
			c = tr.HTTPClient
		}
		if c.Timeout != 42*time.Second {
			t.Errorf("%T (auth %v): HTTPClient.Timeout = %v, want http.DefaultClient's 42s", tr, cfg.Auth != nil, c.Timeout)
		}
		if c == http.DefaultClient {
			t.Errorf("%T: http.DefaultClient was used directly instead of copied", tr)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
