package exec

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/command"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/iostreams"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func TestNDJSONConnectionFailureRetainsCause(t *testing.T) {
	// A file in place of HOME makes token-cache creation fail locally before any
	// cloud client is constructed, exercising the real connection failure path.
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ios, _, stdout, _ := iostreams.Test()
	result, err := runExecStreamNDJSON(t.Context(), command.Deps{IO: ios}, execOptions{}, &cli.ResolvedOverlay{}, "ins-test", "true", nil)
	if err != nil || result == nil || !result.StreamDone || result.ExitCode != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	var pathErr *os.PathError
	if !errors.As(result.Cause, &pathErr) {
		t.Fatalf("lost filesystem cause: %v", result.Cause)
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("stream=%s", stdout)
	}
	got := cli.ClassifyCLIError(result.Cause)
	if got.Failure.Details["Stage"] != "remote_connect" || got.Failure.Details["InstanceId"] != "ins-test" {
		t.Fatalf("missing connection context: %#v", got.Failure)
	}
	for i, line := range lines {
		var event output.NDJSONEvent
		if json.Unmarshal(line, &event) != nil || event.Type != []string{"started", "failed"}[i] {
			t.Fatalf("event=%s", line)
		}
	}
}
