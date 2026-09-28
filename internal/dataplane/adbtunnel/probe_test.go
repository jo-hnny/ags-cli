package adbtunnel

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/gorilla/websocket"
)

func TestProbeRetainsHandshakeStatusWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-TC-RequestId", "request-test")
		w.Header().Set("Authorization", "secret-response-header")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("sensitive-response-body"))
	}))
	defer server.Close()
	tunnel := &Tunnel{ctx: t.Context(), wsURL: "ws" + strings.TrimPrefix(server.URL, "http"), options: TunnelOptions{TokenProvider: func() (string, error) { return "fake-token", nil }}}
	err := tunnel.Probe()
	var handshake *HandshakeError
	if !errors.As(err, &handshake) || handshake.HTTPStatus != 403 || errors.Unwrap(handshake) == nil {
		t.Fatalf("missing handshake observation: %v", err)
	}
	if strings.Contains(err.Error(), "sensitive-response-body") {
		t.Fatal("response body leaked")
	}
	if handshake.Endpoint != tunnel.wsURL || handshake.TimeoutMs != 10000 || handshake.RequestID != "request-test" || strings.Contains(err.Error(), "secret-response-header") {
		t.Fatalf("missing or unsafe context: %#v", handshake)
	}
	local, peer := net.Pipe()
	defer func() { _ = local.Close(); _ = peer.Close() }()
	_, err = tunnel.handleConnection(local)
	if !errors.As(err, &handshake) || handshake.TimeoutMs != 15000 || handshake.RequestID != "request-test" || handshake.Endpoint != tunnel.wsURL {
		t.Fatalf("runtime handshake context lost: %v", err)
	}
}

func TestProbePreservesTokenFailure(t *testing.T) {
	cause := errors.New("token failed")
	tunnel := &Tunnel{ctx: t.Context(), options: TunnelOptions{TokenProvider: func() (string, error) { return "", cause }}}
	err := tunnel.Probe()
	var handshake *HandshakeError
	if !errors.Is(err, cause) || errors.As(err, &handshake) {
		t.Fatalf("token misclassified: %v", err)
	}
}

func TestProbePreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tunnel := &Tunnel{ctx: ctx, wsURL: "ws://127.0.0.1:1", options: TunnelOptions{TokenProvider: func() (string, error) { return "test", nil }}}
	if err := tunnel.Probe(); !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "\n") {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestHandshakeTimeoutMessageIsSingleLine(t *testing.T) {
	cause := errors.Join(errors.New("transport timeout"), context.DeadlineExceeded)
	err := &HandshakeError{Cause: cause}
	if strings.Contains(err.Error(), "\n") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
}

func TestHandshakeNetworkObservations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{"dns", &net.DNSError{Err: "no such host", Name: "endpoint.invalid", IsNotFound: true}},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}},
		{"transport_timeout_before_deadline", &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tunnel := &Tunnel{wsURL: "wss://endpoint.invalid/adb/ws", options: TunnelOptions{TokenProvider: func() (string, error) { return "test", nil }}}
			dialer := &websocket.Dialer{NetDialContext: func(context.Context, string, string) (net.Conn, error) { return nil, tc.cause }}
			_, err := tunnel.dialWebSocket(t.Context(), dialer, probeTimeout)
			var handshake *HandshakeError
			if !errors.As(err, &handshake) || !errors.Is(err, tc.cause) || handshake.Endpoint != tunnel.wsURL || handshake.TimeoutMs != 10000 || handshake.HTTPStatus != 0 || handshake.RequestID != "" {
				t.Fatalf("observations=%v", err)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("unexpired context was classified as deadline exceeded")
			}
		})
	}
}

func TestProbeHandshakeDeadline(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	tunnel := &Tunnel{ctx: ctx, wsURL: "ws" + strings.TrimPrefix(server.URL, "http"), options: TunnelOptions{TokenProvider: func() (string, error) { return "test", nil }}}
	err := tunnel.Probe()
	var handshake *HandshakeError
	if !errors.As(err, &handshake) || !errors.Is(err, context.DeadlineExceeded) || handshake.TimeoutMs <= 0 || handshake.TimeoutMs > 200 || handshake.HTTPStatus != 0 {
		t.Fatalf("deadline context lost: %v", err)
	}
	if failure := output.ClassifyError(err).Failure; failure.Code != "TIMEOUT" {
		t.Fatalf("deadline misclassified: %#v", failure)
	}
	select {
	case <-entered:
	default:
		t.Fatal("handshake did not reach the local server")
	}
}

func TestProbeConnectionRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "ws://" + listener.Addr().String() + "/adb/ws"
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	tunnel := &Tunnel{ctx: t.Context(), wsURL: endpoint, options: TunnelOptions{TokenProvider: func() (string, error) { return "test", nil }}}
	err = tunnel.Probe()
	var handshake *HandshakeError
	var network *net.OpError
	if !errors.As(err, &handshake) || !errors.As(err, &network) || handshake.Endpoint != endpoint || handshake.HTTPStatus != 0 || handshake.RequestID != "" {
		t.Fatalf("missing connection failure context: %v", err)
	}
}

func TestDiagnosticEndpointAndRequestID(t *testing.T) {
	if got := diagnosticEndpoint("wss://user:password@host/adb/ws?Signature=secret&plain=value#private"); got != "wss://host/adb/ws" {
		t.Fatalf("unsafe endpoint: %s", got)
	}
	for _, tc := range []struct{ name, value, want string }{
		{"X-TC-RequestId", "req-1", "req-1"},
		{"X-Request-Id", "req-2", "req-2"},
		{"Authorization", "Bearer secret", ""},
		{"Trace-Id", "unknown", ""},
		{"X-Request-Id", strings.Repeat("a", 257), ""},
		{"X-Request-Id", "line\nbreak", ""},
	} {
		headers := http.Header{}
		headers.Set(tc.name, tc.value)
		if got := responseRequestID(headers); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
