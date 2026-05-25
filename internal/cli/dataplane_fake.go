package cli

import (
	"context"
	"io"
)

type dataPlaneOverride interface {
	RunCode(ctx context.Context, instanceID, code, language string) (stdout, stderr string, results any, remoteErr any, executionCount int, err error)
	Exec(ctx context.Context, instanceID string, argv []string) (stdout, stderr string, exitCode int, remoteErr any, err error)
	Upload(ctx context.Context, instanceID, localPath, remotePath string, r io.Reader) (path string, size int64, err error)
	Download(ctx context.Context, instanceID, remotePath string) (io.Reader, int64, error)
}

var testDataPlane dataPlaneOverride
