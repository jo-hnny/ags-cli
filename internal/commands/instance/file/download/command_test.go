package download

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

func TestModuleDownloadsWithTestDataPlane(t *testing.T) {
	setupConfig(t)
	dp := &fakeFileDataPlane{downloadBody: "hello"}
	defer cli.SetTestDataPlaneForTest(dp)()

	localPath := filepath.Join(t.TempDir(), "data.txt")
	ios, _, stdout, _ := iostreams.Test()
	runtime, err := Module().Build(command.Deps{IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args: []string{"ins-1", "/tmp/data.txt", localPath},
		ArgValues: map[string]string{
			"instance-id": "ins-1",
			"remote-path": "/tmp/data.txt",
			"local-path":  localPath,
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if dp.downloadInstanceID != "ins-1" || dp.downloadPath != "/tmp/data.txt" {
		t.Fatalf("dp=%#v", dp)
	}
	body, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read local file: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body=%q", body)
	}
	data := result.Data.(map[string]any)
	if data["Operation"] != "download" || data["Size"] != int64(5) {
		t.Fatalf("data=%#v", data)
	}
	result.Text(stdout)
	if !strings.Contains(stdout.String(), "Downloaded") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestModuleDownloadsToStdout(t *testing.T) {
	setupConfig(t)
	dp := &fakeFileDataPlane{downloadBody: "stdout"}
	defer cli.SetTestDataPlaneForTest(dp)()
	ios, _, stdout, _ := iostreams.Test()
	runtime, err := Module().Build(command.Deps{IO: ios})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args: []string{"ins-1", "/tmp/data.txt", "-"},
		ArgValues: map[string]string{
			"instance-id": "ins-1",
			"remote-path": "/tmp/data.txt",
			"local-path":  "-",
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.StreamDone || stdout.String() != "stdout" {
		t.Fatalf("result=%#v stdout=%q", result, stdout.String())
	}
}

func TestWriteDownloadResultRejectsBadLocalPath(t *testing.T) {
	_, err := writeDownloadResult(bytes.NewBufferString("x"), 1, "/remote", filepath.Join(t.TempDir(), "missing", "file"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "failed to create local file") {
		t.Fatalf("error=%v, want invalid local path", err)
	}
}

type fakeFileDataPlane struct {
	downloadInstanceID string
	downloadPath       string
	downloadBody       string
}

func (f *fakeFileDataPlane) RunCode(context.Context, string, string, string) (string, string, any, any, int, error) {
	return "", "", nil, nil, 0, nil
}

func (f *fakeFileDataPlane) Exec(context.Context, string, []string) (string, string, int, any, error) {
	return "", "", 0, nil, nil
}

func (f *fakeFileDataPlane) Upload(context.Context, string, string, string, io.Reader) (string, int64, error) {
	return "", 0, nil
}

func (f *fakeFileDataPlane) Download(_ context.Context, instanceID, remotePath string) (io.Reader, int64, error) {
	f.downloadInstanceID = instanceID
	f.downloadPath = remotePath
	return bytes.NewBufferString(f.downloadBody), int64(len(f.downloadBody)), nil
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
