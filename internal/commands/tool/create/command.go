package create

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apicli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// Module returns this package's command module.
func Module() command.Module {
	api := APIDescriptor()
	spec := api.CommandSpec()
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Generated: &command.Descriptor{
				Spec:   spec,
				Groups: api.Groups,
				API:    api,
				Source: "apicli",
			},
			Groups: api.Groups,
			API:    api,
			Source: "mixed-api",
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			builder := apicli.NewRequestBuilder(api)
			executor := apicli.NewExecutor(api, deps.ControlPlane)
			return command.Runtime{
				Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
					apiReq, err := builder.Build(req)
					if err != nil {
						return nil, err
					}
					if !requestFlag(req) {
						if err := validateConvenienceRequest(apiReq); err != nil {
							return nil, err
						}
					}
					result, err := executor.Execute(ctx, apiReq)
					if err != nil {
						return nil, err
					}
					applyCreateResultText(result, req)
					return result, nil
				}),
			}, nil
		},
	}
}

func applyCreateResultText(result *command.Result, req command.Request) {
	if result == nil {
		return
	}
	response, ok := result.Data.(*ags.CreateSandboxToolResponseParams)
	if !ok {
		return
	}
	toolID := derefString(response.ToolId)
	result.Effects = append(result.Effects, output.Effect{Kind: "create", Resource: "tool", Id: toolID})
	result.Text = func(w io.Writer) {
		fmt.Fprintf(w, "Tool created: %s\n", toolID)
		if requestFlag(req) {
			return
		}
		printKV(w, []kv{
			{key: "ID", value: toolID},
			{key: "Name", value: stringFlag(req, "tool-name")},
			{key: "Type", value: stringFlag(req, "tool-type")},
			{key: "Description", value: stringFlag(req, "description")},
		})
	}
}

type kv struct {
	key   string
	value string
}

func printKV(w io.Writer, pairs []kv) {
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		fmt.Fprintf(w, "%-14s %s\n", pair.key+":", pair.value)
	}
}

func stringFlag(req command.Request, name string) string {
	flag, ok := req.Flags[name]
	if !ok {
		return ""
	}
	return flag.String
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validateConvenienceRequest(req map[string]any) error {
	if strings.TrimSpace(stringValue(req["ToolName"])) == "" {
		return output.NewUsageError("MISSING_REQUIRED_FLAG", "tool name (-n/--tool-name) is required", "Provide a non-empty value for --tool-name.")
	}
	if strings.TrimSpace(stringValue(req["ToolType"])) == "" {
		return output.NewUsageError("MISSING_REQUIRED_FLAG", "tool type (-t/--tool-type) is required", "Provide a non-empty value for --tool-type.")
	}
	if mounts, ok := req["StorageMounts"]; ok && collectionLen(mounts) > 0 && strings.TrimSpace(stringValue(req["RoleArn"])) == "" {
		return output.NewUsageError("MISSING_REQUIRED_FLAG", "--role-arn is required when --storage-mounts is specified", "Provide --role-arn when using --storage-mounts.")
	}
	return nil
}

func collectionLen(value any) int {
	switch v := value.(type) {
	case []any:
		return len(v)
	case []map[string]any:
		return len(v)
	default:
		rv := fmt.Sprintf("%v", value)
		if rv == "[]" || rv == "<nil>" {
			return 0
		}
		return 1
	}
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func requestFlag(req command.Request) bool {
	flag, ok := req.Flags["request"]
	return ok && flag.Changed && strings.TrimSpace(flag.String) != ""
}
