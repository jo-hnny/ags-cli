//go:build windows

package pty

import (
	"context"

	"github.com/TencentCloudAgentRuntime/ags-go-sdk/pb/process/processconnect"
)

func startResizeWatcher(cancelCtx context.Context, rpcClient processconnect.ProcessClient, pid uint32, accessToken string) chan struct{} {
	done := make(chan struct{})
	go func() {
		<-cancelCtx.Done()
		close(done)
	}()
	return done
}
