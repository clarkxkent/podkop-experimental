package lists

import (
	"bytes"
	"net/netip"
	"sort"
	"testing"

	"github.com/podkop-net/podkop-experimental/src/backend/firewall"
	"github.com/sagernet/sing-box/common/srs"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"

	"github.com/stretchr/testify/require"
)

// the community list of Discord as its file is (a part of the addresses)
const discord = `{"version": 3, "rules": [
  {"domain_suffix": ["dis.gd", "discord.com", "discord.gg"]},
  {"network": "udp", "ip_cidr": ["5.200.14.128/25", "34.0.0.0/14", "66.22.192.0/18"], "port_range": "50000:65535"},
  {"network": "udp", "ip_cidr": "104.16.0.0/12", "port_range": "19000:20000"}
]}`

const telegram = `{"version": 3, "rules": [{"domain_suffix": ["telegram.org", "t.me"], "ip_cidr": ["149.154.160.0/20", "91.108.4.0/22", "2001:67c:4e8::/48"]}]}`

func texts(prefixes []netip.Prefix) []string {
	result := make([]string, len(prefixes))
	for index, prefix := range prefixes {
		result[index] = prefix.String()
	}
	sort.Strings(result)
	return result
}

// binary compiles a source rule set the way `sing-box rule-set compile` does.
func binary(t *testing.T, source string) []byte {
	compat, err := json.UnmarshalExtended[option.PlainRuleSetCompat]([]byte(source))
	require.NoError(t, err)
	ruleSet, err := compat.Upgrade()
	require.NoError(t, err)
	var buffer bytes.Buffer
	require.NoError(t, srs.Write(&buffer, ruleSet, compat.Version))
	return buffer.Bytes()
}

func TestDerive(t *testing.T) {
	user := `{"version": 3, "rules": [{"ip_cidr": ["198.51.100.8/30", "198.51.100.20"]}]}`
	names := `{"version": 3, "rules": [{"domain_suffix": ["one.lab.example"]}]}`
	odd := `{"version": 3, "rules": [
	  {"network": "tcp", "port": [443, 8443], "port_range": [":1023", "9000:"], "ip_cidr": "203.0.113.0/24"},
	  {"network": "udp", "port_range": "50000:65535", "ip_cidr": "192.0.2.0/24"},
	  {"network": ["tcp", "udp"], "port": 53, "ip_cidr": "198.18.5.0/24"},
	  {"network": "udp", "port": 53, "domain": "x.example", "ip_cidr": "198.18.6.0/24"},
	  {"port": 53, "ip_cidr": "198.18.7.0/24"},
	  {"invert": true, "ip_cidr": "198.18.8.0/24"},
	  {"type": "logical", "mode": "and", "rules": [{"network": "udp", "port": 53, "ip_cidr": "198.18.9.0/24"}, {"domain": "y.example"}]},
	  {"type": "logical", "mode": "or", "invert": true, "rules": [{"ip_cidr": "198.18.10.0/24"}]}
	]}`
	for _, compiled := range []bool{false, true} {
		source := func(tag string, text string) Source {
			if compiled {
				return Source{Tag: tag, Format: C.RuleSetFormatBinary, Content: binary(t, text)}
			}
			return Source{Tag: tag, Content: []byte(text)}
		}
		got, problems := Derive([]Source{source("discord", discord), source("telegram", telegram), source("user", user), source("names", names), source("odd", odd),
			{Tag: "broken", Content: []byte("{")}, {Tag: "not binary", Format: C.RuleSetFormatBinary, Content: []byte("text")}})
		require.Len(t, problems, 2, "compiled %v", compiled)
		// the addresses of rules with more than ports go to the common set, IPv6 is left out
		require.Equal(t, []string{"149.154.160.0/20", "198.18.5.0/24", "198.18.6.0/24", "198.18.7.0/24", "198.18.9.0/24", "198.51.100.20/32", "198.51.100.8/30", "91.108.4.0/22"},
			texts(got.Subnets), "compiled %v", compiled)
		require.Len(t, got.Groups, 3)
		byPorts := map[string]firewall.Group{}
		for _, group := range got.Groups {
			byPorts[group.Set] = group
		}
		// the same ports in two rule sets are one group
		require.Equal(t, firewall.Group{Set: "podkop_subnets_udp_1", Protocol: 17, Ports: []firewall.PortRange{{From: 19000, To: 20000}}, Prefixes: byPorts["podkop_subnets_udp_1"].Prefixes}, byPorts["podkop_subnets_udp_1"])
		require.Equal(t, []string{"104.16.0.0/12"}, texts(byPorts["podkop_subnets_udp_1"].Prefixes))
		require.Equal(t, []firewall.PortRange{{From: 50000, To: 65535}}, byPorts["podkop_subnets_udp_2"].Ports)
		require.Equal(t, []string{"192.0.2.0/24", "34.0.0.0/14", "5.200.14.128/25", "66.22.192.0/18"}, texts(byPorts["podkop_subnets_udp_2"].Prefixes))
		require.Equal(t, byte(6), byPorts["podkop_subnets_tcp_3"].Protocol)
		// 443 is within 0-1023: one range, as the interval set of nftables takes them
		require.Equal(t, []firewall.PortRange{{From: 0, To: 1023}, {From: 8443, To: 8443}, {From: 9000, To: 65535}}, byPorts["podkop_subnets_tcp_3"].Ports)
	}

	// a list written as text: its IPv4 addresses go to the common set
	got, problems := Derive([]Source{{Tag: "text", Format: C.RuleSetFormatText, Content: []byte("one.lab.example\n100.71.62.0/24\n2001:db8::/32\n")}})
	require.Empty(t, problems)
	require.Equal(t, []string{"100.71.62.0/24"}, texts(got.Subnets))
}

func TestDeriveNothing(t *testing.T) {
	got, problems := Derive(nil)
	require.Empty(t, problems)
	require.Empty(t, got.Subnets)
	require.Empty(t, got.Groups)
}
