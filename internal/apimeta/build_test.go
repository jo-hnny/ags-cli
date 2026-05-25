package apimeta_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/apimeta"
)

func loadInputs(t *testing.T) (*apimeta.Spec, *apimeta.Mapping) {
	t.Helper()
	root := filepath.Join("..", "..")
	spec, err := apimeta.LoadSpec(filepath.Join(root, "api", "ags", "v20250920", "api.json"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	mapping, err := apimeta.LoadMapping(filepath.Join(root, "api", "ags", "v20250920", "mapping.yaml"))
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	return spec, mapping
}

func TestBuildCatalog_DeterministicOutputs(t *testing.T) {
	spec, mapping := loadInputs(t)
	a, err := json.Marshal(apimeta.BuildCatalog(spec, mapping))
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	b, err := json.Marshal(apimeta.BuildCatalog(spec, mapping))
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("generated metadata model differs between runs")
	}
}

func TestGenerate_FlagsContainBaseLongFlags(t *testing.T) {
	spec, mapping := loadInputs(t)
	flags := apimeta.BuildFlags(spec, mapping)
	have := map[string]bool{}
	for _, f := range flags.Flags {
		have[f.Action+"|"+f.Field+"|"+f.Flag] = true
	}
	expectations := []struct{ action, field, flag string }{
		{"StartSandboxInstance", "ToolName", "tool-name"},
		{"StartSandboxInstance", "ToolId", "tool-id"},
		{"StartSandboxInstance", "MountOptions", "mount-options"},
		{"CreateSandboxTool", "StorageMounts", "storage-mounts"},
		{"CreateSandboxTool", "NetworkConfiguration", "network-configuration"},
		{"CreatePreCacheImageTask", "Image", "image"},
	}
	for _, e := range expectations {
		key := e.action + "|" + e.field + "|" + e.flag
		if !have[key] {
			t.Errorf("missing generated flag: %s -> --%s", e.field, e.flag)
		}
	}
}

func TestGenerate_CoverageMatchesMappingStats(t *testing.T) {
	spec, mapping := loadInputs(t)
	rep := apimeta.BuildCoverage(spec, mapping)
	if rep.TotalActions != len(spec.Actions) {
		t.Errorf("TotalActions=%d", rep.TotalActions)
	}
	if rep.MappedActions+rep.RawOnlyActions+rep.DeferredActions != rep.TotalActions {
		t.Errorf("status counts do not add up: %+v", rep)
	}
	if len(rep.UnmappedActions) > 0 {
		t.Errorf("expected no unmapped actions, got %v", rep.UnmappedActions)
	}
}

func TestBuildCatalog_IncludesRawOnlyReason(t *testing.T) {
	spec, mapping := loadInputs(t)
	data, err := json.Marshal(apimeta.BuildCatalog(spec, mapping))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cat map[string]any
	if err := json.Unmarshal(data, &cat); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	actions, _ := cat["Actions"].([]any)
	foundRaw := false
	for _, a := range actions {
		m := a.(map[string]any)
		if m["Status"] == "raw_only" {
			foundRaw = true
			if r, _ := m["Reason"].(string); r == "" {
				t.Errorf("raw_only action missing Reason: %v", m)
			}
		}
	}
	if !foundRaw {
		t.Fatalf("expected at least one raw_only action")
	}
}
