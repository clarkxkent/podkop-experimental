package subscription

import (
	"syscall"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
)

// markedControl gives a socket the mark of what sing-box sends (dataplane.OutboundMark): the
// firewall keeps it out of the proxy whatever its address is. Without the right to set the
// mark the socket goes without it.
func markedControl(network string, address string, conn syscall.RawConn) error {
	return conn.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_MARK, int(dataplane.OutboundMark))
	})
}
