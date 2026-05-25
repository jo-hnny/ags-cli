package disconnect

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/tunnelstore"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
)

func TestModuleDisconnectsSingleConnection(t *testing.T) {
	store := &fakeStore{entries: map[string]tunnelstore.TunnelEntry{"ins-1": {Port: 5555}}}
	rt, stderr := runWithStore(t, store, command.Request{
		Args:      []string{"ins-1"},
		ArgValues: map[string]string{"instance-id": "ins-1"},
	})
	if rt.Data.(map[string]any)["InstanceId"] != "ins-1" || store.cleaned != "ins-1" {
		t.Fatalf("result=%#v store=%#v", rt, store)
	}
	rt.Text(ioDiscard{})
	if !strings.Contains(stderr.String(), "disconnected from ins-1") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestModuleDisconnectsAllConnections(t *testing.T) {
	store := &fakeStore{entries: map[string]tunnelstore.TunnelEntry{
		"ins-1": {Port: 5555, CreatedAt: time.Now()},
		"ins-2": {Port: 5556, CreatedAt: time.Now()},
	}}
	result, _ := runWithStore(t, store, command.Request{
		Flags: map[string]command.FlagValue{"all": {Name: "all", Type: command.FlagBool, Bool: true, Changed: true}},
	})
	if result.Data.(map[string]any)["Count"] != 2 || !store.cleanedAll {
		t.Fatalf("result=%#v store=%#v", result, store)
	}
}

func TestModuleRejectsAllWithInstanceID(t *testing.T) {
	_, err := buildRuntime(t, &fakeStore{}).Handler.Run(context.Background(), command.Request{
		Args:  []string{"ins-1"},
		Flags: map[string]command.FlagValue{"all": {Name: "all", Type: command.FlagBool, Bool: true, Changed: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "--all cannot be used") {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestModuleReturnsNoActiveTunnel(t *testing.T) {
	_, err := buildRuntime(t, &fakeStore{entries: map[string]tunnelstore.TunnelEntry{}}).Handler.Run(context.Background(), command.Request{Args: []string{"ins-1"}})
	if err == nil || !strings.Contains(err.Error(), "no active tunnel") {
		t.Fatalf("error=%v, want no active tunnel", err)
	}
}

func runWithStore(t *testing.T, store *fakeStore, req command.Request) (*command.Result, *bytes.Buffer) {
	t.Helper()
	ios, _, _, stderr := iostreams.Test()
	runtime := buildRuntimeWithIO(t, store, ios)
	result, err := runtime.Handler.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	return result, stderr
}

func buildRuntime(t *testing.T, store *fakeStore) command.Runtime {
	t.Helper()
	return buildRuntimeWithIO(t, store, testIO())
}

func buildRuntimeWithIO(t *testing.T, store *fakeStore, ios *iostreams.IOStreams) command.Runtime {
	t.Helper()
	runtime, err := Module().Build(command.Deps{
		IO: ios,
		DataPlane: RuntimeDeps{
			NewStore:   func() (Store, error) { return store, nil },
			RequireADB: func() (string, error) { return "", nil },
			RunADB:     func(string, ...string) error { return nil },
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	return runtime
}

type fakeStore struct {
	entries    map[string]tunnelstore.TunnelEntry
	cleaned    string
	cleanedAll bool
}

func (f *fakeStore) Get(id string) (tunnelstore.TunnelEntry, bool, error) {
	entry, ok := f.entries[id]
	return entry, ok, nil
}

func (f *fakeStore) List() (map[string]tunnelstore.TunnelEntry, error) {
	return f.entries, nil
}

func (f *fakeStore) Cleanup(id string) error {
	f.cleaned = id
	return nil
}

func (f *fakeStore) CleanupAll() error {
	f.cleanedAll = true
	return nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func testIO() *iostreams.IOStreams {
	return &iostreams.IOStreams{In: &bytes.Buffer{}, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
}
