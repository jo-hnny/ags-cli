package delete

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apicli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// ControlPlane is the minimal tool deletion dependency required by the
// workflow. It keeps multi-delete behavior testable without a full SDK client.
type ControlPlane interface {
	DeleteTool(ctx context.Context, toolID string) error
}

// Summary aggregates per-tool delete outcomes for text and JSON rendering.
type Summary struct {
	Deleted    int
	Failed     int
	DeletedIDs []string
	FailedIDs  []string
}

// Data converts the summary into the command's canonical JSON shape.
func (s Summary) Data() map[string]any {
	data := map[string]any{"Deleted": s.Deleted, "Failed": s.Failed}
	if len(s.FailedIDs) > 0 {
		data["FailedIds"] = append([]string(nil), s.FailedIDs...)
	}
	return data
}

// Module returns this package's command module.
func Module() command.Module {
	api := APIDescriptor()
	generatedSpec := api.CommandSpec()
	spec := generatedSpec
	spec.Use = "delete <tool-id> [tool-id...]"
	spec.Args = []command.ArgSpec{
		{Name: "tool-id", Required: true, Repeatable: true, Description: "Sandbox tool ID."},
	}
	spec.Output = command.OutputSpec{
		DataType:    "DeleteData",
		Description: "Delete result with multi-tool handling.",
	}

	return command.Module{
		Descriptor: command.Descriptor{
			Spec: spec,
			Generated: &command.Descriptor{
				Spec:   generatedSpec,
				Groups: api.Groups,
				API:    api,
				Source: "apicli",
			},
			Groups: api.Groups,
			API:    api,
			Source: "mixed-api",
		},
		Build: func(deps command.Deps) (command.Runtime, error) {
			deps = deps.WithDefaults()
			cp, ok := deps.ControlPlane.(ControlPlane)
			if !ok {
				return command.Runtime{}, fmt.Errorf("tool.delete requires command.Deps.ControlPlane implementing tool/delete.ControlPlane")
			}
			builder := apicli.NewRequestBuilder(api)
			return command.Runtime{
				Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
					if requestFlag(req) {
						if len(req.Args) > 1 {
							return nil, output.NewUsageError("REQUEST_FLAG_CONFLICT", "--request only supports a single tool id", "Use --request for one ToolId at a time, or pass multiple positional arguments without --request.")
						}
						apiReq, err := builder.Build(req)
						if err != nil {
							return nil, err
						}
						toolID, _ := apiReq["ToolId"].(string)
						if strings.TrimSpace(toolID) == "" {
							return nil, output.NewUsageError("MISSING_REQUIRED_ARG", "missing tool id", "Provide <tool-id>.")
						}
						if err := cp.DeleteTool(ctx, toolID); err != nil {
							return nil, err
						}
						return resultFromSummary(Summary{Deleted: 1, DeletedIDs: []string{toolID}}, nil, deps.IO.ErrOut), nil
					}

					summary := Summary{}
					var warnings []string
					for _, toolID := range req.Args {
						if err := cp.DeleteTool(ctx, toolID); err != nil {
							summary.Failed++
							summary.FailedIDs = append(summary.FailedIDs, toolID)
							warnings = append(warnings, fmt.Sprintf("failed to delete %s: %v", toolID, err))
							continue
						}
						summary.Deleted++
						summary.DeletedIDs = append(summary.DeletedIDs, toolID)
					}
					return resultFromSummary(summary, warnings, deps.IO.ErrOut), nil
				}),
			}, nil
		},
	}
}

func resultFromSummary(summary Summary, warnings []string, errOut io.Writer) *command.Result {
	result := &command.Result{
		Data:     summary.Data(),
		Warnings: warnings,
		Text: func(w io.Writer) {
			for _, toolID := range summary.DeletedIDs {
				fmt.Fprintf(w, "Tool deleted: %s\n", toolID)
			}
			for _, warning := range warnings {
				fmt.Fprintf(errOut, "Warning: %s\n", warning)
			}
		},
	}
	if summary.Failed > 0 {
		result.Failure = &output.Failure{
			Code:    "PARTIAL_DELETE_FAILED",
			Kind:    output.KindPartialSuccess,
			Message: "failed to delete one or more tools",
			Hint:    "Inspect Data.FailedIds and retry failed tool IDs.",
		}
		result.ExitCode = output.ExitPartialSuccess
	}
	return result
}

func requestFlag(req command.Request) bool {
	flag, ok := req.Flags["request"]
	return ok && flag.Changed && strings.TrimSpace(flag.String) != ""
}
