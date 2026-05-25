package create

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

type fakeMixedControlPlane struct {
	action  string
	request map[string]any
}

func (f *fakeMixedControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	keyID, name, apiKey := "ak-unit", "unit-key", "secret"
	return &ags.CreateAPIKeyResponseParams{KeyId: &keyID, Name: &name, APIKey: &apiKey}, nil
}

func TestModuleCreatesAPIKeyAndRendersText(t *testing.T) {
	ios, _, _, stderr := iostreams.Test()
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{IO: ios, ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"name": {Name: "name", Type: command.FlagString, String: "unit-key", Changed: true},
	}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "CreateAPIKey" || cp.request["Name"] != "unit-key" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	if result.Data.(map[string]any)["KeyId"] != "ak-unit" || len(result.Effects) != 1 {
		t.Fatalf("result = %#v", result)
	}
	var stdout bytes.Buffer
	result.Text(&stdout)
	if !strings.Contains(stdout.String(), "API key created: ak-unit") || !strings.Contains(stdout.String(), "APIKey:") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Save this API key securely") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestModuleRejectsMissingName(t *testing.T) {
	runtime, err := Module().Build(command.Deps{ControlPlane: &fakeMixedControlPlane{}})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"name": {Name: "name", Type: command.FlagString},
	}})
	if err == nil || !strings.Contains(err.Error(), "API key name") {
		t.Fatalf("error = %v, want missing name", err)
	}
}
