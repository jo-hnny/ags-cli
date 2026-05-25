package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"

	workflow "github.com/TencentCloudAgentRuntime/ags-cli/internal/cli/overlay"
)

// CleanupPolicy re-exports the overlay workflow cleanup policy for command
// packages that still import internal/cli.
type CleanupPolicy = workflow.CleanupPolicy

const (
	// CleanupAlways deletes temporary sandbox instances after every execution.
	CleanupAlways = workflow.CleanupAlways
	// CleanupSuccess deletes temporary sandbox instances only after successful execution.
	CleanupSuccess = workflow.CleanupSuccess
	// CleanupNever keeps temporary sandbox instances after execution.
	CleanupNever = workflow.CleanupNever
)

// ExecutionContext re-exports the JSON metadata attached to overlay executions.
type ExecutionContext = workflow.ExecutionContext

// ExecutionCleanupResult re-exports the cleanup result recorded in overlay JSON
// output.
type ExecutionCleanupResult = workflow.ExecutionCleanupResult

// OverlayFlags re-exports the shared temporary-instance flag set.
type OverlayFlags = workflow.OverlayFlags

// ResolvedOverlay re-exports the resolved instance and cleanup callbacks for an
// overlay execution.
type ResolvedOverlay = workflow.ResolvedOverlay

// RegisterOverlayFlags installs the workflow flag set on the given cobra command.
func RegisterOverlayFlags(cmd *cobra.Command, flags *OverlayFlags) {
	cmd.Flags().BoolVar(&flags.CreateTempInstance, "create-temp-instance", false, "Create a temporary sandbox instance, run, then clean up per --cleanup")
	cmd.Flags().StringVar(&flags.Cleanup, "cleanup", string(CleanupAlways), "Cleanup policy for temporary instance: always|success|never")
	// -t shorthand: only attach when not already taken on the command.
	if cmd.Flags().ShorthandLookup("t") == nil {
		cmd.Flags().StringVarP(&flags.ToolName, "tool-name", "t", "", "Tool name for temporary instance")
	} else {
		cmd.Flags().StringVar(&flags.ToolName, "tool-name", "", "Tool name for temporary instance")
	}
	cmd.Flags().StringVar(&flags.ToolID, "tool-id", "", "Tool ID for temporary instance")
}

// ResolveCleanupPolicy validates --cleanup and returns the canonical value.
func ResolveCleanupPolicy(value string) (CleanupPolicy, error) {
	return workflow.ResolveCleanupPolicy(value)
}

// ResolveOverlay implements the create -> execute -> cleanup workflow.
//
//	flags        - the parsed overlay flag set
//	args         - the positional args of the command (first one, if any,
//	               is treated as the existing instance id)
//	apiCreate    - injection point that creates a sandbox instance and
//	               returns the new id; tests stub this to a fake. The
//	               production callers wire it to cloudStartSandboxInstance.
//	apiDelete    - injection point that deletes a sandbox instance.
//
// The returned ResolvedOverlay is never nil on success.
func ResolveOverlay(
	ctx context.Context,
	flags OverlayFlags,
	args []string,
	apiCreate func(ctx context.Context, toolName, toolID string) (string, error),
	apiDelete func(ctx context.Context, instanceID string) error,
) (*ResolvedOverlay, error) {
	return workflow.ResolveOverlay(ctx, flags, args, apiCreate, apiDelete, stderr)
}

// overlayCloudCreate is the production wiring used by the overlay.
// It creates a sandbox instance via the typed SDK request.
func overlayCloudCreate(ctx context.Context, toolName, toolID string) (string, error) {
	client, err := newCloudClient()
	if err != nil {
		return "", err
	}
	req := ags.NewStartSandboxInstanceRequest()
	if toolID != "" {
		req.ToolId = &toolID
	} else {
		req.ToolName = &toolName
	}
	resp, err := cloudStartSandboxInstance(ctx, client, req)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Instance == nil || resp.Instance.InstanceId == nil {
		return "", fmt.Errorf("StartSandboxInstance returned no instance id")
	}
	return *resp.Instance.InstanceId, nil
}

// overlayCloudDelete is the production wiring used by the overlay.
func overlayCloudDelete(ctx context.Context, instanceID string) error {
	client, err := newCloudClient()
	if err != nil {
		return err
	}
	req := ags.NewStopSandboxInstanceRequest()
	req.InstanceId = &instanceID
	_, err = cloudStopSandboxInstance(ctx, client, req)
	return err
}
