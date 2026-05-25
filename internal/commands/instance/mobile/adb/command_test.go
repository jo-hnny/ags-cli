package adb

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/tunnelstore"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestModuleDescriptor(t *testing.T) {
	module := Module()
	spec := module.Descriptor.Spec
	if spec.ID != "instance.mobile.adb" || spec.Args[0].Name != "args" || !spec.Args[0].Repeatable || !spec.SupportsJSON {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestRunADBRequiresSeparator(t *testing.T) {
	_, err := buildRuntime(t, &fakeStore{}).Handler.Run(context.Background(), command.Request{Args: []string{"ins-1"}, DashPos: -1})
	if err == nil || !strings.Contains(err.Error(), "Use '--'") {
		t.Fatalf("error = %v, want separator usage", err)
	}
}

func TestRunADBStreamsCommand(t *testing.T) {
	store := &fakeStore{entries: map[string]tunnelstore.TunnelEntry{"ins-1": {Port: 5555}}}
	runtime := buildRuntime(t, store)
	result, err := runtime.Handler.Run(context.Background(), command.Request{Args: []string{"ins-1", "shell", "pwd"}, DashPos: 1})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.StreamDone || result.ExitCode != 7 {
		t.Fatalf("result = %#v", result)
	}
	if got := strings.Join(store.streamingArgs, " "); got != "-s 127.0.0.1:5555 shell pwd" {
		t.Fatalf("adb args = %q", got)
	}
}

func TestRunADBReturnsBufferedJSONResult(t *testing.T) {
	config.SetOutput("json")
	t.Cleanup(func() { config.SetOutput("text") })

	store := &fakeStore{entries: map[string]tunnelstore.TunnelEntry{"ins-1": {Port: 5555}}}
	runtime := buildRuntime(t, store)
	result, err := runtime.Handler.Run(context.Background(), command.Request{Args: []string{"ins-1", "shell", "false"}, DashPos: 1})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	data := result.Data.(map[string]any)
	if result.ExitCode != 9 || data["Stdout"] != "out" || data["Stderr"] != "err" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunADBReturnsNoActiveTunnel(t *testing.T) {
	_, err := buildRuntime(t, &fakeStore{entries: map[string]tunnelstore.TunnelEntry{}}).Handler.Run(context.Background(), command.Request{Args: []string{"ins-1", "shell"}, DashPos: 1})
	if err == nil || !strings.Contains(err.Error(), "no active tunnel") {
		t.Fatalf("error = %v, want no active tunnel", err)
	}
}

func buildRuntime(t *testing.T, store *fakeStore) command.Runtime {
	t.Helper()
	runtime, err := Module().Build(command.Deps{
		IO: &iostreams.IOStreams{In: &bytes.Buffer{}, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}},
		DataPlane: RuntimeDeps{
			NewStore:        func() (Store, error) { return store, nil },
			RequireADB:      func() (string, error) { return "/adb", nil },
			RunADBBuffered:  store.runBuffered,
			RunADBStreaming: store.runStreaming,
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return runtime
}

type fakeStore struct {
	entries       map[string]tunnelstore.TunnelEntry
	streamingArgs []string
	bufferedArgs  []string
}

func (f *fakeStore) Get(id string) (tunnelstore.TunnelEntry, bool, error) {
	entry, ok := f.entries[id]
	return entry, ok, nil
}

func (f *fakeStore) runBuffered(_ string, args ...string) (string, string, int, error) {
	f.bufferedArgs = append([]string(nil), args...)
	return "out", "err", 9, nil
}

func (f *fakeStore) runStreaming(_ string, args []string, _ io.Reader, _ io.Writer, _ io.Writer) (int, error) {
	f.streamingArgs = append([]string(nil), args...)
	return 7, nil
}
