package netcfg

import (
	"encoding/json"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

// A dump of netifd with a PPPoE WAN on a DSA port, DHCPv6 on the same port, a second PPPoE
// that is down, a guest Wi-Fi without a bridge, WireGuard, an LTE modem that is down and
// an alias.
const dump = `{"interface": [
	{"interface": "lan", "up": true, "proto": "static", "device": "br-lan", "l3_device": "br-lan", "ipv4-address": [{"address": "192.168.1.1", "mask": 24}]},
	{"interface": "wan", "up": true, "proto": "pppoe", "device": "wan", "l3_device": "pppoe-wan", "ipv4-address": [{"address": "203.0.113.7", "mask": 32}]},
	{"interface": "wan6", "up": true, "proto": "dhcpv6", "device": "wan", "l3_device": "wan6-dev", "ipv4-address": []},
	{"interface": "dsl", "up": false, "proto": "pppoe", "device": "eth0.7"},
	{"interface": "isp2", "up": true, "proto": "pppoe", "device": "eth0.2", "l3_device": "pppoe-isp2", "ipv4-address": [{"address": "198.51.100.9", "mask": 32}]},
	{"interface": "guest", "up": true, "proto": "static", "device": "phy0-ap1", "l3_device": "phy0-ap1", "ipv4-address": [{"address": "192.168.3.1", "mask": 24}]},
	{"interface": "wg0", "up": true, "proto": "wireguard", "l3_device": "wg0", "ipv4-address": [{"address": "10.8.0.2", "mask": 24}]},
	{"interface": "lte", "up": false, "proto": "qmi", "device": "/dev/cdc-wdm0"},
	{"interface": "lan6", "up": false, "proto": "static", "device": "@lan"}
]}`

func TestDevicesOf(t *testing.T) {
	var parsed struct {
		Interfaces []netifdInterface `json:"interface"`
	}
	require.NoError(t, json.Unmarshal([]byte(dump), &parsed))
	names := []string{"wan", "@wan", "lan", "guest", "dsl", "eth0.7", "eth0.2", "wan6", "br-lan", "wg0", "tun0", "@nothere", "lan6", "pppoe-wan"}
	require.Equal(t, map[string]string{
		// a logical interface: its device of layer 3, in either form
		"wan":  "pppoe-wan",
		"@wan": "pppoe-wan",
		"lan":  "br-lan",
		// a Wi-Fi without a bridge has its own device of layer 3
		"guest": "phy0-ap1",
		// PPPoE that is down, by its name and by its port: the device pppd will make
		"dsl":    "pppoe-dsl",
		"eth0.7": "pppoe-dsl",
		// the VLAN of a PPPoE connection: its device of layer 3
		"eth0.2": "pppoe-isp2",
		"wan6":   "wan6-dev",
	}, devicesOf(parsed.Interfaces, names))

	// the DSA port "wan" is also the name of the interface: the interface is meant,
	// whatever the order
	reversed := []netifdInterface{parsed.Interfaces[2], parsed.Interfaces[1]}
	require.Equal(t, "pppoe-wan", deviceOf(reversed, "wan"))
	// the port of two interfaces, neither named after it: the one with an IPv4 address
	unnamed := []netifdInterface{parsed.Interfaces[2], parsed.Interfaces[1]}
	unnamed[0].Interface, unnamed[1].Interface = "v6", "internet"
	require.Equal(t, "pppoe-wan", deviceOf(unnamed, "wan"))

	require.Empty(t, devicesOf(nil, names))
	require.Nil(t, Devices(nil))
}

func TestUsableAddress(t *testing.T) {
	for text, usable := range map[string]bool{
		"192.168.1.1": true, "203.0.113.7": true, "198.18.0.1": true,
		"169.254.10.1": false, "127.0.0.1": false, "0.0.0.0": false, "2001:db8::1": false, "::ffff:10.0.0.1": true,
	} {
		require.Equal(t, usable, UsableAddress(netip.MustParseAddr(text)), text)
	}
}
