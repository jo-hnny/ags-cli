package delete

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

type fakeMixedControlPlane struct {
	action  string
	request map[string]any
}

func (f *fakeMixedControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	return &ags.DeleteAPIKeyResponseParams{}, nil
}

func TestModuleDeletesAPIKeyAndRendersText(t *testing.T) {
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"ak-unit"},
		ArgValues: map[string]string{"key-id": "ak-unit"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DeleteAPIKey" || cp.request["KeyId"] != "ak-unit" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	if result.Data.(map[string]any)["KeyId"] != "ak-unit" || len(result.Effects) != 1 {
		t.Fatalf("result = %#v", result)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "API key deleted: ak-unit") {
		t.Fatalf("text = %q", text.String())
	}
}
