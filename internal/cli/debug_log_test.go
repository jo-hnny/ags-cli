package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestDebugLogProcess(t *testing.T) {
	for _, mode := range []string{"text", "json", "ndjson"} {
		for _, destination := range []string{"off", "default", "custom", "unwritable"} {
			t.Run(mode+"/"+destination, func(t *testing.T) {
				home := t.TempDir()
				path, debug := "", "1"
				switch destination {
				case "off":
					debug = "0"
				case "custom":
					path = filepath.Join(home, "support log.log")
					if err := os.WriteFile(path, []byte("previous run\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					debug = "0" // --debug-log itself enables diagnostics.
				case "unwritable":
					path = home // An existing directory cannot be opened as a log file.
				}
				cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
				cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_ROUTE=registry", "AGR_TEST_DEBUG="+debug, "AGR_TEST_LONG_CHAIN=1", "AGR_TEST_DEBUG_LOG="+path, "AGR_DEBUG=0", "HOME="+home, "USERPROFILE="+home)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				var exit *exec.ExitError
				if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
					t.Fatalf("original exit code changed: %v", err)
				}
				if mode != "text" {
					lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
					if mode == "json" && len(lines) != 1 || mode == "ndjson" && len(lines) != 2 {
						t.Fatalf("machine framing changed: %s", &stdout)
					}
					for _, line := range lines {
						if !json.Valid(line) {
							t.Fatalf("non-JSON stdout: %s", line)
						}
					}
				}
				files, err := filepath.Glob(filepath.Join(home, ".agr", "logs", "agr-*.log"))
				if err != nil {
					t.Fatal(err)
				}
				if destination == "off" {
					if len(files) != 0 || strings.Contains(stderr.String(), "Debug log:") || strings.Contains(stderr.String(), "Debug: error=") {
						t.Fatal("debug-off created logs or diagnostics")
					}
					return
				}
				if destination == "unwritable" {
					if !strings.Contains(stderr.String(), "cannot create debug log") || strings.Contains(stderr.String(), "Debug log:") || !strings.Contains(stderr.String(), "unique-original-cause") {
						t.Fatal("log failure hid the error or claimed a complete file")
					}
					return
				}
				if destination == "default" {
					if len(files) != 1 {
						t.Fatalf("default logs=%v", files)
					}
					path = files[0]
				}
				contents, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				full := string(contents)
				if !strings.Contains(full, strings.Repeat("错", 3000)) || !strings.Contains(full, "unique-original-cause") || strings.Contains(full, "[truncated]") || !strings.Contains(full, "Code: INTERNAL_ERROR") && mode == "text" {
					t.Fatal("file did not preserve the complete diagnostic and stderr")
				}
				if !strings.Contains(stderr.String(), "Debug log: "+path) || !strings.Contains(stderr.String(), "[truncated]") {
					t.Fatal("missing log path or unbounded terminal output")
				}
				for _, secret := range []string{"test-secret", "proxy-pass", "signed-value"} {
					if strings.Contains(full+stdout.String()+stderr.String(), secret) {
						t.Fatalf("secret leaked: %s", secret)
					}
				}
				if destination == "custom" && !strings.HasPrefix(full, "previous run\n") {
					t.Fatal("custom log was overwritten")
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Fatalf("log permissions=%v", info.Mode())
				}
			})
		}
	}
}

func TestDebugLogConcurrentWritersAndWriteFailure(t *testing.T) {
	oldIO, oldDebug, oldPath := ios, debugFlag, debugLogFlag
	t.Cleanup(func() { closeDebugLog(); ios, debugFlag, debugLogFlag = oldIO, oldDebug, oldPath })
	var stderr *bytes.Buffer
	ios, _, _, stderr = iostreams.Test()
	debugFlag = true
	debugLogFlag = filepath.Join(t.TempDir(), "nested", "run.log")
	startDebugLog()
	if activeDebugLog.Load() == nil {
		t.Fatal("log did not start")
	}
	var writers sync.WaitGroup
	for range 16 {
		writers.Go(func() { log.Print("full logger message") })
		writers.Go(func() { debugf("Debug: concurrent message\n") })
	}
	writers.Wait()
	contents, err := os.ReadFile(debugLogFlag)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(contents, []byte("full logger message")) != 16 || bytes.Count(contents, []byte("concurrent message")) != 16 {
		t.Fatal("concurrent log entries lost")
	}
	if err := activeDebugLog.Load().file.Close(); err != nil {
		t.Fatal(err)
	}
	debugf("Debug: file write must fail but stderr continues\n")
	closeDebugLog()
	if !strings.Contains(stderr.String(), "file write must fail but stderr continues") || !strings.Contains(stderr.String(), "debug log is incomplete") || strings.Contains(stderr.String(), "Debug log:") {
		t.Fatal("write failure hid stderr or claimed a complete file")
	}
}

func TestDebugLogSuccessAndEarlyHelpExit(t *testing.T) {
	for _, earlyHelp := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "early-help-error"}[earlyHelp], func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "run.log")
			mode, help, wantExit := "json", "0", 0
			if earlyHelp {
				mode, help, wantExit = "text", "1", 2
			}
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
			cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_ROUTE=registry", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_SUCCESS=1", "AGR_TEST_HELP_EXIT="+help, "AGR_TEST_DEBUG_LOG="+path, "AGR_TEST_DEBUG=0", "AGR_DEBUG=0", "HOME="+home, "USERPROFILE="+home)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != wantExit {
				t.Fatalf("exit=%v stderr=%s", err, &stderr)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stderr.String(), "Debug log: "+path) {
				t.Fatal("exit did not report the log path")
			}
			if earlyHelp {
				if !bytes.Contains(contents, []byte("--jq can only be used with explicit -o json")) {
					t.Fatal("early help error was not saved")
				}
			} else if !json.Valid(stdout.Bytes()) || !bytes.Contains(contents, []byte("Debug: command=diagnostic-test")) {
				t.Fatal("success lost its diagnostic or changed JSON framing")
			}
		})
	}
}

