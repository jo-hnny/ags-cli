package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRejectedUpstreamDiagnostics(t *testing.T) {
	for _, status := range []int{401, 404, 503} {
		for _, verbose := range []bool{false, true} {
			for _, ws := range []bool{false, true} {
				t.Run(fmt.Sprintf("status=%d/verbose=%v/ws=%v", status, verbose, ws), func(t *testing.T) {
					upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("X-Request-Id", "req-rejected")
						w.Header().Set("X-Private", "private-header")
						w.WriteHeader(status)
						_, _ = io.WriteString(w, "private-response-body")
					}))
					defer upstream.Close()
					var logs bytes.Buffer
					p, err := New(Options{InstanceID: "ins-test", Domain: "example.test", RemotePort: 8080, Token: "test-token", Verbose: verbose, ListenAddress: "127.0.0.1:0", Insecure: true, Logger: log.New(&logs, "", 0), DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
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
						_, response, err := websocket.DefaultDialer.DialContext(t.Context(), "ws://"+addr+"/socket?signature=query-secret", nil)
						if response != nil {
							_ = response.Body.Close()
						}
						if err == nil {
							t.Fatal("expected rejection")
						}
					} else {
						resp, err := http.Get("http://" + addr + "/resource?signature=query-secret")
						if err != nil {
							t.Fatal(err)
						}
						body, _ := io.ReadAll(resp.Body)
						_ = resp.Body.Close()
						if resp.StatusCode != status || string(body) != "private-response-body" {
							t.Fatal("proxy response changed")
						}
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

					for _, want := range []string{fmt.Sprintf("HTTPStatus:%d", status), "RequestId:req-rejected", "Endpoint:https://"} {
						if ws && want == "Endpoint:https://" {
							want = "Endpoint:wss://"
						}
						if strings.Contains(text, want) == quiet {
							t.Fatalf("unexpected diagnostic presence for %s (quiet=%v): %s", want, quiet, text)
						}
					}
					for _, secret := range []string{"query-secret", "private-response-body", "private-header", "test-token"} {
						if strings.Contains(text, secret) {
							t.Fatalf("diagnostic contains %s", secret)
						}
					}
				})
			}
		}
	}
}
