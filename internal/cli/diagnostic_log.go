package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// DiagnosticWriter redacts each complete log record before it reaches its sink.
// Use with log.Logger, which writes one complete record per Write call.
func DiagnosticWriter(w io.Writer) io.Writer { return diagnosticWriter{w} }

type diagnosticWriter struct{ out io.Writer }

func (w diagnosticWriter) Write(p []byte) (int, error) {
	text := redactDiagnostic(string(p))
	if strings.HasSuffix(string(p), "\n") && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if _, err := io.WriteString(w.out, text); err != nil {
		return 0, err
	}
	return len(p), nil
}

func debugTunnelLog(err error) {
	var source interface{ DiagnosticLogPath() string }
	if !errors.As(err, &source) || source.DiagnosticLogPath() == "" {
		return
	}
	file, readErr := os.Open(source.DiagnosticLogPath())
	if readErr != nil {
		debugf("tunnel log tail: unavailable (%v)\n", readErr)
		return
	}
	defer func() { _ = file.Close() }()
	stat, readErr := file.Stat()
	if readErr != nil {
		debugf("tunnel log tail: unavailable (%v)\n", readErr)
		return
	}
	const window = 64 * 1024
	offset := max(int64(0), stat.Size()-window)
	data, readErr := io.ReadAll(io.NewSectionReader(file, offset, window))
	if readErr != nil {
		debugf("tunnel log tail: unavailable (%v)\n", readErr)
		return
	}
	if offset > 0 {
		// Never print a partial first line: it may start inside a credential.
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		} else {
			data = nil
		}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	tail := redactSensitive(strings.Join(lines, "\n"))
	if len(tail) > diagnosticLimit {
		start := len(tail) - diagnosticLimit
		for start < len(tail) && !utf8.RuneStart(tail[start]) {
			start++
		}
		tail = "[truncated]\n" + tail[start:]
	}
	if tail == "" {
		tail = "(no complete log lines available)"
	}
	// Quote and close the tail: it holds the child's own error output, which
	// must not read as a second copy of the parent's error.
	fmt.Fprintf(ios.ErrOut, "tunnel log tail:\n  | %s\nend of tunnel log tail\n", strings.ReplaceAll(tail, "\n", "\n  | "))
}
