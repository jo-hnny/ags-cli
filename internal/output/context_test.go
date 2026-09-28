package output

import (
	"errors"
	"os"
	"testing"
)

func TestBoundaryContextPreservesClassification(t *testing.T) {
	original := NewUsageError("BAD_INPUT", "bad input", "check input")
	original.Failure.Details = map[string]any{"Stage": "parse"}
	cause := WithContext(original, map[string]any{"Stage": "outer", "Path": "input.json"})
	got := ClassifyError(cause)
	if got.Failure.Code != "BAD_INPUT" || got.ExitCode != ExitUsage || got.Failure.Details["Stage"] != "parse" || got.Failure.Details["Path"] != "input.json" || !errors.Is(got, original) {
		t.Fatalf("classification/cause lost: %#v", got)
	}
	if _, ok := original.Failure.Details["Path"]; ok {
		t.Fatal("mutated original failure")
	}
}

func TestLocalFileContext(t *testing.T) {
	path := t.TempDir() + "/missing"
	_, err := os.ReadFile(path)
	got := ClassifyError(err)
	if !errors.Is(got, os.ErrNotExist) || got.Failure.Details["Path"] != path || got.Failure.Details["Operation"] != "open" {
		t.Fatalf("missing file context: %#v", got.Failure)
	}
}
