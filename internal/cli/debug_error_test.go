package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/token"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

// Exercise the real process exit with an unregistered handler: diagnostic
// coverage must come from the wrapper, not a list of production command names.
func TestDebugErrorProcess(t *testing.T) {
	if os.Getenv("AGR_TEST_DEBUG_HELPER") == "1" {
		mode, route := os.Getenv("AGR_TEST_OUTPUT"), os.Getenv("AGR_TEST_ROUTE")
		cause := fmt.Errorf("opening test connection: %w", errors.New("unique-original-cause credential=test-secret https://user:proxy-pass@localhost/path?Signature=signed-value"))
		if os.Getenv("AGR_TEST_LONG_CHAIN") == "1" {
			cause = fmt.Errorf("opening test connection %s: %w", strings.Repeat("错", 3000), cause)
		}
		if os.Getenv("AGR_TEST_AUTH_PROSE") == "1" {
			cause = output.NewCLIError(&output.Failure{Code: "CUSTOM", Kind: output.KindGenericError, Message: "failed to set authorization: permission denied by policy"})
		}
		if os.Getenv("AGR_TEST_PRESERVE") == "1" {
			cause = output.NewCLIError(&output.Failure{Code: "CUSTOM", Kind: output.KindGenericError, Message: strings.Repeat("x", 9000) + " Authorization: Bearer private-token-0123456789; upstream returned 403"})
		}
		spec := command.Spec{ID: "diagnostic-test", Path: []string{"diagnostic-test"}, Use: "diagnostic-test", Short: "test", SupportsJSON: route != "text-only"}
		if mode == "ndjson" {
			spec.ID = "instance.exec"
			spec.Path = []string{"instance", "exec"}
			spec.Use = "exec"
			spec.SupportsNDJSON = true
		}
		handler := func(context.Context, command.Request) (*command.Result, error) {
			if os.Getenv("AGR_TEST_SUCCESS") == "1" {
				return &command.Result{Data: map[string]any{"OK": true}}, nil
			}
			if mode == "ndjson" {
				nw := output.NewNDJSONWriter(ios.Out, spec.ID)
				_ = nw.WriteStarted(nil)
				classified := classifyCLIError(cause)
				_ = nw.WriteFailed(nil, classified.Failure)
				return &command.Result{StreamDone: true, ExitCode: classified.ExitCode, Cause: cause}, nil
			}
			return nil, cause
		}
		var cmd *cobra.Command
		if route == "legacy" {
			cmd = &cobra.Command{Use: spec.Use}
			cmd.RunE = Wrap(spec.ID, func(*cobra.Command, []string) (*CmdResult, error) {
				result, err := handler(t.Context(), command.Request{})
				return FromCommandResult(result), err
			})
		} else {
			var err error
			cmd, err = buildCLIRegistryCommand(command.Module{Descriptor: command.Descriptor{Spec: spec}, Build: func(command.Deps) (command.Runtime, error) {
				return command.Runtime{Handler: command.HandlerFunc(handler)}, nil
			}}, command.Deps{})
			if err != nil {
				t.Fatal(err)
			}
		}
		if mode == "ndjson" {
			group := &cobra.Command{Use: "instance"}
			group.AddCommand(cmd)
			rootCmd.AddCommand(group)
		} else {
			rootCmd.AddCommand(cmd)
		}
		os.Args = []string{"agr", "--non-interactive", "--secret-key", "test-secret", "-o", mode}
		if os.Getenv("AGR_TEST_DEBUG") == "1" {
			os.Args = append(os.Args, "--debug")
		}
		if arg := os.Getenv("AGR_TEST_DEBUG_ARG"); arg != "" {
			os.Args = append(os.Args, arg)
		}
		if path := os.Getenv("AGR_TEST_DEBUG_LOG"); path != "" {
			os.Args = append(os.Args, "--debug-log", path)
		}
		os.Args = append(os.Args, spec.Path...)
		if os.Getenv("AGR_TEST_HELP_EXIT") == "1" {
			os.Args = append(os.Args, "--jq=.", "--help")
		}
		Execute()
		os.Exit(0)
	}
	for _, route := range []string{"legacy", "registry", "text-only"} {
		for _, mode := range []string{"text", "json", "ndjson"} {
			if route == "text-only" && mode != "text" {
				continue
			}
			for _, debug := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/debug=%t", route, mode, debug), func(t *testing.T) {
					cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
					debugValue := "0"
					if debug {
						debugValue = "1"
					}
					home := t.TempDir()
					cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_ROUTE="+route, "AGR_TEST_DEBUG="+debugValue, "HOME="+home, "USERPROFILE="+home)
					var stdout, stderr bytes.Buffer
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					err := cmd.Run()
					var exit *exec.ExitError
					if !errors.As(err, &exit) || exit.ExitCode() != 1 {
						t.Fatalf("exit=%v stderr=%s", err, &stderr)
					}
					want := 0
					if debug {
						want = 1
					}
					if strings.Count(stderr.String(), "unique-original-cause") != want || strings.Count(stderr.String(), "Debug: error=") != want {
						t.Fatalf("lost/duplicated diagnostic: %s", &stderr)
					}
					for _, secret := range []string{"test-secret", "proxy-pass", "signed-value"} {
						if strings.Contains(stderr.String()+stdout.String(), secret) {
							t.Fatalf("secret leaked: %s / %s", &stdout, &stderr)
						}
					}
					if debug && !strings.Contains(stderr.String(), "opening test connection") {
						t.Fatalf("context lost: %s", &stderr)
					}
					if mode == "json" {
						var env output.Envelope
						if json.Unmarshal(stdout.Bytes(), &env) != nil || env.Failure.Code != "INTERNAL_ERROR" {
							t.Fatalf("invalid envelope: %s", &stdout)
						}
					}
					if mode == "ndjson" {
						lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
						if len(lines) != 2 {
							t.Fatalf("invalid stream: %s", &stdout)
						}
						for i, line := range lines {
							var event output.NDJSONEvent
							if json.Unmarshal(line, &event) != nil {
								t.Fatalf("invalid event: %s", line)
							}
							if event.Type != []string{"started", "failed"}[i] {
								t.Fatalf("event=%#v", event)
							}
						}
					}
				})
			}
		}
	}
}

