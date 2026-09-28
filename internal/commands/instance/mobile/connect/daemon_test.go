package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	tunnelcmd "github.com/TencentCloudAgentRuntime/ags-cli/internal/commands/instance/mobile/tunnel"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/adbtunnel"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// The parent helper runs the real CLI wrapper/root, and starts an actual child
// process. The child runs the tunnel handler and a real rejected WS handshake.
func TestDaemonParent(t *testing.T) {
	if os.Getenv("AGR_TEST_DAEMON_PARENT") != "1" {
		return
	}
	mode := os.Getenv("AGR_TEST_DAEMON_MODE")
	module := Module()
	build := module.Build
	module.Build = func(deps command.Deps) (command.Runtime, error) {
		deps.DataPlane = RuntimeDeps{
			RequireADB:     func() (string, error) { return "unused-adb", nil },
			ValidateConfig: func() error { return nil },
			NewStore:       func() (Store, error) { return &fakeStore{}, nil },
			StartTunnel: func(ctx context.Context, id string, port int) (TunnelReady, error) {
				args := append([]string{"-test.run=^TestDaemonChild$", "--"}, tunnelArguments(id, port)...)
				cmd := exec.Command(os.Args[0], args...)
				cmd.Env = append(tunnelEnv(), "AGR_TEST_DAEMON_CHILD=1")
				logID := id
				if mode == "log-failure" {
					logID = "missing/directory"
				}
				logPath, logFile := openTunnelLog(logID)
				if mode == "start" {
					cmd.Path = filepath.Join(t.TempDir(), "missing-program")
				}
				timeout := 5 * time.Second
				if mode == "timeout" {
					timeout = 200 * time.Millisecond
				}
				if mode == "deadline" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 200*time.Millisecond)
					defer cancel()
				}
				if mode == "cancel" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					defer cancel()
					timer := time.AfterFunc(200*time.Millisecond, cancel)
					defer timer.Stop()
				}
				ready, err := startTunnelProcess(ctx, cmd, logPath, logFile, timeout)
				if err != nil && cmd.Process != nil && cmd.ProcessState == nil {
					t.Fatal("child was not reaped")
				}
				return ready, err
			},
			ConnectADB: func(string, string, int, io.Writer) error { return nil },
		}
		return build(deps)
	}
	registry := command.NewRegistry()
	registry.MustRegister(module)
	if err := cli.AttachCommandRegistry(registry); err != nil {
		t.Fatal(err)
	}
	os.Args = []string{os.Args[0], "instance", "mobile", "connect", "ins-test", "-o", os.Getenv("AGR_TEST_DAEMON_OUTPUT"), "--secret-id", "test-secret-id", "--secret-key", "test-secret-key", "--token", "test-session-token"}
	if endpoint := os.Getenv("AGR_TEST_ENDPOINT_FLAG"); endpoint != "" {
		os.Args = append(os.Args, "--cloud-endpoint", endpoint)
	}
	if os.Getenv("AGR_TEST_DAEMON_DEBUG") == "1" {
		os.Args = append(os.Args, "--debug")
	}
	cli.Execute()
}

