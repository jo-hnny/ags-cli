package output

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassificationPreservesCause(t *testing.T) {
	for _, cause := range []error{errors.New("unclassified failure"), context.DeadlineExceeded, context.Canceled} {
		wrapped := fmt.Errorf("connect upstream: %w", cause)
		classified := ClassifyError(wrapped)
		if !errors.Is(classified, cause) {
			t.Errorf("classification lost %v", cause)
		}
	}
}

func TestWrappedClassificationPreservesContextWithoutCycles(t *testing.T) {
	cause := context.DeadlineExceeded
	original := NewConflictError("CUSTOM", "public message", "hint").WithCause(cause)
	original.ExitCode = 7
	original.Failure.Details = map[string]any{"RequestId": "req-1"}
	wrapped := fmt.Errorf("opening connection: %w", original)
	classified := ClassifyError(wrapped)
	if classified.ExitCode != 7 || classified.Failure != original.Failure || classified.Unwrap() != wrapped {
		t.Fatalf("classification/context changed: %#v", classified)
	}
	if !errors.Is(classified, cause) || !errors.Is(classified, original) || original.Unwrap() != cause {
		t.Fatal("cause chain lost or original mutated")
	}
	var again *CLIError
	if !errors.As(classified, &again) || again != classified {
		t.Fatal("classified error must remain the first CLIError")
	}
}
