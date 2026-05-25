package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
)

func TestControlPlaneEndpoint_Default(t *testing.T) {
	resetConfig(t)
	got := config.GetCloudEndpoint()
	if got != "ags.tencentcloudapi.com" {
		t.Fatalf("default cloud endpoint = %q, want ags.tencentcloudapi.com", got)
	}
}

func TestControlPlaneEndpoint_Override(t *testing.T) {
	resetConfig(t)
	config.SetCloudEndpoint("ags.example.com")
	if got := config.GetCloudEndpoint(); got != "ags.example.com" {
		t.Fatalf("cloud endpoint = %q, want ags.example.com", got)
	}
	if got := config.GetCloudEndpointRaw(); got != "ags.example.com" {
		t.Fatalf("raw cloud endpoint = %q", got)
	}
}

func TestControlPlaneEndpoint_FromConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "agr.toml")
	body := []byte("region = \"ap-guangzhou\"\ncloud_endpoint = \"ags.example.test\"\ndomain = \"tencentags.com\"\n")
	if err := os.WriteFile(cfg, body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	config.SetConfigFile(cfg)
	if err := config.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if got := config.GetCloudEndpoint(); got != "ags.example.test" {
		t.Fatalf("cloud endpoint = %q", got)
	}
}

func TestControlPlaneEndpoint_RejectsInvalidValues(t *testing.T) {
	cases := []string{
		"https://ags.example.com",
		"ags.example.com/path",
		"ags  example.com",
	}
	for _, c := range cases {
		resetConfig(t)
		config.SetCloudEndpoint(c)
		if err := config.ValidateBasics(); err == nil {
			t.Errorf("expected validation error for %q", c)
		}
	}
}

func resetConfig(t *testing.T) {
	t.Helper()
	config.SetConfigFile("")
	if err := config.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
}
