package netcfg

import (
	"errors"
	"slices"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
	"github.com/sagernet/netlink"
	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/sys/unix"
)

const (
	uplinkPriority = dataplane.UplinkRulePriority
	// markOutbound is the mark of what sing-box sends (route.default_mark)
	markOutbound = dataplane.OutboundMark
)

// RouteUplinks keeps a rule for the device of every uplink of the router: what sing-box
// sends (dataplane.OutboundMark) from a socket bound to that device is routed by the main
// table, which has the default route of every uplink. Without it a router that routes by
// marks of its own loses such a packet: mwan3 marks what the router sends by its policy,
// the table of the uplink the policy chose has no route through another device, and the
// rule of mwan3 after it makes the packet unreachable. sing-box binds a socket to an
// uplink for the DNS server of that uplink's provider (its DNS fallback), for the output
// interface of the settings and for a section bound to a WAN. A socket that is not bound
// is left to the routing of the router. The rules of devices that are no uplinks now
// are removed. It tells the devices it added a rule for.
func RouteUplinks() ([]string, error) {
	interfaces, err := dumpInterfaces()
	if err != nil {
		return nil, err
	}
	return syncUplinkRules(uplinksOf(interfaces))
}

// uplinksOf are the devices of the interfaces that are up with a default route.
func uplinksOf(interfaces []netifdInterface) []string {
	var devices []string
	for _, networkInterface := range interfaces {
		if !networkInterface.Up || networkInterface.L3Device == "" || slices.Contains(devices, networkInterface.L3Device) {
			continue
		}
		for _, route := range networkInterface.Routes {
			if route.Mask == 0 && (route.Target == "0.0.0.0" || route.Target == "::") {
				devices = append(devices, networkInterface.L3Device)
				break
			}
		}
	}
	slices.Sort(devices)
	return devices
}

// syncUplinkRules makes the rules of the devices, IPv4 and IPv6, and removes the other
// rules of uplinks. It tells the devices it added an IPv4 rule for.
func syncUplinkRules(devices []string) ([]string, error) {
	var added []string
	for _, family := range []int{unix.AF_INET, unix.AF_INET6} {
		rules, err := netlink.RuleList(family)
		if err != nil {
			if family == unix.AF_INET6 && errors.Is(err, unix.EAFNOSUPPORT) {
				continue // a kernel without IPv6
			}
			return added, E.Cause(err, "list the routing rules")
		}
		present := make(map[string]bool)
		for _, rule := range rules {
			if rule.Priority != uplinkPriority || rule.Mark != markOutbound || uint32(rule.Mask) != markOutbound || rule.Table != unix.RT_TABLE_MAIN {
				continue
			}
			if slices.Contains(devices, rule.OifName) {
				present[rule.OifName] = true
				continue
			}
			err = netlink.RuleDel(uplinkRule(family, rule.OifName))
			if err != nil && !errors.Is(err, unix.ENOENT) {
				return added, E.Cause(err, "delete the rule of the device ", rule.OifName)
			}
		}
		for _, device := range devices {
			if present[device] {
				continue
			}
			err = netlink.RuleAdd(uplinkRule(family, device))
			if err != nil && !errors.Is(err, unix.EEXIST) {
				return added, E.Cause(err, "add the rule of the device ", device)
			}
			if family == unix.AF_INET {
				added = append(added, device)
			}
		}
	}
	return added, nil
}

func uplinkRule(family int, device string) *netlink.Rule {
	rule := netlink.NewRule()
	rule.Family = family
	rule.Priority = uplinkPriority
	rule.Table = unix.RT_TABLE_MAIN
	rule.Mark = markOutbound
	rule.MarkSet = true
	rule.Mask = markOutbound
	rule.OifName = device
	return rule
}
