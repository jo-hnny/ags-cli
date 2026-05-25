package exec

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

func TestModuleDescriptor(t *testing.T) {
	module := Module()
	spec := module.Descriptor.Spec
	if spec.ID != "instance.exec" || !spec.SupportsJSON || !spec.SupportsNDJSON {
		t.Fatalf("spec = %#v", spec)
	}
	if spec.Args[0].Name != "args" || !spec.Args[0].Repeatable {
		t.Fatalf("args = %#v", spec.Args)
	}
}

func TestRunExecWithTestDataPlaneAndTempOverlay(t *testing.T) {
	setupConfig(t)
	createCalls := 0
	deleteCalls := 0
	defer cli.SetCloudStartSandboxInstanceForTest(func(_ context.Context, _ *ags.Client, req *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
		createCalls++
		id := "ins-temp-exec"
		toolName := stringValue(req.ToolName)
		status := "RUNNING"
		return &ags.StartSandboxInstanceResponseParams{Instance: &ags.SandboxInstance{InstanceId: &id, ToolName: &toolName, Status: &status}}, nil
	})()
	defer cli.SetCloudStopSandboxInstanceForTest(func(_ context.Context, _ *ags.Client, req *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
		deleteCalls++
		if id := stringValue(req.InstanceId); id != "ins-temp-exec" {
			t.Fatalf("unexpected delete id: %s", id)
		}
		rid := "rid"
		return &ags.StopSandboxInstanceResponseParams{RequestId: &rid}, nil
	})()

	dp := &fakeExecDataPlane{}
	defer cli.SetTestDataPlaneForTest(dp)()

	ios, _, stdout, stderr := iostreams.Test()
	runtime, err := Module().Build(command.Deps{IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:    []string{"ls"},
		DashPos: 0,
		Flags: map[string]command.FlagValue{
			"create-temp-instance": {Name: "create-temp-instance", Type: command.FlagBool, Bool: true, Changed: true},
			"tool-name":            {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
			"cleanup":              {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if createCalls != 1 || deleteCalls != 1 {
		t.Fatalf("create=%d delete=%d", createCalls, deleteCalls)
	}
	if dp.gotInstanceID != "ins-temp-exec" {
		t.Fatalf("data-plane saw instanceID=%s", dp.gotInstanceID)
	}
	data, ok := result.Data.(*output.ExecData)
	if !ok {
		t.Fatalf("data type=%T", result.Data)
	}
	ec, ok := data.ExecutionContext.(*cli.ExecutionContext)
	if !ok || !ec.TemporarySandboxInstance || ec.Cleanup == nil || ec.Cleanup.Status != "deleted" {
		t.Fatalf("execution context=%#v", data.ExecutionContext)
	}
	result.Text(stdout)
	if stdout.String() != "ok\n" || stderr.String() != "" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunExecRejectsMissingSeparator(t *testing.T) {
	setupConfig(t)
	runtime, err := Module().Build(command.Deps{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:    []string{"ins-1", "ls"},
		DashPos: -1,
		Flags:   map[string]command.FlagValue{"cleanup": {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)}},
	})
	if err == nil || !strings.Contains(err.Error(), "usage: agr instance exec") {
		t.Fatalf("error = %v, want separator usage error", err)
	}
}

func TestRunExecLocalValidationHappensBeforeTempOverlay(t *testing.T) {
	setupConfig(t)
	createCalls := 0
	deleteCalls := 0
	defer cli.SetCloudStartSandboxInstanceForTest(func(_ context.Context, _ *ags.Client, _ *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
		createCalls++
		id := "ins-temp-exec"
		return &ags.StartSandboxInstanceResponseParams{Instance: &ags.SandboxInstance{InstanceId: &id}}, nil
	})()
	defer cli.SetCloudStopSandboxInstanceForTest(func(_ context.Context, _ *ags.Client, _ *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
		deleteCalls++
		rid := "rid"
		return &ags.StopSandboxInstanceResponseParams{RequestId: &rid}, nil
	})()
	runtime, err := Module().Build(command.Deps{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		DashPos: -1,
		Flags: map[string]command.FlagValue{
			"create-temp-instance": {Name: "create-temp-instance", Type: command.FlagBool, Bool: true, Changed: true},
			"tool-name":            {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
			"cleanup":              {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "usage: agr instance exec") {
		t.Fatalf("error = %v, want separator usage error", err)
	}
	if createCalls != 0 || deleteCalls != 0 {
		t.Fatalf("missing separator should not create temp instance: create=%d delete=%d", createCalls, deleteCalls)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:    []string{"echo", "hi"},
		DashPos: 0,
		Flags: map[string]command.FlagValue{
			"create-temp-instance": {Name: "create-temp-instance", Type: command.FlagBool, Bool: true, Changed: true},
			"tool-name":            {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
			"cleanup":              {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)},
			"env":                  {Name: "env", Type: command.FlagStringArray, Strings: []string{"=bad"}, Changed: true},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid environment variable format") {
		t.Fatalf("error = %v, want invalid env error", err)
	}
	if createCalls != 0 || deleteCalls != 0 {
		t.Fatalf("invalid env should not create temp instance: create=%d delete=%d", createCalls, deleteCalls)
	}
}

func TestShellJoinPreservesArgumentBoundaries(t *testing.T) {
	got := shellJoin([]string{"printf", "%s\n", "a b", "quote's"})
	want := `'printf' '%s
' 'a b' 'quote'"'"'s'`
	if got != want {
		t.Fatalf("shellJoin = %q, want %q", got, want)
	}
}

func TestParseExecEnv(t *testing.T) {
	envs, err := parseExecEnv([]string{"A=B"})
	if err != nil {
		t.Fatalf("parseExecEnv returned error: %v", err)
	}
	if envs["A"] != "B" {
		t.Fatalf("envs = %#v", envs)
	}
	if _, err := parseExecEnv([]string{"=B"}); err == nil {
		t.Fatalf("expected invalid env error")
	}
}

type fakeExecDataPlane struct {
	gotInstanceID string
}

func (f *fakeExecDataPlane) RunCode(context.Context, string, string, string) (string, string, any, any, int, error) {
	return "", "", nil, nil, 0, nil
}

func (f *fakeExecDataPlane) Exec(_ context.Context, instanceID string, _ []string) (string, string, int, any, error) {
	f.gotInstanceID = instanceID
	return "ok\n", "", 0, nil, nil
}

func (f *fakeExecDataPlane) Upload(context.Context, string, string, string, io.Reader) (string, int64, error) {
	return "", 0, nil
}

func (f *fakeExecDataPlane) Download(context.Context, string, string) (io.Reader, int64, error) {
	return nil, 0, nil
}

func setupConfig(t *testing.T) {
	t.Helper()
	cli.SetIOStreams(&iostreams.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}})
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	config.SetSecretID("AKIDfake")
	config.SetSecretKey("fakeSecretKey")
	config.SetRegion("ap-guangzhou")
}

func stringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
