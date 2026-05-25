package cli

import (
	"context"
	"fmt"

	"github.com/TencentCloudAgentRuntime/ags-go-sdk/connection"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/constant"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/sandbox/code"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/sandbox/core"
	toolcode "github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/code"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/command"
	"github.com/TencentCloudAgentRuntime/ags-go-sdk/tool/filesystem"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/token"
	ags "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ags/v20250920"
)

// resolveUser returns the effective sandbox user.
// Priority: flag value > config default_user > "user".
func resolveUser(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if cfgUser := config.GetSandboxUser(); cfgUser != "" {
		return cfgUser
	}
	return "user"
}

// ConnectWithToken connects to an existing sandbox instance using an access token.
// Only data-plane clients (Files, Commands, Code) are initialized.
func ConnectWithToken(ctx context.Context, instanceID string, accessToken string) (*code.Sandbox, error) {
	cfg := config.Get()
	domain := cfg.DataPlaneRegionDomain()

	connConfig := &connection.Config{
		Domain:      domain,
		AccessToken: accessToken,
	}

	coreInstance := core.NewCore(nil, instanceID, connConfig)
	sandbox := &code.Sandbox{Core: coreInstance}

	var err error
	sandbox.Files, err = filesystem.New(&connection.Config{
		Domain:      sandbox.GetHost(constant.EnvdPort),
		AccessToken: accessToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize filesystem client: %w", err)
	}

	sandbox.Commands, err = command.New(&connection.Config{
		Domain:      sandbox.GetHost(constant.EnvdPort),
		AccessToken: accessToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize command client: %w", err)
	}

	sandbox.Code = toolcode.New(&connection.Config{
		Domain:      sandbox.GetHost(constant.CodePort),
		AccessToken: accessToken,
	})

	return sandbox, nil
}

// acquireInstanceToken acquires an access token for the given instance
// by checking cache first, then calling the control plane API.
func acquireInstanceToken(ctx context.Context, instanceID string) (string, error) {
	tokenCache, err := token.NewCache()
	if err == nil {
		if cachedToken, ok := tokenCache.Get(instanceID); ok && cachedToken != "" {
			return cachedToken, nil
		}
	}

	apiClient, err := newCloudClient()
	if err != nil {
		return "", fmt.Errorf("failed to create API client: %w", err)
	}

	req := ags.NewAcquireSandboxInstanceTokenRequest()
	req.InstanceId = &instanceID
	resp, err := cloudAcquireSandboxInstanceToken(ctx, apiClient, req)
	if err != nil {
		return "", err
	}
	accessToken := derefString(resp.Token)

	if tokenCache != nil {
		_ = tokenCache.Set(instanceID, accessToken)
	}

	return accessToken, nil
}

// GetCachedTokenOrAcquire gets the access token from cache, or acquires a new one.
func GetCachedTokenOrAcquire(ctx context.Context, instanceID string) (string, error) {
	tokenCache, err := token.NewCache()
	if err != nil {
		return "", fmt.Errorf("failed to create token cache: %w", err)
	}

	if cachedToken, found := tokenCache.Get(instanceID); found {
		return cachedToken, nil
	}

	accessToken, err := acquireInstanceToken(ctx, instanceID)
	if err != nil {
		return "", err
	}

	_ = tokenCache.Set(instanceID, accessToken)
	return accessToken, nil
}

// connectSandbox is the injectable factory for data-plane sandbox connections.
// Tests can override this to inject fakes.
var connectSandbox = connectSandboxDefault

func connectSandboxDefault(ctx context.Context, instanceID string) (*code.Sandbox, error) {
	accessToken, err := GetCachedTokenOrAcquire(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return ConnectWithToken(ctx, instanceID, accessToken)
}

// ConnectSandboxWithCache connects to an existing sandbox using cached token.
func ConnectSandboxWithCache(ctx context.Context, instanceID string) (*code.Sandbox, error) {
	return connectSandbox(ctx, instanceID)
}
