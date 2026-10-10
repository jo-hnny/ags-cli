package client

import (
	"context"
	"errors"
	"testing"
	"time"

	sdkerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

func TestCloudContextPreservesServiceFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	annotate := CloudCallContext(ctx, "DescribeSandboxInstanceList")
	cause := sdkerrors.NewTencentCloudSDKError("AuthFailure", "denied", "req-original")
	got := ClassifyError(annotate(cause))
	if !errors.Is(got, cause) || got.Failure.Code != "AuthFailure" || got.Failure.Message != "denied" || got.Failure.Details["RequestId"] != "req-original" || got.Failure.Details["Stage"] != "http_request" {
		t.Fatalf("lost cloud context: %#v", got.Failure)
	}
	if budget, ok := got.Failure.Details["TimeoutMs"].(int64); !ok || budget <= 0 || budget > 1000 {
		t.Fatalf("invalid budget: %#v", got.Failure.Details)
	}
	if _, ok := got.Failure.Details["HTTPStatus"]; ok {
		t.Fatal("invented status")
	}
}

func TestCallCloudCapturesBeforeInvocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	cause := sdkerrors.NewTencentCloudSDKError("AuthFailure", "denied", "req-call")
	calls := 0
	_, err := CallCloud(ctx, "Action", 42, func(received context.Context, request int) (string, error) {
		calls++
		if received != ctx || request != 42 {
			t.Fatal("invocation arguments changed")
		}
		cancel()
		return "", cause
	})
	got := ClassifyError(err)
	if calls != 1 || !errors.Is(got, cause) || got.Failure.Code != "AuthFailure" || got.Failure.Message != "denied" || got.Failure.Details["RequestId"] != "req-call" || got.Failure.Details["Operation"] != "Action" {
		t.Fatalf("lost cloud failure: %#v", got)
	}
	if budget, ok := got.Failure.Details["TimeoutMs"].(int64); !ok || budget <= 0 || budget > 1000 {
		t.Fatalf("budget captured after invocation: %#v", got.Failure.Details)
	}
	value, err := CallCloud(t.Context(), "Action", 42, func(context.Context, int) (string, error) { return "response", nil })
	if err != nil || value != "response" {
		t.Fatalf("success changed: %q %v", value, err)
	}
}

func TestCallCloudRetainsNonSDKCause(t *testing.T) {
	_, err := CallCloud(t.Context(), "Action", 0, func(context.Context, int) (int, error) { return 0, context.DeadlineExceeded })
	got := ClassifyError(err)
	if !errors.Is(got, context.DeadlineExceeded) || got.Failure.Code != "TIMEOUT" || got.Failure.Details["Stage"] != "http_request" {
		t.Fatalf("lost timeout: %#v", got)
	}
}
