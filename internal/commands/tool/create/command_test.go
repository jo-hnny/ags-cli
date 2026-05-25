package create

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apicli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

type fakeControlPlane struct {
	action  string
	request map[string]any
}

func (f *fakeControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	return map[string]any{"ok": true}, nil
}

func TestModuleBuildsCreateRequest(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-name":             {Name: "tool-name", Type: command.FlagString, String: "demo", Changed: true},
			"tool-type":             {Name: "tool-type", Type: command.FlagString, String: "code-interpreter", Changed: true},
			"network-configuration": {Name: "network-configuration", Type: command.FlagString, String: `{"NetworkMode":"SANDBOX"}`, Changed: true},
			"tags":                  {Name: "tags", Type: command.FlagString, String: `[{"Key":"env","Value":"unit"}]`, Changed: true},
			"request":               {Name: "request", Type: command.FlagString},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "CreateSandboxTool" {
		t.Fatalf("action = %q", cp.action)
	}
	if cp.request["ToolName"] != "demo" || cp.request["ToolType"] != "code-interpreter" {
		t.Fatalf("request = %#v", cp.request)
	}
	network := cp.request["NetworkConfiguration"].(map[string]any)
	if network["NetworkMode"] != "SANDBOX" {
		t.Fatalf("NetworkConfiguration = %#v", network)
	}
}

func TestModuleRejectsMissingName(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-type": {Name: "tool-type", Type: command.FlagString, String: "code-interpreter", Changed: true},
			"request":   {Name: "request", Type: command.FlagString},
		},
	})
	if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "MISSING_REQUIRED_FLAG" {
		t.Fatalf("error = %#v, want MISSING_REQUIRED_FLAG", err)
	}
}

func TestModuleRejectsMissingType(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-name": {Name: "tool-name", Type: command.FlagString, String: "demo", Changed: true},
			"request":   {Name: "request", Type: command.FlagString},
		},
	})
	if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "MISSING_REQUIRED_FLAG" {
		t.Fatalf("error = %#v, want MISSING_REQUIRED_FLAG", err)
	}
}

func TestModuleRejectsMissingNetworkConfiguration(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-name": {Name: "tool-name", Type: command.FlagString, String: "demo", Changed: true},
			"tool-type": {Name: "tool-type", Type: command.FlagString, String: "code-interpreter", Changed: true},
			"request":   {Name: "request", Type: command.FlagString},
		},
	})
	if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "MISSING_REQUIRED_FLAG" {
		t.Fatalf("error = %#v, want MISSING_REQUIRED_FLAG", err)
	}
}

func TestValidateConvenienceRequestRequiresRoleArnForMounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mounts any
	}{
		{name: "any slice", mounts: []any{map[string]any{"Bucket": "b"}}},
		{name: "map slice", mounts: []map[string]any{{"Bucket": "b"}}},
		{name: "string fallback", mounts: "cos://bucket/path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateConvenienceRequest(map[string]any{
				"ToolName":      "demo",
				"ToolType":      "code-interpreter",
				"StorageMounts": tc.mounts,
			})
			if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "MISSING_REQUIRED_FLAG" {
				t.Fatalf("error = %#v, want MISSING_REQUIRED_FLAG", err)
			}
		})
	}
}

func TestValidateConvenienceRequestAllowsEmptyMounts(t *testing.T) {
	for _, mounts := range []any{[]any{}, []map[string]any{}, nil} {
		err := validateConvenienceRequest(map[string]any{
			"ToolName":      "demo",
			"ToolType":      "code-interpreter",
			"StorageMounts": mounts,
		})
		if err != nil {
			t.Fatalf("validateConvenienceRequest(%#v) = %v", mounts, err)
		}
	}
}

func TestModuleAcceptsRequestBody(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"request": {Name: "request", Type: command.FlagString, String: `{"ToolName":"json","ToolType":"code-interpreter","NetworkConfiguration":{"NetworkMode":"PUBLIC"}}`, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.request["ToolName"] != "json" {
		t.Fatalf("request = %#v", cp.request)
	}
}

var _ apicli.ControlPlane = (*fakeControlPlane)(nil)
