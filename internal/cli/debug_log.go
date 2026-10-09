package cli

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The terminal keeps bounded diagnostics; the file receives complete redacted
// diagnostics and a copy of stderr. Stdout (including machine output) is untouched.
type debugLog struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	err      error
	pending  string // a tail that may be the start of a known secret
	terminal io.Writer
	logger   io.Writer
}

var activeDebugLog atomic.Pointer[debugLog]

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

// writeFile is called with mu held. Stderr is copied unchanged except for known
// secrets. A Write is a transport chunk that may split a secret, so only a tail
// that could still become one is held back (bounded by the longest secret).
// File failures never change results.
func (l *debugLog) writeFile(text string) {
	if l.file == nil {
		return
	}
	secrets := knownSecrets()
	text = l.pending + text
	hold := secretPrefixLen(text, secrets)
	l.pending = text[len(text)-hold:]
	l.write(maskSecrets(text[:len(text)-hold], secrets))
}

// secretPrefixLen returns the length of the longest suffix of text that is a
// proper prefix of a secret.
func secretPrefixLen(text string, secrets []string) int {
	longest := 0
	for _, secret := range secrets {
		for n := min(len(secret)-1, len(text)); n > longest; n-- {
			if strings.HasSuffix(text, secret[:n]) {
				longest = n
				break
			}
		}
	}
	return longest
}

func (l *debugLog) write(text string) {
	if l.err == nil && text != "" {
		_, l.err = io.WriteString(l.file, text)
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
		l.writeFile(redactSensitive(full))
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
	// Output ended inside what may be a secret; do not write its prefix.
	if l.pending != "" {
		l.write("[REDACTED]")
		l.pending = ""
	}
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
