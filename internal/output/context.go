package output

import (
	"errors"
	"maps"
	"os"
)

// WithContext annotates an observed boundary without classifying its cause.
// Callers must supply metadata only, never arguments, credentials or payloads.
func WithContext(err error, details map[string]any) error {
	if err == nil {
		return nil
	}
	return &contextError{error: err, details: maps.Clone(details)}
}

type contextError struct {
	error
	details map[string]any
}

func (e *contextError) Unwrap() error { return e.error }

func withErrorContext(f *Failure, err error) *Failure {
	details := map[string]any{}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		details["Operation"] = pathErr.Op
		details["Path"] = pathErr.Path
		details["Stage"] = "local_file"
	}
	// Inner boundaries are more specific than their callers. Existing classified
	// details remain authoritative (for example a child's ready-stage failure).
	for current := err; current != nil; current = errors.Unwrap(current) {
		if contextual, ok := current.(*contextError); ok {
			maps.Copy(details, contextual.details)
		}
	}
	if len(details) == 0 {
		return f
	}
	maps.Copy(details, f.Details)
	copy := *f
	copy.Details = details
	return &copy
}
