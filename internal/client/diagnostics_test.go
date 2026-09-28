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