func TestDebugLogGlobalSchemaAndEarlyArguments(t *testing.T) {
	oldDebug, oldPath := debugFlag, debugLogFlag
	t.Cleanup(func() { debugFlag, debugLogFlag = oldDebug, oldPath })
	for _, args := range [][]string{
		{"--debug-log", "support log.log", "instance", "get"},
		{"--debug-log=support log.log", "instance", "get"},
	} {
		debugFlag, debugLogFlag = false, ""
		applyRawGlobalArgs(args)
		if !debugFlag || debugLogFlag != "support log.log" || inferRequestedCommandID(args) != "instance.get" {
			t.Fatalf("early debug log arguments failed: %v", args)
		}
		if strings.Join(extractHelpTopics(args), " ") != "instance get" {
			t.Fatalf("log path leaked into help topic: %v", args)
		}
	}
	result, err := schemaFn(rootCmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	flags := result.Data.(map[string]any)["GlobalFlags"].([]FlagSchema)
	for _, name := range []string{"debug", "debug-log"} {
		found := false
		for _, schema := range flags {
			if schema.Name == name {
				flag := rootCmd.PersistentFlags().Lookup(name)
				found = schema.Type == flag.Value.Type() && schema.Description == flag.Usage
			}
		}
		if !found {
			t.Fatalf("global flag missing or stale in schema: %s", name)
		}
	}
}

func TestDebugLogRedactsAcrossWriteBoundaries(t *testing.T) {
	oldID, oldKey, oldToken := secretID, secretKey, tokenFlag
	secretID, secretKey, tokenFlag = "test-access-id", "active-private\nkey", "test-session-token"
	t.Cleanup(func() { secretID, secretKey, tokenFlag = oldID, oldKey, oldToken })
	for _, raw := range []string{
		"Authorization: Bearer split-secret\n",
		"Proxy-Authorization: Basic split-secret\n",
		`{"Authorization": "Bearer split-secret"}` + "\n",
		"Authorization:\nBearer split-secret\n",
		"Authorization\n:\nBearer\nsplit-secret\n",
		"Authorization: Bearer \"private\nvalue\"\n",
		"Authorization: \"\nprivate\nvalue\"\n",
		"Proxy-Authorization: '\nprivate\nvalue'\n",
		"Authorization: Digest realm=\"private\nvalue\n",
		"Cookie: session=split-cookie; other=second-secret\n",
		"Cookie:\nsession=split-cookie\n",
		"Cookie: session=\nsplit-cookie\n",
		"Set-Cookie: session= \nsplit-cookie\n",
		"Cookie: session=\"private\nvalue\"\n",
		"Cookie: first=split-cookie;\nsecond=second-secret\n",
		"Cookie: first=split-cookie;\nsecond=second-secret ordinary text\n",
		"Cookie: first=\"private\nvalue\"; ordinary text\n",
		"Cookie: session=\"private\nvalue\" ordinary text\n",
		"Cookie:\nsession=split-cookie ordinary text\n",
		"Set-Cookie: split-cookie\n",
		"credentials: " + secretID + " " + secretKey + " " + tokenFlag + "\n",
		"escaped credential: " + url.QueryEscape(secretKey) + "\n",
		"request https://user:private-pass@example.invalid/path?%53ignature=private-signature&keep=visible\n",
		"错误信息：凭据 " + tokenFlag + "；原因保持可见\n",
		"failed to set authorization: permission denied by policy\nfailed to set cookie: permission denied\n",
	} {
		for _, raw := range []string{raw, strings.TrimSuffix(raw, "\n"), raw + "next ordinary line\n"} {
			for cut := range len(raw) + 1 {
				got := captureDebugLogChunks(t, []string{raw[:cut], raw[cut:]})
				if want := redactSensitive(raw); got != want {
					t.Fatalf("redaction changed at split %d: got %q, want %q", cut, got, want)
				}
			}
			var chunks []string
			for i := range len(raw) {
				chunks = append(chunks, raw[i:i+1])
			}
			if got, want := captureDebugLogChunks(t, chunks), redactSensitive(raw); got != want {
				t.Fatalf("bytewise redaction changed: got %q, want %q", got, want)
			}
		}
	}
	// A long line must remain complete, including a final line without newline.
	raw := strings.Repeat("错", 6000) + "\nAuthorization: Bearer split-secret"
	var chunks []string
	for i := range len(raw) {
		chunks = append(chunks, raw[i:i+1])
	}
	if got := captureDebugLogChunks(t, chunks); got != redactSensitive(raw) {
		t.Fatal("long diagnostic or unterminated tail was lost")
	}
}

func captureDebugLogChunks(t *testing.T, chunks []string) string {
	t.Helper()
	oldIO, oldDebug, oldPath := ios, debugFlag, debugLogFlag
	defer func() { closeDebugLog(); ios, debugFlag, debugLogFlag = oldIO, oldDebug, oldPath }()
	var stderr *bytes.Buffer
	ios, _, _, stderr = iostreams.Test()
	debugFlag, debugLogFlag = true, filepath.Join(t.TempDir(), "run.log")
	startDebugLog()
	if activeDebugLog.Load() == nil {
		t.Fatal("log did not start")
	}
	for _, chunk := range chunks {
		if n, err := ios.ErrOut.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("terminal write changed: %d, %v", n, err)
		}
	}
	if stderr.String() != strings.Join(chunks, "") {
		t.Fatal("terminal business stderr changed")
	}
	closeDebugLog()
	contents, err := os.ReadFile(debugLogFlag)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(contents), "\n") // Per-run metadata precedes the stream.
	if !ok || !strings.Contains(stderr.String(), "Debug log: "+debugLogFlag) {
		t.Fatal("metadata or successful close hint missing")
	}
	return body
}

