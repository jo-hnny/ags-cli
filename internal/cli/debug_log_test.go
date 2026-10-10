package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/rand/v2"
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

// useTestSecrets installs configured and runtime-registered test credentials.
func useTestSecrets(t *testing.T) {
	t.Helper()
	oldID, oldKey, oldToken := secretID, secretKey, tokenFlag
	runtimeSecrets.Lock()
	oldRuntime := runtimeSecrets.values
	runtimeSecrets.values = nil
	runtimeSecrets.Unlock()
	t.Cleanup(func() {
		secretID, secretKey, tokenFlag = oldID, oldKey, oldToken
		runtimeSecrets.Lock()
		runtimeSecrets.values = oldRuntime
		runtimeSecrets.Unlock()
	})
	secretID, secretKey, tokenFlag = "test-access-id", "active-private/key", "test-session-token"
	MaskSecret("runtime-access-token-value")
}

var testSecrets = []string{"test-access-id", "active-private/key", url.QueryEscape("active-private/key"), "test-session-token", "runtime-access-token-value"}

// The file contract: stderr is copied unchanged except that known secrets are
// replaced, wherever transport chunks split them. Nothing else is dropped.
func TestDebugLogMasksKnownSecretsAcrossWriteBoundaries(t *testing.T) {
	useTestSecrets(t)
	for _, tc := range []struct{ raw, want string }{
		{"credentials: test-access-id active-private/key test-session-token\n", "credentials: [REDACTED] [REDACTED] [REDACTED]\n"},
		{"escaped credential: " + url.QueryEscape("active-private/key") + "\n", "escaped credential: [REDACTED]\n"},
		{"remote stderr: token=runtime-access-token-value; upstream returned 403\n", "remote stderr: token=[REDACTED]; upstream returned 403\n"},
		{"错误信息：凭据 test-session-token；原因保持可见", "错误信息：凭据 [REDACTED]；原因保持可见"},
		{"Authorization: can't parse header\nreason: upstream returned 403\n", "Authorization: can't parse header\nreason: upstream returned 403\n"},
		{`{"Cookie":["session=\"opaque\""],"Reason":["upstream returned 403"]}` + "\n", `{"Cookie":["session=\"opaque\""],"Reason":["upstream returned 403"]}` + "\n"},
		{"test-test-session-token test-", "test-[REDACTED] [REDACTED]"}, // A final fragment that may start a secret is not written.
	} {
		for cut := range len(tc.raw) + 1 {
			if got := captureDebugLogChunks(t, []string{tc.raw[:cut], tc.raw[cut:]}); got != tc.want {
				t.Fatalf("split %d: got %q, want %q", cut, got, tc.want)
			}
		}
		var chunks []string
		for i := range len(tc.raw) {
			chunks = append(chunks, tc.raw[i:i+1])
		}
		if got := captureDebugLogChunks(t, chunks); got != tc.want {
			t.Fatalf("bytewise: got %q, want %q", got, tc.want)
		}
	}
}

// Any split of the stream gives the same file as one write, and no registered
// value survives, including values that overlap themselves or each other.
func TestDebugLogChunkingEquivalence(t *testing.T) {
	useTestSecrets(t)
	selfOverlap, left, right := "ABCD1234567890ABCD", "cross-left-XYZW", "XYZW-cross-right"
	secrets := []string{selfOverlap, left, right}
	for _, secret := range secrets {
		MaskSecret(secret)
	}
	for _, tc := range []struct{ raw, want string }{
		{selfOverlap + "\n", "[REDACTED]\n"},
		{selfOverlap, "[REDACTED]"},
		{"x" + selfOverlap + "1234567890ABCD tail\n", "x[REDACTED] tail\n"},
		{left + "-cross-right\n", "[REDACTED]\n"},
		{left + "\n" + right + "\n", "[REDACTED]\n[REDACTED]\n"},
		{"ordinary ABCD and XYZW text\n", "ordinary ABCD and XYZW text\n"},
		{"ends inside ABCD12", "ends inside [REDACTED]"},
	} {
		if got := captureDebugLogChunks(t, []string{tc.raw}); got != tc.want {
			t.Fatalf("single write %q: got %q, want %q", tc.raw, got, tc.want)
		}
		for cut := range len(tc.raw) + 1 {
			if got := captureDebugLogChunks(t, []string{tc.raw[:cut], tc.raw[cut:]}); got != tc.want {
				t.Fatalf("%q split %d: got %q, want %q", tc.raw, cut, got, tc.want)
			}
		}
		var chunks []string
		for i := range len(tc.raw) {
			chunks = append(chunks, tc.raw[i:i+1])
		}
		if got := captureDebugLogChunks(t, chunks); got != tc.want {
			t.Fatalf("%q bytewise: got %q, want %q", tc.raw, got, tc.want)
		}
	}
	rng := rand.New(rand.NewPCG(138, 139))
	pieces := append([]string{"ABCD", "XYZW", "1234567890", "cross-", "-", " ", "\n"}, secrets...)
	for range 300 {
		var raw strings.Builder
		for range rng.IntN(12) + 1 {
			raw.WriteString(pieces[rng.IntN(len(pieces))])
		}
		text := raw.String()
		want := captureDebugLogChunks(t, []string{text})
		var chunks []string
		for remaining := text; remaining != ""; {
			n := min(rng.IntN(8)+1, len(remaining))
			chunks = append(chunks, remaining[:n])
			remaining = remaining[n:]
		}
		if got := captureDebugLogChunks(t, chunks); got != want {
			t.Fatalf("chunks %q: got %q, want %q", chunks, got, want)
		}
		for _, secret := range secrets {
			if strings.Contains(want, secret) {
				t.Fatalf("%q leaked %q: %q", text, secret, want)
			}
		}
	}
}

