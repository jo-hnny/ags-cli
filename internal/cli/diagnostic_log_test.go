package cli

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

type testLogSource struct {
	error
	path string
}

func (e testLogSource) DiagnosticLogPath() string { return e.path }

func TestDiagnosticWriterRedactsBeforeWrite(t *testing.T) {
	useTestSecrets(t)
	MaskSecret("short-tok")
	var disk bytes.Buffer
	logger := log.New(DiagnosticWriter(&disk), "", 0)
	logger.Print("handshake rejected id=test-access-id key=active-private/key https://user:proxy-secret@host/?Signature=signature-secret\n" +
		"Authorization: Bearer short-tok\n" +
		`{"Authorization":["Bearer short-tok"],"X-Tc-Token":["test-session-token"]} map[Authorization:[Bearer runtime-access-token-value]]`)
	want := "handshake rejected id=[REDACTED] key=[REDACTED] https://user:REDACTED@host/?Signature=REDACTED\n" +
		"Authorization: Bearer [REDACTED]\n" +
		`{"Authorization":["Bearer [REDACTED]"],"X-Tc-Token":["[REDACTED]"]} map[Authorization:[Bearer [REDACTED]]]` + "\n"
	if disk.String() != want {
		t.Fatalf("log record:\n%s\nwant:\n%s", disk.String(), want)
	}
	disk.Reset()
	logger.Print(strings.Repeat("错", 5000))
	if !utf8.Valid(disk.Bytes()) || !strings.HasSuffix(disk.String(), "[truncated]\n") {
		t.Fatal("invalid bounded log record")
	}
}

func TestDebugTunnelLogTail(t *testing.T) {
	oldDebug, oldIO := debugFlag, ios
	debugFlag = true
	var stderr bytes.Buffer
	ios = &iostreams.IOStreams{ErrOut: &stderr}
	t.Cleanup(func() { debugFlag, ios = oldDebug, oldIO })
	path := filepath.Join(t.TempDir(), "tunnel.log")
	content := strings.Repeat("x", 70000) + "must-not-appear\n" + strings.Repeat("错误", 4000) + "\nlatest-cause https://user:proxy-secret@host/?Signature=signature-secret\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	debugError(testLogSource{error: errors.New("parent cause"), path: path})
	got := stderr.String()
	if strings.Count(got, "Debug: error=") != 1 || strings.Count(got, "tunnel log tail:") != 1 || !strings.Contains(got, "latest-cause") || !strings.Contains(got, "[truncated]") || len(got) > diagnosticLimit+200 || !utf8.ValidString(got) {
		t.Fatalf("invalid tail length=%d", len(got))
	}
	_, tail, _ := strings.Cut(got, "tunnel log tail:\n")
	tail, rest, closed := strings.Cut(tail, "end of tunnel log tail\n")
	if !closed || rest != "" {
		t.Fatalf("tail is not closed: %q", got)
	}
	for _, line := range strings.Split(strings.TrimSuffix(tail, "\n"), "\n") {
		if !strings.HasPrefix(line, "  | ") {
			t.Fatalf("unquoted tail line %q", line)
		}
	}
	for _, forbidden := range []string{"must-not-appear", "proxy-secret", "signature-secret"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("tail leaked %q", forbidden)
		}
	}
}
