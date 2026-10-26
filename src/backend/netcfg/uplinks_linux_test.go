package netcfg

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The uplinks are the interfaces that are up with a default route, IPv4 or IPv6, each
// device once: a WAN and its DHCPv6 on one device, a modem; not the LAN, a tunnel that
// routes only its subnet, an interface that is down.
func TestUplinksOf(t *testing.T) {
	const uplinkDump = `{"interface": [
	{"interface": "lan", "up": true, "proto": "static", "l3_device": "br-lan", "route": []},
	{"interface": "wan", "up": true, "proto": "dhcp", "l3_device": "eth1", "route": [{"target": "0.0.0.0", "mask": 0, "nexthop": "10.97.0.254"}]},
	{"interface": "wan6", "up": true, "proto": "dhcpv6", "l3_device": "eth1", "route": [{"target": "::", "mask": 0, "nexthop": "fe80::1"}]},
	{"interface": "modem", "up": true, "proto": "dhcp", "l3_device": "wwan0", "route": [{"target": "10.64.0.0", "mask": 24}, {"target": "0.0.0.0", "mask": 0, "nexthop": "10.64.0.1"}]},
	{"interface": "wg0", "up": true, "proto": "wireguard", "l3_device": "wg0", "route": [{"target": "10.8.0.0", "mask": 24}]},
	{"interface": "lte", "up": false, "proto": "qmi", "route": [{"target": "0.0.0.0", "mask": 0}]},
	{"interface": "isp6", "up": true, "proto": "static", "l3_device": "eth2", "route": [{"target": "::", "mask": 0}]}
]}`
	var parsed struct {
		Interfaces []netifdInterface `json:"interface"`
	}
	require.NoError(t, json.Unmarshal([]byte(uplinkDump), &parsed))
	require.Equal(t, []string{"eth1", "eth2", "wwan0"}, uplinksOf(parsed.Interfaces))
	require.Empty(t, uplinksOf(nil))
}

// The DNS servers of the uplinks, each once, in the order of netifd: not those of the LAN
// or of an interface that is down or routes only its subnet.
func TestUplinkDNSServersOf(t *testing.T) {
	const dump = `{"interface": [
	{"interface": "lan", "up": true, "l3_device": "br-lan", "route": [], "dns-server": ["192.168.1.1"]},
	{"interface": "wan", "up": true, "l3_device": "pppoe-wan", "route": [{"target": "0.0.0.0", "mask": 0}], "dns-server": ["1.1.1.1", "10.0.0.53"]},
	{"interface": "wan6", "up": true, "l3_device": "pppoe-wan", "route": [{"target": "::", "mask": 0}], "dns-server": ["2001:db8::53", "1.1.1.1"]},
	{"interface": "wg0", "up": true, "l3_device": "wg0", "route": [{"target": "10.8.0.0", "mask": 24}], "dns-server": ["10.8.0.1"]},
	{"interface": "lte", "up": false, "route": [{"target": "0.0.0.0", "mask": 0}], "dns-server": ["10.64.0.53"]}
]}`
	var parsed struct {
		Interfaces []netifdInterface `json:"interface"`
	}
	require.NoError(t, json.Unmarshal([]byte(dump), &parsed))
	require.Equal(t, []string{"1.1.1.1", "10.0.0.53", "2001:db8::53"}, uplinkDNSServersOf(parsed.Interfaces))
	require.Empty(t, uplinkDNSServersOf(nil))
}

func rules(t *testing.T) string {
	return ip(t, "-4", "rule", "list") + ip(t, "-6", "rule", "list")
}

