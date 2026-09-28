//go:build !windows

package tunnel

import (
	"errors"
	"syscall"
)

func isAddressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
