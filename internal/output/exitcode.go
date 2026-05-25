package output

import (
	"context"
	"errors"
	"net"
)

// Exit codes per agr.v1 CLI contract.
const (
	ExitSuccess          = 0
	ExitGenericError     = 1
	ExitUsage            = 2
	ExitAuthOrPermission = 4
	ExitRemoteExecFailed = 255

	// Detailed failure kinds still collapse to the coarse shell exit code above.
	ExitNotFound       = ExitGenericError
	ExitConflict       = ExitGenericError
	ExitRateLimit      = ExitGenericError
	ExitTimeout        = ExitGenericError
	ExitNetwork        = ExitGenericError
	ExitPartialSuccess = ExitGenericError
)

// Failure kinds.
const (
	KindSuccess          = "success"
	KindGenericError     = "error"
	KindUsage            = "usage"
	KindNotFound         = "not_found"
	KindAuthOrPermission = "auth"
	KindConflict         = "conflict"
	KindRateLimit        = "rate_limit"
	KindTimeout          = "timeout"
	KindNetwork          = "network"
	KindPartialSuccess   = "partial"
	KindRemoteExecFailed = "remote_execution_failed"
)

// ExitCodeForKind maps a detailed failure kind to the coarse shell exit code.
func ExitCodeForKind(kind string) int {
	switch kind {
	case KindSuccess:
		return ExitSuccess
	case KindUsage:
		return ExitUsage
	case KindAuthOrPermission:
		return ExitAuthOrPermission
	case KindRemoteExecFailed:
		return ExitRemoteExecFailed
	default:
		return ExitGenericError
	}
}

// CLIError is a structured error that carries failure info and exit code.
type CLIError struct {
	Failure  *Failure
	ExitCode int
}

func (e *CLIError) Error() string { return e.Failure.Message }

// NewCLIError creates a CLIError from a Failure.
func NewCLIError(f *Failure) *CLIError {
	return &CLIError{Failure: f, ExitCode: ExitCodeForKind(f.Kind)}
}

// NewUsageError creates a usage error (exit 2).
func NewUsageError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{Code: code, Kind: KindUsage, Message: message, Hint: hint})
}

// NewNotFoundError creates a not-found error (exit 1, Kind=not_found).
func NewNotFoundError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{Code: code, Kind: KindNotFound, Message: message, Hint: hint})
}

// NewAuthError creates an auth/permission error (exit 4).
func NewAuthError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{Code: code, Kind: KindAuthOrPermission, Message: message, Hint: hint})
}

// NewConflictError creates a conflict error (exit 1, Kind=conflict).
func NewConflictError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{Code: code, Kind: KindConflict, Message: message, Hint: hint})
}

// NewRemoteExecutionError creates a remote execution failure (exit 255).
func NewRemoteExecutionError(code, message, hint string) *CLIError {
	return NewCLIError(&Failure{Code: code, Kind: KindRemoteExecFailed, Message: message, Hint: hint})
}

// ClassifyError translates a Go error into a CLIError.
func ClassifyError(err error) *CLIError {
	if err == nil {
		return nil
	}

	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}

	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) {
		return NewCLIError(&Failure{Code: "TIMEOUT", Kind: KindTimeout, Message: msg, Hint: "Safe to retry. Run the same command again after a brief wait.", Retryable: true})
	}
	if errors.Is(err, context.Canceled) {
		return NewCLIError(&Failure{Code: "CANCELED", Kind: KindGenericError, Message: msg, Hint: "Run the command again if the cancellation was unexpected."})
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return NewCLIError(&Failure{Code: "DNS_ERROR", Kind: KindNetwork, Message: msg, Hint: "Check network connectivity and DNS settings, then retry.", Retryable: true})
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return NewCLIError(&Failure{Code: "NETWORK_ERROR", Kind: KindNetwork, Message: msg, Hint: "Check network connectivity, then retry.", Retryable: true})
	}

	return NewCLIError(&Failure{Code: "INTERNAL_ERROR", Kind: KindGenericError, Message: "internal error", Hint: "Run 'agr doctor'. If the problem persists, rerun with --debug and share stderr diagnostics."})
}
