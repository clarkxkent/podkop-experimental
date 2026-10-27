//go:build !linux

package subscription

import "syscall"

func markedControl(network string, address string, conn syscall.RawConn) error {
	return nil
}
