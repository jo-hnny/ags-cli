package vnc

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestModuleBuildsBrowserURLs(t *testing.T) {
	setupConfig(t)
	ios, _, stdout, _ := iostreams.Test()
	runtime, err := Module().Build(command.Deps{
		IO: ios,
		DataPlane: RuntimeDeps{
			AcquireToken: func(context.Context, string) (string, error) { return "token", nil },
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"ins-1"},
		ArgValues: map[string]string{"instance-id": "ins-1"},
		Flags:     map[string]command.FlagValue{"port": {Name: "port", Type: command.FlagInt, Int: 9000}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data := result.Data.(map[string]any)
	if data["InstanceId"] != "ins-1" || !strings.Contains(data["VncUrl"].(string), "access_token=token") {
		t.Fatalf("data=%#v", data)
	}
	result.Text(stdout)
	if !strings.Contains(stdout.String(), "VNC URL:") || !strings.Contains(stdout.String(), "CDP URL:") {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestModuleRejectsInvalidPort(t *testing.T) {
	setupConfig(t)
	runtime, err := Module().Build(command.Deps{IO: testIO()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"ins-1"},
		ArgValues: map[string]string{"instance-id": "ins-1"},
		Flags:     map[string]command.FlagValue{"port": {Name: "port", Type: command.FlagInt, Int: 70000}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid port") {
		t.Fatalf("error=%v, want invalid port", err)
	}
}

func TestBuildURLs(t *testing.T) {
	vncURL := buildVNCURL("ins-1", "ap-guangzhou", "example.com", "token", 9000)
	if !strings.Contains(vncURL, "9000-ins-1.ap-guangzhou.example.com") || !strings.Contains(vncURL, "access_token=token") {
		t.Fatalf("vncURL=%q", vncURL)
	}
	cdpURL := buildCDPURL("ins-1", "ap-guangzhou", "example.com", "token", 9000)
	if !strings.Contains(cdpURL, "/cdp?access_token=token") {
		t.Fatalf("cdpURL=%q", cdpURL)
	}
}

func setupConfig(t *testing.T) {
	t.Helper()
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	config.SetSecretID("AKIDfake")
	config.SetSecretKey("fakeSecretKey")
	config.SetRegion("ap-guangzhou")
	config.SetDomain("example.com")
}

func testIO() *iostreams.IOStreams {
	return &iostreams.IOStreams{In: &bytes.Buffer{}, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
}
