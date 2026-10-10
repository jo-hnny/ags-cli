package output

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// HTTPContext records only observations and public metadata at a request boundary.
// Call before issuing the request so TimeoutMs describes its initial budget.
func HTTPContext(ctx context.Context, stage, endpoint string, budget time.Duration, response *http.Response) map[string]any {
	details := map[string]any{"Stage": stage}
	if endpoint = DiagnosticEndpoint(endpoint); endpoint != "" {
		details["Endpoint"] = endpoint
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := max(0, time.Until(deadline))
		if budget <= 0 || remaining < budget {
			budget = remaining
		}
	}
	if budget > 0 {
		details["TimeoutMs"] = budget.Milliseconds()
	}
	if response != nil {
		details["HTTPStatus"] = response.StatusCode
		if id := ResponseRequestID(response.Header); id != "" {
			details["RequestId"] = id
		}
	}
	return details
}

// DiagnosticEndpoint omits URL credentials, query strings and fragments.
func DiagnosticEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = nil, "", "", "", false
	return u.String()
}

// ResponseRequestID accepts only bounded printable IDs from an explicit header allowlist.
func ResponseRequestID(headers http.Header) string {
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