func TestDebugLogKeepsLongLinesWhole(t *testing.T) {
	useTestSecrets(t)
	long := strings.Repeat("长", 50000) + " test-session-token " + strings.Repeat("x", 200000)
	for _, raw := range []string{long + "\nnext\n", long} {
		want := strings.Replace(raw, "test-session-token", "[REDACTED]", 1)
		for _, size := range []int{1, 7, 4096, len(raw)} {
			var chunks []string
			for remaining := raw; remaining != ""; {
				n := min(size, len(remaining))
				chunks = append(chunks, remaining[:n])
				remaining = remaining[n:]
			}
			if got := captureDebugLogChunks(t, chunks); got != want {
				t.Fatalf("chunk size %d: got length %d, want length %d", size, len(got), len(want))
			}
		}
	}
}

func TestDebugLogHoldsOnlyPossibleSecretPrefixes(t *testing.T) {
	useTestSecrets(t)
	l := newStreamTestLog(t)
	l.writeFile("ordinary output without newline ")
	if l.pending != "" {
		t.Fatalf("ordinary text held back: %q", l.pending)
	}
	l.writeFile("prefix test-sess")
	if l.pending != "test-sess" {
		t.Fatalf("pending=%q", l.pending)
	}
	l.writeFile("ion-token suffix\n")
	longest := 0
	for _, secret := range testSecrets {
		longest = max(longest, len(secret))
	}
	for range 10000 {
		l.writeFile(strings.Repeat("x", 99) + "runtime-access-token")
		if len(l.pending) >= longest {
			t.Fatalf("pending exceeds the longest secret: %d", len(l.pending))
		}
	}
	contents, err := os.ReadFile(l.path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(contents), "ordinary output without newline prefix [REDACTED] suffix\n") {
		t.Fatalf("file=%.80q", contents)
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

func TestDebugLogPendingLifecycle(t *testing.T) {
	for _, tc := range []struct{ name, tail, want string }{
		{"mixed-writers", "ion-token\n", "credential: [REDACTED]\n"},
		{"ends-inside-secret", "", "credential: [REDACTED]"},
		{"close-write-failure", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useTestSecrets(t)
			oldIO, oldDebug, oldPath := ios, debugFlag, debugLogFlag
			t.Cleanup(func() { closeDebugLog(); ios, debugFlag, debugLogFlag = oldIO, oldDebug, oldPath })
			var stderr *bytes.Buffer
			ios, _, _, stderr = iostreams.Test()
			debugFlag, debugLogFlag = true, filepath.Join(t.TempDir(), "run.log")
			startDebugLog()
			writer := activeDebugLog.Load()
			if writer == nil {
				t.Fatal("log did not start")
			}
			if _, err := writer.Write([]byte("credential: test-sess")); err != nil {
				t.Fatal(err)
			}
			fail := tc.name == "close-write-failure"
			if fail {
				if err := writer.file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if tc.tail != "" {
				debugf("%s", tc.tail) // Complete stderr using the diagnostic entry point.
			}
			closeDebugLog()
			before := stderr.String()
			closeDebugLog() // Idempotent: no repeated path or warning.
			if stderr.String() != before || writer.pending != "" {
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
				_, body, _ := strings.Cut(string(contents), "\n")
				if body != tc.want {
					t.Fatalf("file=%q, want %q", body, tc.want)
				}
			}
			if _, err := writer.Write([]byte("late stderr")); err != nil || writer.pending != "" || !strings.HasSuffix(stderr.String(), "late stderr") {
				t.Fatal("closed writer retained new data or changed terminal stderr")
			}
		})
	}
}
