package cli

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
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

// writeFile is called with mu held. File failures never change command results.
func (l *debugLog) writeFile(text string) {
	if l.file != nil && l.err == nil {
		_, l.err = io.WriteString(l.file, redactSensitive(text))
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
