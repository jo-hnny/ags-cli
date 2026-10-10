package mobileadb

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/output"
)

func TestMissingExecutableContext(t *testing.T) {
	path := t.TempDir() + "/missing-adb"
	_, _, _, err := RunBuffered(path, "argument-must-not-appear")
	failure := output.ClassifyError(err)
	if !errors.Is(failure, os.ErrNotExist) || failure.Failure.Details["Program"] != path || failure.Failure.Details["Stage"] != "subprocess_start" {
		t.Fatalf("missing process context: %#v", failure)
	}
}

func TestNonzeroExitRemainsBusinessData(t *testing.T) {
	if os.Getenv("AGR_ADB_EXIT_HELPER") == "1" {
		os.Exit(17)
	}
	t.Setenv("AGR_ADB_EXIT_HELPER", "1")
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-test.run=^TestNonzeroExitRemainsBusinessData$"}
	_, _, code, err := RunBuffered(program, args...)
	if err != nil || code != 17 {
		t.Fatalf("buffered: code=%d err=%v", code, err)
	}
	code, err = RunStreaming(program, args, nil, io.Discard, io.Discard)
	if err != nil || code != 17 {
		t.Fatalf("streaming: code=%d err=%v", code, err)
	}
}
