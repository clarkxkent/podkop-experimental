//go:build !linux

package clock

import "syscall"

func markedControl(network string, address string, conn syscall.RawConn) error {
	return nil
}
