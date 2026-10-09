package cli

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

// The terminal keeps bounded diagnostics; the file receives complete redacted
// diagnostics and a copy of stderr. Stdout (including machine output) is untouched.
type debugLog struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	err          error
	pending      bytes.Buffer
	scanned      int
	waitQuote    byte
	discardQuote byte
	discardUntil string
	quarantined  bool
	seed         string
	line         diagnosticLineContext
	scheme       bool
	terminal     io.Writer
	logger       io.Writer
}

var activeDebugLog atomic.Pointer[debugLog]

const diagnosticPendingLimit = 16 << 10

const diagnosticOverflowMarker = "[REDACTED: incomplete diagnostic exceeded 16 KiB]"

func startDebugLog() {
	if !debugFlag || activeDebugLog.Load() != nil {
		return
	}
	file, err := openDebugLog()
	if err != nil {
		fmt.Fprintf(ios.ErrOut, "Warning: cannot create debug log: %s\n", redactSensitive(err.Error()))
		return
	}
	terminal := ios.ErrOut
	if previous, ok := terminal.(*debugLog); ok {
		terminal = previous.terminal
	}
	l := &debugLog{file: file, path: file.Name(), terminal: terminal, logger: log.Writer()}
	activeDebugLog.Store(l)
	ios.ErrOut = l // Installed before command goroutines start; stable through close.
	log.SetOutput(l)
	l.mu.Lock()
	l.writeFile(fmt.Sprintf("Debug: started=%s pid=%d\n", time.Now().UTC().Format(time.RFC3339Nano), os.Getpid()))
	l.mu.Unlock()
}

