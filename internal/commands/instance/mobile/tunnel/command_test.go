package tunnel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/adbtunnel"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestModuleDescriptor(t *testing.T) {
	module := Module()
	spec := module.Descriptor.Spec
	if spec.ID != "instance.mobile.tunnel" || spec.Output.DataType != "MobileTunnel" {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestRunTunnelForegroundLifecycle(t *testing.T) {
	out := &bytes.Buffer{}
	fake := &fakeTunnel{addr: "127.0.0.1:5555"}
	result, err := buildRuntime(t, out, fake, nil).Handler.Run(context.Background(), request(false, 0))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.StreamDone || !fake.stopped || !strings.Contains(out.String(), "ADB Tunnel established") {
		t.Fatalf("result=%#v stopped=%t out=%q", result, fake.stopped, out.String())
	}
}

func TestRunTunnelDaemonWritesReadyMessage(t *testing.T) {
	out := &bytes.Buffer{}
	fake := &fakeTunnel{addr: "127.0.0.1:5555"}
	_, err := buildRuntime(t, out, fake, nil).Handler.Run(context.Background(), request(true, 0))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	var msg readyMessage
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &msg); err != nil {
		t.Fatalf("ready json=%q err=%v", out.String(), err)
	}
	if msg.Status != "ready" || msg.Port != 5555 {
		t.Fatalf("message=%#v", msg)
	}
}

func TestRunTunnelRejectsInvalidPort(t *testing.T) {
	_, err := buildRuntime(t, &bytes.Buffer{}, &fakeTunnel{}, nil).Handler.Run(context.Background(), request(false, 70000))
	if err == nil || !strings.Contains(err.Error(), "--port must be between") {
		t.Fatalf("error=%v, want invalid port", err)
	}
}

func TestRunTunnelDaemonWritesProbeError(t *testing.T) {
	out := &bytes.Buffer{}
	fake := &fakeTunnel{addr: "127.0.0.1:5555", probeErr: errors.New("upstream unavailable")}
	_, err := buildRuntime(t, out, fake, nil).Handler.Run(context.Background(), request(true, 0))
	if err == nil || !strings.Contains(err.Error(), "upstream probe failed") {
		t.Fatalf("error=%v, want probe error", err)
	}
	if !fake.stopped || !strings.Contains(out.String(), `"status":"error"`) {
		t.Fatalf("stopped=%t out=%q", fake.stopped, out.String())
	}
}

func buildRuntime(t *testing.T, out *bytes.Buffer, tunnel *fakeTunnel, newErr error) command.Runtime {
	t.Helper()
	runtime, err := Module().Build(command.Deps{
		IO: &iostreams.IOStreams{In: &bytes.Buffer{}, Out: out, ErrOut: &bytes.Buffer{}},
		DataPlane: RuntimeDeps{
			AcquireToken:   func(context.Context, string) (string, error) { return "token", nil },
			ValidateConfig: func() error { return nil },
			NewTunnel: func(opts adbtunnel.TunnelOptions) (Tunnel, error) {
				tunnel.opts = opts
				return tunnel, newErr
			},
			Wait:        func(context.Context) {},
			StopTimeout: time.Millisecond,
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return runtime
}

func request(daemon bool, port int) command.Request {
	return command.Request{
		Args:      []string{"ins-1"},
		ArgValues: map[string]string{"instance-id": "ins-1"},
		Flags: map[string]command.FlagValue{
			"daemon": {Name: "daemon", Type: command.FlagBool, Bool: daemon, Changed: daemon},
			"port":   {Name: "port", Type: command.FlagInt, Int: port, Changed: port != 0},
		},
	}
}

type fakeTunnel struct {
	addr     string
	probeErr error
	stopped  bool
	opts     adbtunnel.TunnelOptions
}

func (f *fakeTunnel) Start() (string, error) {
	if f.addr == "" {
		return "127.0.0.1:5555", nil
	}
	return f.addr, nil
}

func (f *fakeTunnel) Probe() error { return f.probeErr }

func (f *fakeTunnel) Stop() { f.stopped = true }
