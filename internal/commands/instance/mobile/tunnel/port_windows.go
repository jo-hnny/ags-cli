package tunnel

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func isAddressInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE) || errors.Is(err, syscall.EADDRINUSE)
}
