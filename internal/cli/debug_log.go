package cli

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The terminal keeps bounded diagnostics; the file receives complete redacted
// diagnostics and a copy of stderr. Stdout (including machine output) is untouched.
type debugLog struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	err      error
	pending  bytes.Buffer
	terminal io.Writer
	logger   io.Writer
}

var activeDebugLog *debugLog

func startDebugLog() {
	if !debugFlag || activeDebugLog != nil {
		return
	}
	file, err := openDebugLog()
	if err != nil {
		fmt.Fprintf(ios.ErrOut, "Warning: cannot create debug log: %s\n", redactSensitive(err.Error()))
		return
	}
	activeDebugLog = &debugLog{file: file, path: file.Name(), terminal: ios.ErrOut, logger: log.Writer()}
	ios.ErrOut = activeDebugLog
	log.SetOutput(activeDebugLog)
	activeDebugLog.mu.Lock()
	activeDebugLog.writeFile(fmt.Sprintf("Debug: started=%s pid=%d\n", time.Now().UTC().Format(time.RFC3339Nano), os.Getpid()))
	activeDebugLog.mu.Unlock()
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
	l.pending.WriteString(text)
	// Revisit buffered expressions at a new line or on close, not for every
	// byte of a long unterminated line.
	if strings.ContainsRune(text, '\n') {
		if end := safeDebugLogPrefix(l.pending.String()); end > 0 {
			_, l.err = io.WriteString(l.file, redactSensitive(string(l.pending.Next(end))))
		}
	}
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
		match := diagnosticHeader.FindStringIndex(text[field[0]:])
		if match != nil {
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
	if l := activeDebugLog; l != nil {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.writeFile(full)
		fmt.Fprint(l.terminal, redactDiagnostic(terminal))
		return
	}
	fmt.Fprint(ios.ErrOut, redactDiagnostic(terminal))
}

func closeDebugLog() {
	l := activeDebugLog
	if l == nil {
		return
	}
	activeDebugLog = nil
	ios.ErrOut = l.terminal
	log.SetOutput(l.logger)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		_, l.err = io.WriteString(l.file, redactSensitive(l.pending.String()))
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
