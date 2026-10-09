package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
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
	if activeDebugLog == nil {
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
	if err := activeDebugLog.file.Close(); err != nil {
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
