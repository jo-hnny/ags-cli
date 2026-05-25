package list

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apicli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	instanceview "github.com/TencentCloudAgentRuntime/ags-cli/internal/commands/instance/internal/instanceview"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// Module returns this package's command module.
func Module() command.Module {
	api := APIDescriptor()
	spec := api.CommandSpec()
	spec.Output = command.OutputSpec{
		DataType:    "InstanceListData",
		Description: "Instance list with normalized items and pagination metadata.",
	}
	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Generated: &command.Descriptor{
				Spec:   api.CommandSpec(),
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
					if !requestFlag(req) {
						if err := validateRequest(req); err != nil {
							return nil, err
						}
					}
					apiReq, err := builder.Build(req)
					if err != nil {
						return nil, err
					}
					result, err := executor.Execute(ctx, apiReq)
					if err != nil {
						return nil, err
					}
					response, ok := result.Data.(*ags.DescribeSandboxInstanceListResponseParams)
					if !ok {
						return result, nil
					}
					offset := intFlag(req, "offset")
					limit := effectiveLimit(req)
					return instanceListResult(response, offset, limit, result), nil
				}),
			}, nil
		},
	}
}

func validateRequest(req command.Request) error {
	offset := intFlag(req, "offset")
	if offset < 0 {
		return output.NewUsageError("INVALID_PAGINATION", fmt.Sprintf("--offset must be >= 0 (got %d)", offset), "Use a non-negative pagination offset.")
	}
	if limitChanged(req) {
		limit := intFlag(req, "limit")
		if limit < 0 {
			return output.NewUsageError("INVALID_PAGINATION", fmt.Sprintf("--limit must be >= 0 (got %d)", limit), "Use a non-negative pagination limit.")
		}
	}
	return nil
}

func instanceListResult(response *ags.DescribeSandboxInstanceListResponseParams, offset int, limit *int, base *command.Result) *command.Result {
	items := make([]map[string]any, len(response.InstanceSet))
	for i, instance := range response.InstanceSet {
		items[i] = instanceview.CanonicalData(instance)
	}
	pagination := map[string]any{
		"Offset":     offset,
		"Total":      instanceview.DerefInt64(response.TotalCount),
		"NextCursor": nil,
	}
	if limit != nil {
		pagination["Limit"] = *limit
	}
	data := map[string]any{
		"Items":      items,
		"Pagination": pagination,
	}
	return &command.Result{
		Data:      data,
		Warnings:  base.Warnings,
		Effects:   base.Effects,
		ExitCode:  base.ExitCode,
		Failure:   base.Failure,
		MetaExtra: base.MetaExtra,
		Text: func(w io.Writer) {
			renderInstanceList(w, response)
		},
	}
}

func effectiveLimit(req command.Request) *int {
	if !limitChanged(req) {
		return nil
	}
	limit := intFlag(req, "limit")
	return &limit
}

func limitChanged(req command.Request) bool {
	flag, ok := req.Flags["limit"]
	return ok && flag.Changed
}

func requestFlag(req command.Request) bool {
	flag, ok := req.Flags["request"]
	return ok && flag.Changed && strings.TrimSpace(flag.String) != ""
}

func intFlag(req command.Request, name string) int {
	flag, ok := req.Flags[name]
	if !ok {
		return 0
	}
	return flag.Int
}

func renderInstanceList(w io.Writer, response *ags.DescribeSandboxInstanceListResponseParams) {
	if len(response.InstanceSet) == 0 {
		fmt.Fprintln(w, "No instances found")
		return
	}

	headers := []string{"ID", "TOOL", "STATUS", "TIMEOUT", "EXPIRES", "MOUNTS", "CREATED"}
	rows := make([][]string, len(response.InstanceSet))
	for i, inst := range response.InstanceSet {
		timeout := "-"
		if inst.TimeoutSeconds != nil {
			timeout = instanceview.Timeout(*inst.TimeoutSeconds)
		}
		expires := "-"
		if inst.ExpiresAt != nil && *inst.ExpiresAt != "" {
			expires = instanceview.TimeShort(*inst.ExpiresAt)
		}
		rows[i] = []string{
			instanceview.DerefString(inst.InstanceId),
			instanceview.DerefString(inst.ToolName),
			instanceview.DerefString(inst.Status),
			timeout,
			expires,
			instanceview.MountOptionsSummary(inst.MountOptions),
			instanceview.TimeShort(instanceview.DerefString(inst.CreateTime)),
		}
	}
	instanceview.PrintTableWithPagination(w, headers, rows, len(response.InstanceSet), instanceview.DerefInt64(response.TotalCount))
}
