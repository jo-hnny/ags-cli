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

// ControlPlane is the minimal instance deletion dependency required by the
// workflow. It keeps multi-delete behavior testable without a full SDK client.
type ControlPlane interface {
	DeleteInstance(ctx context.Context, instanceID string) error
}

// NotFoundClassifier lets the workflow apply --ignore-not-found without
// depending on the concrete control-plane error type.
type NotFoundClassifier interface {
	IsNotFound(err error) bool
}

// Summary aggregates per-instance delete outcomes for text and JSON rendering.
type Summary struct {
	Deleted       int
	Failed        int
	DeletedIDs    []string
	FailedIDs     []string
	AlreadyAbsent []string
}

// Data converts the summary into the command's canonical JSON shape.
func (s Summary) Data() map[string]any {
	return map[string]any{
		"Deleted":       s.Deleted,
		"Failed":        s.Failed,
		"FailedIds":     append([]string(nil), s.FailedIDs...),
		"AlreadyAbsent": append([]string(nil), s.AlreadyAbsent...),
	}
}

// Module returns this package's command module.
func Module() command.Module {
	api := APIDescriptor()
	generatedSpec := api.CommandSpec()
	spec := generatedSpec
	spec.Use = "delete <instance-id> [instance-id...]"
	spec.Short = "Delete instances"
	spec.Long = "Delete one or more sandbox instances. This operation executes immediately and does not prompt for confirmation."
	spec.Aliases = []string{"rm", "del"}
	spec.Args = []command.ArgSpec{
		{Name: "instance-id", Required: true, Repeatable: true, Description: "Sandbox instance ID."},
	}
	spec.Flags = append(spec.Flags, command.FlagSpec{
		Name:     "ignore-not-found",
		Usage:    "Treat a missing instance as a successful delete",
		Type:     command.FlagBool,
		Workflow: true,
	})
	spec.Output = command.OutputSpec{
		DataType:    "DeleteData",
		Description: "Delete result with workflow-level handling.",
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
				return command.Runtime{}, fmt.Errorf("instance.delete requires command.Deps.ControlPlane implementing instance/delete.ControlPlane")
			}
			builder := apicli.NewRequestBuilder(api)
			return command.Runtime{
				Handler: command.HandlerFunc(func(ctx context.Context, req command.Request) (*command.Result, error) {
					if requestFlag(req) {
						if len(req.Args) > 1 {
							return nil, output.NewUsageError("REQUEST_FLAG_CONFLICT", "--request only supports a single instance id", "Use --request for one InstanceId at a time, or pass multiple positional arguments without --request.")
						}
						apiReq, err := builder.Build(req)
						if err != nil {
							return nil, err
						}
						instanceID, _ := apiReq["InstanceId"].(string)
						if strings.TrimSpace(instanceID) == "" {
							return nil, output.NewUsageError("MISSING_REQUIRED_ARG", "missing instance id", "Provide <instance-id>.")
						}
						summary, warnings, err := deleteOne(ctx, cp, deps.ControlPlane, instanceID, ignoreNotFound(req))
						if err != nil {
							return nil, err
						}
						return resultFromSummary(summary, warnings, deps.IO.ErrOut), nil
					}

					summary := Summary{}
					var warnings []string
					for _, instanceID := range req.Args {
						item, itemWarnings, err := deleteOne(ctx, cp, deps.ControlPlane, instanceID, ignoreNotFound(req))
						warnings = append(warnings, itemWarnings...)
						if err != nil {
							summary.Failed++
							summary.FailedIDs = append(summary.FailedIDs, instanceID)
							warnings = append(warnings, fmt.Sprintf("Failed to delete %s: %v", instanceID, err))
							continue
						}
						summary.Deleted += item.Deleted
						summary.DeletedIDs = append(summary.DeletedIDs, item.DeletedIDs...)
						summary.AlreadyAbsent = append(summary.AlreadyAbsent, item.AlreadyAbsent...)
					}
					return resultFromSummary(summary, warnings, deps.IO.ErrOut), nil
				}),
			}, nil
		},
	}
}

func deleteOne(ctx context.Context, cp ControlPlane, classifier any, instanceID string, ignoreMissing bool) (Summary, []string, error) {
	if err := cp.DeleteInstance(ctx, instanceID); err != nil {
		if ignoreMissing && isNotFound(classifier, err) {
			return Summary{AlreadyAbsent: []string{instanceID}}, []string{fmt.Sprintf("Instance %s: AlreadyAbsent", instanceID)}, nil
		}
		return Summary{}, nil, err
	}
	return Summary{Deleted: 1, DeletedIDs: []string{instanceID}}, nil, nil
}

func resultFromSummary(summary Summary, warnings []string, errOut io.Writer) *command.Result {
	result := &command.Result{
		Data:     summary.Data(),
		Warnings: warnings,
		Text: func(w io.Writer) {
			for _, id := range summary.DeletedIDs {
				fmt.Fprintf(w, "Instance deleted: %s\n", id)
			}
			for _, id := range summary.AlreadyAbsent {
				fmt.Fprintf(errOut, "Instance %s not found (ignored)\n", id)
			}
			if summary.Failed > 0 {
				fmt.Fprintf(errOut, "failed to delete %d instance(s)\n", summary.Failed)
			}
		},
	}
	if summary.Failed > 0 {
		result.Failure = &output.Failure{
			Code:    "PARTIAL_DELETE_FAILED",
			Kind:    output.KindPartialSuccess,
			Message: "failed to delete one or more instances",
			Hint:    "Inspect Data.FailedIds and retry failed instance IDs.",
		}
		result.ExitCode = output.ExitPartialSuccess
	}
	return result
}

func requestFlag(req command.Request) bool {
	flag, ok := req.Flags["request"]
	return ok && flag.Changed && strings.TrimSpace(flag.String) != ""
}

func ignoreNotFound(req command.Request) bool {
	flag, ok := req.Flags["ignore-not-found"]
	return ok && flag.Changed && flag.Bool
}

func isNotFound(classifier any, err error) bool {
	if c, ok := classifier.(NotFoundClassifier); ok {
		return c.IsNotFound(err)
	}
	return false
}
