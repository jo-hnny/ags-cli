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

func TestGeneratedModuleBuildsListAPIKeysRequest(t *testing.T) {
	module := GeneratedModule()
	if len(module.Descriptor.Spec.Flags) != 0 {
		t.Fatalf("apikey.list should not expose --request: %#v", module.Descriptor.Spec.Flags)
	}
	cp := &fakeControlPlane{}
	runtime, err := module.Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DescribeAPIKeyList" || len(cp.request) != 0 {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
}
