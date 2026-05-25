package get

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

func TestGeneratedModuleBuildsGetPrecacheRequest(t *testing.T) {
	cp := &fakeControlPlane{}
	runtime, err := GeneratedModule().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		ArgValues: map[string]string{"image-digest": "sha256:unit"},
		Flags: map[string]command.FlagValue{
			"image":               {Name: "image", Type: command.FlagString, String: "nginx:latest", Changed: true},
			"image-registry-type": {Name: "image-registry-type", Type: command.FlagString, String: "personal", Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DescribePreCacheImageTask" || cp.request["ImageDigest"] != "sha256:unit" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
}
