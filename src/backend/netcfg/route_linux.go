package netcfg

import (
	"net/netip"

	"github.com/sagernet/netlink"
	"github.com/sagernet/netlink/nl"
	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/sys/unix"
)

// Route is where the kernel sends a packet.
type Route struct {
	// Local tells that the router keeps the packet (a route of the type local): what the
	// transparent proxy took reaches its socket only so.
	Local bool
	// Table is the routing table that decided.
	Table int
	// Device and Gateway are where a packet that is not local goes.
	Device  string
	Gateway netip.Addr
}

// MarkedRoute asks the kernel where it routes an IPv4 packet that came in on a device from
// a source to a destination with a mark, as it decides it for a real packet: every rule of
// the routing policy, the routes, and the checks of the source take part. It is what
// "ip route get <destination> from <source> iif <device> mark <mark>" asks (the ip of
// busybox has no "route get"); netlink.RouteGetWithOptions has no mark to give.
func MarkedRoute(device string, source netip.Addr, destination netip.Addr, mark uint32) (Route, error) {
	link, err := netlink.LinkByName(device)
	if err != nil {
		return Route{}, E.Cause(err, "find the device ", device)
	}
	request := nl.NewNetlinkRequest(unix.RTM_GETROUTE, unix.NLM_F_REQUEST)
	message := &nl.RtMsg{}
	message.Family = unix.AF_INET
	message.Dst_len = 32
	message.Src_len = 32
	message.Flags = unix.RTM_F_LOOKUP_TABLE
	request.AddData(message)
	request.AddData(nl.NewRtAttr(unix.RTA_DST, destination.AsSlice()))
	request.AddData(nl.NewRtAttr(unix.RTA_SRC, source.AsSlice()))
	request.AddData(nl.NewRtAttr(unix.RTA_IIF, nl.Uint32Attr(uint32(link.Attrs().Index))))
	request.AddData(nl.NewRtAttr(unix.RTA_MARK, nl.Uint32Attr(mark)))
	messages, err := request.Execute(unix.NETLINK_ROUTE, unix.RTM_NEWROUTE)
	if err != nil {
		return Route{}, E.Cause(err, "ask the kernel for the route")
	}
	if len(messages) == 0 {
		return Route{}, E.New("the kernel gave no route")
	}
	answer := nl.DeserializeRtMsg(messages[0])
	route := Route{Local: answer.Type == unix.RTN_LOCAL, Table: int(answer.Table)}
	attributes, err := nl.ParseRouteAttr(messages[0][answer.Len():])
	if err != nil {
		return Route{}, E.Cause(err, "read the route")
	}
	for _, attribute := range attributes {
		switch attribute.Attr.Type {
		case unix.RTA_TABLE:
			route.Table = int(nl.NativeEndian().Uint32(attribute.Value))
		case unix.RTA_OIF:
			if output, err := netlink.LinkByIndex(int(nl.NativeEndian().Uint32(attribute.Value))); err == nil {
				route.Device = output.Attrs().Name
			}
		case unix.RTA_GATEWAY:
			route.Gateway, _ = netip.AddrFromSlice(attribute.Value)
		}
	}
	return route, nil
}