func TestDebugLogClosedQuotedAuthorizationFlushes(t *testing.T) {
	for _, header := range []string{
		"Authorization: \"private-token\"\n",
		"Proxy-Authorization: 'private-token'\n",
		`{"Authorization": "Bearer private-token"}` + "\n",
	} {
		t.Run(header, func(t *testing.T) {
			l := newStreamTestLog(t)
			followup := "ordinary follow-up\nordinary \"unfinished prose\n"
			l.writeFile(header + followup)
			contents, err := os.ReadFile(l.path)
			if err != nil {
				t.Fatal(err)
			}
			if l.pending.Len() != 0 || string(contents) != redactSensitive(header)+followup {
				t.Fatalf("complete header held until close: pending=%d, file=%q", l.pending.Len(), contents)
			}
		})
	}
}

func TestDebugLogBoundedUnfinishedValues(t *testing.T) {
	for _, pair := range [][2]string{
		{"Cookie: session=\"", "\"; other=second-secret ordinary tail\n"},
		{"Cookie: session=\"Proxy-Authorization: 'private-value' ", "\"; other=second-secret ordinary tail\n"},
		{"Authorization: \"", "\" ordinary tail\n"},
		{"Authorization: Bearer \"", "\" ordinary tail\n"},
		{"Proxy-Authorization: '", "' ordinary tail\n"},
		{"Authorization: Digest realm=\"", "\", nonce=second-secret; ordinary tail\n"},
		{`{"Cookie": "session=`, `"} ordinary tail` + "\n"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			l := newStreamTestLog(t)
			l.writeFile("ordinary prefix\n" + pair[0])
			line := strings.Repeat("x", 99) + "\n"
			for range 14000 {
				if n, err := l.Write([]byte(line)); n != len(line) || err != nil {
					t.Fatalf("stderr write changed: %d, %v", n, err)
				}
				if l.pending.Len() > diagnosticPendingLimit || l.pending.Cap() > 2*diagnosticPendingLimit {
					t.Fatalf("unbounded pending: len=%d cap=%d", l.pending.Len(), l.pending.Cap())
				}
			}
			// Recovery must use the real closing quote, not the next newline.
			l.writeFile("private-continuation" + pair[1])
			l.flush(l.pending.Len())
			contents, err := os.ReadFile(l.path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(contents)
			if !strings.HasPrefix(got, "ordinary prefix\n") || !strings.Contains(got, diagnosticOverflowMarker) || !strings.Contains(got, "ordinary tail") {
				t.Fatalf("overflow/recovery lost: %.200q", got)
			}
			for _, secret := range []string{"xxx", "private-continuation", "second-secret"} {
				if strings.Contains(got, secret) {
					t.Fatalf("overflow continuation leaked %q", secret)
				}
			}
		})
	}
}

