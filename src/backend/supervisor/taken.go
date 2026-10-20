package supervisor

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"syscall"

	"github.com/itdoginfo/podkop/src/backend/sockets"
)

// socketsRoot is where /proc is looked for; a test may point it elsewhere.
var socketsRoot = "/"

// takenBy names who holds the address an inbound of sing-box could not listen on, for an
// error of a listen that found the address in use: the process bound to that address, or
// to every address of the system on that port (dnsmasq without nonwildcard, say). It is
// empty for any other error.
func takenBy(err error) string {
	var operation *net.OpError
	if !errors.Is(err, syscall.EADDRINUSE) || !errors.As(err, &operation) || operation.Op != "listen" || operation.Addr == nil {
		return ""
	}
	at, parseErr := netip.ParseAddrPort(operation.Addr.String())
	if parseErr != nil {
		return ""
	}
	protocol := strings.TrimRight(operation.Net, "46")
	listeners, _ := sockets.List(socketsRoot)
	holding := sockets.Holding(listeners, protocol, at)
	if len(holding) == 0 {
		return at.String() + " (" + protocol + ") is taken, by a socket not found in /proc/net"
	}
	return at.String() + " (" + protocol + ") is taken: " + sockets.Describe(holding)
}
