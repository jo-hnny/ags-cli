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

// Header values end at a semicolon in inline diagnostics; do not consume the
// following operation/status text. Cookie pairs are handled separately.
// A quoted value without its closing quote (for example, cut by truncation)
// extends to the end of its line, never into the next line.
var diagnosticHeader = regexp.MustCompile(`(?im)(\b(?:authorization|proxy-authorization)["']?\s*[:=]\s*["']?)(?:(?:Bearer|Basic)\s+(?:"[^"\r\n]*"?|'[^'\r\n]*'?|[^\s;,"']+)|(?:[^;\r\n"']+|"[^"\r\n]*"?|'[^'\r\n]*'?)+)`)
var diagnosticAuthScheme = regexp.MustCompile(`(?i)^(?:Bearer|Basic|Digest|Negotiate|NTLM|(?:AWS4|TC3)-HMAC-SHA256)\s+`)
var diagnosticCookie = regexp.MustCompile(`(?i)\b(cookie|set-cookie)["']?\s*[:=]\s*["']?`)

// Unquoted cookie values may contain single quotes and commas (RFC 6265 allows
// any octet except controls, whitespace, DQUOTE, semicolon and backslash).
var diagnosticCookiePair = regexp.MustCompile(`^(\s*[^=\s;"']+\s*=\s*)("[^"\r\n]*"?|[^;\s"]+)(;\s*)?`)
var diagnosticQuery = regexp.MustCompile(`([?&;])([^=&#;]+)=([^&#;]*)`)

// Normal failures redact sensitive values without imposing a diagnostic limit.
func redactSensitive(text string) string {
	text = diagnosticURL.ReplaceAllStringFunc(text, redactURL)
	// Explicit credential schemes can occur inline. Other values need a header
	// boundary so prose such as "failed to set authorization: denied" survives.
	headers := diagnosticHeader.FindAllStringSubmatchIndex(text, -1)
	for i := len(headers) - 1; i >= 0; i-- {
		match := headers[i]
		if isDiagnosticHeaderStart(text, match[0]) || diagnosticAuthScheme.MatchString(text[match[3]:match[1]]) {
			text = text[:match[3]] + "[REDACTED]" + text[match[1]:]
		}
	}
	// Work backwards so replacing cookie values does not invalidate offsets.
	matches := diagnosticCookie.FindAllStringIndex(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		end := matches[i][1]
		rest := text[end:]
		var clean strings.Builder
		for {
			pair := diagnosticCookiePair.FindStringSubmatch(rest)
			if pair == nil {
				// Bare values need a header boundary; prose such as
				// "failed to set cookie: permission denied" is not a header.
				if clean.Len() == 0 && isDiagnosticHeaderStart(text, matches[i][0]) {
					n := strings.IndexAny(rest, "; \t\r\n\"")
					if n < 0 {
						n = len(rest)
					}
					if n > 0 {
						clean.WriteString("[REDACTED]")
						rest = rest[n:]
					}
				}
				break
			}
			clean.WriteString(pair[1])
			clean.WriteString("[REDACTED]")
			clean.WriteString(pair[3])
			rest = rest[len(pair[0]):]
		}
		text = text[:end] + clean.String() + rest
	}
	secrets := diagnosticSecrets()
	// Replace longer overlapping values first.
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
			text = strings.ReplaceAll(text, url.QueryEscape(secret), "[REDACTED]")
		}
	}
	return text
}

func diagnosticSecrets() []string {
	return []string{secretID, secretKey, tokenFlag, config.GetSecretID(), config.GetSecretKey(), config.GetToken()}
}

func isDiagnosticHeaderStart(text string, start int) bool {
	lineStart := strings.LastIndexByte(text[:start], '\n') + 1
	prefix := strings.TrimSpace(text[lineStart:start])
	if prefix == "" {
		return true
	}
	// A quoted field at the start of a line or after an object delimiter.
	if strings.HasSuffix(prefix, "\"") || strings.HasSuffix(prefix, "'") {
		prefix = strings.TrimSpace(prefix[:len(prefix)-1])
		return prefix == "" || strings.HasSuffix(prefix, "{") || strings.HasSuffix(prefix, ",")
	}
	return false
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
