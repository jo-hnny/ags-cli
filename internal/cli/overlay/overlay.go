// Package overlay implements the temporary sandbox workflow shared by
// `instance code run` and `instance exec`.
//
// The overlay can spawn a temporary sandbox instance for a single
// execution and clean it up according to the user-selected policy.
package overlay

import (
	"context"
	"fmt"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// CleanupPolicy is the user-selected lifecycle policy for the temporary
// sandbox instance created by the overlay.
//
// Allowed values match the public flag surface:
//
//	always   - delete the temporary instance regardless of execution outcome
//	success  - delete only when execution succeeded
//	never    - keep the temporary instance even after execution
type CleanupPolicy string

const (
	// CleanupAlways deletes the temporary sandbox after every execution.
	CleanupAlways CleanupPolicy = "always"
	// CleanupSuccess deletes the temporary sandbox only after successful execution.
	CleanupSuccess CleanupPolicy = "success"
	// CleanupNever keeps the temporary sandbox after execution.
	CleanupNever CleanupPolicy = "never"
)

// ExecutionContext is attached to the JSON output of overlay-driven
// commands so machine consumers can see what the overlay did.
type ExecutionContext struct {
	SandboxInstanceId        string                  `json:"SandboxInstanceId"`
	TemporarySandboxInstance bool                    `json:"TemporarySandboxInstance"`
	Cleanup                  *ExecutionCleanupResult `json:"Cleanup,omitempty"`
}

// ExecutionCleanupResult records the outcome of the cleanup step.
type ExecutionCleanupResult struct {
	Policy string `json:"Policy"`
	Status string `json:"Status"` // deleted | skipped | failed
	Reason string `json:"Reason,omitempty"`
}

// OverlayFlags holds the shared `--create-temp-instance` flag set.
type OverlayFlags struct {
	CreateTempInstance bool
	Cleanup            string
	ToolName           string
	ToolID             string
}

// ResolvedOverlay returns the instance id the caller should execute against.
// When a temp instance was created, Cleanup must be called after execution.
type ResolvedOverlay struct {
	InstanceID     string
	IsTemp         bool
	Policy         CleanupPolicy
	ExecContext    *ExecutionContext
	cleanup        func(success bool)
	preExecCleanup func()
}

// Cleanup invokes the cleanup function with the execution outcome.
func (r *ResolvedOverlay) Cleanup(success bool) {
	if r == nil || r.cleanup == nil {
		return
	}
	r.cleanup(success)
}

// CleanupForPreExecutionFailure runs the cleanup path used when execution
// never started because validation failed after temp-instance creation.
func (r *ResolvedOverlay) CleanupForPreExecutionFailure() {
	if r == nil {
		return
	}
	if r.preExecCleanup != nil {
		r.preExecCleanup()
		return
	}
	r.Cleanup(false)
}

// ResolveCleanupPolicy validates --cleanup and returns the canonical value.
func ResolveCleanupPolicy(value string) (CleanupPolicy, error) {
	switch CleanupPolicy(value) {
	case CleanupAlways, CleanupSuccess, CleanupNever:
		return CleanupPolicy(value), nil
	}
	return "", output.NewUsageError("INVALID_CLEANUP",
		fmt.Sprintf("invalid --cleanup value: %q", value),
		"Use one of: always, success, never. To keep a temporary instance, use --cleanup never.")
}

// ResolveOverlay implements the create -> execute -> cleanup workflow.
func ResolveOverlay(
	ctx context.Context,
	flags OverlayFlags,
	args []string,
	apiCreate func(ctx context.Context, toolName, toolID string) (string, error),
	apiDelete func(ctx context.Context, instanceID string) error,
	notify func(format string, args ...any),
) (*ResolvedOverlay, error) {
	if notify == nil {
		notify = func(string, ...any) {}
	}

	policyValue := flags.Cleanup
	if policyValue == "" {
		policyValue = string(CleanupAlways)
	}
	policy, err := ResolveCleanupPolicy(policyValue)
	if err != nil {
		return nil, err
	}

	hasInstanceID := len(args) > 0 && args[0] != ""
	if hasInstanceID {
		if flags.CreateTempInstance {
			return nil, output.NewUsageError("CONFLICTING_FLAGS",
				"--create-temp-instance cannot be used together with a positional instance id",
				"Provide either an existing instance id or --create-temp-instance, not both.")
		}
		return &ResolvedOverlay{
			InstanceID:     args[0],
			IsTemp:         false,
			Policy:         policy,
			ExecContext:    &ExecutionContext{SandboxInstanceId: args[0]},
			cleanup:        func(bool) {},
			preExecCleanup: func() {},
		}, nil
	}

	if !flags.CreateTempInstance {
		return nil, output.NewUsageError("MISSING_INSTANCE",
			"no instance id provided",
			"Provide an instance id, or add --create-temp-instance to spin up a temporary sandbox.")
	}

	if flags.ToolName == "" && flags.ToolID == "" {
		return nil, output.NewUsageError("MISSING_REQUIRED_FLAG",
			"--create-temp-instance requires --tool-name/-t or --tool-id",
			"Provide --tool-name <existing-tool-name> or --tool-id sdt-xxxx.")
	}
	if flags.ToolName != "" && flags.ToolID != "" {
		return nil, output.NewUsageError("CONFLICTING_FLAGS",
			"--tool-name and --tool-id are mutually exclusive",
			"Pick exactly one.")
	}

	id, err := apiCreate(ctx, flags.ToolName, flags.ToolID)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary instance: %w", err)
	}

	exec := &ExecutionContext{
		SandboxInstanceId:        id,
		TemporarySandboxInstance: true,
		Cleanup: &ExecutionCleanupResult{
			Policy: string(policy),
			Status: "skipped",
		},
	}

	cleanupFn := func(success bool) {
		switch policy {
		case CleanupNever:
			exec.Cleanup.Status = "skipped"
			exec.Cleanup.Reason = "policy=never"
			notify("Temporary sandbox %s kept (--cleanup never)\n", id)
			return
		case CleanupSuccess:
			if !success {
				exec.Cleanup.Status = "skipped"
				exec.Cleanup.Reason = "policy=success and execution did not succeed"
				notify("Temporary sandbox %s kept (--cleanup success but execution failed)\n", id)
				return
			}
		}
		if err := apiDelete(ctx, id); err != nil {
			exec.Cleanup.Status = "failed"
			exec.Cleanup.Reason = err.Error()
			notify("Warning: failed to delete temporary sandbox %s: %v\n", id, err)
			return
		}
		exec.Cleanup.Status = "deleted"
		notify("Temporary sandbox %s deleted (--cleanup %s)\n", id, policy)
	}

	preExecCleanupFn := func() {
		cleanupFn(false)
	}

	return &ResolvedOverlay{
		InstanceID:     id,
		IsTemp:         true,
		Policy:         policy,
		ExecContext:    exec,
		cleanup:        cleanupFn,
		preExecCleanup: preExecCleanupFn,
	}, nil
}
