package proxy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/proxy"
	"github.com/gorilla/websocket"
)

func TestRejectedUpstreamDiagnostics(t *testing.T) {
	for _, status := range []int{401, 404, 503} {
		for _, verbose := range []bool{false, true} {
			for _, ws := range []bool{false, true} {
				t.Run(fmt.Sprintf("status=%d/verbose=%v/ws=%v", status, verbose, ws), func(t *testing.T) {
					const requestURI = "/reset/one-time-7f2c?signature=query-secret"
					requests := make(chan string, 1)
					upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests <- r.URL.RequestURI()
						w.Header().Set("X-Request-Id", "req-rejected")
						w.Header().Set("X-Private", "private-header")
						w.WriteHeader(status)
						_, _ = io.WriteString(w, "private-response-body")
					}))
					defer upstream.Close()
					var logs bytes.Buffer
					p, err := proxy.New(proxy.Options{InstanceID: "ins-test", Domain: "example.test", RemotePort: 8080, Token: "test-token", Verbose: verbose, ListenAddress: "127.0.0.1:0", Insecure: true, Logger: log.New(cli.DiagnosticWriter(&logs), "", 0), DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
						return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
					}})
					if err != nil {
						t.Fatal(err)
					}
					addr, err := p.Start()
					if err != nil {
						t.Fatal(err)
					}
					defer p.Stop()
					if ws {
						_, response, err := websocket.DefaultDialer.DialContext(t.Context(), "ws://"+addr+requestURI, nil)
						if response != nil {
							_ = response.Body.Close()
						}
						if err == nil {
							t.Fatal("expected rejection")
						}
					} else {
						resp, err := http.Get("http://" + addr + requestURI)
						if err != nil {
							t.Fatal(err)
						}
						body, _ := io.ReadAll(resp.Body)
						_ = resp.Body.Close()
						if resp.StatusCode != status || string(body) != "private-response-body" {
							t.Fatal("proxy response changed")
						}
					}
					if got := <-requests; got != requestURI {
						t.Fatalf("upstream URI = %q, want %q", got, requestURI)
					}
					text := logs.String()
					quiet := !ws && status < 500 && !verbose
					if quiet && (strings.Contains(text, "[HTTP]") || strings.Contains(text, "[ERROR]")) {
						t.Fatalf("non-verbose business response emitted request logs: %s", text)
					}
					if !ws && !quiet {
						level := "HTTP"
						if status >= 500 {
							level = "ERROR"
						}
						if !strings.Contains(text, "["+level+"] Proxy upstream response:") {
							t.Fatalf("wrong severity: %s", text)
						}
						if status < 500 && strings.Contains(text, "[ERROR]") {
							t.Fatalf("business response marked as proxy failure: %s", text)
						}
					}

					stage := "http_response"
					if ws {
						stage = "ws_handshake"
						if !strings.Contains(text, "TimeoutMs:15000") {
							t.Fatalf("missing handshake budget: %s", text)
						}
					}
					endpoint := "Endpoint:https://8080-ins-test.example.test"
					if ws {
						endpoint = "Endpoint:wss://8080-ins-test.example.test"
					}
					if verbose {
						endpoint += "/reset/one-time-7f2c"
					}
					for _, want := range []string{fmt.Sprintf("HTTPStatus:%d", status), "RequestId:req-rejected", endpoint + " ", "Stage:" + stage} {
						if strings.Contains(text, want) == quiet {
							t.Fatalf("unexpected diagnostic presence for %s (quiet=%v): %s", want, quiet, text)
						}
					}
					if verbose && !ws && !strings.Contains(text, "[HTTP] GET /reset/one-time-7f2c\n") {
						t.Fatalf("verbose request line lost its path: %s", text)
					}
					secrets := []string{"query-secret", "private-response-body", "private-header", "test-token"}
					if !verbose {
						secrets = append(secrets, "one-time-7f2c")
					}
					for _, secret := range secrets {
						if strings.Contains(text, secret) {
							t.Fatalf("diagnostic contains %s: %s", secret, text)
						}
					}
				})
			}
		}
	}
}

func TestProxyTransportFailureDiagnostics(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		for _, ws := range []bool{false, true} {
			t.Run(fmt.Sprintf("verbose=%v/ws=%v", verbose, ws), func(t *testing.T) {
				var logs bytes.Buffer
				p, err := proxy.New(proxy.Options{
					InstanceID: "ins-test", Domain: "example.test", RemotePort: 8080,
					Token: "test-token", Verbose: verbose, ListenAddress: "127.0.0.1:0",
					Logger: log.New(cli.DiagnosticWriter(&logs), "", 0),
					DialContext: func(context.Context, string, string) (net.Conn, error) {
						return nil, errors.New("upstream unavailable")
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				addr, err := p.Start()
				if err != nil {
					t.Fatal(err)
				}
				defer p.Stop()
				const requestURI = "/reset/one-time-7f2c?signature=query-secret"
				if ws {
					_, response, err := websocket.DefaultDialer.DialContext(t.Context(), "ws://"+addr+requestURI, nil)
					if response != nil {
						_ = response.Body.Close()
					}
					if err == nil || response == nil || response.StatusCode != http.StatusBadGateway {
						t.Fatalf("WebSocket response = %v, error = %v", response, err)
					}
				} else {
					response, err := http.Get("http://" + addr + requestURI)
					if err != nil {
						t.Fatal(err)
					}
					_ = response.Body.Close()
					if response.StatusCode != http.StatusBadGateway {
						t.Fatalf("HTTP status = %d", response.StatusCode)
					}
				}
				stage, endpoint := "http_request", "https://8080-ins-test.example.test"
				if ws {
					stage, endpoint = "ws_handshake", "wss://8080-ins-test.example.test"
				}
				secrets := []string{"query-secret", "test-token"}
				if verbose {
					endpoint += "/reset/one-time-7f2c"
				} else {
					secrets = append(secrets, "one-time-7f2c")
				}
				text := logs.String()
				for _, want := range []string{"Stage:" + stage, "Endpoint:" + endpoint + " ", "upstream unavailable"} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing %q: %s", want, text)
					}
				}
				for _, secret := range secrets {
					if strings.Contains(text, secret) {
						t.Fatalf("diagnostic contains %s: %s", secret, text)
					}
				}
			})
		}
	}
}

func TestWebSocketVerboseDiagnosticsOmitQuery(t *testing.T) {
	const requestURI = "/reset/one-time-7f2c?signature=query-secret"
	requests := make(chan string, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.RequestURI()
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("ready"))
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "proxy.log")
	logs, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = logs.Close() }()
	p, err := proxy.New(proxy.Options{
		InstanceID: "ins-test", Domain: "example.test", RemotePort: 8080,
		Token: "test-token", Verbose: true, ListenAddress: "127.0.0.1:0", Insecure: true,
		Logger: log.New(cli.DiagnosticWriter(logs), "", 0),
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	addr, err := p.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	conn, response, err := websocket.DefaultDialer.DialContext(t.Context(), "ws://"+addr+requestURI, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, payload, err := conn.ReadMessage()
	if err != nil || string(payload) != "ready" {
		t.Fatalf("WebSocket payload = %q, error = %v", payload, err)
	}
	if got := <-requests; got != requestURI {
		t.Fatalf("upstream URI = %q, want %q", got, requestURI)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "WebSocket connection established: /reset/one-time-7f2c\n") {
		t.Fatalf("missing connection path: %s", text)
	}
	for _, secret := range []string{"query-secret", "test-token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("persisted secret %q: %s", secret, text)
		}
	}
}
