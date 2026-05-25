//go:build !windows

package pty

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/TencentCloudAgentRuntime/ags-go-sdk/pb/process/processconnect"
)

func startResizeWatcher(cancelCtx context.Context, rpcClient processconnect.ProcessClient, pid uint32, accessToken string) chan struct{} {
	sigwinchCh := make(chan os.Signal, 1)
	signal.Notify(sigwinchCh, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		defer func() {
			signal.Stop(sigwinchCh)
			close(done)
		}()
		for {
			select {
			case <-sigwinchCh:
				newCols, newRows := termSize()
				resizePTY(cancelCtx, rpcClient, pid, uint32(newCols), uint32(newRows), accessToken)
			case <-cancelCtx.Done():
				return
			}
		}
	}()
	return done
}
