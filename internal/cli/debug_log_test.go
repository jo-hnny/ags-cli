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

func TestDebugLogExplicitDebugValues(t *testing.T) {
	for _, tc := range []struct {
		arg, env string
		want     bool
	}{
		{"--debug=true", "0", true},
		{"--debug=1", "0", true},
		{"--debug=false", "0", false},
		{"--debug=false", "1", false}, // An explicit flag overrides AGR_DEBUG.
		{"--debug=true", "1", true},
	} {
		t.Run(tc.arg+"/AGR_DEBUG="+tc.env, func(t *testing.T) {
			home := t.TempDir()
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
			cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_ROUTE=registry", "AGR_TEST_OUTPUT=json", "AGR_TEST_SUCCESS=1", "AGR_TEST_DEBUG=0", "AGR_TEST_DEBUG_ARG="+tc.arg, "AGR_DEBUG="+tc.env, "HOME="+home, "USERPROFILE="+home)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("exit=%v stderr=%s", err, &stderr)
			}
			files, err := filepath.Glob(filepath.Join(home, ".agr", "logs", "agr-*.log"))
			if err != nil {
				t.Fatal(err)
			}
			got := len(files) == 1 && strings.Contains(stderr.String(), "Debug log: "+files[0])
			if got != tc.want || !tc.want && (len(files) != 0 || strings.Contains(stderr.String(), "Debug")) {
				t.Fatalf("logs=%v stderr=%s", files, &stderr)
			}
			if !json.Valid(stdout.Bytes()) {
				t.Fatalf("JSON framing changed: %s", &stdout)
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
	secretID, secretKey, tokenFlag = "test-access-id", "active-private/key", "test-session-token"
	t.Cleanup(func() { secretID, secretKey, tokenFlag = oldID, oldKey, oldToken })
	for _, raw := range []string{
		"Authorization: Bearer split-secret\n",
		"Proxy-Authorization: Basic split-secret\n",
		`{"Authorization": "Bearer split-secret"}` + "\n",
		"Authorization: Digest username=\"user\", response=\"split-secret\"; upstream returned 403\n",
		"Cookie: session=split-cookie; other=second-secret\n",
		"Cookie: session=\"split-cookie\" ordinary text\n",
		"Set-Cookie: split-cookie\n",
		`headers: {"Cookie":["session=\"split-cookie\""],"Set-Cookie":["a=split-cookie","b=second-secret"]}` + "\n",
		"credentials: " + secretID + " " + secretKey + " " + tokenFlag + "\n",
		"escaped credential: " + url.QueryEscape(secretKey) + "\n",
		"request https://user:private-pass@example.invalid/path?%53ignature=private-signature&keep=visible\n",
		"错误信息：凭据 " + tokenFlag + "；原因保持可见\n",
		"failed to set authorization: permission denied by policy\nfailed to set cookie: permission denied\n",
	} {
		for _, raw := range []string{raw, strings.TrimSuffix(raw, "\n"), raw + "next ordinary line\n"} {
			want := redactDebugLogLines(raw)
			for _, secret := range []string{"split-secret", "split-cookie", "second-secret", "private-pass", "private-signature", secretID, secretKey, tokenFlag, url.QueryEscape(secretKey)} {
				if strings.Contains(want, secret) {
					t.Fatalf("line redaction leaked %q: %q", secret, want)
				}
			}
			for cut := range len(raw) + 1 {
				if got := captureDebugLogChunks(t, []string{raw[:cut], raw[cut:]}); got != want {
					t.Fatalf("redaction changed at split %d: got %q, want %q", cut, got, want)
				}
			}
			var chunks []string
			for i := range len(raw) {
				chunks = append(chunks, raw[i:i+1])
			}
			if got := captureDebugLogChunks(t, chunks); got != want {
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
	if got := captureDebugLogChunks(t, chunks); got != redactDebugLogLines(raw) {
		t.Fatal("long diagnostic or unterminated tail was lost")
	}
}

// The file contract: every line is redacted on its own, regardless of chunking.
func redactDebugLogLines(raw string) string {
	var out strings.Builder
	for _, line := range strings.SplitAfter(raw, "\n") {
		out.WriteString(redactSensitive(line))
	}
	return out.String()
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

func TestDebugLogLongLines(t *testing.T) {
	oldToken := tokenFlag
	tokenFlag = "private-session-token"
	t.Cleanup(func() { tokenFlag = oldToken })
	fill := strings.Repeat("x", debugLogLineLimit-10)
	header := strings.Repeat("x", 100) + " Authorization: Bearer private-value"
	for _, tc := range []struct{ name, raw, want string }{
		{"at limit", strings.Repeat("y", debugLogLineLimit) + "\nnext\n", strings.Repeat("y", debugLogLineLimit) + "\nnext\n"},
		{"ordinary overflow", fill + " " + strings.Repeat("z", 100) + "\nnext\n", fill + " " + debugLogOverflowMarker + "\nnext\n"},
		{"credential split by cut", fill + " Bearer=" + tokenFlag + " tail\nnext\n", fill + " " + debugLogOverflowMarker + "\nnext\n"},
		{"credential before cut", header + " " + strings.Repeat("w", debugLogLineLimit) + "\nnext\n", redactSensitive(header) + " " + debugLogOverflowMarker + "\nnext\n"},
		{"quoted header split by cut", `Authorization: Bearer "private-value ` + strings.Repeat("w ", debugLogLineLimit) + "tail\"\nnext\n", "Authorization: [REDACTED] " + debugLogOverflowMarker + "\nnext\n"},
		{"quoted cookie split by cut", `Cookie: session="private-value ` + strings.Repeat("w ", debugLogLineLimit) + "tail\"\nnext\n", "Cookie: session=[REDACTED] " + debugLogOverflowMarker + "\nnext\n"},
		{"no whitespace", strings.Repeat("q", debugLogLineLimit+1) + "\nnext\n", debugLogOverflowMarker + "\nnext\n"},
		{"unterminated", fill + " " + strings.Repeat("z", 100), fill + " " + debugLogOverflowMarker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{len(tc.raw), debugLogLineLimit, 100} {
				var chunks []string
				for remaining := tc.raw; remaining != ""; {
					n := min(size, len(remaining))
					chunks = append(chunks, remaining[:n])
					remaining = remaining[n:]
				}
				got := captureDebugLogChunks(t, chunks)
				if got != tc.want {
					t.Fatalf("chunk size %d: got length %d (%.80q...), want length %d", size, len(got), got, len(tc.want))
				}
				for _, secret := range []string{"private-value", "private-session-token", "priv"} {
					if strings.Contains(got, secret) {
						t.Fatalf("chunk size %d leaked %q", size, secret)
					}
				}
			}
		})
	}
}

func TestDebugLogPendingIsBounded(t *testing.T) {
	l := newStreamTestLog(t)
	chunk := strings.Repeat("x", 99) + "Cookie: \""
	for range 20000 {
		l.writeFile(chunk)
		if l.pending.Len() > debugLogLineLimit {
			t.Fatalf("unbounded pending: %d", l.pending.Len())
		}
	}
	l.writeFile("\nnext ordinary line\n")
	contents, err := os.ReadFile(l.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(contents), debugLogOverflowMarker+"\nnext ordinary line\n") {
		t.Fatalf("over-limit line did not recover at its newline: %.80q", contents)
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
