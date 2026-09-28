package output

import (
	"errors"
	"fmt"
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

func TestJoinedBoundaryContext(t *testing.T) {
	first := errors.New("first")
	second := errors.New("second")
	left := WithContext(first, map[string]any{"Stage": "left", "Endpoint": "https://left"})
	right := WithContext(second, map[string]any{"Stage": "right", "Endpoint": "https://right", "Program": "unrelated"})
	for _, joined := range []error{errors.Join(left, right), fmt.Errorf("two failures: %w and %w", left, right)} {
		got := ClassifyError(WithContext(joined, map[string]any{"Stage": "outer", "Operation": "outer"}))
		if got.Failure.Details["Stage"] != "left" || got.Failure.Details["Endpoint"] != "https://left" || got.Failure.Details["Operation"] != "outer" {
			t.Fatalf("wrong branch: %#v", got.Failure.Details)
		}
		if _, ok := got.Failure.Details["Program"]; ok {
			t.Fatal("merged unrelated sibling metadata")
		}
		if !errors.Is(got, first) || !errors.Is(got, second) {
			t.Fatal("lost joined cause")
		}
	}
	got := ClassifyError(errors.Join(first, fmt.Errorf("nested: %w", right)))
	if got.Failure.Details["Stage"] != "right" {
		t.Fatalf("skipped later annotated branch: %#v", got.Failure.Details)
	}
	path := &os.PathError{Op: "open", Path: "unrelated-file", Err: os.ErrNotExist}
	got = ClassifyError(errors.Join(left, path))
	if _, ok := got.Failure.Details["Path"]; ok {
		t.Fatal("merged unrelated filesystem cause")
	}
	got = ClassifyError(WithContext(errors.Join(first, path), map[string]any{"Stage": "subprocess_start"}))
	if got.Failure.Details["Path"] != "unrelated-file" || got.Failure.Details["Stage"] != "subprocess_start" {
		t.Fatalf("lost file fallback: %#v", got.Failure.Details)
	}
}
