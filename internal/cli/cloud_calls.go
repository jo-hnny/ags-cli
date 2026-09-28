package cli

import (
	"context"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

var cloudStartSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
	resp, err := client.CallCloud(ctx, "StartSandboxInstance", req, sdk.StartSandboxInstanceWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudDescribeSandboxInstanceList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeSandboxInstanceListRequest) (*ags.DescribeSandboxInstanceListResponseParams, error) {
	resp, err := client.CallCloud(ctx, "DescribeSandboxInstanceList", req, sdk.DescribeSandboxInstanceListWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudUpdateSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.UpdateSandboxInstanceRequest) (*ags.UpdateSandboxInstanceResponseParams, error) {
	resp, err := client.CallCloud(ctx, "UpdateSandboxInstance", req, sdk.UpdateSandboxInstanceWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudPauseSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.PauseSandboxInstanceRequest) (*ags.PauseSandboxInstanceResponseParams, error) {
	resp, err := client.CallCloud(ctx, "PauseSandboxInstance", req, sdk.PauseSandboxInstanceWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudResumeSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.ResumeSandboxInstanceRequest) (*ags.ResumeSandboxInstanceResponseParams, error) {
	resp, err := client.CallCloud(ctx, "ResumeSandboxInstance", req, sdk.ResumeSandboxInstanceWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudStopSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
	resp, err := client.CallCloud(ctx, "StopSandboxInstance", req, sdk.StopSandboxInstanceWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudAcquireSandboxInstanceToken = func(ctx context.Context, sdk *ags.Client, req *ags.AcquireSandboxInstanceTokenRequest) (*ags.AcquireSandboxInstanceTokenResponseParams, error) {
	resp, err := client.CallCloud(ctx, "AcquireSandboxInstanceToken", req, sdk.AcquireSandboxInstanceTokenWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudCreateSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.CreateSandboxToolRequest) (*ags.CreateSandboxToolResponseParams, error) {
	resp, err := client.CallCloud(ctx, "CreateSandboxTool", req, sdk.CreateSandboxToolWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudDescribeSandboxToolList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeSandboxToolListRequest) (*ags.DescribeSandboxToolListResponseParams, error) {
	resp, err := client.CallCloud(ctx, "DescribeSandboxToolList", req, sdk.DescribeSandboxToolListWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudUpdateSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.UpdateSandboxToolRequest) (*ags.UpdateSandboxToolResponseParams, error) {
	resp, err := client.CallCloud(ctx, "UpdateSandboxTool", req, sdk.UpdateSandboxToolWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudDeleteSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.DeleteSandboxToolRequest) (*ags.DeleteSandboxToolResponseParams, error) {
	resp, err := client.CallCloud(ctx, "DeleteSandboxTool", req, sdk.DeleteSandboxToolWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudCreateAPIKey = func(ctx context.Context, sdk *ags.Client, req *ags.CreateAPIKeyRequest) (*ags.CreateAPIKeyResponseParams, error) {
	resp, err := client.CallCloud(ctx, "CreateAPIKey", req, sdk.CreateAPIKeyWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudDescribeAPIKeyList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeAPIKeyListRequest) (*ags.DescribeAPIKeyListResponseParams, error) {
	resp, err := client.CallCloud(ctx, "DescribeAPIKeyList", req, sdk.DescribeAPIKeyListWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudDeleteAPIKey = func(ctx context.Context, sdk *ags.Client, req *ags.DeleteAPIKeyRequest) (*ags.DeleteAPIKeyResponseParams, error) {
	resp, err := client.CallCloud(ctx, "DeleteAPIKey", req, sdk.DeleteAPIKeyWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudCreatePreCacheImageTask = func(ctx context.Context, sdk *ags.Client, req *ags.CreatePreCacheImageTaskRequest) (*ags.CreatePreCacheImageTaskResponseParams, error) {
	resp, err := client.CallCloud(ctx, "CreatePreCacheImageTask", req, sdk.CreatePreCacheImageTaskWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}

var cloudDescribePreCacheImageTask = func(ctx context.Context, sdk *ags.Client, req *ags.DescribePreCacheImageTaskRequest) (*ags.DescribePreCacheImageTaskResponseParams, error) {
	resp, err := client.CallCloud(ctx, "DescribePreCacheImageTask", req, sdk.DescribePreCacheImageTaskWithContext)
	if err != nil {
		return nil, err
	}
	return resp.Response, nil
}
