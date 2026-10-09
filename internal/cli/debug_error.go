package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

const diagnosticLimit = 8192

var diagnosticURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s<>"']+`)

// Only the credential after an explicit scheme is replaced. Real Bearer/Basic
// credentials are long; the length floor keeps prose such as "basic
// authentication failed" intact.
var diagnosticAuthToken = regexp.MustCompile(`(?i)\b((?:Bearer|Basic)\s+["']?)[A-Za-z0-9._~+/-]{16,}=*`)
var diagnosticQuery = regexp.MustCompile(`([?&;])([^=&#;]+)=([^&#;]*)`)

// Redaction replaces credential values only and never removes surrounding
// text: lost failure context defeats the purpose of diagnostics. Text formats
// are not parsed; credentials the CLI holds are replaced by exact value.
func redactSensitive(text string) string {
	text = diagnosticURL.ReplaceAllStringFunc(text, redactURL)
	text = diagnosticAuthToken.ReplaceAllString(text, "${1}[REDACTED]")
	return maskSecrets(text, knownSecrets())
}

var runtimeSecrets struct {
	sync.Mutex
	values []string
}

// MaskSecret registers a credential obtained at runtime, such as a data-plane
// access token, so diagnostics and debug logs replace it.
func MaskSecret(secret string) {
	runtimeSecrets.Lock()
	defer runtimeSecrets.Unlock()
	if secret != "" && !slices.Contains(runtimeSecrets.values, secret) {
		runtimeSecrets.values = append(runtimeSecrets.values, secret)
	}
}

// knownSecrets returns raw and URL-encoded credential values, longest first so
// overlapping values are replaced whole.
func knownSecrets() []string {
	runtimeSecrets.Lock()
	values := append([]string{secretID, secretKey, tokenFlag, config.GetSecretID(), config.GetSecretKey(), config.GetToken()}, runtimeSecrets.values...)
	runtimeSecrets.Unlock()
	var secrets []string
	for _, value := range values {
		for _, form := range []string{value, url.QueryEscape(value)} {
			if form != "" && !slices.Contains(secrets, form) {
				secrets = append(secrets, form)
			}
		}
	}
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	return secrets
}

func maskSecrets(text string, secrets []string) string {
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, "[REDACTED]")
	}
	return text
}

// Redact before truncating: truncation must not leave a partial credential.
func redactDiagnostic(text string) string {
	return truncateDiagnostic(redactSensitive(text), diagnosticLimit)
}

func truncateDiagnostic(text string, limit int) string {
	if len(text) > limit {
		end := limit
		for end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[:end] + " [truncated]"
	}
	return text
}

// Redact components in place instead of parsing/re-encoding the whole URL.
// A malformed escape in an unrelated parameter must not hide the whole query.
func redactURL(raw string) string {
	schemeEnd := strings.Index(raw, "://") + 3
	authorityEnd := len(raw)
	if i := strings.IndexAny(raw[schemeEnd:], "/?#"); i >= 0 {
		authorityEnd = schemeEnd + i
	}
	authority := raw[schemeEnd:authorityEnd]
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		if colon := strings.IndexByte(authority[:at], ':'); colon >= 0 {
			raw = raw[:schemeEnd] + authority[:colon+1] + "REDACTED" + authority[at:] + raw[authorityEnd:]
		}
	}
	queryStart := strings.IndexByte(raw, '?')
	fragment := strings.IndexByte(raw, '#')
	if queryStart < 0 || (fragment >= 0 && fragment < queryStart) {
		return raw
	}
	queryEnd := len(raw)
	if fragment >= 0 {
		queryEnd = fragment
	}
	query := diagnosticQuery.ReplaceAllStringFunc(raw[queryStart:queryEnd], func(part string) string {
		key, _, _ := strings.Cut(part[1:], "=")
		decoded, err := url.QueryUnescape(key)
		if err == nil && sensitiveDiagnosticKey(decoded) {
			return part[:strings.IndexByte(part, '=')+1] + "REDACTED"
		}
		return part
	})
	return raw[:queryStart] + query + raw[queryEnd:]
}

// Only unambiguous credential/header fields are sensitive outside URL queries.
// Generic token/signature/sig fields may describe application data or functions.
func sensitiveDetailKey(key string) bool {
	switch strings.ToLower(key) {
	case "token", "signature", "sig":
		return false
	default:
		return sensitiveDiagnosticKey(key)
	}
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
	var nodes []error
	remaining := 32
	var visit func(error)
	visit = func(current error) {
		if current == nil || remaining == 0 {
			return
		}
		remaining--
		nodes = append(nodes, current)
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
	if len(nodes) == 0 {
		return
	}
	const separator = "\n  caused by: "
	// Bound each node before deduplication: a long wrapper must not hide the
	// child merely because its full Error() contains a cause beyond the limit.
	limit := (diagnosticLimit-len("Debug: error=\n")-(len(nodes)-1)*len(separator))/len(nodes) - len(" [truncated]")
	var parts []string
	var fullParts []string
	for _, current := range nodes {
		message := redactSensitive(current.Error())
		if message != "" && !strings.Contains(strings.Join(fullParts, "\n"), message) {
			fullParts = append(fullParts, fmt.Sprintf("%T: %s", current, message))
		}
		if message != "" && !strings.Contains(strings.Join(parts, "\n"), message) {
			parts = append(parts, truncateDiagnostic(fmt.Sprintf("%T: %s", current, message), limit))
		}
	}
	if len(parts) > 0 {
		writeDebugDiagnostic("Debug: error="+strings.Join(parts, separator)+"\n", "Debug: error="+strings.Join(fullParts, separator)+"\n")
	}
}

// Sanitize a copy, keeping the in-memory cause and classification intact.
func sanitizeFailure(failure *output.Failure) *output.Failure {
	if failure == nil {
		return nil
	}
	copy := *failure
	copy.Message = redactSensitive(copy.Message)
	copy.Hint = redactSensitive(copy.Hint)
	copy.Fix = redactSensitive(copy.Fix)
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
		return redactSensitive(typed)
	case map[string]any:
		for key, item := range typed {
			if sensitiveDetailKey(key) {
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
