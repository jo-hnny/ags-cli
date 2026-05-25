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
	action   string
	request  map[string]any
	response any
}

func (f *fakeMixedControlPlane) Call(_ context.Context, action string, request map[string]any) (any, error) {
	f.action = action
	f.request = request
	if f.response != nil {
		return f.response, nil
	}
	return map[string]any{"ok": true}, nil
}

func TestModuleBuildsAndRendersInstanceList(t *testing.T) {
	id := "ins-unit"
	toolName := "tool"
	status := "RUNNING"
	created := "2026-05-21T10:00:00Z"
	expires := "2026-05-21T11:00:00Z"
	timeout := uint64(300)
	total := int64(2)
	mountName := "workspace"
	cp := &fakeMixedControlPlane{response: &ags.DescribeSandboxInstanceListResponseParams{
		TotalCount: &total,
		InstanceSet: []*ags.SandboxInstance{{
			InstanceId:     &id,
			ToolName:       &toolName,
			Status:         &status,
			CreateTime:     &created,
			ExpiresAt:      &expires,
			TimeoutSeconds: &timeout,
			MountOptions:   []*ags.MountOption{{Name: &mountName}},
		}},
	}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{Flags: validFlags()})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.action != "DescribeSandboxInstanceList" || cp.request["Limit"] != 20 {
		t.Fatalf("action=%q request=%#v", cp.action, cp.request)
	}
	data := result.Data.(map[string]any)
	items := data["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["InstanceId"] != id {
		t.Fatalf("items = %#v", items)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), id) || !strings.Contains(text.String(), "workspace") {
		t.Fatalf("text = %q", text.String())
	}
}

func TestModuleRendersEmptyInstanceList(t *testing.T) {
	total := int64(0)
	cp := &fakeMixedControlPlane{response: &ags.DescribeSandboxInstanceListResponseParams{TotalCount: &total}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{Flags: validFlags()})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "No instances found") {
		t.Fatalf("text = %q", text.String())
	}
}

func TestModuleUsesDefaultLimitWhenUnset(t *testing.T) {
	total := int64(0)
	cp := &fakeMixedControlPlane{response: &ags.DescribeSandboxInstanceListResponseParams{TotalCount: &total}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"offset": {Name: "offset", Type: command.FlagInt, Int: 0},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if _, ok := cp.request["Limit"]; ok {
		t.Fatalf("Limit = %#v, want omitted", cp.request["Limit"])
	}
	pagination := result.Data.(map[string]any)["Pagination"].(map[string]any)
	if _, ok := pagination["Limit"]; ok {
		t.Fatalf("Pagination limit = %#v, want omitted", pagination["Limit"])
	}
}

func TestModuleAllowsZeroLimit(t *testing.T) {
	total := int64(0)
	cp := &fakeMixedControlPlane{response: &ags.DescribeSandboxInstanceListResponseParams{TotalCount: &total}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Flags: map[string]command.FlagValue{
			"offset": {Name: "offset", Type: command.FlagInt, Int: 0, Changed: true},
			"limit":  {Name: "limit", Type: command.FlagInt, Int: 0, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := cp.request["Limit"]; got != 0 {
		t.Fatalf("Limit = %#v, want 0", got)
	}
	pagination := result.Data.(map[string]any)["Pagination"].(map[string]any)
	if got := pagination["Limit"]; got != 0 {
		t.Fatalf("Pagination limit = %#v, want 0", got)
	}
}

func TestModuleRejectsInvalidListFlags(t *testing.T) {
	runtime, err := Module().Build(command.Deps{ControlPlane: &fakeMixedControlPlane{}})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	for _, tc := range []struct {
		name  string
		flags map[string]command.FlagValue
		want  string
	}{
		{name: "offset", flags: withFlag(validFlags(), "offset", command.FlagValue{Name: "offset", Type: command.FlagInt, Int: -1, Changed: true}), want: "offset"},
		{name: "limit", flags: withFlag(validFlags(), "limit", command.FlagValue{Name: "limit", Type: command.FlagInt, Int: -1, Changed: true}), want: "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runtime.Handler.Run(context.Background(), command.Request{Flags: tc.flags})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func validFlags() map[string]command.FlagValue {
	return map[string]command.FlagValue{
		"offset": {Name: "offset", Type: command.FlagInt, Int: 0, Changed: true},
		"limit":  {Name: "limit", Type: command.FlagInt, Int: 20, Changed: true},
	}
}

func withFlag(flags map[string]command.FlagValue, name string, value command.FlagValue) map[string]command.FlagValue {
	out := map[string]command.FlagValue{}
	for k, v := range flags {
		out[k] = v
	}
	out[name] = value
	return out
}