func TestDaemonChild(t *testing.T) {
	if os.Getenv("AGR_TEST_DAEMON_CHILD") != "1" {
		return
	}
	mode := os.Getenv("AGR_TEST_DAEMON_MODE")
	switch mode {
	case "ready":
		_ = json.NewEncoder(os.Stdout).Encode(adbtunnel.ReadyMessage{Status: "ready", Port: 5555, PID: os.Getpid()})
		os.Exit(0)
	case "invalid-kind":
		fmt.Println(`{"status":"error","failure":{"Code":"BOGUS","Kind":"unknown-kind","Message":"invalid"},"exit_code":1}`)
		time.Sleep(time.Minute)
		os.Exit(0)
	case "exit":
		os.Exit(23)
	case "empty":
		os.Exit(0)
	case "timeout", "cancel", "deadline":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "malformed":
		fmt.Println("not-json")
		time.Sleep(time.Minute)
		os.Exit(0)
	case "invalid":
		fmt.Println(`{"status":"error","failure":{"Code":"BOGUS"}}`)
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		t.Fatal("missing child arguments")
	}
	args := append([]string{os.Args[0]}, os.Args[separator+1:]...)
	if (os.Getenv("AGR_TEST_DAEMON_DEBUG") == "1") != slices.Contains(args, "--debug") {
		t.Fatal("debug flag not forwarded")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "private-response-body")
	}))
	defer server.Close()
	module := tunnelcmd.Module()
	build := module.Build
	module.Build = func(deps command.Deps) (command.Runtime, error) {
		deps.DataPlane = tunnelcmd.RuntimeDeps{
			ValidateConfig: func() error { return nil },
			AcquireToken: func(context.Context, string) (string, error) {
				if want := os.Getenv("AGR_TEST_EXPECT_ENDPOINT"); want != "" && config.GetCloudEndpoint() != want {
					return "", fmt.Errorf("child endpoint = %q, want %q", config.GetCloudEndpoint(), want)
				}
				if mode == "classified" {
					err := output.NewConflictError("CUSTOM_CHILD", "child classified failure", "child hint")
					err.ExitCode = 7
					err.Failure.Details = map[string]any{"RequestId": "req-child", "access_token": "child-private-token", "URL": "https://user:proxy-password@example.test/?Signature=signed-value"}
					return "", err
				}
				return "sandbox-token", nil
			},
			NewTunnel: func(opts adbtunnel.TunnelOptions) (tunnelcmd.Tunnel, error) {
				if config.GetSecretID() != "test-secret-id" || config.GetSecretKey() != "test-secret-key" || config.GetToken() != "test-session-token" {
					t.Fatal("credentials not forwarded")
				}
				logger := opts.Logger
				if logger == nil {
					logger = log.Default()
				}
				logger.Printf("child-context test-secret-id test-secret-key test-session-token https://user:proxy-password@example.test/?Signature=signed-value\nAuthorization: Bearer header-secret\nCookie: session=cookie-secret")
				opts.Endpoint = strings.TrimPrefix(server.URL, "https://")
				opts.Insecure = true // test-only loopback TLS endpoint
				return adbtunnel.New(opts)
			},
		}
		return build(deps)
	}
	registry := command.NewRegistry()
	registry.MustRegister(module)
	if err := cli.AttachCommandRegistry(registry); err != nil {
		t.Fatal(err)
	}
	os.Args = args
	cli.Execute()
}

func TestDaemonFailureProcess(t *testing.T) {
	for _, tc := range []struct {
		mode, code, stage string
		exit              int
	}{
		{"classified", "CUSTOM_CHILD", "", 7},
		{"deadline", "TIMEOUT", "readiness_wait", 1},
		{"handshake", "TUNNEL_AUTH_FAILED", "websocket_handshake", 4},
		{"log-failure", "TUNNEL_AUTH_FAILED", "websocket_handshake", 4},
		{"start", "TUNNEL_START_FAILED", "start", 1},
		{"exit", "TUNNEL_EXITED", "exit", 1},
		{"empty", "TUNNEL_EXITED", "exit", 1},
		{"malformed", "TUNNEL_PROTOCOL_ERROR", "readiness_protocol", 1},
		{"invalid", "TUNNEL_PROTOCOL_ERROR", "readiness_protocol", 1},
		{"invalid-kind", "TUNNEL_PROTOCOL_ERROR", "readiness_protocol", 1},
		{"timeout", "TUNNEL_READY_TIMEOUT", "readiness_wait", 1},
		{"cancel", "CANCELED", "readiness_wait", 1},
	} {
		for _, debug := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/debug=%t", tc.mode, debug), func(t *testing.T) {
				home := t.TempDir()
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDaemonParent$")
				debugValue := "0"
				if debug {
					debugValue = "1"
				}
				cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "AGR_TEST_DAEMON_PARENT=1", "AGR_TEST_DAEMON_MODE="+tc.mode, "AGR_TEST_DAEMON_DEBUG="+debugValue, "AGR_TEST_DAEMON_OUTPUT=json")
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				var exit *exec.ExitError
				if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != tc.exit {
					t.Fatalf("exit=%v stdout=%s stderr=%s", err, &stdout, &stderr)
				}
				var envelope output.Envelope
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("invalid envelope: %s (%v)", stdout.String(), err)
				}
				if envelope.Failure.Code != tc.code || (tc.stage != "" && envelope.Failure.Details["Stage"] != tc.stage) {
					t.Fatalf("failure=%#v", envelope.Failure)
				}
				if tc.code == "TUNNEL_AUTH_FAILED" && (envelope.Failure.Kind != output.KindAuthOrPermission || envelope.Failure.Details["HTTPStatus"] != float64(403)) {
					t.Fatalf("lost child failure: %#v", envelope.Failure)
				}
				if tc.mode == "classified" && (envelope.Failure.Kind != output.KindConflict || envelope.Failure.Hint != "child hint" || envelope.Failure.Details["RequestId"] != "req-child") {
					t.Fatalf("lost child contract: %#v", envelope.Failure)
				}
				if tc.mode == "exit" && envelope.Failure.Details["ExitCode"] != float64(23) {
					t.Fatal("lost observed exit code")
				}
				logPath, hasLog := envelope.Failure.Details["LogPath"].(string)
				if hasLog != (tc.mode != "log-failure") {
					t.Fatalf("LogPath presence=%t", hasLog)
				}
				var logData []byte
				if hasLog {
					var err error
					logData, err = os.ReadFile(logPath)
					if err != nil {
						t.Fatal(err)
					}
				}
				all := stdout.String() + stderr.String() + string(logData)
				for _, secret := range []string{"test-secret-id", "test-secret-key", "test-session-token", "proxy-password", "signed-value", "header-secret", "cookie-secret", "private-response-body", "child-private-token"} {
					if strings.Contains(all, secret) {
						t.Fatalf("secret %q leaked", secret)
					}
				}
				// Child debug lines in the separately labeled tail are not parent duplicates.
				parent, _, _ := strings.Cut(stderr.String(), "tunnel log tail:")
				want := 0
				if debug {
					want = 1
				}
				if strings.Count(parent, "Debug: error=") != want {
					t.Fatalf("parent diagnostics=%s", parent)
				}
				if strings.Contains(stderr.String(), "tunnel log tail:") != (debug && hasLog) {
					t.Fatalf("tail=%s", stderr.String())
				}
				if tc.mode == "handshake" && (!strings.Contains(string(logData), "child-context") || strings.Contains(string(logData), "Debug: error=") != debug) {
					t.Fatalf("child log=%s", logData)
				}
			})
		}
	}
}

