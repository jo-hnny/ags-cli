package connect

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"time"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/cli"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/dataplane/adbtunnel"
	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

const readinessTimeout = 30 * time.Second
const processExitTimeout = 2 * time.Second

type startupError struct {
	error
	logPath string
}

func (e *startupError) Unwrap() error             { return e.error }
func (e *startupError) DiagnosticLogPath() string { return e.logPath }

func startTunnelDaemon(ctx context.Context, instanceID string, port int) (TunnelReady, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return TunnelReady{}, daemonFailure("TUNNEL_START_FAILED", "start", "failed to locate tunnel executable", err)
	}
	cmd := exec.Command(selfPath, tunnelArguments(instanceID, port)...)
	cmd.Env = tunnelEnv()
	logPath, logFile := openTunnelLog(instanceID)
	return startTunnelProcess(ctx, cmd, logPath, logFile, readinessTimeout)
}

func tunnelArguments(instanceID string, port int) []string {
	args := []string{"instance", "mobile", "tunnel", instanceID, "--daemon", fmt.Sprintf("--port=%d", port)}
	if cli.DebugEnabled() {
		args = append(args, "--debug")
	}
	for _, flag := range []struct{ name, value string }{
		{"--config", cli.CfgFile()}, {"--region", cli.RegionFlag()}, {"--domain", cli.DomainFlag()},
		{"--cloud-endpoint", cli.CloudEndpointFlag()},
	} {
		if flag.value != "" {
			args = append(args, flag.name, flag.value)
		}
	}
	return args
}

// The pipe is owned here rather than by Cmd.Wait: a fast child must not lose
// its final readiness message when Wait closes Cmd.StdoutPipe.
func startTunnelProcess(ctx context.Context, cmd *exec.Cmd, logPath string, logFile *os.File, timeout time.Duration) (ready TunnelReady, resultErr error) {
	defer func() {
		if resultErr == nil {
			return
		}
		classified := cli.ClassifyCLIError(output.WithContext(resultErr, map[string]any{"Program": cmd.Path}))
		if logFile != nil {
			failure := *classified.Failure
			failure.Details = maps.Clone(failure.Details)
			if failure.Details == nil {
				failure.Details = map[string]any{}
			}
			failure.Details["LogPath"] = logPath
			copy := *classified
			copy.Failure = &failure
			resultErr = &startupError{error: &copy, logPath: logPath}
		} else {
			resultErr = classified
		}
	}()
	if logFile == nil {
		logPath = ""
	} else {
		cmd.Stderr = logFile
	}
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		_ = closeIfOpen(logFile)
		return ready, daemonFailure("TUNNEL_START_FAILED", "start", "failed to create tunnel output pipe", err)
	}
	defer func() { _ = stdout.Close() }() // unblocks the reader on all outcomes
	cmd.Stdout = childStdout
	if err := ctx.Err(); err != nil {
		_ = childStdout.Close()
		_ = closeIfOpen(logFile)
		return ready, daemonContextFailure(err)
	}
	if err := cmd.Start(); err != nil {
		_ = childStdout.Close()
		_ = closeIfOpen(logFile)
		return ready, daemonFailure("TUNNEL_START_FAILED", "start", "failed to start tunnel process", err)
	}
	_ = childStdout.Close()
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = closeIfOpen(logFile)
		exited <- err
	}()
	// Errors reported by the child are followed by its root debug output. Allow
	// that short exit to finish before killing, then bound cleanup regardless.
	finish := func(grace bool) error {
		if grace {
			select {
			case err := <-exited:
				return err
			case <-time.After(processExitTimeout):
			}
		}
		_ = cmd.Process.Kill()
		select {
		case err := <-exited:
			return err
		case <-time.After(processExitTimeout):
			return errors.New("tunnel process did not exit after termination")
		}
	}
	msg, err := readTunnelReady(ctx, stdout, timeout)
	if err != nil {
		exitErr := finish(errors.Is(err, io.EOF))
		if errors.Is(err, io.EOF) {
			failure := daemonFailure("TUNNEL_EXITED", "exit", "tunnel process exited without a ready message", exitErr)
			var exit *exec.ExitError
			if errors.As(exitErr, &exit) {
				failure.Failure.Details["ExitCode"] = exit.ExitCode()
			} else if exitErr == nil {
				failure.Failure.Details["ExitCode"] = 0
			}
			return ready, failure
		}
		return ready, err
	}
	if validChildFailure(msg) {
		_ = finish(true)
		return ready, &output.CLIError{Failure: msg.Failure, ExitCode: msg.ExitCode}
	}
	if msg.Status != "ready" || msg.Failure != nil || msg.ExitCode != 0 || msg.Port <= 0 || msg.Port > 65535 || msg.PID != cmd.Process.Pid {
		_ = finish(false)
		return ready, daemonFailure("TUNNEL_PROTOCOL_ERROR", "readiness_protocol", "invalid tunnel ready message", nil)
	}
	return TunnelReady{Port: msg.Port, PID: msg.PID, ExePath: cmd.Path, LogPath: logPath}, nil
}

func validChildFailure(msg adbtunnel.ReadyMessage) bool {
	if msg.Status != "error" || msg.Failure == nil || msg.Failure.Code == "" || msg.Failure.Message == "" || msg.ExitCode <= 0 || msg.ExitCode > 255 {
		return false
	}
	switch msg.Failure.Kind {
	case output.KindGenericError, output.KindUsage, output.KindNotFound, output.KindAuthOrPermission, output.KindConflict, output.KindRateLimit, output.KindTimeout, output.KindNetwork, output.KindPartialSuccess, output.KindRemoteExecFailed:
		return true
	default:
		return false
	}
}

func daemonContextFailure(cause error) *output.CLIError {
	failure := output.ClassifyError(cause)
	failure.Failure.Details = map[string]any{"Stage": "readiness_wait"}
	return failure
}

func daemonFailure(code, stage, message string, cause error) *output.CLIError {
	kind := output.KindGenericError
	if code == "TUNNEL_READY_TIMEOUT" {
		kind = output.KindTimeout
	}
	return output.NewCLIError(&output.Failure{Code: code, Kind: kind, Message: message, Hint: "Inspect Failure.Details and use --debug for tunnel diagnostics.", Details: map[string]any{"Stage": stage}}).WithCause(cause)
}

func readTunnelReady(ctx context.Context, stdout io.Reader, timeout time.Duration) (adbtunnel.ReadyMessage, error) {
	type result struct {
		msg adbtunnel.ReadyMessage
		err error
	}
	results := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		if !scanner.Scan() {
			err := scanner.Err()
			if err == nil {
				err = io.EOF
			}
			results <- result{err: err}
			return
		}
		var msg adbtunnel.ReadyMessage
		err := json.Unmarshal(scanner.Bytes(), &msg)
		results <- result{msg: msg, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-results:
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			return result.msg, daemonFailure("TUNNEL_PROTOCOL_ERROR", "readiness_protocol", "failed to read tunnel ready message", result.err)
		}
		return result.msg, result.err
	case <-ctx.Done():
		return adbtunnel.ReadyMessage{}, daemonContextFailure(ctx.Err())
	case <-timer.C:
		err := daemonFailure("TUNNEL_READY_TIMEOUT", "readiness_wait", "tunnel did not become ready before the deadline", context.DeadlineExceeded)
		err.Failure.Details["TimeoutMs"] = timeout.Milliseconds()
		return adbtunnel.ReadyMessage{}, err
	}
}