func TestDiagnosticRedaction(t *testing.T) {
	useTestSecrets(t)
	raw := "test-access-id active-private/key test-session-token runtime-access-token-value https://user:proxy-password@host/path?Signature=signed-value&x-amz-security-token=other-token&ok=value\nAuthorization: Bearer auth-value-0123456789abcdef\n" + strings.Repeat("x", 9000) + "active-private/key"
	got := redactDiagnostic(raw)
	for _, secret := range append(testSecrets, "proxy-password", "signed-value", "other-token", "auth-value") {
		if strings.Contains(got, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if !strings.Contains(got, "ok=value") || !strings.HasSuffix(got, "[truncated]") || len(got) > diagnosticLimit+20 {
		t.Fatalf("invalid bounded diagnostic length=%d", len(got))
	}
	f := &output.Failure{Code: "CUSTOM", Message: "active-private/key", Details: map[string]any{"HTTPStatus": 403, "nested": map[string]any{"Authorization": "secret-header", "url": "https://host?Signature=signed-value"}}}
	clean := sanitizeFailure(f)
	data, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "active-private/key") || strings.Contains(string(data), "secret-header") || strings.Contains(string(data), "signed-value") {
		t.Fatalf("leaked failure: %s", data)
	}
	if f.Message != "active-private/key" {
		t.Fatal("original failure mutated")
	}
}

func TestOrdinaryFailureCompatibility(t *testing.T) {
	original := &output.Failure{Code: "CUSTOM", Kind: output.KindGenericError, Message: "无法打开配置文件 https://example.com/a%2fb?b=2&a=1", Hint: "Check the file permissions.", Details: map[string]any{"HTTPStatus": 502, "RequestId": "req-1"}}
	before, _ := json.Marshal(original)
	after, _ := json.Marshal(sanitizeFailure(original))
	if !bytes.Equal(before, after) {
		t.Fatalf("ordinary failure changed: %s -> %s", before, after)
	}
	for _, length := range []int{diagnosticLimit - 1, diagnosticLimit - 2} {
		raw := strings.Repeat("x", length) + "错误"
		clean := redactDiagnostic(raw)
		if !utf8.ValidString(clean) || !strings.HasSuffix(clean, " [truncated]") {
			t.Fatalf("invalid truncation: %q", clean[length-1:])
		}
	}
}

// Redaction replaces credential values only; every other byte survives.
func TestRedactionPreservesFailureContext(t *testing.T) {
	useTestSecrets(t)
	const bearer = "eyJhbGciOiJIUzI1NiJ9.private-payload.signature"
	for _, tc := range []struct{ raw, want string }{
		{"failed to set authorization: permission denied by policy", "failed to set authorization: permission denied by policy"},
		{"failed to set cookie: permission denied", "failed to set cookie: permission denied"},
		{"basic authentication failed; bearer token expired", "basic authentication failed; bearer token expired"},
		{"Authorization: can't parse header\nreason: upstream returned 403", "Authorization: can't parse header\nreason: upstream returned 403"},
		{`Authorization: "opaque"; upstream returned 403`, `Authorization: "opaque"; upstream returned 403`},
		{`{"Authorization":[],"Proxy-Authorization":null,"Reason":["upstream returned 403"]}`, `{"Authorization":[],"Proxy-Authorization":null,"Reason":["upstream returned 403"]}`},
		{"Authorization: Bearer " + bearer + "; upstream returned 403", "Authorization: Bearer [REDACTED]; upstream returned 403"},
		{`Authorization: Bearer "` + bearer + `"; upstream returned 403`, `Authorization: Bearer "[REDACTED]"; upstream returned 403`},
		{"Proxy-Authorization: Basic dXNlcjpwcml2YXRlLXBhc3N3b3Jk upstream returned 403", "Proxy-Authorization: Basic [REDACTED] upstream returned 403"},
		{"map[Authorization:[Bearer " + bearer + " Basic dXNlcjpwcml2YXRlLXBhc3N3b3Jk] Reason:[upstream returned 403]]", "map[Authorization:[Bearer [REDACTED] Basic [REDACTED]] Reason:[upstream returned 403]]"},
		{`{"Authorization":["Bearer ` + bearer + `"],"Reason":["upstream returned 403"]}`, `{"Authorization":["Bearer [REDACTED]"],"Reason":["upstream returned 403"]}`},
		{"Authorization: TC3-HMAC-SHA256 Credential=test-access-id/2026-10-09/ags/tc3_request; upstream returned 403", "Authorization: TC3-HMAC-SHA256 Credential=[REDACTED]/2026-10-09/ags/tc3_request; upstream returned 403"},
		{`{"X-Access-Token":"runtime-access-token-value","Reason":"upstream returned 403"}`, `{"X-Access-Token":"[REDACTED]","Reason":"upstream returned 403"}`},
		{"https://user:password@host/bad%zz?Signature=secret&normal=%zz&last=keep", "https://user:REDACTED@host/bad%zz?Signature=REDACTED&normal=%zz&last=keep"},
		{"https://host/?normal=%zz&signature=secret;last=keep", "https://host/?normal=%zz&signature=REDACTED;last=keep"},
		{"https://host/?normal=%zz&last=keep", "https://host/?normal=%zz&last=keep"},
	} {
		if got := redactSensitive(tc.raw); got != tc.want {
			t.Errorf("redaction: got %q want %q", got, tc.want)
		}
	}
	long := strings.Repeat("错", 4000) + "root cause at end"
	original := &output.Failure{Message: long, Hint: long, Fix: long, Details: map[string]any{"signature": "func(context.Context) error", "token": "unexpected identifier", "nested": map[string]any{"reason": long, "SecretKey": "private-key"}}}
	got := sanitizeFailure(original)
	if got.Message != long || got.Hint != long || got.Fix != long || got.Details["signature"] != original.Details["signature"] || got.Details["token"] != original.Details["token"] {
		t.Fatal("ordinary failure context lost")
	}
	nested := got.Details["nested"].(map[string]any)
	if nested["reason"] != long || nested["SecretKey"] != "[REDACTED]" {
		t.Fatal("nested detail policy incorrect")
	}
}

func TestDataPlaneTokensAreMasked(t *testing.T) {
	useTestSecrets(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cache, err := token.NewCache()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		acquire func(context.Context, string) (string, error)
	}{{"cached", GetCachedTokenOrAcquire}, {"acquire", acquireInstanceToken}} {
		t.Run(tc.name, func(t *testing.T) {
			value := "data-plane-token-" + tc.name
			if err := cache.Set("ins-"+tc.name, value); err != nil {
				t.Fatal(err)
			}
			got, err := tc.acquire(t.Context(), "ins-"+tc.name)
			if err != nil || got != value {
				t.Fatalf("token=%q err=%v", got, err)
			}
			if masked := redactSensitive("connect failed with " + value); masked != "connect failed with [REDACTED]" {
				t.Fatalf("data-plane token not masked: %q", masked)
			}
		})
	}
}

// Header maps printed or marshaled in any shape keep every neighbouring field;
// only the credentials the CLI holds are replaced, in all three output paths.
func TestRedactionHTTPHeaderMatrix(t *testing.T) {
	useTestSecrets(t)
	oldIO, oldDebug := ios, debugFlag
	t.Cleanup(func() { ios, debugFlag = oldIO, oldDebug })
	debugFlag = true
	for _, key := range []string{"Authorization", "Proxy-Authorization", "X-Access-Token", "Cookie", "Set-Cookie"} {
		for _, values := range [][]string{nil, {}, {"Bearer runtime-access-token-value"}, {"session=test-session-token", "Basic active-private/key"}} {
			header := http.Header{key: values, "A-Reason": {"denied by policy"}, "Reason": {"upstream returned 403"}}
			encoded, err := json.Marshal(header)
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range []string{fmt.Sprint(header), string(encoded)} {
				want := maskSecrets(raw, testSecrets)
				failure := sanitizeFailure(&output.Failure{Message: raw})
				var stderr *bytes.Buffer
				ios, _, _, stderr = iostreams.Test()
				debugError(errors.New(raw))
				for path, got := range map[string]string{"failure": failure.Message, "stderr": stderr.String(), "log": captureDebugLogChunks(t, []string{raw + "\n"})} {
					if !strings.Contains(got, want) {
						t.Errorf("%s: %q -> %q, want %q", path, raw, got, want)
					}
				}
			}
		}
	}
}

func TestFailureProcessPreservesAuthorizationReason(t *testing.T) {
	for _, mode := range []string{"text", "json", "ndjson"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
			home := t.TempDir()
			cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_ROUTE=registry", "AGR_TEST_DEBUG=0", "AGR_TEST_AUTH_PROSE=1", "HOME="+home, "USERPROFILE="+home)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("exit=%v", err)
			}
			if !strings.Contains(stdout.String()+stderr.String(), "failed to set authorization: permission denied by policy") {
				t.Fatalf("authorization reason lost in %s output", mode)
			}
		})
	}
}

