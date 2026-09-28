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
	oldKey := secretKey
	secretKey = "test-secret-key"
	t.Cleanup(func() { secretKey = oldKey })
	var disk bytes.Buffer
	logger := log.New(DiagnosticWriter(&disk), "", 0)
	logger.Print("test-secret-key https://user:proxy-secret@host/?Signature=signature-secret\nAuthorization: Bearer auth-secret\nCookie: session=cookie-secret")
	for _, secret := range []string{"test-secret-key", "proxy-secret", "signature-secret", "auth-secret", "cookie-secret"} {
		if strings.Contains(disk.String(), secret) {
			t.Fatalf("persisted secret %q", secret)
		}
	}
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
	for _, forbidden := range []string{"must-not-appear", "proxy-secret", "signature-secret"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("tail leaked %q", forbidden)
		}
	}
}
