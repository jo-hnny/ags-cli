package adbtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// HandshakeError retains observations at the WebSocket boundary, never response
// bodies, arbitrary headers, or credentials from the destination URL.
type HandshakeError struct {
	Cause      error
	HTTPStatus int
	Endpoint   string
	TimeoutMs  int64
	RequestID  string
}

func (e *HandshakeError) Error() string {
	var context []string
	if e.Endpoint != "" {
		context = append(context, "endpoint="+e.Endpoint)
	}
	if e.TimeoutMs > 0 {
		context = append(context, fmt.Sprintf("timeout=%dms", e.TimeoutMs))
	}
	if e.HTTPStatus != 0 {
		context = append(context, fmt.Sprintf("HTTP %d", e.HTTPStatus))
	}
	if e.RequestID != "" {
		context = append(context, "request_id="+e.RequestID)
	}
	prefix := "upstream WS handshake failed"
	if len(context) > 0 {
		prefix += " (" + strings.Join(context, ", ") + ")"
	}
	return prefix + ": " + strings.ReplaceAll(fmt.Sprint(e.Cause), "\n", "; ")
}

func (e *HandshakeError) Unwrap() error { return e.Cause }

func (t *Tunnel) dialWebSocket(ctx context.Context, dialer *websocket.Dialer, budget time.Duration) (*websocket.Conn, error) {
	token, err := t.options.TokenProvider()
	if err != nil {
		return nil, fmt.Errorf("token provider failed: %w", err)
	}
	headers := http.Header{"Authorization": {"Bearer " + token}}
	if t.options.Endpoint != "" {
		headers.Set("Host", t.e2bHost)
	}
	// Token acquisition has its own boundary; this budget starts at the dial.
	if deadline, ok := ctx.Deadline(); ok {
		budget = min(budget, max(0, time.Until(deadline)))
	}
	dialCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	conn, response, err := dialer.DialContext(dialCtx, t.wsURL, headers)
	if err == nil {
		return conn, nil
	}
	contextErr := dialCtx.Err()
	var networkErr net.Error
	// The socket deadline can fire before the context timer is scheduled. Only
	// normalize a transport timeout when the effective dial deadline has expired.
	if contextErr == nil && errors.As(err, &networkErr) && networkErr.Timeout() {
		if deadline, ok := dialCtx.Deadline(); ok && time.Until(deadline) <= 0 {
			contextErr = context.DeadlineExceeded
		}
	}
	if contextErr != nil && !errors.Is(err, contextErr) {
		err = errors.Join(err, contextErr)
	}
	failure := &HandshakeError{Cause: err, Endpoint: diagnosticEndpoint(t.wsURL), TimeoutMs: budget.Milliseconds()}
	if response != nil {
		failure.HTTPStatus = response.StatusCode
		failure.RequestID = responseRequestID(response.Header)
	}
	return nil, failure
}

func diagnosticEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = nil, "", "", "", false
	return u.String()
}

func responseRequestID(headers http.Header) string {
	for _, name := range []string{"X-TC-RequestId", "X-Request-Id"} {
		value := headers.Get(name)
		if value == "" || len(value) > 256 {
			continue
		}
		valid := true
		for _, c := range value {
			if c < '!' || c > '~' {
				valid = false
				break
			}
		}
		if valid {
			return value
		}
	}
	return ""
}
