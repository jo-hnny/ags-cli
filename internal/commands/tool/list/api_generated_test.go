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

func TestGeneratedModuleBuildsListToolsRequest(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := GeneratedModule().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"tool-ids": {Name: "tool-ids", Type: command.FlagStringArray, Strings: []string{"sdt-a"}, Changed: true},
			"limit":    {Name: "limit", Type: command.FlagInt, Int: 20, Changed: true},
			"filters":  {Name: "filters", Type: command.FlagString, String: `[{"Name":"Tag","Values":["env=unit"]}]`, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DescribeSandboxToolList" || cp.request["Limit"] != 20 {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	ids := cp.request["ToolIds"].([]string)
	if len(ids) != 1 || ids[0] != "sdt-a" {
		t.Fatalf("ToolIds = %#v", cp.request["ToolIds"])
	}
	filters, ok := cp.request["Filters"].([]any)
	if !ok || len(filters) != 1 {
		t.Fatalf("Filters = %#v", cp.request["Filters"])
	}
}