// The rules of the uplinks made over netlink read back, by the ip program, as the ones
// "ip rule add priority 106 fwmark 0x200000/0x200000 oif <device> lookup main" makes; a
// device that is no uplink any more loses its rules, and Remove takes them all.
func TestUplinkRulesAsTheIPProgramMakesThem(t *testing.T) {
	needIP(t)
	if _, err := syncUplinkRules(nil); err != nil {
		t.Fatal(err)
	}
	clean := rules(t)
	for _, family := range []string{"-4", "-6"} {
		ip(t, family, "rule", "add", "priority", "106", "fwmark", "0x200000/0x200000", "oif", "upltest0", "lookup", "main")
	}
	want := rules(t)
	if want == clean {
		t.Fatal("the commands changed nothing")
	}
	for _, family := range []string{"-4", "-6"} {
		ip(t, family, "rule", "del", "priority", "106", "fwmark", "0x200000/0x200000", "oif", "upltest0", "lookup", "main")
	}
	added, err := syncUplinkRules([]string{"upltest0"})
	require.NoError(t, err)
	require.Equal(t, []string{"upltest0"}, added)
	require.Equal(t, want, rules(t))
	// what is there already is not doubled
	added, err = syncUplinkRules([]string{"upltest0"})
	require.NoError(t, err)
	require.Empty(t, added)
	require.Equal(t, want, rules(t))
	// another uplink: the rules of the one that is gone are removed
	added, err = syncUplinkRules([]string{"upltest1"})
	require.NoError(t, err)
	require.Equal(t, []string{"upltest1"}, added)
	now := rules(t)
	require.NotContains(t, now, "upltest0")
	require.Equal(t, 2, strings.Count(now, "oif upltest1"))
	require.NoError(t, Remove())
	require.Equal(t, clean, rules(t))
}

// A router that routes by marks as mwan3 does: the policy marks what the router sends for
// the first uplink, its table has the route through that one only, and the rule after it
// makes the packet unreachable. A socket of sing-box bound to the second uplink then has
// no route through its gateway (the kernel takes the destination for one on the link);
// with the rule of the uplinks it goes through the gateway of the second uplink.
func TestUplinkRuleRoutesABoundSocket(t *testing.T) {
	needIP(t)
	for index, device := range []string{"upltest0", "upltest1"} {
		_ = exec.Command("ip", "link", "del", device).Run()
		ip(t, "link", "add", device, "type", "dummy")
		defer exec.Command("ip", "link", "del", device).Run()
		subnet := []string{"10.97.0", "10.98.0"}[index]
		ip(t, "addr", "add", subnet+".2/24", "dev", device)
		ip(t, "link", "set", device, "up")
		ip(t, "route", "add", "default", "via", subnet+".254", "dev", device, "metric", []string{"10", "20"}[index])
		defer exec.Command("ip", "route", "del", "default", "via", subnet+".254").Run()
	}
	ip(t, "route", "add", "default", "via", "10.97.0.254", "dev", "upltest0", "table", "201")
	defer exec.Command("ip", "route", "flush", "table", "201").Run()
	ip(t, "rule", "add", "priority", "2001", "fwmark", "0x100/0x3f00", "lookup", "201")
	defer exec.Command("ip", "rule", "del", "priority", "2001").Run()
	ip(t, "rule", "add", "priority", "3001", "fwmark", "0x100/0x3f00", "unreachable")
	defer exec.Command("ip", "rule", "del", "priority", "3001").Run()
	// the mark of sing-box with the bits of the policy
	get := func() string {
		return ip(t, "route", "get", "203.0.113.1", "oif", "upltest1", "mark", "0x200100")
	}
	require.NotContains(t, get(), "via 10.98.0.254", "the second gateway without the rule")
	require.Contains(t, ip(t, "route", "get", "203.0.113.1", "mark", "0x200100"), "via 10.97.0.254 dev upltest0", "a socket that is not bound follows the policy")
	_, err := syncUplinkRules([]string{"upltest0", "upltest1"})
	require.NoError(t, err)
	defer Remove()
	require.Contains(t, get(), "via 10.98.0.254 dev upltest1")
	require.Contains(t, ip(t, "route", "get", "203.0.113.1", "oif", "upltest0", "mark", "0x200100"), "via 10.97.0.254 dev upltest0")
	require.Contains(t, ip(t, "route", "get", "203.0.113.1", "mark", "0x200100"), "via 10.97.0.254 dev upltest0", "a socket that is not bound still follows the policy")
	// a packet of another program bound to the device is left to the policy
	require.NotContains(t, ip(t, "route", "get", "203.0.113.1", "oif", "upltest1", "mark", "0x100"), "via 10.98.0.254")
}
