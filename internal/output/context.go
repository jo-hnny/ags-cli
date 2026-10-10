package output

import (
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
	details, pathErr := errorContext(err)
	if pathErr != nil {
		defaults := map[string]any{"Operation": pathErr.Op, "Path": pathErr.Path, "Stage": "local_file"}
		maps.Copy(defaults, details)
		details = defaults
	}
	if len(details) == 0 {
		return f
	}
	maps.Copy(details, f.Details)
	copy := *f
	copy.Details = details
	return &copy
}

// Inner boundaries override outer metadata; classified Failure details remain
// authoritative. At a join, select the first branch containing metadata in
// depth-first, left-to-right order. Never mix sibling targets/stages: they may
// describe different operations. All branches remain accessible via Is/As.
func errorContext(err error) (map[string]any, *os.PathError) {
	var pathErr *os.PathError
	details := map[string]any{}
	switch e := err.(type) {
	case *os.PathError:
		pathErr = e
	case *contextError:
		maps.Copy(details, e.details)
	}
	switch e := err.(type) {
	case interface{ Unwrap() error }:
		inner, path := errorContext(e.Unwrap())
		maps.Copy(details, inner)
		if path != nil {
			pathErr = path
		}
	case interface{ Unwrap() []error }:
		for _, child := range e.Unwrap() {
			if branch, path := errorContext(child); len(branch) > 0 || path != nil {
				pathErr = path
				maps.Copy(details, branch)
				break
			}
		}
	}
	return details, pathErr
}
