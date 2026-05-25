package get

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	instanceview "github.com/TencentCloudAgentRuntime/ags-cli/internal/commands/instance/internal/instanceview"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// ControlPlane supplies the instance lookup used by the get workflow.
type ControlPlane interface {
	GetInstance(ctx context.Context, instanceID string) (*ags.SandboxInstance, error)
}

// Module returns this package's command module.
func Module() command.Module {
	spec := command.Spec{
		ID:           "instance.get",
		Path:         []string{"instance", "get"},
		Use:          "get <instance-id>",
		Short:        "Get instance details",
		Long:         "Get detailed information about a specific instance.",
		Args:         []command.ArgSpec{{Name: "instance-id", Required: true}},
		SupportsJSON: true,
		Output:       command.OutputSpec{DataType: "Instance"},
	}
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Groups: []command.GroupSpec{{
				Path:    []string{"instance"},
				Use:     "instance",
				Short:   "Manage sandbox instances",
				Long:    "Manage sandbox instances and related data-plane workflows.",
				Aliases: []string{"i"},
			}},
			Source: "workflow",
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			cp, ok := deps.ControlPlane.(ControlPlane)
			if !ok {
				return command.Runtime{}, fmt.Errorf("instance.get requires command.Deps.ControlPlane implementing instance/get.ControlPlane")
			}
			return command.Runtime{Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
				instanceID := req.ArgValues["instance-id"]
				if instanceID == "" && len(req.Args) > 0 {
					instanceID = req.Args[0]
				}
				if strings.TrimSpace(instanceID) == "" {
					return nil, output.NewUsageError("MISSING_REQUIRED_ARG", "missing instance id", "Provide <instance-id>.")
				}
				instance, err := cp.GetInstance(ctx, instanceID)
				if err != nil {
					return nil, err
				}
				return &command.Result{
					Data: instanceview.CanonicalData(instance),
					Text: func(w io.Writer) {
						renderInstanceDetails(w, instance)
					},
				}, nil
			})}, nil
		},
	}
}

func renderInstanceDetails(w io.Writer, instance *ags.SandboxInstance) {
	kvs := []instanceview.KeyValue{
		{Key: "ID", Value: instanceview.DerefString(instance.InstanceId)},
		{Key: "ToolID", Value: instanceview.DerefString(instance.ToolId)},
		{Key: "ToolName", Value: instanceview.DerefString(instance.ToolName)},
		{Key: "Status", Value: instanceview.DerefString(instance.Status)},
		{Key: "Created", Value: instanceview.DerefString(instance.CreateTime)},
	}
	if instance.UpdateTime != nil && *instance.UpdateTime != "" {
		kvs = append(kvs, instanceview.KeyValue{Key: "Updated", Value: *instance.UpdateTime})
	}
	if instance.TimeoutSeconds != nil {
		kvs = append(kvs, instanceview.KeyValue{Key: "Timeout", Value: instanceview.Timeout(*instance.TimeoutSeconds)})
	}
	if instance.ExpiresAt != nil && *instance.ExpiresAt != "" {
		kvs = append(kvs, instanceview.KeyValue{Key: "Expires", Value: *instance.ExpiresAt})
	}
	if instance.StopReason != nil && *instance.StopReason != "" {
		kvs = append(kvs, instanceview.KeyValue{Key: "StopReason", Value: *instance.StopReason})
	}
	if mountOpts := instanceview.MountOptionsDetail(instance.MountOptions); mountOpts != "" {
		kvs = append(kvs, instanceview.KeyValue{Key: "MountOptions", Value: mountOpts})
	}
	instanceview.PrintKV(w, kvs)
}
