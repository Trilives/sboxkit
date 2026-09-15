//go:build linux

package bootstrapdns

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func bindToDevice(interfaceName string) func(string, string, syscall.RawConn) error {
	return func(_, _ string, raw syscall.RawConn) error {
		if err := validateInterfaceName(interfaceName); err != nil {
			return err
		}
		var socketErr error
		if err := raw.Control(func(fd uintptr) {
			socketErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, interfaceName)
		}); err != nil {
			return fmt.Errorf("access socket for SO_BINDTODEVICE: %w", err)
		}
		if socketErr != nil {
			return fmt.Errorf("SO_BINDTODEVICE %q: %w", interfaceName, socketErr)
		}
		return nil
	}
}
