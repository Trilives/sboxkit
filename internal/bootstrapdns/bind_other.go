//go:build !linux

package bootstrapdns

import (
	"errors"
	"syscall"
)

func bindToDevice(string) func(string, string, syscall.RawConn) error {
	return func(string, string, syscall.RawConn) error {
		return errors.New("binding a DNS socket to an interface is only supported on Linux")
	}
}
