package list

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
	name, id, status, masked, created := "unit-key", "ak-unit", "ACTIVE", "abc***", "2026-05-21T10:00:00Z"
	return &ags.DescribeAPIKeyListResponseParams{
		APIKeySet: []*ags.APIKeyInfo{{Name: &name, KeyId: &id, Status: &status, MaskedKey: &masked, CreatedAt: &created}},
	}, nil
}

func TestModuleListsAPIKeysAndRendersText(t *testing.T) {
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DescribeAPIKeyList" || len(cp.request) != 0 {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	items := result.Data.(map[string]any)["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["KeyId"] != "ak-unit" {
		t.Fatalf("items = %#v", items)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "KEY ID") || !strings.Contains(text.String(), "ak-unit") {
		t.Fatalf("text = %q", text.String())
	}
}

func TestRenderEmptyAPIKeyList(t *testing.T) {
	var text bytes.Buffer
	renderAPIKeyList(&text, &ags.DescribeAPIKeyListResponseParams{})
	if !strings.Contains(text.String(), "No API keys found") {
		t.Fatalf("text = %q", text.String())
	}
}
