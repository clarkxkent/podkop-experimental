package plan

import (
	"net/netip"
	"testing"

	"github.com/podkop-net/podkop-experimental/src/backend/model"
	"github.com/podkop-net/podkop-experimental/src/backend/uci"

	"github.com/stretchr/testify/require"
)

func buildText(t *testing.T, settings string, facts Facts) *Result {
	parsed, err := uci.Parse([]byte(settings))
	require.NoError(t, err)
	config, err := model.Load(parsed)
	require.NoError(t, err)
	result, err := Build(config, facts, Options{DecodeLink: func(string) ([]byte, []string, error) {
		return []byte(`{"type": "socks", "server": "203.0.113.15", "server_port": 1080}`), nil, nil
	}})
	require.NoError(t, err)
	return result
}

// What the router needs around the configuration comes with it: the rules of the
// firewall, the DNS of the LAN, the Clash API.
func TestDataplane(t *testing.T) {
	const head = "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '8.8.8.8'\n"
	const proxy = "\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n"

	// the defaults: the bridge of the LAN, NTP through, the DNS of the LAN taken, the API
	// on this host only, whatever the address of the LAN
	result := buildText(t, head+proxy, Facts{LANAddress: "192.168.1.1"})
	require.Equal(t, Dataplane{Interfaces: []string{"br-lan"}, TakeDNS: true, ClashAPI: "127.0.0.1:9090"}, result.Dataplane)
	require.Contains(t, string(result.Config), `"external_controller": "127.0.0.1:9090"`)

	// what the settings say; devices routed whole are IPv4 prefixes, a word is left out
	result = buildText(t, head+"\tlist source_network_interfaces 'br-guest'\n\tlist source_network_interfaces 'eth2'\n\toption exclude_ntp '1'\n\toption dont_touch_dhcp '1'\n"+
		"\toption enable_yacd '1'\n\tlist yacd_interfaces 'br-guest'\n\tlist yacd_interfaces 'wan'\n\toption yacd_secret_key 'k'\n"+
		proxy+"\tlist fully_routed_ips '192.168.1.23'\n\tlist fully_routed_ips '192.168.2.0/24'\n\tlist fully_routed_ips '2001:db8::1'\n\tlist fully_routed_ips 'word'\n",
		Facts{LANAddress: "192.168.1.1"})
	require.Equal(t, Dataplane{
		Interfaces: []string{"br-guest", "eth2"}, ExcludeNTP: true,
		Sources: []netip.Prefix{netip.MustParsePrefix("192.168.1.23/32"), netip.MustParsePrefix("192.168.2.0/24")},
		// with YACD the API listens on every address, kept by the firewall to the devices
		// of YACD, and is asked on this host
		ClashAPI: "127.0.0.1:9090", ClashSecret: "k", ClashAPIInterfaces: []string{"br-guest", "wan"},
	}, result.Dataplane)
	require.Contains(t, string(result.Config), `"external_controller": "0.0.0.0:9090"`)

	// YACD without devices of its own: those the traffic is taken from
	result = buildText(t, head+"\tlist source_network_interfaces 'br-guest'\n\toption enable_yacd '1'\n"+proxy, Facts{})
	require.Equal(t, []string{"br-guest"}, result.Dataplane.ClashAPIInterfaces)
}
