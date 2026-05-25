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

func TestGeneratedModuleBuildsCreateAPIKeyRequest(t *testing.T) {
	module := GeneratedModule()
	if module.Descriptor.Spec.ID != "apikey.create" || module.Descriptor.Source != "apicli" {
		t.Fatalf("descriptor = %#v", module.Descriptor)
	}
	cp := &fakeControlPlane{}
	runtime, err := module.Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"name": {Name: "name", Type: command.FlagString, String: "unit-key", Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "CreateAPIKey" || cp.request["Name"] != "unit-key" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
}