func TestDaemonTextAndSuccessProcess(t *testing.T) {
	for _, tc := range []struct {
		mode string
		exit int
	}{{"handshake", 4}, {"ready", 0}} {
		t.Run(tc.mode, func(t *testing.T) {
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDaemonParent$")
			cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "AGR_TEST_DAEMON_PARENT=1", "AGR_TEST_DAEMON_MODE="+tc.mode, "AGR_TEST_DAEMON_OUTPUT=text", "AGR_TEST_DAEMON_DEBUG=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if tc.exit == 0 {
				if err != nil || !strings.Contains(stderr.String(), "connected to ins-test") {
					t.Fatalf("result=%v %s", err, stderr.String())
				}
				return
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != tc.exit || stdout.Len() != 0 {
				t.Fatalf("exit=%v stdout=%s stderr=%s", err, &stdout, &stderr)
			}
			parent, _, _ := strings.Cut(stderr.String(), "tunnel log tail:")
			if strings.Count(parent, "Debug: error=") != 1 || !strings.Contains(stderr.String(), "Code: TUNNEL_AUTH_FAILED") || !strings.Contains(stderr.String(), "LogPath:") {
				t.Fatalf("stderr=%s", stderr.String())
			}
		})
	}
}

func TestDaemonEndpointProcess(t *testing.T) {
	for _, tc := range []struct {
		name, file, env, flag, want string
	}{
		{"default", "", "", "", "ags.tencentcloudapi.com"},
		{"file", "file.example.test", "", "", "file.example.test"},
		{"env_over_file", "file.example.test", "env.example.test", "", "env.example.test"},
		{"flag", "", "", "flag.example.test", "flag.example.test"},
		{"flag_over_file", "file.example.test", "", "flag.example.test", "flag.example.test"},
		{"flag_over_env_and_file", "file.example.test", "env.example.test", "flag.example.test", "flag.example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".agr"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".agr", "config.toml"), []byte(fmt.Sprintf("cloud_endpoint = %q\n", tc.file)), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDaemonParent$")
			cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "AGR_TEST_DAEMON_PARENT=1", "AGR_TEST_DAEMON_MODE=handshake", "AGR_TEST_DAEMON_OUTPUT=json", "AGR_TEST_DAEMON_DEBUG=1", "AGR_CLOUD_ENDPOINT="+tc.env, "AGR_TEST_ENDPOINT_FLAG="+tc.flag, "AGR_TEST_EXPECT_ENDPOINT="+tc.want)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 4 {
				t.Fatalf("exit=%v stdout=%s stderr=%s", err, &stdout, &stderr)
			}
			var envelope output.Envelope
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope.Failure == nil || envelope.Failure.Code != "TUNNEL_AUTH_FAILED" {
				t.Fatalf("unexpected result: %s (%v)", &stdout, err)
			}
		})
	}
}
