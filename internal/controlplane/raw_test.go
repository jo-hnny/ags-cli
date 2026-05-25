package controlplane

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

func TestRawAPIClientUsesInjectedSender(t *testing.T) {
	config.SetSecretID("sid")
	config.SetSecretKey("skey")
	var gotAction, gotEndpoint string
	var gotPayload []byte
	client := RawAPIClient{Sender: func(_ context.Context, action, cloudEndpoint string, payload []byte) ([]byte, error) {
		gotAction = action
		gotEndpoint = cloudEndpoint
		gotPayload = append([]byte(nil), payload...)
		return []byte(`{"Response":{"ok":true}}`), nil
	}}

	result, err := client.RawCall(context.Background(), "DescribeSandboxInstanceList", []byte(`{"Limit":1}`))
	if err != nil {
		t.Fatalf("RawCall returned error: %v", err)
	}
	if gotAction != "DescribeSandboxInstanceList" || gotEndpoint != "ags.tencentcloudapi.com" || string(gotPayload) != `{"Limit":1}` {
		t.Fatalf("sender got action=%q endpoint=%q payload=%q", gotAction, gotEndpoint, gotPayload)
	}
	if len(result.Warnings) != 1 || result.MetaExtra["Action"] != "DescribeSandboxInstanceList" {
		t.Fatalf("result = %#v", result)
	}
	response := result.Response.(map[string]any)
	if response["Response"] == nil {
		t.Fatalf("response = %#v", result.Response)
	}
}

func TestRawAPIClientKeepsNonJSONResponseAsString(t *testing.T) {
	config.SetSecretID("sid")
	config.SetSecretKey("skey")
	client := RawAPIClient{Sender: func(context.Context, string, string, []byte) ([]byte, error) {
		return []byte("plain text"), nil
	}}
	result, err := client.RawCall(context.Background(), "Action", []byte(`{}`))
	if err != nil {
		t.Fatalf("RawCall returned error: %v", err)
	}
	if result.Response != "plain text" {
		t.Fatalf("response = %#v", result.Response)
	}
}

func TestRawAPIClientReturnsSenderError(t *testing.T) {
	config.SetSecretID("sid")
	config.SetSecretKey("skey")
	client := RawAPIClient{Sender: func(context.Context, string, string, []byte) ([]byte, error) {
		return nil, errors.New("boom")
	}}
	_, err := client.RawCall(context.Background(), "Action", []byte(`{}`))
	if err == nil || err.Error() != "api call Action: boom" {
		t.Fatalf("error = %v, want wrapped sender error", err)
	}
}

func TestFillRequestUsesCanonicalPrecacheCommandIDInHint(t *testing.T) {
	req := ags.NewCreatePreCacheImageTaskRequest()
	err := fillRequest("pre-cache-image-task.create", map[string]any{
		"Image":             123,
		"ImageRegistryType": "personal",
	}, req)
	if err == nil {
		t.Fatalf("expected parse error")
	}
	cliErr := output.ClassifyError(err)
	if cliErr == nil || cliErr.Failure == nil {
		t.Fatalf("expected CLI error, got %v", err)
	}
	if !strings.Contains(cliErr.Failure.Hint, "agr schema pre-cache-image-task.create -o json") {
		t.Fatalf("hint = %q", cliErr.Failure.Hint)
	}
}
