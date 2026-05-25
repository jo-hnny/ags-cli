package proxy

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/proxy"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestModuleDescriptor(t *testing.T) {
	module := Module()
	spec := module.Descriptor.Spec
	if spec.ID != "instance.proxy" || len(spec.Args) != 2 || spec.Flags[0].Name != "address" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestRunProxyStartsAndStops(t *testing.T) {
	setupConfig(t)
	fake := &fakeProxy{addr: "127.0.0.1:3000"}
	var opts proxy.Options
	ios, _, stdout, stderr := iostreams.Test()
	runtime, err := Module().Build(command.Deps{
		IO: ios,
		DataPlane: RuntimeDeps{
			AcquireToken: func(context.Context, string) (string, error) { return "token", nil },
			NewProxy: func(o proxy.Options) (Proxy, error) {
				opts = o
				return fake, nil
			},
			Wait: func(context.Context) {},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	result, err := runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"ins-1", "3000:8080"},
		ArgValues: map[string]string{"instance-id": "ins-1", "port": "3000:8080"},
		Flags: map[string]command.FlagValue{
			"address": {Name: "address", Type: command.FlagString, String: "0.0.0.0"},
			"verbose": {Name: "verbose", Type: command.FlagBool, Bool: true, Changed: true},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.StreamDone || !fake.started || !fake.stopped {
		t.Fatalf("result=%#v fake=%#v", result, fake)
	}
	if opts.InstanceID != "ins-1" || opts.RemotePort != 8080 || opts.ListenAddress != "0.0.0.0:3000" || !opts.Verbose {
		t.Fatalf("opts=%#v", opts)
	}
	if !strings.Contains(stdout.String(), "Forwarding from 127.0.0.1:3000 -> 8080") {
		t.Fatalf("stdout=%q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "binding to 0.0.0.0 exposes") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestRunProxyRejectsInvalidPort(t *testing.T) {
	setupConfig(t)
	runtime, err := Module().Build(command.Deps{IO: testIO()})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	_, err = runtime.Handler.Run(context.Background(), command.Request{
		Args:      []string{"ins-1", "70000"},
		ArgValues: map[string]string{"instance-id": "ins-1", "port": "70000"},
		Flags:     map[string]command.FlagValue{"address": {Name: "address", Type: command.FlagString, String: "127.0.0.1"}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid port specification") {
		t.Fatalf("error=%v, want invalid port", err)
	}
}

func TestParsePortSpec(t *testing.T) {
	for _, tc := range []struct {
		spec       string
		wantLocal  int
		wantRemote int
	}{
		{spec: "3000", wantLocal: 3000, wantRemote: 3000},
		{spec: "3000:8080", wantLocal: 3000, wantRemote: 8080},
		{spec: "65535", wantLocal: 65535, wantRemote: 65535},
		{spec: "1", wantLocal: 1, wantRemote: 1},
	} {
		local, remote, err := parsePortSpec(tc.spec)
		if err != nil {
			t.Fatalf("parsePortSpec(%q): %v", tc.spec, err)
		}
		if local != tc.wantLocal || remote != tc.wantRemote {
			t.Fatalf("parsePortSpec(%q)=(%d,%d), want (%d,%d)", tc.spec, local, remote, tc.wantLocal, tc.wantRemote)
		}
	}
	for _, spec := range []string{"abc", "0", "70000", "-1", "abc:3000", "3000:abc", "0:3000", "3000:0", "70000:3000", "3000:70000", "", "8080:80:8080"} {
		if _, _, err := parsePortSpec(spec); err == nil {
			t.Fatalf("parsePortSpec(%q) expected error", spec)
		}
	}
}

type fakeProxy struct {
	addr    string
	started bool
	stopped bool
}

func (f *fakeProxy) Start() (string, error) {
	f.started = true
	return f.addr, nil
}

func (f *fakeProxy) Stop() { f.stopped = true }

func setupConfig(t *testing.T) {
	t.Helper()
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
