package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func TestTextFailureShowsObservedBoundary(t *testing.T) {
	var text bytes.Buffer
	writeFailureText(&text, &output.Failure{Code: "INTERNAL_ERROR", Message: "internal error", Details: map[string]any{"Stage": "subprocess_start", "Program": "adb", "Path": "missing-adb", "Operation": "open"}})
	for _, want := range []string{"Stage: subprocess_start", "Program: adb", "Path: missing-adb", "Operation: open"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("missing %s: %s", want, &text)
		}
	}
}
