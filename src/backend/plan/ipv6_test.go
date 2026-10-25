package plan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// DNS answers IPv4 only, and the IPv6 addresses of a name that has no IPv4 address
// (ledger item 15).
func TestIPv6IfNoIPv4(t *testing.T) {
	const settings = "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '8.8.8.8'\n" +
		"\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n"
	result := buildText(t, settings, Facts{})
	var document struct {
		DNS map[string]any `json:"dns"`
	}
	require.NoError(t, json.Unmarshal(result.Config, &document))
	require.Equal(t, "ipv4_only", document.DNS["strategy"])
	require.Equal(t, true, document.DNS["ipv6_if_no_ipv4"])
}
