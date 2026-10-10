package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/config"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

// Failed loads have not registered credentials with the redactor, so the
// reported reason must be value-free while still saying what is wrong.
func TestConfigParseErrorReasonOmitsValues(t *testing.T) {
	for _, tc := range []struct {
		name, body, stage, reason, value string
	}{
		{"unterminated string", "[auth]\nsecret_key = \"cfg-secret-1\n", "config_parse", "failed to parse config file: basic strings cannot have new lines", "cfg-secret"},
		{"unquoted value", "[auth]\nsecret_key = cfg-secret-1\n", "config_parse", "failed to parse config file: incomplete number", "cfg-secret"},
		{"input character", "[auth]\nsecret_key = 0xcfg-secret-1\n", "config_parse", "failed to parse config file: expected newline but got [REDACTED]", "'g'"},
		{"number quoted by strconv", "[auth]\nsecret_key = 98765432109876543210\n", "config_parse", "failed to parse config file: couldn't parse decimal number", "98765432109876543210"},
		{"table instead of string", "[auth.secret_key]\nvalue = \"cfg-secret-1\"\n", "config_decode", `failed to parse config file: field "auth.secret_key" expected type 'string', got unconvertible type 'map[string]interface {}'`, "cfg-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			config.SetConfigFile(path)
			t.Cleanup(func() { config.SetConfigFile("") })
			err := config.Init()
			if err == nil {
				t.Fatal("expected config failure")
			}
			failure := output.ClassifyError(err).Failure
			if failure.Details["Stage"] != tc.stage || err.Error() != tc.reason {
				t.Fatalf("stage=%v error=%q, want %s with %q", failure.Details["Stage"], err, tc.stage, tc.reason)
			}
			for chain := error(err); chain != nil; chain = errors.Unwrap(chain) {
				if strings.Contains(chain.Error(), tc.value) {
					t.Fatalf("config value leaked: %q", chain.Error())
				}
			}
		})
	}
}
