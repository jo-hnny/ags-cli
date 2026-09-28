package cli

import (
	"context"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

var cloudStartSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.StartSandboxInstanceRequest) (*ags.StartSandboxInstanceResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "StartSandboxInstance")
	resp, err := sdk.StartSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudDescribeSandboxInstanceList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeSandboxInstanceListRequest) (*ags.DescribeSandboxInstanceListResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "DescribeSandboxInstanceList")
	resp, err := sdk.DescribeSandboxInstanceListWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudUpdateSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.UpdateSandboxInstanceRequest) (*ags.UpdateSandboxInstanceResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "UpdateSandboxInstance")
	resp, err := sdk.UpdateSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudPauseSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.PauseSandboxInstanceRequest) (*ags.PauseSandboxInstanceResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "PauseSandboxInstance")
	resp, err := sdk.PauseSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudResumeSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.ResumeSandboxInstanceRequest) (*ags.ResumeSandboxInstanceResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "ResumeSandboxInstance")
	resp, err := sdk.ResumeSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudStopSandboxInstance = func(ctx context.Context, sdk *ags.Client, req *ags.StopSandboxInstanceRequest) (*ags.StopSandboxInstanceResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "StopSandboxInstance")
	resp, err := sdk.StopSandboxInstanceWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudAcquireSandboxInstanceToken = func(ctx context.Context, sdk *ags.Client, req *ags.AcquireSandboxInstanceTokenRequest) (*ags.AcquireSandboxInstanceTokenResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "AcquireSandboxInstanceToken")
	resp, err := sdk.AcquireSandboxInstanceTokenWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudCreateSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.CreateSandboxToolRequest) (*ags.CreateSandboxToolResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "CreateSandboxTool")
	resp, err := sdk.CreateSandboxToolWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudDescribeSandboxToolList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeSandboxToolListRequest) (*ags.DescribeSandboxToolListResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "DescribeSandboxToolList")
	resp, err := sdk.DescribeSandboxToolListWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudUpdateSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.UpdateSandboxToolRequest) (*ags.UpdateSandboxToolResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "UpdateSandboxTool")
	resp, err := sdk.UpdateSandboxToolWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudDeleteSandboxTool = func(ctx context.Context, sdk *ags.Client, req *ags.DeleteSandboxToolRequest) (*ags.DeleteSandboxToolResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "DeleteSandboxTool")
	resp, err := sdk.DeleteSandboxToolWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudCreateAPIKey = func(ctx context.Context, sdk *ags.Client, req *ags.CreateAPIKeyRequest) (*ags.CreateAPIKeyResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "CreateAPIKey")
	resp, err := sdk.CreateAPIKeyWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudDescribeAPIKeyList = func(ctx context.Context, sdk *ags.Client, req *ags.DescribeAPIKeyListRequest) (*ags.DescribeAPIKeyListResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "DescribeAPIKeyList")
	resp, err := sdk.DescribeAPIKeyListWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudDeleteAPIKey = func(ctx context.Context, sdk *ags.Client, req *ags.DeleteAPIKeyRequest) (*ags.DeleteAPIKeyResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "DeleteAPIKey")
	resp, err := sdk.DeleteAPIKeyWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudCreatePreCacheImageTask = func(ctx context.Context, sdk *ags.Client, req *ags.CreatePreCacheImageTaskRequest) (*ags.CreatePreCacheImageTaskResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "CreatePreCacheImageTask")
	resp, err := sdk.CreatePreCacheImageTaskWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}

var cloudDescribePreCacheImageTask = func(ctx context.Context, sdk *ags.Client, req *ags.DescribePreCacheImageTaskRequest) (*ags.DescribePreCacheImageTaskResponseParams, error) {
	diagnose := client.CloudCallContext(ctx, "DescribePreCacheImageTask")
	resp, err := sdk.DescribePreCacheImageTaskWithContext(ctx, req)
	if err != nil {
		return nil, client.ClassifyCloudError(diagnose(err))
	}
	return resp.Response, nil
}
