package cli

import (
	"context"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

var cloudStartSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
	resp, err := sdk.StartSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudDescribeSandboxInstanceList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeSandboxInstanceListRequest) (*ags.DescribeSandboxInstanceListResponseParams, error) {
	resp, err := sdk.DescribeSandboxInstanceListWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudUpdateSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.UpdateSandboxInstanceRequest) (*ags.UpdateSandboxInstanceResponseParams, error) {
	resp, err := sdk.UpdateSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudPauseSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.PauseSandboxInstanceRequest) (*ags.PauseSandboxInstanceResponseParams, error) {
	resp, err := sdk.PauseSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudResumeSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.ResumeSandboxInstanceRequest) (*ags.ResumeSandboxInstanceResponseParams, error) {
	resp, err := sdk.ResumeSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudStopSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
	resp, err := sdk.StopSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudAcquireSandboxInstanceToken = func(ctx context.Context, sdk *ags.Client, req *ags.AcquireSandboxInstanceTokenRequest) (*ags.AcquireSandboxInstanceTokenResponseParams, error) {
	resp, err := sdk.AcquireSandboxInstanceTokenWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudCreateSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.CreateSandboxToolRequest) (*ags.CreateSandboxToolResponseParams, error) {
	resp, err := sdk.CreateSandboxToolWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudDescribeSandboxToolList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeSandboxToolListRequest) (*ags.DescribeSandboxToolListResponseParams, error) {
	resp, err := sdk.DescribeSandboxToolListWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudUpdateSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.UpdateSandboxToolRequest) (*ags.UpdateSandboxToolResponseParams, error) {
	resp, err := sdk.UpdateSandboxToolWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudDeleteSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.DeleteSandboxToolRequest) (*ags.DeleteSandboxToolResponseParams, error) {
	resp, err := sdk.DeleteSandboxToolWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudCreateAPIKey = func(ctx context.Context, sdk *ags.Client, req *ags.CreateAPIKeyRequest) (*ags.CreateAPIKeyResponseParams, error) {
	resp, err := sdk.CreateAPIKeyWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudDescribeAPIKeyList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeAPIKeyListRequest) (*ags.DescribeAPIKeyListResponseParams, error) {
	resp, err := sdk.DescribeAPIKeyListWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudDeleteAPIKey = func(ctx context.Context, sdk *ags.Client, req *ags.DeleteAPIKeyRequest) (*ags.DeleteAPIKeyResponseParams, error) {
	resp, err := sdk.DeleteAPIKeyWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudCreatePreCacheImageTask = func(ctx context.Context, sdk *ags.Client, req *ags.CreatePreCacheImageTaskRequest) (*ags.CreatePreCacheImageTaskResponseParams, error) {
	resp, err := sdk.CreatePreCacheImageTaskWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}

var cloudDescribePreCacheImageTask = func(ctx context.Context, sdk *ags.Client, req *ags.DescribePreCacheImageTaskRequest) (*ags.DescribePreCacheImageTaskResponseParams, error) {
	resp, err := sdk.DescribePreCacheImageTaskWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(err)
	}
	return resp.Response, nil
}