func openDebugLog() (*os.File, error) {
	if debugLogFlag != "" {
		path, err := filepath.Abs(debugLogFlag)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".agr", "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, "agr-"+time.Now().UTC().Format("20060102T150405")+"-*.log")
}

// writeFile is called with mu held. A Write is a transport chunk, not a
// redaction boundary. File failures never change command results.
func (l *debugLog) writeFile(text string) {
	if l.file == nil || l.err != nil {
		return
	}
	for text != "" && l.err == nil {
		if l.quarantined {
			return // An over-limit credential with no provable boundary stays opaque.
		}
		if l.discardQuote != 0 || l.discardUntil != "" {
			text = l.discard(text)
			continue
		}
		n := min(len(text), diagnosticPendingLimit-l.pending.Len())
		chunk := text[:n]
		text = text[n:]
		l.pending.WriteString(chunk)
		// Failed scans are revisited geometrically, or when the expected quote
		// closes. Unfinished multi-line values cannot cause a scan per line.
		if strings.ContainsRune(chunk, '\n') && (l.scanned == 0 || l.pending.Len() >= 2*l.scanned) || l.waitQuote != 0 && strings.ContainsRune(chunk, rune(l.waitQuote)) {
			pending := l.pending.String()
			l.flush(safeDebugLogPrefix(pending))
			l.scanned = l.pending.Len()
			l.waitQuote, _ = pendingDiagnosticQuote(l.pending.String())
		}
		if l.pending.Len() == diagnosticPendingLimit {
			l.flush(l.safeFragmentPrefix())
			if l.pending.Len() == diagnosticPendingLimit {
				l.overflow()
			}
		}
	}
}

func (l *debugLog) flush(end int) {
	if end == 0 || l.err != nil {
		return
	}
	raw := string(l.pending.Next(end))
	// A scheme can start in an earlier ordinary fragment. Restore its leading
	// letter only for URL redaction, then remove it before writing.
	clean := raw
	if l.scheme {
		clean = diagnosticURLSuffix.ReplaceAllStringFunc(clean, func(value string) string {
			return redactURL("a" + value)[1:]
		})
	}
	clean = redactSensitiveContext(clean, l.line)
	if l.seed != "" {
		prefix := redactSensitive(l.seed)
		var ok bool
		clean, ok = strings.CutPrefix(clean, prefix)
		if !ok {
			clean = diagnosticOverflowMarker
		}
		raw = strings.TrimPrefix(raw, l.seed)
		l.seed = ""
	}
	_, l.err = io.WriteString(l.file, clean)
	l.advance(raw)
	l.scanned, l.waitQuote = 0, 0
}

func (l *debugLog) advance(text string) {
	for _, r := range text {
		l.line.word = r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_'
		if r == '\n' {
			l.line.last = 0
		} else if !unicode.IsSpace(r) {
			l.line.last = r
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			l.scheme = true
		} else if (r < '0' || r > '9') && r != '+' && r != '-' && r != '.' {
			l.scheme = false
		}
	}
}

// Flush ordinary long text, retaining enough lookbehind for fields/credentials.
// Expressions crossing the fragment boundary are handled by overflow instead.
func (l *debugLog) safeFragmentPrefix() int {
	text := l.pending.String()
	keep := 32
	var secrets []string
	for _, secret := range diagnosticSecrets() {
		keep = max(keep, len(secret), len(url.QueryEscape(secret)))
		if secret != "" {
			secrets = append(secrets, secret, url.QueryEscape(secret))
		}
	}
	end := max(0, len(text)-keep)
	for _, field := range diagnosticLogField.FindAllStringIndex(text, -1) {
		if field[0] < end {
			rest := strings.TrimLeft(text[field[1]:], " \t\r\n\f")
			if rest != "" && rest[0] != ':' && rest[0] != '=' {
				continue // An ordinary word, not a header field.
			}
			if header := diagnosticHeader.FindStringIndex(text[field[0]:]); header != nil && header[0] == 0 {
				value := strings.TrimLeft(rest[1:], " \t\r\n\f")
				if header[1]+field[0] <= end && (strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") || strings.HasPrefix(strings.ToLower(value), "bearer ") || strings.HasPrefix(strings.ToLower(value), "basic ")) {
					continue // The quoted value/token already ended before the cut.
				}
				if !isDiagnosticHeaderStartContext(text, field[0], l.line) && !diagnosticAuthScheme.MatchString(strings.TrimLeft(value, "\"'")) && len(value) > 32 {
					continue // Prose cannot turn into a known credential scheme.
				}
			}
			if cookie := diagnosticCookie.FindStringIndex(text[field[0]:]); cookie != nil && cookie[0] == 0 && !isDiagnosticHeaderStartContext(text, field[0], l.line) {
				value := text[field[0]+cookie[1]:]
				if diagnosticCookiePair.FindStringIndex(value) == nil && !incompleteDiagnosticCookie(value, len(value)) {
					continue
				}
			}
			end = min(end, max(0, field[0]-1))
		}
	}
	for _, match := range diagnosticURL.FindAllStringIndex(text, -1) {
		if match[0] < end && match[1] > end {
			// A long scheme carries no credentials. Keep the separator and
			// authority together, using scheme context for the next fragment.
			separator := match[0] + strings.Index(text[match[0]:match[1]], "://")
			if separator >= end {
				end = min(end, separator)
			} else {
				end = match[0]
			}
		}
	}
	if l.scheme && diagnosticURLSuffix.MatchString(text) {
		end = 0
	}
	for _, secret := range secrets {
		for offset := 0; offset < end; {
			at := strings.Index(text[offset:], secret)
			if at < 0 {
				break
			}
			start := offset + at
			if start+len(secret) > end {
				end = start
				break
			}
			offset = start + len(secret)
		}
	}
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return end
}

var diagnosticURLSuffix = regexp.MustCompile(`^[a-zA-Z0-9+.-]*://[^\s<>"']+`)

// A quoted overflow stays masked across newlines until its real closing quote.
// The synthetic Cookie pair keeps subsequent pairs under the same redactor.
func (l *debugLog) overflow() {
	text := l.pending.String()
	l.discardQuote, l.seed = pendingDiagnosticQuote(text)
	if l.discardQuote == 0 {
		cookie := diagnosticCookie.FindStringIndex(text)
		header := diagnosticHeader.FindStringSubmatchIndex(text)
		// Unquoted tokens have a provable terminator too. Whitespace after a
		// field/scheme or an incomplete cookie name is not a token boundary.
		if cookie != nil && (header == nil || cookie[0] < header[0]) {
			value := text[cookie[1]:]
			if pair := diagnosticCookiePair.FindStringSubmatch(value); pair != nil {
				l.discardUntil = "; \t\r\n\f\"'"
				l.seed = `Cookie: _=masked`
			} else if isDiagnosticHeaderStartContext(text, cookie[0], l.line) && value != "" && !strings.ContainsAny(value, "=; \t\r\n\f\"'") {
				l.discardUntil = "; \t\r\n\f\"'"
			}
		} else if header != nil {
			value := text[header[3]:header[1]]
			if (strings.HasPrefix(strings.ToLower(value), "bearer ") || strings.HasPrefix(strings.ToLower(value), "basic ")) && len(strings.Fields(value)) == 2 {
				l.discardUntil = "; \t\r\n\f,\"'"
			} else if strings.TrimSpace(value) != "" {
				l.discardUntil = ";\r\n\"'"
				l.seed = `Authorization: _=""`
			}
		} else if diagnosticURL.MatchString(text) || l.scheme && diagnosticURLSuffix.MatchString(text) {
			l.discardUntil = " \t\r\n\f<>\"'"
		}
		l.quarantined = l.discardUntil == ""
	}
	_, l.err = io.WriteString(l.file, diagnosticOverflowMarker)
	l.advance(text)
	l.pending.Reset()
	l.scanned, l.waitQuote = 0, 0
}

func (l *debugLog) discard(text string) string {
	var end int
	if l.discardQuote != 0 {
		end = strings.IndexByte(text, l.discardQuote)
	} else {
		end = strings.IndexAny(text, l.discardUntil)
	}
	if end < 0 {
		l.advance(text)
		return ""
	}
	if l.discardQuote == 0 && (text[end] == '"' || text[end] == '\'') && l.seed != "" {
		// A generic header can acquire a quoted component during discard.
		// Stay opaque across its newlines until the matching quote closes.
		l.discardQuote, l.discardUntil = text[end], ""
		l.advance(text[:end+1])
		return text[end+1:]
	}
	if l.discardQuote != 0 {
		end++
	}
	l.advance(text[:end])
	l.discardQuote, l.discardUntil = 0, ""
	if l.seed != "" {
		l.pending.WriteString(l.seed)
		l.line = diagnosticLineContext{}
	}
	return text[end:]
}

func pendingDiagnosticQuote(text string) (byte, string) {
	for _, field := range diagnosticLogField.FindAllStringIndex(text, -1) {
		rest := strings.TrimLeft(text[field[1]:], " \t\r\n\f")
		if rest == "" || rest[0] != ':' && rest[0] != '=' {
			continue
		}
		value := strings.TrimLeft(rest[1:], " \t\r\n\f")
		if quote, i := unmatchedDiagnosticQuote(value); quote != 0 {
			seed := ""
			if strings.Contains(strings.ToLower(text[field[0]:field[1]]), "cookie") {
				seed = `Cookie: _=""`
			} else if i > 0 && !strings.HasPrefix(strings.ToLower(value), "bearer ") && !strings.HasPrefix(strings.ToLower(value), "basic ") {
				seed = `Authorization: _=""`
			}
			return quote, seed
		}
	}
	return 0, ""
}

func unmatchedDiagnosticQuote(value string) (byte, int) {
	for offset := 0; offset < len(value); {
		at := strings.IndexAny(value[offset:], "\"'")
		if at < 0 {
			break
		}
		at += offset
		quote := value[at]
		end := strings.IndexByte(value[at+1:], quote)
		if end < 0 {
			return quote, at
		}
		offset = at + end + 2
	}
	return 0, 0
}

var diagnosticLogField = regexp.MustCompile(`(?i)\b(?:authorization|proxy-authorization|cookie|set-cookie)["']?`)

// Newlines normally finish a record, but the existing redaction grammar also
// accepts header whitespace, quoted values and active credentials across lines.
// Keep any expression that could still extend past the last complete line.
func safeDebugLogPrefix(text string) int {
	end := strings.LastIndexByte(text, '\n') + 1
	retain := func(start int) { end = min(end, strings.LastIndexByte(text[:start], '\n')+1) }
	for _, field := range diagnosticLogField.FindAllStringIndex(text, -1) {
		if field[0] >= end {
			break
		}
		rest := strings.TrimLeft(text[field[1]:], " \t\r\n\f")
		if rest == "" {
			retain(field[0])
			continue
		}
		if rest[0] != ':' && rest[0] != '=' {
			continue
		}
		value := strings.TrimLeft(rest[1:], " \t\r\n\f")
		if value == "" {
			retain(field[0])
			continue
		}
		if (value[0] == '"' || value[0] == '\'') && !strings.ContainsRune(value[1:], rune(value[0])) {
			retain(field[0])
			continue
		}
		if strings.Contains(strings.ToLower(text[field[0]:field[1]]), "cookie") {
			prefix := diagnosticCookie.FindStringIndex(text[field[0]:])
			if prefix != nil {
				start := field[0] + prefix[1]
				if start > end || incompleteDiagnosticCookie(text[start:], end-start) {
					retain(field[0])
				}
			}
			continue
		}
		match := diagnosticHeader.FindStringSubmatchIndex(text[field[0]:])
		if match != nil && match[0] == 0 {
			matchedValue := text[field[0]+match[3] : field[0]+match[1]]
			if quote, _ := unmatchedDiagnosticQuote(matchedValue); quote != 0 {
				retain(field[0])
				continue
			}
			stop := field[0] + match[1]
			if stop > end || stop < len(text) && (text[stop] == '"' || text[stop] == '\'') {
				retain(field[0])
			}
		}
		// A scheme's whitespace may span a newline before its token/quoted value.
		value = strings.TrimLeft(value, "\"'")
		if scheme := diagnosticAuthScheme.FindStringIndex(value); scheme != nil {
			tail := value[scheme[1]:]
			if tail == "" || (tail[0] == '"' || tail[0] == '\'') && !strings.ContainsRune(tail[1:], rune(tail[0])) {
				retain(field[0])
			}
		}
	}
	for _, secret := range diagnosticSecrets() {
		if !strings.ContainsRune(secret, '\n') {
			continue
		}
		for offset := 0; offset < end; {
			match := strings.Index(text[offset:], secret)
			if match < 0 {
				break
			}
			start := offset + match
			if start < end && start+len(secret) > end {
				retain(start)
				break
			}
			offset = start + len(secret)
		}
		// Only a suffix shorter than the credential can remain an unfinished match.
		for start := max(0, len(text)-len(secret)+1); start < end; start++ {
			if strings.HasPrefix(secret, text[start:]) {
				retain(start)
				break
			}
		}
	}
	return end
}

func incompleteDiagnosticCookie(rest string, end int) bool {
	for {
		if pair := diagnosticCookiePair.FindString(rest); pair != "" {
			if len(pair) > end {
				return true // A complete pair can still cross the file flush boundary.
			}
			end -= len(pair)
			rest = rest[len(pair):]
			continue
		}
		rest = strings.TrimLeft(rest, " \t\r\n\f")
		if rest == "" {
			return true
		}
		keyEnd := strings.IndexAny(rest, "=; \t\r\n\f\"'")
		if keyEnd < 0 {
			return true // A name may acquire '=value' in a later chunk/line.
		}
		if keyEnd == 0 {
			return false
		}
		value := strings.TrimLeft(rest[keyEnd:], " \t\r\n\f")
		if value == "" {
			return true
		}
		if value[0] != '=' {
			return false
		}
		value = strings.TrimLeft(value[1:], " \t\r\n\f")
		return value == "" || value[0] == '"' && !strings.ContainsRune(value[1:], '"')
	}
}

func (l *debugLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.writeFile(string(p))
	return l.terminal.Write(p)
}

func writeDebugDiagnostic(terminal, full string) {
	initIOStreams()
	if l := activeDebugLog.Load(); l != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.writeFile(full)
		fmt.Fprint(l.terminal, redactDiagnostic(terminal))
		return
	}
	fmt.Fprint(ios.ErrOut, redactDiagnostic(terminal))
}

func closeDebugLog() {
	l := activeDebugLog.Swap(nil)
	if l == nil {
		return
	}
	// Leave ios.ErrOut stable: late stderr/debug writes use the closed writer,
	// which serializes terminal output but no longer writes to the file.
	log.SetOutput(l.logger)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		l.flush(l.pending.Len())
	}
	l.pending.Reset()
	if err := l.file.Close(); err != nil && l.err == nil {
		l.err = err
	}
	l.file = nil
	if l.err != nil {
		fmt.Fprintf(l.terminal, "Warning: debug log is incomplete: %s (%s)\n", redactSensitive(l.path), redactSensitive(l.err.Error()))
	} else {
		fmt.Fprintf(l.terminal, "Debug log: %s\n", redactSensitive(l.path))
	}
}

// os.Exit does not run defers; finish the log before every explicit CLI exit.
func exitWithDebugLog(code int) {
	closeDebugLog()
	os.Exit(code)
}
