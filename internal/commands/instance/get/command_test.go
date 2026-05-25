package get

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	instanceview "github.com/TencentCloudAgentRuntime/ags-cli/internal/commands/instance/internal/instanceview"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

func TestModuleDescriptor(t *testing.T) {
	module := Module()
	spec := module.Descriptor.Spec
	if spec.ID != "instance.get" || spec.Output.DataType != "Instance" {
		t.Fatalf("spec = %#v", spec)
	}
	if !spec.SupportsJSON {
		t.Fatalf("get should support JSON")
	}
}

func TestModuleGetsInstance(t *testing.T) {
	id := "ins-unit"
	toolID := "sdt-unit"
	toolName := "unit-tool"
	status := "RUNNING"
	created := "2026-05-21T10:00:00Z"
	updated := "2026-05-21T10:05:00Z"
	expires := "2026-05-21T11:00:00Z"
	stopReason := "manual"
	networkMode := "PUBLIC"
	authMode := "TOKEN"
	timeout := uint64(300)
	mountName := "workspace"
	mountPath := "/workspace"
	subPath := "src"
	readOnly := true
	cp := &fakeControlPlane{instance: &ags.SandboxInstance{
		InstanceId:     &id,
		ToolId:         &toolID,
		ToolName:       &toolName,
		Status:         &status,
		CreateTime:     &created,
		UpdateTime:     &updated,
		ExpiresAt:      &expires,
		StopReason:     &stopReason,
		NetworkMode:    &networkMode,
		AuthMode:       &authMode,
		TimeoutSeconds: &timeout,
		MountOptions: []*ags.MountOption{{
			Name:      &mountName,
			MountPath: &mountPath,
			SubPath:   &subPath,
			ReadOnly:  &readOnly,
		}},
	}}
	runtime, err := Module().Build(command.Deps{ControlPlane: cp})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{id},
		ArgValues: map[string]string{"instance-id": id},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if cp.instanceID != id {
		t.Fatalf("instanceID = %q, want %q", cp.instanceID, id)
	}
	data := result.Data.(map[string]any)
	if data["InstanceId"] != id || data["TimeoutSeconds"] != timeout {
		t.Fatalf("data = %#v", data)
	}
	var text bytes.Buffer
	result.Text(&text)
	if !strings.Contains(text.String(), "ToolName:") || !strings.Contains(text.String(), toolName) {
		t.Fatalf("text = %q", text.String())
	}
	if !strings.Contains(text.String(), "MountOptions:") || !strings.Contains(text.String(), "ReadOnly:  true") {
		t.Fatalf("text = %q", text.String())
	}
}

func TestModuleRequiresControlPlane(t *testing.T) {
	_, err := Module().Build(command.Deps{})
	if err == nil || !strings.Contains(err.Error(), "ControlPlane") {
		t.Fatalf("error = %v, want missing control plane", err)
	}
}

func TestModuleRejectsMissingInstanceID(t *testing.T) {
	runtime, err := Module().Build(command.Deps{ControlPlane: &fakeControlPlane{}})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{})
	if err == nil || !strings.Contains(err.Error(), "missing instance id") {
		t.Fatalf("error = %v, want missing instance id", err)
	}
}

type fakeControlPlane struct {
	instanceID string
	instance   *ags.SandboxInstance
}

func (f *fakeControlPlane) GetInstance(_ context.Context, instanceID string) (*ags.SandboxInstance, error) {
	f.instanceID = instanceID
	return f.instance, nil
}

func TestFormatTimeout(t *testing.T) {
	for _, tc := range []struct {
		seconds uint64
		want    string
	}{
		{seconds: 7200, want: "2h"},
		{seconds: 300, want: "5m"},
		{seconds: 45, want: "45s"},
	} {
		if got := instanceview.Timeout(tc.seconds); got != tc.want {
			t.Fatalf("Timeout(%d) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}

func TestFormatMountOptionsDetailDefaults(t *testing.T) {
	name := "workspace"
	detail := instanceview.MountOptionsDetail([]*ags.MountOption{{Name: &name}})
	if !strings.Contains(detail, "MountPath: (default)") || !strings.Contains(detail, "ReadOnly:  (default)") {
		t.Fatalf("detail = %q", detail)
	}
	if got := instanceview.MountOptionsDetail(nil); got != "" {
		t.Fatalf("empty detail = %q", got)
	}
}
