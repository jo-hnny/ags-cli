package create

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

type fakeMixedControlPlane struct {
	action  string
	request map[string]any
	resp    *ags.StartSandboxInstanceResponseParams
}

func (f *fakeMixedControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	if f.resp != nil {
		return f.resp, nil
	}
	id := "ins-created"
	name, _ := request["ToolName"].(string)
	status := "RUNNING"
	created := "2026-05-21T10:00:00Z"
	mountName := "workspace"
	return &ags.StartSandboxInstanceResponseParams{
		Instance: &ags.SandboxInstance{
			InstanceId:   &id,
			ToolName:     &name,
			Status:       &status,
			CreateTime:   &created,
			MountOptions: []*ags.MountOption{{Name: &mountName}},
		},
	}, nil
}

func TestModuleCreatesInstanceAndRendersText(t *testing.T) {
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"tool-name":     {Name: "tool-name", Type: command.FlagString, String: "demo", Changed: true},
		"timeout":       {Name: "timeout", Type: command.FlagString, String: "600s", Changed: true},
		"auth-mode":     {Name: "auth-mode", Type: command.FlagString, String: "NONE", Changed: true},
		"mount-options": {Name: "mount-options", Type: command.FlagString, String: `[{"Name":"data","MountPath":"/workspace"}]`, Changed: true},
		"metadata":      {Name: "metadata", Type: command.FlagString, String: `[{"Name":"owner","Value":"ci"}]`, Changed: true},
	}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "StartSandboxInstance" || cp.request["ToolName"] != "demo" || cp.request["Timeout"] != "600s" || cp.request["AuthMode"] != "NONE" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	mounts, ok := cp.request["MountOptions"].([]any)
	if !ok || len(mounts) != 1 {
		t.Fatalf("MountOptions = %#v", cp.request["MountOptions"])
	}
	mount, ok := mounts[0].(map[string]any)
	if !ok || mount["Name"] != "data" || mount["MountPath"] != "/workspace" {
		t.Fatalf("MountOptions = %#v", cp.request["MountOptions"])
	}
	data := result.Data.(map[string]any)
	if data["InstanceId"] != "ins-created" || len(result.Effects) != 1 {
		t.Fatalf("result = %#v", result)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "Instance created: ins-created") || !strings.Contains(text.String(), "MountOptions:") {
		t.Fatalf("text = %q", text.String())
	}
}

func TestModuleValidatesToolSelection(t *testing.T) {
	runtime, err := Module().Build(command.Deps{ControlPlane: &fakeMixedControlPlane{}})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	for _, tc := range []struct {
		name  string
		flags map[string]command.FlagValue
		want  string
	}{
		{name: "missing", flags: map[string]command.FlagValue{}, want: "must specify either"},
		{name: "conflict", flags: map[string]command.FlagValue{
			"tool-name": {Name: "tool-name", Type: command.FlagString, String: "demo", Changed: true},
			"tool-id":   {Name: "tool-id", Type: command.FlagString, String: "sdt-a", Changed: true},
		}, want: "cannot specify both"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runtime.Handler.Run(context.Background(), command.Request{Flags: tc.flags})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestModuleAcceptsRequestBody(t *testing.T) {
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"request": {Name: "request", Type: command.FlagString, String: `{"ToolName":"demo"}`, Changed: true},
	}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.request["ToolName"] != "demo" {
		t.Fatalf("request = %#v", cp.request)
	}
}

func TestModuleReturnsStructuredErrorWhenAPIResponseIsMissingInstance(t *testing.T) {
	cp := &fakeMixedControlPlane{resp: &ags.StartSandboxInstanceResponseParams{}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"tool-name": {Name: "tool-name", Type: command.FlagString, String: "demo", Changed: true},
	}})
	if err == nil {
		t.Fatal("expected structured error")
	}
	cliErr := output.ClassifyError(err)
	if cliErr == nil || cliErr.Failure == nil {
		t.Fatalf("error = %#v", err)
	}
	if cliErr.Failure.Code != "INTERNAL_ERROR" || cliErr.Failure.Message != "no instance returned from API" {
		t.Fatalf("failure = %#v", cliErr.Failure)
	}
}
