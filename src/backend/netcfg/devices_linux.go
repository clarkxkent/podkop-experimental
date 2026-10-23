package netcfg

import (
	"context"
	"encoding/json"
	"net/netip"
	"os/exec"
	"strings"
)

// netifdInterface is what netifd reports of a logical interface (ubus call
// network.interface dump).
type netifdInterface struct {
	Interface string `json:"interface"`
	Up        bool   `json:"up"`
	Proto     string `json:"proto"`
	Device    string `json:"device"`
	L3Device  string `json:"l3_device"`
	Addresses []struct {
		Address string `json:"address"`
	} `json:"ipv4-address"`
}

// pppPrefixes are the protocols of netifd whose device of layer 3 is made by pppd when the
// connection is up, and the prefix of its name: the interface "wan" of PPPoE has
// "pppoe-wan".
var pppPrefixes = map[string]string{
	"pppoe": "pppoe",
	"pppoa": "pppoa",
	"ppp":   "ppp",
	"3g":    "3g",
	"pptp":  "pptp",
	"l2tp":  "l2tp",
}

// Devices tells the devices of layer 3 that interface names stand for now, by what netifd
// reports (ubus call network.interface dump). Only the device of layer 3 carries the
// addresses and the routes: the firewall sees traffic come in through it, and a socket
// bound to another device has no route. A name is
//   - a logical interface of netifd ("wan", or "@wan" as LuCI writes it): its device of
//     layer 3 when it is up; when it is not, the device pppd will make for it (PPP), or
//     its device;
//   - the device of layer 2 of an interface that is up with another device of layer 3
//     (the port or VLAN of a PPPoE connection): that device of layer 3;
//   - anything else, a device of its own included (br-lan, wg0, a tun of OpenVPN): itself.
//
// The result holds the names that stand for another device. Without netifd it is empty,
// and the names are used as they are.
func Devices(names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	// netifd answers at once or is not there; a bus that hangs must not hold the caller
	ctx, cancel := context.WithTimeout(context.Background(), netifdTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ubus", "-t", "2", "call", "network.interface", "dump").Output()
	if err != nil {
		return nil
	}
	var dump struct {
		Interfaces []netifdInterface `json:"interface"`
	}
	if json.Unmarshal(output, &dump) != nil {
		return nil
	}
	return devicesOf(dump.Interfaces, names)
}

func devicesOf(interfaces []netifdInterface, names []string) map[string]string {
	result := make(map[string]string)
	for _, name := range names {
		if device := deviceOf(interfaces, name); device != "" && device != name {
			result[name] = device
		}
	}
	return result
}

func deviceOf(interfaces []netifdInterface, name string) string {
	logical := strings.TrimPrefix(name, "@")
	for _, candidate := range interfaces {
		if candidate.Interface != logical {
			continue
		}
		if candidate.L3Device != "" {
			return candidate.L3Device
		}
		if prefix := pppPrefixes[candidate.Proto]; prefix != "" {
			return prefix + "-" + candidate.Interface
		}
		if !strings.HasPrefix(candidate.Device, "@") {
			return candidate.Device
		}
		return ""
	}
	if logical != name {
		return ""
	}
	// a device of layer 2: the interface with an IPv4 address first (the PPPoE "wan" over
	// "wan6" of DHCPv6 on the same port)
	var found string
	for _, candidate := range interfaces {
		if !candidate.Up || candidate.Device != name || candidate.L3Device == "" || candidate.L3Device == name {
			continue
		}
		if len(candidate.Addresses) > 0 {
			return candidate.L3Device
		}
		if found == "" {
			found = candidate.L3Device
		}
	}
	return found
}

// UsableAddress tells whether an IPv4 address can be the source of what goes out through
// a device: not of the loopback and not link-local (169.254.0.0/16, which a device gives
// itself when DHCP does not answer).
func UsableAddress(address netip.Addr) bool {
	address = address.Unmap()
	return address.Is4() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsUnspecified()
}
