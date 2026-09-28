package adbtunnel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeRetainsHandshakeStatusWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
