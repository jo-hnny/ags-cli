// Package mobileadb wraps the local adb executable used by mobile sandbox
// commands. It validates the executable path and normalizes process exit codes.
package mobileadb

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Require resolves the adb executable, preferring ADB_PATH when set. ADB_PATH is
// validated more strictly than PATH lookup because it is an explicit executable
// override supplied by the user or environment.
func Require() (string, error) {
	if p := os.Getenv("ADB_PATH"); p != "" {
		info, err := os.Lstat(p)
		if err != nil {
			return "", fmt.Errorf("ADB_PATH=%q not accessible: %w", p, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("ADB_PATH=%q is a symlink (not allowed for security)", p)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("ADB_PATH=%q is not a regular file", p)
		}
		if runtime.GOOS == "windows" {
			if !strings.HasSuffix(strings.ToLower(p), ".exe") {
				return "", fmt.Errorf("ADB_PATH=%q does not have .exe extension (required on Windows)", p)
			}
		} else if info.Mode()&0111 == 0 {
			return "", fmt.Errorf("ADB_PATH=%q is not executable", p)
		}
		return p, nil
	}

	path, err := exec.LookPath("adb")
	if err != nil {
		return "", fmt.Errorf("adb not found in PATH; install Android SDK Platform-Tools or set ADB_PATH")
	}
	return path, nil
}

// Run executes adb with inherited stdout/stderr for commands that should behave
// like direct adb passthrough.
func Run(adbPath string, args ...string) error {
	cmd := exec.Command(adbPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunStreaming executes adb with caller-provided streams and returns adb's exit
// code as data instead of turning non-zero exits into Go errors.
func RunStreaming(adbPath string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(adbPath, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

// RunBuffered executes adb and captures stdout/stderr while preserving adb's
// process exit code for JSON output.
func RunBuffered(adbPath string, args ...string) (stdout string, stderr string, exitCode int, err error) {
	cmd := exec.Command(adbPath, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if runErr := cmd.Run(); runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return "", "", 0, runErr
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

// ConnectWithRetry retries "adb connect" until adb reports a connected state or
// maxRetries is exhausted. Mobile tunnels may need a short warm-up after the
// tunnel daemon announces readiness.
func ConnectWithRetry(adbPath, addr string, maxRetries int, out io.Writer) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		raw, err := exec.Command(adbPath, "connect", addr).CombinedOutput()
		if err != nil {
			lastErr = err
			continue
		}
		outStr := strings.TrimSpace(string(raw))
		fmt.Fprintln(out, outStr)
		if strings.Contains(outStr, "connected") {
			time.Sleep(2 * time.Second)
			return nil
		}
		lastErr = fmt.Errorf("adb connect: %s", outStr)
	}
	return fmt.Errorf("adb connect failed after %d attempts: %w", maxRetries, lastErr)
}
