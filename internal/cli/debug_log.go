package cli

import (
	"bytes"
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
	pending  bytes.Buffer // the current unterminated line, at most debugLogLineLimit bytes
	skipping bool         // discarding the rest of an over-limit line
	terminal io.Writer
	logger   io.Writer
}

var activeDebugLog atomic.Pointer[debugLog]

const debugLogLineLimit = 64 << 10

const debugLogOverflowMarker = "[REDACTED: line exceeded 64 KiB]"

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

// writeFile is called with mu held. A Write is a transport chunk, so each line
// is buffered and redacted as a whole once its newline arrives (or on close).
// Values spanning lines are not recognized. File failures never change results.
func (l *debugLog) writeFile(text string) {
	for text != "" && l.file != nil && l.err == nil {
		chunk, rest, complete := strings.Cut(text, "\n")
		text = rest
		if l.skipping {
			if complete {
				l.skipping = false
				l.write("\n")
			}
			continue
		}
		if l.pending.Len()+len(chunk) > debugLogLineLimit {
			l.pending.WriteString(chunk[:debugLogLineLimit-l.pending.Len()])
			l.writeTruncatedLine()
			l.skipping = !complete
			if complete {
				l.write("\n")
			}
			continue
		}
		l.pending.WriteString(chunk)
		if complete {
			l.pending.WriteByte('\n')
			l.flushLine()
		}
	}
}

func (l *debugLog) flushLine() {
	l.write(redactSensitive(l.pending.String()))
	l.pending.Reset()
}

// The cut may split a credential, so drop the token it lands in; the rest of
// the line is discarded.
func (l *debugLog) writeTruncatedLine() {
	text := l.pending.String()
	l.pending.Reset()
	if cut := strings.LastIndexAny(text, " \t"); cut > 0 {
		l.write(redactSensitive(text[:cut]) + " ")
	}
	l.write(debugLogOverflowMarker)
}

func (l *debugLog) write(text string) {
	if l.err == nil {
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
	if l.pending.Len() > 0 {
		l.flushLine()
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
