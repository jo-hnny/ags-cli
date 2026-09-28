package config

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func TestConfigFailureContext(t *testing.T) {
	old := cfgFile
	t.Cleanup(func() { cfgFile = old })
	path := t.TempDir() + "/config.toml"
	SetConfigFile(path)
	got := output.ClassifyError(Init())
	if !errors.Is(got, os.ErrNotExist) || got.Failure.Details["Path"] != path || got.Failure.Details["Stage"] != "config_read" {
		t.Fatalf("missing read context: %#v", got.Failure)
	}
	for _, content := range []string{`auth = "unregistered-secret"`, "[auth]\nsecret_key = \"unregistered-secret\"\n[broken"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		err := Init()
		if err == nil {
			t.Fatal("expected invalid config")
		}
		for cause := err; cause != nil; cause = errors.Unwrap(cause) {
			if strings.Contains(cause.Error(), "unregistered-secret") {
				t.Fatal("config value exposed")
			}
		}
		got = output.ClassifyError(err)
		if got.Failure.Details["Path"] != path {
			t.Fatalf("missing parse path: %#v", got.Failure)
		}
	}
}
