package plan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The names of the NTP servers of the router go to the DNS server before the rule of the
// lists, never to FakeIP; without such names there is no rule (ledger item 16).
func TestNTPServerNames(t *testing.T) {
	const settings = "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '8.8.8.8'\n" +
		"\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n" +
		"\toption user_domain_list_type 'dynamic'\n\tlist user_domains 'pool.ntp.org'\n"
	rules := func(facts Facts) []map[string]any {
		var document struct {
			DNS struct {
				Rules []map[string]any `json:"rules"`
			} `json:"dns"`
		}
		require.NoError(t, json.Unmarshal(buildText(t, settings, facts).Config, &document))
		return document.DNS.Rules
	}
	withNames := rules(Facts{NTPServerNames: []string{"0.openwrt.pool.ntp.org", "1.openwrt.pool.ntp.org"}})
	ntp, fakeIP := -1, -1
	for index, rule := range withNames {
		if rule["server"] == tagDNSServer && rule["domain"] != nil {
			ntp = index
			require.Equal(t, []any{"0.openwrt.pool.ntp.org", "1.openwrt.pool.ntp.org"}, rule["domain"])
		}
		if rule["server"] == tagFakeIPServer && rule["rule_set"] != nil && fakeIP < 0 {
			fakeIP = index
		}
	}
	require.GreaterOrEqual(t, ntp, 0, "no rule for the NTP names")
	require.Greater(t, fakeIP, ntp, "the rule of the lists comes first")
	require.Len(t, withNames, len(rules(Facts{}))+1)
}
