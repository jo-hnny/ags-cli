package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

const diagnosticLimit = 8192

var diagnosticURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s<>"']+`)
var diagnosticHeader = regexp.MustCompile(`(?im)\b(authorization|proxy-authorization|cookie|set-cookie)["']?\s*[:=]\s*[^\r\n]+`)

// Redact before truncating: truncation must not leave a partial credential.
func redactDiagnostic(text string) string {
	text = diagnosticURL.ReplaceAllStringFunc(text, func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return "[REDACTED URL]"
		}
		changed := false
		if u.User != nil {
			if _, ok := u.User.Password(); ok {
				u.User = url.UserPassword(u.User.Username(), "REDACTED")
				changed = true
			}
		}
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			u.RawQuery = "REDACTED"
			changed = true
		} else {
			for key := range query {
				if sensitiveDiagnosticKey(key) {
					query.Set(key, "REDACTED")
					changed = true
				}
			}
			if changed {
				u.RawQuery = query.Encode()
			}
		}
		if !changed {
			return raw
		}
		return u.String()
	})
	text = diagnosticHeader.ReplaceAllString(text, "$1: [REDACTED]")
	secrets := []string{secretID, secretKey, tokenFlag, config.GetSecretID(), config.GetSecretKey(), config.GetToken()}
	// Replace longer overlapping values first.
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
			text = strings.ReplaceAll(text, url.QueryEscape(secret), "[REDACTED]")
		}
	}
	if len(text) > diagnosticLimit {
		end := diagnosticLimit
		for end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[:end] + " [truncated]"
	}
	return text
}

func sensitiveDiagnosticKey(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie",
		"secretid", "secretkey", "token", "signature", "sig", "access_token",
		"security-token", "x-tc-token", "q-signature", "q-ak",
		"x-amz-signature", "x-amz-credential", "x-amz-security-token",
		"x-goog-signature", "x-goog-credential":
		return true
	default:
		return false
	}
}

// debugError is called only at the process error exit. Error() on wrappers often
// already includes the child message, so do not repeat those messages per link.
func debugError(err error) {
	if !debugFlag || err == nil {
		return
	}
	if done, ok := err.(*envelopeAlreadyWritten); ok {
		err = done.cause
	}
	var parts []string
	remaining := 32
	var visit func(error)
	visit = func(current error) {
		if current == nil || remaining == 0 {
			return
		}
		remaining--
		message := current.Error()
		if message != "" && !strings.Contains(strings.Join(parts, "\n"), message) {
			parts = append(parts, fmt.Sprintf("%T: %s", current, message))
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(wrapped.Unwrap())
		}
	}
	visit(err)
	if len(parts) > 0 {
		debugf("Debug: error=%s\n", strings.Join(parts, "\n  caused by: "))
	}
}

// Sanitize a copy, keeping the in-memory cause and classification intact.
func sanitizeFailure(failure *output.Failure) *output.Failure {
	if failure == nil {
		return nil
	}
	copy := *failure
	copy.Message = redactDiagnostic(copy.Message)
	copy.Hint = redactDiagnostic(copy.Hint)
	copy.Fix = redactDiagnostic(copy.Fix)
	if failure.Details != nil {
		data, err := json.Marshal(failure.Details)
		if err != nil {
			copy.Details = map[string]any{"Diagnostic": "details could not be encoded"}
			return &copy
		}
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.UseNumber()
		var details map[string]any
		if decoder.Decode(&details) == nil {
			copy.Details = sanitizeDiagnosticValue(details).(map[string]any)
		}
	}
	return &copy
}

func sanitizeDiagnosticValue(value any) any {
	switch typed := value.(type) {
	case string:
		return redactDiagnostic(typed)
	case map[string]any:
		for key, item := range typed {
			if sensitiveDiagnosticKey(key) {
				typed[key] = "[REDACTED]"
			} else {
				typed[key] = sanitizeDiagnosticValue(item)
			}
		}
	case []any:
		for i, item := range typed {
			typed[i] = sanitizeDiagnosticValue(item)
		}
	}
	return value
}
