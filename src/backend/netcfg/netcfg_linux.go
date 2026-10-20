// Package netcfg keeps what the backend needs of the routing of the router: the rule and
// the route that hand the packets the firewall marked to the transparent proxy, and the
// address of the LAN. It talks netlink; the ip program is not run.
package netcfg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
	"github.com/podkop-net/podkop-experimental/src/backend/uci"
	"github.com/sagernet/netlink"
	E "github.com/sagernet/sing/common/exceptions"

	"golang.org/x/sys/unix"
)

const (
	tableID      = dataplane.RoutingTable
	tableName    = dataplane.RoutingTableName
	rulePriority = dataplane.RulePriority
	// markProxy is the mark of the firewall table (firewall)
	markProxy = dataplane.ProxyMark
	// netifdTimeout is how long the answer of netifd is waited for.
	netifdTimeout = 3 * time.Second
)

// TableNamesPath is where iproute2 keeps the names of the routing tables. The shell
// backend finds its rule and its route by the name, so the name is kept there.
var TableNamesPath = "/etc/iproute2/rt_tables"

// NetworkConfigPath is the UCI file of netifd.
var NetworkConfigPath = "/etc/config/network"

// Apply makes the route and the rule; what is there already is left as it is.
func Apply() error {
	nameTable()
	loopback, err := netlink.LinkByName("lo")
	if err != nil {
		return E.Cause(err, "find the loopback interface")
	}
	// every marked packet is for this host: the transparent proxy inbound takes it
	err = netlink.RouteReplace(&netlink.Route{
		LinkIndex: loopback.Attrs().Index,
		Dst:       &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
		Table:     tableID,
		Type:      unix.RTN_LOCAL,
		Scope:     netlink.SCOPE_HOST,
	})
	if err != nil {
		return E.Cause(err, "add the route of the table ", tableID)
	}
	exists, err := ruleExists()
	if err != nil {
		return err
	}
	if !exists {
		err = netlink.RuleAdd(markRule())
		if err != nil && !errors.Is(err, unix.EEXIST) {
			return E.Cause(err, "add the rule for marked packets")
		}
	}
	return nil
}

// Present tells whether the rule and the route are both there. netifd, when it is
// restarted, flushes the rules it does not own, and a loopback that goes down takes the
// route with it.
func Present() (bool, error) {
	exists, err := ruleExists()
	if err != nil || !exists {
		return false, err
	}
	routes, err := netlink.RouteListFiltered(unix.AF_INET, &netlink.Route{Table: tableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return false, E.Cause(err, "list the routes of the table ", tableID)
	}
	return len(routes) > 0, nil
}

// Remove deletes the rule and the routes of the table; what is not there is no error.
func Remove() error {
	for {
		exists, err := ruleExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}
		err = netlink.RuleDel(markRule())
		if err != nil && !errors.Is(err, unix.ENOENT) {
			return E.Cause(err, "delete the rule for marked packets")
		}
	}
	routes, err := netlink.RouteListFiltered(unix.AF_INET, &netlink.Route{Table: tableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return E.Cause(err, "list the routes of the table ", tableID)
	}
	for index := range routes {
		if routes[index].Dst == nil {
			// the list gives a default route without its destination, and a route
			// without one cannot be named for deletion
			routes[index].Dst = &net.IPNet{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}
		}
		err = netlink.RouteDel(&routes[index])
		if err != nil && !errors.Is(err, unix.ESRCH) {
			return E.Cause(err, "delete a route of the table ", tableID)
		}
	}
	return nil
}

func markRule() *netlink.Rule {
	rule := netlink.NewRule()
	rule.Family = unix.AF_INET
	rule.Priority = rulePriority
	rule.Table = tableID
	rule.Mark = markProxy
	rule.MarkSet = true
	rule.Mask = markProxy
	return rule
}

func ruleExists() (bool, error) {
	rules, err := netlink.RuleList(unix.AF_INET)
	if err != nil {
		return false, E.Cause(err, "list the routing rules")
	}
	for _, rule := range rules {
		if rule.Table == tableID && rule.Priority == rulePriority && rule.Mark == markProxy && uint32(rule.Mask) == markProxy {
			return true, nil
		}
	}
	return false, nil
}

// nameTable writes the name of the table where iproute2 looks for it, once. A system
// without that file gets none: the name is for people and for the shell backend.
func nameTable() {
	content, err := os.ReadFile(TableNamesPath)
	if err != nil {
		return
	}
	line := strconv.Itoa(tableID) + " " + tableName
	for _, existing := range strings.Split(string(content), "\n") {
		if strings.Join(strings.Fields(existing), " ") == line {
			return
		}
	}
	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		content = append(content, '\n')
	}
	_ = os.WriteFile(TableNamesPath, append(content, line+"\n"...), 0o644)
}

