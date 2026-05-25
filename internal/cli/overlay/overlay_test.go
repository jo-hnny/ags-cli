package overlay

import (
	"context"
	"errors"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func failureCode(err error) string {
	if err == nil {
		return ""
	}
	var cli *output.CLIError
	if errors.As(err, &cli) && cli.Failure != nil {
		return cli.Failure.Code
	}
	return ""
}

func TestResolveOverlay_ExistingInstance(t *testing.T) {
	ctx := context.Background()
	r, err := ResolveOverlay(ctx, OverlayFlags{}, []string{"ins-existing"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.IsTemp {
		t.Fatalf("expected not temp")
	}
	if r.InstanceID != "ins-existing" {
		t.Fatalf("instance id: %s", r.InstanceID)
	}
	r.Cleanup(true)
}

func TestResolveOverlay_RequiresEitherIDOrFlag(t *testing.T) {
	ctx := context.Background()
	_, err := ResolveOverlay(ctx, OverlayFlags{}, nil, nil, nil, nil)
	if got := failureCode(err); got != "MISSING_INSTANCE" {
		t.Fatalf("expected MISSING_INSTANCE, got code=%s err=%v", got, err)
	}
}

func TestResolveOverlay_TempCreateAndDelete(t *testing.T) {
	ctx := context.Background()
	created := false
	deleted := false
	create := func(_ context.Context, toolName, toolID string) (string, error) {
		created = true
		if toolName != "code-interpreter-v1" {
			t.Errorf("toolName=%s", toolName)
		}
		return "ins-temp", nil
	}
	del := func(_ context.Context, id string) error {
		deleted = true
		if id != "ins-temp" {
			t.Errorf("delete id=%s", id)
		}
		return nil
	}
	r, err := ResolveOverlay(ctx, OverlayFlags{
		CreateTempInstance: true,
		Cleanup:            string(CleanupAlways),
		ToolName:           "code-interpreter-v1",
	}, nil, create, del, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !created || !r.IsTemp {
		t.Fatalf("create not invoked or not temp")
	}
	r.Cleanup(true)
	if !deleted {
		t.Fatalf("delete not invoked")
	}
	if r.ExecContext.Cleanup.Status != "deleted" {
		t.Fatalf("cleanup status=%s", r.ExecContext.Cleanup.Status)
	}
}

func TestResolveOverlay_CleanupNeverKeepsInstance(t *testing.T) {
	ctx := context.Background()
	deleted := false
	del := func(_ context.Context, id string) error {
		deleted = true
		return nil
	}
	r, err := ResolveOverlay(ctx, OverlayFlags{
		CreateTempInstance: true,
		Cleanup:            string(CleanupNever),
		ToolName:           "code-interpreter-v1",
	}, nil, func(context.Context, string, string) (string, error) { return "ins-temp", nil }, del, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	r.Cleanup(true)
	if deleted {
		t.Fatalf("expected delete NOT invoked under --cleanup never")
	}
	if r.ExecContext.Cleanup.Status != "skipped" {
		t.Fatalf("cleanup status=%s", r.ExecContext.Cleanup.Status)
	}
}

func TestResolveOverlay_CleanupSuccessOnFailure(t *testing.T) {
	ctx := context.Background()
	deleted := false
	del := func(_ context.Context, id string) error { deleted = true; return nil }
	r, err := ResolveOverlay(ctx, OverlayFlags{
		CreateTempInstance: true,
		Cleanup:            string(CleanupSuccess),
		ToolName:           "code-interpreter-v1",
	}, nil, func(context.Context, string, string) (string, error) { return "ins-temp", nil }, del, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	r.Cleanup(false)
	if deleted {
		t.Fatalf("expected NOT to delete on failure under --cleanup success")
	}
	if r.ExecContext.Cleanup.Status != "skipped" {
		t.Fatalf("cleanup status=%s", r.ExecContext.Cleanup.Status)
	}
}

func TestResolveOverlay_RequiresToolNameOrToolID(t *testing.T) {
	ctx := context.Background()
	_, err := ResolveOverlay(ctx, OverlayFlags{CreateTempInstance: true}, nil, nil, nil, nil)
	if got := failureCode(err); got != "MISSING_REQUIRED_FLAG" {
		t.Fatalf("expected MISSING_REQUIRED_FLAG, got code=%s err=%v", got, err)
	}
}

func TestResolveOverlay_RejectsBothIdAndCreate(t *testing.T) {
	ctx := context.Background()
	_, err := ResolveOverlay(ctx, OverlayFlags{CreateTempInstance: true, ToolName: "x"}, []string{"ins-existing"}, nil, nil, nil)
	if got := failureCode(err); got != "CONFLICTING_FLAGS" {
		t.Fatalf("expected CONFLICTING_FLAGS, got code=%s err=%v", got, err)
	}
}

func TestResolveOverlay_RejectsInvalidCleanup(t *testing.T) {
	ctx := context.Background()
	_, err := ResolveOverlay(ctx, OverlayFlags{CreateTempInstance: true, ToolName: "x", Cleanup: "bogus"}, nil, nil, nil, nil)
	if got := failureCode(err); got != "INVALID_CLEANUP" {
		t.Fatalf("expected INVALID_CLEANUP, got code=%s err=%v", got, err)
	}
}

func TestResolveOverlay_DeleteFailureRecorded(t *testing.T) {
	ctx := context.Background()
	r, err := ResolveOverlay(ctx, OverlayFlags{
		CreateTempInstance: true,
		Cleanup:            string(CleanupAlways),
		ToolName:           "code-interpreter-v1",
	}, nil,
		func(context.Context, string, string) (string, error) { return "ins-temp", nil },
		func(context.Context, string) error { return errFakeDelete },
		nil,
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	r.Cleanup(true)
	if r.ExecContext.Cleanup.Status != "failed" {
		t.Fatalf("expected status=failed, got %s", r.ExecContext.Cleanup.Status)
	}
}

var errFakeDelete = &fakeError{}

type fakeError struct{}

func (e *fakeError) Error() string { return "fake delete error" }
