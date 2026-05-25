package client

import (
	"errors"
	"strings"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
	sdkerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
)

// ClassifyCloudError wraps a TencentCloud SDK error into a CLIError with
// the correct kind/exit code based on the SDK error code. This eliminates
// string matching in the generic ClassifyError.
func ClassifyCloudError(err error) error {
	if err == nil {
		return nil
	}
	var sdkErr *sdkerrors.TencentCloudSDKError
	if !errors.As(err, &sdkErr) {
		return err
	}

	code := sdkErr.GetCode()
	msg := sdkErr.GetMessage()

	switch {
	case code == "AuthFailure" || strings.HasPrefix(code, "AuthFailure."):
		return output.NewAuthError(code, msg, "Check your credentials (TENCENTCLOUD_SECRET_ID/TENCENTCLOUD_SECRET_KEY).")
	case code == "ResourceNotFound.SandboxTool":
		return output.NewNotFoundError(code, msg, "Run 'agr tool list' to find available tools.")
	case code == "ResourceNotFound.SandboxInstance":
		return output.NewNotFoundError(code, msg, "Run 'agr instance list' to find active instances.")
	case code == "ResourceNotFound" || strings.HasPrefix(code, "ResourceNotFound."):
		return output.NewNotFoundError(code, msg, "Verify the resource ID is correct and the resource has not been deleted.")
	case code == "ResourceInsufficient" || strings.HasPrefix(code, "LimitExceeded."):
		return output.NewCLIError(&output.Failure{
			Code: code, Kind: output.KindRateLimit, Message: msg, Hint: "Safe to retry after a brief wait.", Retryable: true,
		})
	case strings.Contains(code, "DuplicatedClientToken") || strings.Contains(code, "ClientTokenConflict"):
		return output.NewConflictError("CLIENT_TOKEN_CONFLICT", msg,
			"This token may have been used already. Use a different --client-token or omit it for a new instance.")
	case code == "UnauthorizedOperation" || strings.HasPrefix(code, "UnauthorizedOperation."):
		return output.NewAuthError(code, msg, "Check your permissions.")
	case strings.HasPrefix(code, "InvalidParameter") || strings.HasPrefix(code, "MissingParameter") || strings.HasPrefix(code, "InvalidParameterValue"):
		return output.NewUsageError(code, msg, "Check the command flags or request payload and try again.")
	case code == "RequestLimitExceeded":
		return output.NewCLIError(&output.Failure{
			Code: code, Kind: output.KindRateLimit, Message: msg, Hint: "Safe to retry after a brief wait.", Retryable: true,
		})
	default:
		return output.NewCLIError(&output.Failure{
			Code: code, Kind: output.KindGenericError, Message: msg, Hint: "Run 'agr doctor' to diagnose configuration and connectivity.",
		})
	}
}