func TestDebugErrorProcessPreservesLongChainCause(t *testing.T) {
	for _, mode := range []string{"text", "json", "ndjson"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
			home := t.TempDir()
			cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_ROUTE=registry", "AGR_TEST_DEBUG=1", "AGR_TEST_LONG_CHAIN=1", "HOME="+home, "USERPROFILE="+home)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("exit=%v", err)
			}
			if !strings.Contains(stderr.String(), "unique-original-cause") || !strings.Contains(stderr.String(), "[truncated]") || !strings.Contains(stderr.String(), "opening test connection") {
				t.Fatalf("debug context or cause lost in %s output", mode)
			}
			if !utf8.Valid(stderr.Bytes()) {
				t.Fatal("invalid UTF-8 diagnostic")
			}
			for _, secret := range []string{"test-secret", "proxy-pass", "signed-value"} {
				if strings.Contains(stdout.String()+stderr.String(), secret) {
					t.Fatalf("secret leaked: %s", secret)
				}
			}
		})
	}
}

func TestDebugErrorBoundsEveryCause(t *testing.T) {
	oldIO, oldDebug := ios, debugFlag
	t.Cleanup(func() { ios, debugFlag = oldIO, oldDebug })
	debugFlag = true
	long := strings.Repeat("错", 3000)
	leaf := errors.New("unique-underlying-cause")
	nested := leaf
	for range 8 {
		nested = fmt.Errorf("operation %s: %w", long, nested)
	}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"nested", nested},
		{"joined", errors.Join(fmt.Errorf("operation %s: %w", long, leaf), errors.New("unique-second-cause"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr *bytes.Buffer
			ios, _, _, stderr = iostreams.Test()
			debugError(tc.err)
			got := stderr.String()
			if !strings.Contains(got, "unique-underlying-cause") || !strings.Contains(got, "[truncated]") || len(got) > diagnosticLimit || !utf8.ValidString(got) {
				t.Fatalf("invalid bounded diagnostic length=%d", len(got))
			}
			if tc.name == "joined" && !strings.Contains(got, "unique-second-cause") {
				t.Fatal("joined cause lost")
			}
		})
	}
}

func TestOrdinaryFailureProcessPreservesLongContext(t *testing.T) {
	for _, mode := range []string{"text", "json", "ndjson"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDebugErrorProcess$")
			home := t.TempDir()
			cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_ROUTE=registry", "AGR_TEST_DEBUG=0", "AGR_TEST_PRESERVE=1", "HOME="+home, "USERPROFILE="+home)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("exit=%v", err)
			}
			combined := stdout.String() + stderr.String()
			if strings.Contains(combined, "private-token") || strings.Contains(combined, "[truncated]") || !strings.Contains(combined, strings.Repeat("x", 9000)) || !strings.Contains(combined, "upstream returned 403") {
				t.Fatalf("context lost or secret leaked in %s output", mode)
			}
		})
	}
}
