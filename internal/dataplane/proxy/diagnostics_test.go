package proxy

import (
	"bytes"
	"context"
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
	for _, ws := range []bool{false, true} {
		t.Run(map[bool]string{false: "HTTP", true: "WebSocket"}[ws], func(t *testing.T) {
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "req-rejected")
				w.Header().Set("X-Private", "private-header")
				w.WriteHeader(403)
				_, _ = io.WriteString(w, "private-response-body")
			}))
			defer upstream.Close()
			var logs bytes.Buffer
			p, err := New(Options{InstanceID: "ins-test", Domain: "example.test", RemotePort: 8080, Token: "test-token", ListenAddress: "127.0.0.1:0", Insecure: true, Logger: log.New(&logs, "", 0), DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
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
				if resp.StatusCode != 403 || string(body) != "private-response-body" {
					t.Fatal("proxy response changed")
				}
			}
			text := logs.String()
			for _, want := range []string{"HTTPStatus:403", "RequestId:req-rejected", "Endpoint:https://"} {
				if ws && want == "Endpoint:https://" {
					want = "Endpoint:wss://"
				}
				if !strings.Contains(text, want) {
					t.Fatalf("missing %s: %s", want, text)
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
