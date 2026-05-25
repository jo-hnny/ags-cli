package delete

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

func TestGeneratedModuleBuildsDeleteAPIKeyRequest(t *testing.T) {
	module := GeneratedModule()
	cp := &fakeControlPlane{}
	runtime, err := module.Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		ArgValues: map[string]string{"key-id": "ak-unit"},
		Flags:     map[string]command.FlagValue{},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DeleteAPIKey" || cp.request["KeyId"] != "ak-unit" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
}
