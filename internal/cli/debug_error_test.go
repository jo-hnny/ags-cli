package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	"github.com/spf13/cobra"
)

// Exercise the real process exit with an unregistered handler: diagnostic
// coverage must come from the wrapper, not a list of production command names.
func TestDebugErrorProcess(t *testing.T) {
	if os.Getenv("AGR_TEST_DEBUG_HELPER") == "1" {
		mode, route := os.Getenv("AGR_TEST_OUTPUT"), os.Getenv("AGR_TEST_ROUTE")
		cause := fmt.Errorf("opening test connection: %w", errors.New("unique-original-cause credential=test-secret https://user:proxy-pass@localhost/path?Signature=signed-value"))
		if os.Getenv("AGR_TEST_PRESERVE") == "1" {
			cause = output.NewCLIError(&output.Failure{Code: "CUSTOM", Kind: output.KindGenericError, Message: strings.Repeat("x", 9000) + " Authorization: Bearer private-token; upstream returned 403"})
		}
		spec := command.Spec{ID: "diagnostic-test", Path: []string{"diagnostic-test"}, Use: "diagnostic-test", Short: "test", SupportsJSON: route != "text-only"}
		if mode == "ndjson" {
			spec.ID = "instance.exec"
			spec.Path = []string{"instance", "exec"}
			spec.Use = "exec"
			spec.SupportsNDJSON = true
		}
		handler := func(context.Context, command.Request) (*command.Result, error) {
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
		os.Args = append(os.Args, spec.Path...)
		Execute()
		return
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
					cmd.Env = append(os.Environ(), "AGR_TEST_DEBUG_HELPER=1", "AGR_TEST_OUTPUT="+mode, "AGR_TEST_ROUTE="+route, "AGR_TEST_DEBUG="+debugValue, "HOME="+t.TempDir())
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
	originalID, originalKey, originalToken := secretID, secretKey, tokenFlag
	t.Cleanup(func() { secretID, secretKey, tokenFlag = originalID, originalKey, originalToken })
	secretID, secretKey, tokenFlag = "test-id", "test-secret", "test-token"
	raw := "test-id test-secret test-token https://user:proxy-password@host/path?Signature=signed-value&x-amz-security-token=other-token&ok=value\nAuthorization: Bearer auth-value\nCookie: session=cookie-value\n" + strings.Repeat("x", 9000) + "test-secret"
	got := redactDiagnostic(raw)
	for _, secret := range []string{"test-id", "test-secret", "test-token", "proxy-password", "signed-value", "other-token", "auth-value", "cookie-value"} {
		if strings.Contains(got, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if !strings.Contains(got, "ok=value") || !strings.HasSuffix(got, "[truncated]") || len(got) > diagnosticLimit+20 {
		t.Fatalf("invalid bounded diagnostic length=%d", len(got))
	}
	f := &output.Failure{Code: "CUSTOM", Message: "test-secret", Details: map[string]any{"HTTPStatus": 403, "nested": map[string]any{"Authorization": "secret-header", "url": "https://host?Signature=signed-value"}}}
	clean := sanitizeFailure(f)
	data, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "test-secret") || strings.Contains(string(data), "secret-header") || strings.Contains(string(data), "signed-value") {
		t.Fatalf("leaked failure: %s", data)
	}
	if f.Message != "test-secret" {
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

func TestRedactionPreservesFailureContext(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"failed to set cookie: permission denied", "failed to set cookie: permission denied"},
		{"failed to set Set-Cookie: permission denied", "failed to set Set-Cookie: permission denied"},
		{`failed to set "cookie": permission denied`, `failed to set "cookie": permission denied`},
		{"request headers:\n  Cookie: abc123; upstream returned 403", "request headers:\n  Cookie: [REDACTED]; upstream returned 403"},
		{`{"reason":"denied", "Cookie":"abc123"}`, `{"reason":"denied", "Cookie":"[REDACTED]"}`},
		{"Cookie: abc123", "Cookie: [REDACTED]"},
		{"Cookie: abc123; upstream returned 403", "Cookie: [REDACTED]; upstream returned 403"},
		{"Set-Cookie: abc123 upstream returned 403", "Set-Cookie: [REDACTED] upstream returned 403"},
		{`"Cookie":"abc123", "reason":"upstream returned 403"`, `"Cookie":"[REDACTED]", "reason":"upstream returned 403"`},
		{`Authorization: Digest username="user", response="private-signature"; upstream returned 403`, "Authorization: [REDACTED]; upstream returned 403"},
		{`Authorization: Bearer "private-token"; upstream returned 403`, "Authorization: [REDACTED]; upstream returned 403"},
		{"Authorization: Bearer private-token upstream returned 403", "Authorization: [REDACTED] upstream returned 403"},
		{`"Authorization":"Bearer private-token", "reason":"upstream returned 403"`, `"Authorization":"[REDACTED]", "reason":"upstream returned 403"`},
		{`"Cookie":"session=private-cookie; other=private-value", "reason":"upstream returned 403"`, `"Cookie":"session=[REDACTED]; other=[REDACTED]", "reason":"upstream returned 403"`},
		{"Authorization: Bearer private-token; upstream returned 403", "authorization: [REDACTED]; upstream returned 403"},
		{"Cookie: session=private-cookie; other=private-value; upstream returned 403", "Cookie: session=[REDACTED]; other=[REDACTED]; upstream returned 403"},
		{"https://user:password@host/bad%zz?Signature=secret&normal=%zz&last=keep", "https://user:REDACTED@host/bad%zz?Signature=REDACTED&normal=%zz&last=keep"},
		{"https://host/?normal=%zz&signature=secret;last=keep", "https://host/?normal=%zz&signature=REDACTED;last=keep"},
		{"https://host/?normal=%zz&last=keep", "https://host/?normal=%zz&last=keep"},
	} {
		got := redactSensitive(tc.raw)
		if !strings.EqualFold(got, tc.want) {
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
