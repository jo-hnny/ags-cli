package create

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

type fakeMixedControlPlane struct {
	action  string
	request map[string]any
}

func (f *fakeMixedControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	return &ags.CreatePreCacheImageTaskResponseParams{}, nil
}

func TestModuleCreatesTaskAndRendersOK(t *testing.T) {
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"image":               {Name: "image", Type: command.FlagString, String: "nginx", Changed: true},
		"image-registry-type": {Name: "image-registry-type", Type: command.FlagString, String: "personal", Changed: true},
	}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "CreatePreCacheImageTask" || cp.request["Image"] != "nginx" {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "OK") {
		t.Fatalf("text = %q", text.String())
	}
}

func TestModuleRejectsMissingRequiredFields(t *testing.T) {
	cp := &fakeMixedControlPlane{}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{Flags: map[string]command.FlagValue{
		"image": {Name: "image", Type: command.FlagString, String: "nginx", Changed: true},
	}})
	if err == nil || !strings.Contains(err.Error(), "--image-registry-type is required") {
		t.Fatalf("error = %v, want missing required field error", err)
	}
	if cliErr, ok := err.(*output.CLIError); !ok || cliErr.Failure.Code != "MISSING_REQUIRED_FLAG" {
		t.Fatalf("error = %#v, want MISSING_REQUIRED_FLAG", err)
	}
}
