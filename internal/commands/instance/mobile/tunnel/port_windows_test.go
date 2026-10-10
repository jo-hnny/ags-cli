package tunnel

import (
	"bytes"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/TencentCloudAgentRuntime/ags-cli/internal/client"
	"golang.org/x/sys/windows"
)

func TestWindowsPortConflictClassification(t *testing.T) {
	cause := &net.OpError{Op: "listen", Net: "tcp", Err: &os.SyscallError{Syscall: "bind", Err: windows.WSAEADDRINUSE}}
	_, err := buildRuntime(t, &bytes.Buffer{}, &fakeTunnel{startErr: cause}, nil).Handler.Run(t.Context(), request(false, 5555))
	got := client.ClassifyError(err)
	if got.Failure.Code != "PORT_IN_USE" || got.ExitCode != 2 || !errors.Is(err, cause) {
		t.Fatalf("classification=%#v failure=%#v", got, got.Failure)
	}
	if isAddressInUse(windows.WSAEACCES) {
		t.Fatal("permission error classified as port conflict")
	}
}
