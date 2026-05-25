package list

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

func TestGeneratedModuleBuildsListInstancesRequest(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := GeneratedModule().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-id": {Name: "tool-id", Type: command.FlagString, String: "sdt-unit", Changed: true},
			"limit":   {Name: "limit", Type: command.FlagInt, Int: 20, Changed: true},
			"filters": {Name: "filters", Type: command.FlagString, String: `[{"Name":"Status","Values":["RUNNING"]}]`, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DescribeSandboxInstanceList" || cp.request["ToolId"] != "sdt-unit" || cp.request["Limit"] != 20 {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	filters, ok := cp.request["Filters"].([]any)
	if !ok || len(filters) != 1 {
		t.Fatalf("Filters = %#v", cp.request["Filters"])
	}
}
