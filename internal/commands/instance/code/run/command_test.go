package run

import (
	"bytes"
	"context"
	"encoding/json"
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
	if spec.ID != "instance.code.run" || !spec.SupportsJSON || !spec.SupportsNDJSON {
		t.Fatalf("spec = %#v", spec)
	}
	if len(spec.Flags) == 0 || spec.Flags[0].Name != "code" {
		t.Fatalf("flags = %#v", spec.Flags)
	}
}

func TestValidateRunLanguage(t *testing.T) {
	for _, lang := range []string{"python", "javascript", "typescript", "bash", "r", "java"} {
		if err := validateRunLanguage(lang); err != nil {
			t.Fatalf("validateRunLanguage(%q): %v", lang, err)
		}
	}
	err := validateRunLanguage("ruby")
	if err == nil || !strings.Contains(err.Error(), "unsupported language") {
		t.Fatalf("error = %v, want unsupported language", err)
	}
}

func TestRunCodeWithTestDataPlaneAndTempOverlay(t *testing.T) {
	setupConfig(t)
	createCalls := 0
	deleteCalls := 0
	defer cli.SetCloudStartSandboxInstanceForTest(func(_ context.Context, _ *ags.Client, req *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
		createCalls++
		id := "ins-temp-1"
		toolName := stringValue(req.ToolName)
		status := "RUNNING"
		return &ags.StartSandboxInstanceResponseParams{Instance: &ags.SandboxInstance{InstanceId: &id, ToolName: &toolName, Status: &status}}, nil
	})()
	defer cli.SetCloudStopSandboxInstanceForTest(func(_ context.Context, _ *ags.Client, req *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
		deleteCalls++
		if id := stringValue(req.InstanceId); id != "ins-temp-1" {
			t.Fatalf("unexpected delete id: %s", id)
		}
		rid := "rid"
		return &ags.StopSandboxInstanceResponseParams{RequestId: &rid}, nil
	})()

	dp := &fakeCodeDataPlane{}
	defer cli.SetTestDataPlaneForTest(dp)()

	ios, _, stdout, _ := iostreams.Test()
	runtime, err := Module().Build(command.Deps{IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"create-temp-instance": {Name: "create-temp-instance", Type: command.FlagBool, Bool: true, Changed: true},
			"tool-name":            {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
			"cleanup":              {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)},
			"code":                 {Name: "code", Type: command.FlagString, String: "print('hi')", Changed: true},
			"language":             {Name: "language", Type: command.FlagString, String: "python"},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if createCalls != 1 || deleteCalls != 1 {
		t.Fatalf("create=%d delete=%d", createCalls, deleteCalls)
	}
	if dp.gotInstanceID != "ins-temp-1" || dp.gotCode != "print('hi')" || dp.gotLanguage != "python" {
		t.Fatalf("dp=%#v", dp)
	}
	data, ok := result.Data.(*output.CodeRunData)
	if !ok {
		t.Fatalf("data type=%T", result.Data)
	}
	ec, ok := data.ExecutionContext.(*cli.ExecutionContext)
	if !ok || !ec.TemporarySandboxInstance || ec.Cleanup == nil || ec.Cleanup.Status != "deleted" {
		t.Fatalf("execution context=%#v", data.ExecutionContext)
	}
	result.Text(stdout)
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout=%q", stdout.String())
	}
	b, _ := json.Marshal(data)
	if !strings.Contains(string(b), `"TemporarySandboxInstance":true`) || !strings.Contains(string(b), `"Status":"deleted"`) {
		t.Fatalf("marshal missing overlay fields: %s", b)
	}
}

func TestRunCodeCleanupNever(t *testing.T) {
	setupConfig(t)
	createCalls := 0
	deleteCalls := 0
	defer cli.SetCloudStartSandboxInstanceForTest(func(context.Context, *ags.Client, *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
		createCalls++
		id := "ins-temp-keep"
		return &ags.StartSandboxInstanceResponseParams{Instance: &ags.SandboxInstance{InstanceId: &id}}, nil
	})()
	defer cli.SetCloudStopSandboxInstanceForTest(func(context.Context, *ags.Client, *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
		deleteCalls++
		rid := "rid"
		return &ags.StopSandboxInstanceResponseParams{RequestId: &rid}, nil
	})()
	defer cli.SetTestDataPlaneForTest(&fakeCodeDataPlane{})()

	runtime, err := Module().Build(command.Deps{IO: testIO()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"create-temp-instance": {Name: "create-temp-instance", Type: command.FlagBool, Bool: true, Changed: true},
		"tool-name":            {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
		"cleanup":              {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupNever)},
		"code":                 {Name: "code", Type: command.FlagString, String: "print('hi')", Changed: true},
		"language":             {Name: "language", Type: command.FlagString, String: "python"},
	}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if createCalls != 1 || deleteCalls != 0 {
		t.Fatalf("create=%d delete=%d", createCalls, deleteCalls)
	}
}

func TestRunCodeLocalValidationHappensBeforeTempOverlay(t *testing.T) {
	setupConfig(t)
	createCalls := 0
	deleteCalls := 0
	defer cli.SetCloudStartSandboxInstanceForTest(func(context.Context, *ags.Client, *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
		createCalls++
		id := "ins-tmp-bad"
		return &ags.StartSandboxInstanceResponseParams{Instance: &ags.SandboxInstance{InstanceId: &id}}, nil
	})()
	defer cli.SetCloudStopSandboxInstanceForTest(func(context.Context, *ags.Client, *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
		deleteCalls++
		rid := "rid"
		return &ags.StopSandboxInstanceResponseParams{RequestId: &rid}, nil
	})()

	runtime, err := Module().Build(command.Deps{IO: testIO()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	request := command.Request{Flags: map[string]command.FlagValue{
		"create-temp-instance": {Name: "create-temp-instance", Type: command.FlagBool, Bool: true, Changed: true},
		"tool-name":            {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
		"cleanup":              {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupSuccess)},
		"language":             {Name: "language", Type: command.FlagString, String: "python"},
	}}
	if _, err := runtime.Handler.Run(context.Background(), request); err == nil {
		t.Fatalf("expected pre-execution failure")
	}
	if createCalls != 0 || deleteCalls != 0 {
		t.Fatalf("--cleanup success: create=%d delete=%d", createCalls, deleteCalls)
	}
	request.Flags["cleanup"] = command.FlagValue{Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)}
	if _, err := runtime.Handler.Run(context.Background(), request); err == nil {
		t.Fatalf("expected pre-execution failure")
	}
	if createCalls != 0 || deleteCalls != 0 {
		t.Fatalf("--cleanup always: create=%d delete=%d", createCalls, deleteCalls)
	}
	request.Flags["language"] = command.FlagValue{Name: "language", Type: command.FlagString, String: "ruby"}
	request.Flags["code"] = command.FlagValue{Name: "code", Type: command.FlagString, String: "print('hi')", Changed: true}
	if _, err := runtime.Handler.Run(context.Background(), request); err == nil {
		t.Fatalf("expected invalid language failure")
	}
	if createCalls != 0 || deleteCalls != 0 {
		t.Fatalf("invalid language should not create temp instance: create=%d delete=%d", createCalls, deleteCalls)
	}
}

func TestRunCodeRejectsConflictingInputs(t *testing.T) {
	setupConfig(t)
	runtime, err := Module().Build(command.Deps{IO: testIO()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:  []string{"ins-1"},
		Stdin: bytes.NewBufferString("print('stdin')"),
		Flags: map[string]command.FlagValue{
			"code":     {Name: "code", Type: command.FlagString, String: "print('flag')", Changed: true},
			"language": {Name: "language", Type: command.FlagString, String: "python"},
			"cleanup":  {Name: "cleanup", Type: command.FlagString, String: string(cli.CleanupAlways)},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "provide exactly one code source") {
		t.Fatalf("error = %v, want conflicting inputs", err)
	}
}

type fakeCodeDataPlane struct {
	gotInstanceID string
	gotCode       string
	gotLanguage   string
}

func (f *fakeCodeDataPlane) RunCode(_ context.Context, instanceID, code, language string) (string, string, any, any, int, error) {
	f.gotInstanceID = instanceID
	f.gotCode = code
	f.gotLanguage = language
	return "ok\n", "", nil, nil, 1, nil
}

func (f *fakeCodeDataPlane) Exec(context.Context, string, []string) (string, string, int, any, error) {
	return "", "", 0, nil, nil
}

func (f *fakeCodeDataPlane) Upload(context.Context, string, string, string, io.Reader) (string, int64, error) {
	return "", 0, nil
}

func (f *fakeCodeDataPlane) Download(context.Context, string, string) (io.Reader, int64, error) {
	return nil, 0, nil
}

func setupConfig(t *testing.T) {
	t.Helper()
	cli.SetIOStreams(testIO())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	config.SetSecretID("AKIDfake")
	config.SetSecretKey("fakeSecretKey")
	config.SetRegion("ap-guangzhou")
}

func testIO() *iostreams.IOStreams {
	return &iostreams.IOStreams{In: &bytes.Buffer{}, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
}

func stringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
