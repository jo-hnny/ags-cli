package upload

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestModuleUploadsWithTestDataPlane(t *testing.T) {
	setupConfig(t)
	dp := &fakeFileDataPlane{}
	defer cli.SetTestDataPlaneForTest(dp)()

	localPath := filepath.Join(t.TempDir(), "data.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	ios, _, stdout, _ := iostreams.Test()
	runtime, err := Module().Build(command.Deps{IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args: []string{"ins-1", localPath, "/tmp/data.txt"},
		ArgValues: map[string]string{
			"instance-id": "ins-1",
			"local-path":  localPath,
			"remote-path": "/tmp/data.txt",
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if dp.uploadInstanceID != "ins-1" || dp.uploadBody != "hello" {
		t.Fatalf("dp=%#v", dp)
	}
	data := result.Data.(map[string]any)
	if data["Operation"] != "upload" || data["Path"] != "/tmp/data.txt" {
		t.Fatalf("data=%#v", data)
	}
	result.Text(stdout)
	if !strings.Contains(stdout.String(), "Uploaded") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestModuleUploadsStdin(t *testing.T) {
	setupConfig(t)
	dp := &fakeFileDataPlane{}
	defer cli.SetTestDataPlaneForTest(dp)()
	ios, stdin, _, _ := iostreams.Test()
	stdin.WriteString("from stdin")
	runtime, err := Module().Build(command.Deps{IO: ios, Stdin: stdin})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:  []string{"ins-1", "-", "/tmp/stdin.txt"},
		Stdin: stdin,
		ArgValues: map[string]string{
			"instance-id": "ins-1",
			"local-path":  "-",
			"remote-path": "/tmp/stdin.txt",
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if dp.uploadBody != "from stdin" {
		t.Fatalf("uploadBody=%q", dp.uploadBody)
	}
}

func TestModuleRejectsMissingLocalFile(t *testing.T) {
	setupConfig(t)
	runtime, err := Module().Build(command.Deps{IO: testIO()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args: []string{"ins-1", filepath.Join(t.TempDir(), "missing"), "/tmp/data.txt"},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to stat local file") {
		t.Fatalf("error=%v, want invalid local path", err)
	}
}

type fakeFileDataPlane struct {
	uploadInstanceID string
	uploadBody       string
}

func (f *fakeFileDataPlane) RunCode(context.Context, string, string, string) (string, string, any, any, int, error) {
	return "", "", nil, nil, 0, nil
}

func (f *fakeFileDataPlane) Exec(context.Context, string, []string) (string, string, int, any, error) {
	return "", "", 0, nil, nil
}

func (f *fakeFileDataPlane) Upload(_ context.Context, instanceID, _ string, remotePath string, r io.Reader) (string, int64, error) {
	f.uploadInstanceID = instanceID
	body, err := io.ReadAll(r)
	if err != nil {
		return "", 0, err
	}
	f.uploadBody = string(body)
	return remotePath, int64(len(body)), nil
}

func (f *fakeFileDataPlane) Download(context.Context, string, string) (io.Reader, int64, error) {
	return bytes.NewBufferString(""), 0, nil
}

func setupConfig(t *testing.T) {
	t.Helper()
	cli.SetIOStreams(testIO())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	config.SetSecretID("AKIDfake")
	config.SetSecretKey("fakeSecretKey")
	config.SetRegion("ap-guangzhou")
}

func testIO() *iostreams.IOStreams {
	return &iostreams.IOStreams{In: &bytes.Buffer{}, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
}
