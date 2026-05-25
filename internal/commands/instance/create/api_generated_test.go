package create

import (
	"context"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
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

func TestGeneratedModuleBuildsStartInstanceRequest(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := GeneratedModule().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-name": {Name: "tool-name", Type: command.FlagString, String: "code-interpreter-v1", Changed: true},
			"timeout":   {Name: "timeout", Type: command.FlagString, String: "300s", Changed: true},
			"auth-mode": {Name: "auth-mode", Type: command.FlagString, String: "NONE", Changed: true},
			"metadata":  {Name: "metadata", Type: command.FlagString, String: `[{"Name":"owner","Value":"ci"}]`, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "StartSandboxInstance" || cp.request["ToolName"] != "code-interpreter-v1" || cp.request["Timeout"] != "300s" || cp.request["AuthMode"] != "NONE" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	metadata, ok := cp.request["Metadata"].([]any)
	if !ok || len(metadata) != 1 {
		t.Fatalf("Metadata = %#v", cp.request["Metadata"])
	}
	item, ok := metadata[0].(map[string]any)
	if !ok || item["Name"] != "owner" || item["Value"] != "ci" {
		t.Fatalf("Metadata = %#v", cp.request["Metadata"])
	}
}
