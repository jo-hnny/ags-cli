package client

import (
	"context"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// CloudCallContext captures the configured target and any caller deadline before
// SDK invocation. HTTP response headers/status are not exposed by the typed SDK.
func CloudCallContext(ctx context.Context, action string) func(error) error {
	details := output.HTTPContext(ctx, "http_request", "https://"+config.GetCloudEndpoint(), 0, nil)
	details["Operation"] = action
	return func(err error) error { return output.WithContext(err, details) }
}