// LANAddress returns the IPv4 address of the interface "lan": the first one netifd reports
// for it, which is what the shell backend asks. The device of the interface is no
// substitute: it carries the addresses of every interface that names it, in the order
// they were set, so its first address can be another interface's. The device is asked
// only when netifd does not answer.
func LANAddress() (netip.Addr, error) {
	address, err := lanAddressOfNetifd()
	if err == nil {
		return address, nil
	}
	fromDevice, deviceErr := lanAddressOfDevice()
	if deviceErr == nil {
		return fromDevice, nil
	}
	return netip.Addr{}, E.Cause(deviceErr, "find the address of the LAN (netifd: ", err, ")")
}

func lanAddressOfDevice() (netip.Addr, error) {
	content, err := os.ReadFile(NetworkConfigPath)
	if err != nil {
		return netip.Addr{}, err
	}
	parsed, err := uci.Parse(content)
	if err != nil {
		return netip.Addr{}, err
	}
	section := parsed.Section("lan")
	if section == nil || section.Type != "interface" {
		return netip.Addr{}, E.New("the network settings have no interface lan")
	}
	device := section.Get("device")
	if device == "" {
		// the option of OpenWrt before 21.02
		device = section.Get("ifname")
	}
	if device == "" || strings.ContainsAny(device, " @") {
		return netip.Addr{}, E.New("the interface lan names no single device")
	}
	link, err := netlink.LinkByName(device)
	if err != nil {
		return netip.Addr{}, E.Cause(err, "find the device ", device)
	}
	addresses, err := netlink.AddrList(link, unix.AF_INET)
	if err != nil {
		return netip.Addr{}, E.Cause(err, "list the addresses of ", device)
	}
	// the address the settings give the interface, when the device has it; else its first
	configured := section.List("ipaddr")
	var first netip.Addr
	for _, address := range addresses {
		parsed, isAddress := netip.AddrFromSlice(address.IP)
		if !isAddress {
			continue
		}
		parsed = parsed.Unmap()
		for _, item := range configured {
			item, _, _ = strings.Cut(item, "/")
			if item == parsed.String() {
				return parsed, nil
			}
		}
		if !first.IsValid() {
			first = parsed
		}
	}
	if first.IsValid() {
		return first, nil
	}
	return netip.Addr{}, E.New("the device ", device, " has no IPv4 address")
}

func lanAddressOfNetifd() (netip.Addr, error) {
	// netifd answers at once or is not there; a bus that hangs must not hold the caller
	ctx, cancel := context.WithTimeout(context.Background(), netifdTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ubus", "-t", "2", "call", "network.interface.lan", "status").Output()
	if err != nil {
		return netip.Addr{}, err
	}
	var status struct {
		Addresses []struct {
			Address string `json:"address"`
		} `json:"ipv4-address"`
	}
	err = json.Unmarshal(output, &status)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(status.Addresses) == 0 {
		return netip.Addr{}, E.New("netifd reports no IPv4 address of lan")
	}
	return netip.ParseAddr(status.Addresses[0].Address)
}

// BridgeSysctlDir holds the switches of br_netfilter. It is there whenever br_netfilter is
// in the kernel, as a module or built in: lsmod lists only the module, so the shell
// backend's look at lsmod misses a kernel that has it built in.
var BridgeSysctlDir = "/proc/sys/net/bridge"

// bridgeSwitches are the switches that pass bridged packets through the IP firewall hooks.
var bridgeSwitches = []string{"bridge-nf-call-iptables", "bridge-nf-call-ip6tables"}

// AllowBridgedTraffic turns off the passing of bridged packets through the IP firewall
// hooks where the kernel has it on, and returns the names of the switches it turned off.
// With it on, a packet that came in on a port of the LAN bridge and was taken by the
// transparent proxy is lost after the routing (measured on OpenWrt 24.10, kernel 6.6:
// neither the device check nor any proxied connection passes), and packets between the
// ports of the bridge meet the rules of the backend. The shell backend turns it off too.
func AllowBridgedTraffic() []string {
	var changed []string
	for _, name := range bridgeSwitches {
		path := BridgeSysctlDir + "/" + name
		content, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(content)) == "1" && os.WriteFile(path, []byte("0\n"), 0o644) == nil {
			changed = append(changed, "net.bridge."+name)
		}
	}
	return changed
}

// BridgedTraffic tells whether br_netfilter is in the kernel and, when it is, the value of
// each switch that passes bridged packets through the IP firewall hooks, by name.
func BridgedTraffic() (present bool, values map[string]string) {
	values = make(map[string]string)
	for _, name := range bridgeSwitches {
		content, err := os.ReadFile(BridgeSysctlDir + "/" + name)
		if err != nil {
			continue
		}
		present = true
		values["net.bridge."+name] = strings.TrimSpace(string(content))
	}
	return present, values
}