func TestDebugLogCloseRedactsUnfinishedQuotes(t *testing.T) {
	for _, raw := range []string{
		"Authorization: \"private-value\nprivate-continuation",
		"Authorization: Digest realm=\"private-value\nprivate-continuation",
		"Cookie: session=\"private-value\nprivate-continuation",
	} {
		got := captureDebugLogChunks(t, []string{raw})
		if !strings.Contains(got, "[REDACTED]") || strings.Contains(got, "private-value") || strings.Contains(got, "private-continuation") {
			t.Fatalf("unfinished credential leaked on close: %q", got)
		}
	}
}

func TestDebugLogBoundedTokensAndURLs(t *testing.T) {
	for _, pair := range [][2]string{
		{"Cookie: session=", "; other=second-secret ordinary tail\n"},
		{"Cookie: session=https://example.invalid/", "; other=second-secret ordinary tail\n"},
		{"Authorization: Digest realm=https://example.invalid/", ", nonce=second-secret; ordinary tail\n"},
		{"Cookie: ", " ordinary tail\n"},
		{"Authorization: Bearer ", " ordinary tail\n"},
		{"request https://user:", "@example.invalid/?token=second-secret ordinary tail\n"},
		{"request https://example.invalid/?%53ignature=", "&keep=visible ordinary tail\n"},
	} {
		l := newStreamTestLog(t)
		l.writeFile("ordinary prefix\n" + pair[0])
		for range 2000 {
			l.writeFile(strings.Repeat("x", 100))
			if l.pending.Len() > diagnosticPendingLimit {
				t.Fatal("unbounded token pending")
			}
		}
		l.writeFile("private-continuation" + pair[1])
		l.flush(l.pending.Len())
		contents, err := os.ReadFile(l.path)
		if err != nil {
			t.Fatal(err)
		}
		got := string(contents)
		if !strings.HasPrefix(got, "ordinary prefix\n") || !strings.Contains(got, "ordinary tail") || !strings.Contains(got, diagnosticOverflowMarker) || strings.Contains(got, "xxx") || strings.Contains(got, "private-continuation") || strings.Contains(got, "second-secret") {
			t.Fatalf("overflow token %q did not recover safely: %.200q", pair[0], got)
		}
		raw := "ordinary prefix\n" + pair[0] + strings.Repeat("x", 200000) + "private-continuation" + pair[1]
		for _, chunks := range [][]string{
			{raw},
			{raw[:diagnosticPendingLimit-1], raw[diagnosticPendingLimit-1 : diagnosticPendingLimit+1], raw[diagnosticPendingLimit+1:]},
		} {
			got := captureDebugLogChunks(t, chunks)
			if !strings.Contains(got, "ordinary tail") || strings.Contains(got, "private-continuation") || strings.Contains(got, "second-secret") {
				t.Fatalf("fragmented overflow %q lost redaction context: %.200q", pair[0], got)
			}
		}
	}
}

func TestDebugLogLongFragments(t *testing.T) {
	oldToken := tokenFlag
	tokenFlag = "private/+token"
	t.Cleanup(func() { tokenFlag = oldToken })
	for index, raw := range []string{
		strings.Repeat("ordinary diagnostics ", 4000),
		strings.Repeat("错误信息保持完整；", 4000),
		strings.Repeat("cookie processing succeeded ", 1000) + "\nnext ordinary line\n",
		`Authorization: "private-token" ` + strings.Repeat("x", 20000) + "\nnext ordinary line\n",
		strings.Repeat("failed to set cookie: permission denied; ", 1000),
		strings.Repeat("x", diagnosticPendingLimit-32-1) + tokenFlag + " ordinary tail\n",
		strings.Repeat("x", diagnosticPendingLimit-32-1) + url.QueryEscape(tokenFlag) + " ordinary tail\n",
		strings.Repeat("x", diagnosticPendingLimit-32-1) + "Authorization: Bearer private-value\n",
		strings.Repeat("x", diagnosticPendingLimit-32-1) + "Cookie: private-value\n",
		strings.Repeat("x", diagnosticPendingLimit-32-1) + "https://user:private-pass@example.invalid/?%53ignature=private-signature&keep=visible\n",
		"a" + strings.Repeat("1", diagnosticPendingLimit+100) + "://user:private-pass@example.invalid/?token=private-value\n",
	} {
		var chunks []string
		for remaining := raw; remaining != ""; {
			n := min(100, len(remaining))
			chunks = append(chunks, remaining[:n])
			remaining = remaining[n:]
		}
		if got, want := captureDebugLogChunks(t, chunks), redactSensitive(raw); got != want {
			t.Fatalf("case %d: long fragmented output changed: got length %d, want %d", index, len(got), len(want))
		}
	}
}

