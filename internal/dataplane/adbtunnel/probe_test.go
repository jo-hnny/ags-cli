package adbtunnel

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
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

// A real unresponsive peer: the socket deadline and the context timer race, and
// either one must classify as a timeout.
func TestProbeRealHandshakeTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var conns []net.Conn
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn) // Never answer the HTTP upgrade.
			mu.Unlock()
		}
	}()
	var probes sync.WaitGroup
	for range 30 {
		probes.Go(func() {
			tunnel := &Tunnel{ctx: t.Context(), wsURL: "ws://" + listener.Addr().String(), options: TunnelOptions{TokenProvider: func() (string, error) { return "test", nil }}}
			err := tunnel.probe(300 * time.Millisecond)
			var handshake *HandshakeError
			if !errors.As(err, &handshake) || handshake.HTTPStatus != 0 || !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "\n") {
				t.Errorf("real handshake timeout misclassified: %v", err)
			}
		})
	}
	probes.Wait()
}

func TestHandshakeTimeoutMessageIsSingleLine(t *testing.T) {
	cause := errors.Join(errors.New("transport timeout"), context.DeadlineExceeded)
	err := &HandshakeError{Cause: cause}
	if strings.Contains(err.Error(), "\n") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
}