func newStreamTestLog(t *testing.T) *debugLog {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "stream.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	return &debugLog{file: file, path: file.Name(), terminal: io.Discard}
}

func TestDebugLogConcurrentClose(t *testing.T) {
	oldIO, oldDebug, oldPath := ios, debugFlag, debugLogFlag
	t.Cleanup(func() { closeDebugLog(); ios, debugFlag, debugLogFlag = oldIO, oldDebug, oldPath })
	ios, _, _, _ = iostreams.Test()
	ios.ErrOut = io.Discard
	debugFlag, debugLogFlag = true, filepath.Join(t.TempDir(), "close.log")
	startDebugLog()
	writer := ios.ErrOut
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 100 {
				debugf("Debug: shutdown diagnostic\n")
				if _, err := writer.Write([]byte("ordinary stderr\n")); err != nil {
					t.Error(err)
				}
			}
		})
	}
	workers.Go(closeDebugLog)
	workers.Wait()
	closeDebugLog()
	if ios.ErrOut != writer || activeDebugLog.Load() != nil {
		t.Fatal("close replaced the shared stderr writer or left the log active")
	}
	if n, err := writer.Write([]byte("late stderr\n")); n != len("late stderr\n") || err != nil {
		t.Fatalf("closed wrapper lost terminal writes: %d, %v", n, err)
	}
}

func BenchmarkDebugLogUnfinishedCookie(b *testing.B) {
	file, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			b.Error(err)
		}
	}()
	line := strings.Repeat("x", 99) + "\n"
	b.ReportAllocs()
	b.SetBytes(14000 * int64(len(line)))
	for b.Loop() {
		l := &debugLog{file: file, terminal: io.Discard}
		l.writeFile("Cookie: \"unterminated\n")
		for range 14000 {
			l.writeFile(line)
		}
	}
}

func TestDebugLogPendingLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "mixed-writers", true: "close-write-failure"}[fail], func(t *testing.T) {
			oldIO, oldDebug, oldPath := ios, debugFlag, debugLogFlag
			t.Cleanup(func() { closeDebugLog(); ios, debugFlag, debugLogFlag = oldIO, oldDebug, oldPath })
			var stderr *bytes.Buffer
			ios, _, _, stderr = iostreams.Test()
			debugFlag, debugLogFlag = true, filepath.Join(t.TempDir(), "run.log")
			startDebugLog()
			if activeDebugLog.Load() == nil {
				t.Fatal("log did not start")
			}
			writer := activeDebugLog.Load()
			if _, err := writer.Write([]byte("Authorization: Bear")); err != nil {
				t.Fatal(err)
			}
			if fail {
				if err := writer.file.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				debugf("er split-secret\n") // Complete stderr using the diagnostic entry point.
			}
			closeDebugLog()
			before := stderr.String()
			closeDebugLog() // Idempotent: no repeated path or warning.
			if stderr.String() != before || writer.pending.Len() != 0 {
				t.Fatal("close duplicated its output or retained pending secrets")
			}
			if fail {
				if !strings.Contains(before, "debug log is incomplete") || strings.Contains(before, "Debug log:") {
					t.Fatal("pending flush failure claimed a complete file")
				}
			} else {
				contents, err := os.ReadFile(debugLogFlag)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(contents, []byte("Authorization: [REDACTED]\n")) || bytes.Contains(contents, []byte("split-secret")) {
					t.Fatal("mixed writes bypassed the shared redaction buffer")
				}
			}
			if _, err := writer.Write([]byte("late stderr")); err != nil || writer.pending.Len() != 0 || !strings.HasSuffix(stderr.String(), "late stderr") {
				t.Fatal("closed writer retained new data or changed terminal stderr")
			}
		})
	}
}
