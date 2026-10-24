package plan

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/podkop-net/podkop-experimental/src/backend/model"
	"github.com/podkop-net/podkop-experimental/src/backend/uci"

	"github.com/stretchr/testify/require"
)

const detourHead = "config settings 'settings'\n\toption dns_type 'udp'\n\toption dns_server '8.8.8.8'\n"

func proxySection(name string, more string) string {
	return "\nconfig section '" + name + "'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n" +
		"\toption user_domain_list_type 'dynamic'\n\tlist user_domains '" + name + ".example'\n" + more
}

func buildDetour(settings string, outbound string) (*Result, error) {
	parsed, err := uci.Parse([]byte(detourHead + settings))
	if err != nil {
		return nil, err
	}
	config, err := model.Load(parsed)
	if err != nil {
		return nil, err
	}
	if outbound == "" {
		outbound = `{"type": "socks", "server": "203.0.113.15", "server_port": 1080}`
	}
	return Build(config, Facts{}, Options{DecodeLink: func(string) ([]byte, []string, error) {
		return []byte(outbound), nil, nil
	}})
}

// detoursOf are the detours of the outbounds of a configuration by tag ("" none); every
// outbound is there.
func detoursOf(t *testing.T, result *Result) map[string]string {
	var config struct {
		Outbounds []struct {
			Tag    string `json:"tag"`
			Detour string `json:"detour"`
		} `json:"outbounds"`
	}
	require.NoError(t, json.Unmarshal(result.Config, &config))
	detours := make(map[string]string)
	for _, outbound := range config.Outbounds {
		detours[outbound.Tag] = outbound.Detour
	}
	return detours
}

// A proxy section connects through another one: the outbounds of its servers dial through the
// outbound of that section, a group dials through nothing itself, and the section it goes
// through is left as it is.
func TestDetourOfEveryKindOfSection(t *testing.T) {
	// a link through a link
	result, err := buildDetour(proxySection("main", "\toption detour 'entry'\n")+proxySection("entry", ""), "")
	require.NoError(t, err)
	detours := detoursOf(t, result)
	require.Equal(t, "entry-out", detours["main-out"])
	require.Equal(t, "", detours["entry-out"])

	// a selector through a VPN: each server, not the selector
	result, err = buildDetour("\nconfig section 'sel'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'selector'\n"+
		"\tlist selector_proxy_links 'socks5://203.0.113.15:1080'\n\tlist selector_proxy_links 'socks5://203.0.113.16:1080'\n\toption detour 'awg'\n"+
		"\nconfig section 'awg'\n\toption connection_type 'vpn'\n\toption interface 'awg0'\n", "")
	require.NoError(t, err)
	detours = detoursOf(t, result)
	require.Equal(t, map[string]string{"direct-out": "", "sel-1-out": "awg-out", "sel-2-out": "awg-out", "sel-out": "", "awg-out": ""}, detours)

	// a URL test through a URL test: the transit is the group, which chooses its server
	result, err = buildDetour("\nconfig section 'exit'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'urltest'\n"+
		"\tlist urltest_proxy_links 'socks5://203.0.113.15:1080'\n\tlist urltest_fallback_links 'socks5://203.0.113.17:1080'\n\toption detour 'entry'\n"+
		"\nconfig section 'entry'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'urltest'\n\tlist urltest_proxy_links 'socks5://203.0.113.16:1080'\n", "")
	require.NoError(t, err)
	detours = detoursOf(t, result)
	require.Equal(t, "entry-out", detours["exit-1-out"])
	require.Equal(t, "entry-out", detours["exit-fallback-1-out"])
	require.Equal(t, "", detours["exit-urltest-out"])
	require.Equal(t, "", detours["exit-out"])
	require.Equal(t, "", detours["entry-1-out"])

	// outbound JSON: given the detour, unless it has one of its own
	jsonSection := func(outbound string) string {
		return "\nconfig section 'js'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'outbound'\n\toption outbound_json '" + outbound + "'\n\toption detour 'entry'\n" + proxySection("entry", "")
	}
	result, err = buildDetour(jsonSection(`{"type": "socks", "server": "203.0.113.18", "server_port": 1080}`), "")
	require.NoError(t, err)
	require.Equal(t, "entry-out", detoursOf(t, result)["js-out"])
	result, err = buildDetour(jsonSection(`{"type": "socks", "server": "203.0.113.18", "server_port": 1080, "detour": "direct-out"}`), "")
	require.NoError(t, err)
	require.Equal(t, "direct-out", detoursOf(t, result)["js-out"])
	_, err = buildDetour(jsonSection(`{"type": "selector", "outbounds": ["entry-out"]}`), "")
	require.ErrorContains(t, err, "the outbound JSON is a selector, which connects through nothing")

}

// A detour that cannot be is refused in the words of the settings; one of a section that is no
// proxy is left out with a warning.
func TestDetourRefused(t *testing.T) {
	for settings, message := range map[string]string{
		proxySection("main", "\toption detour 'main'\n"):                                                                                     "section main connects through itself",
		proxySection("main", "\toption detour 'nosuch'\n"):                                                                                   "section main connects through nosuch, and there is no such section",
		proxySection("main", "\toption detour 'blk'\n") + "\nconfig section 'blk'\n\toption connection_type 'block'\n":                       "section main connects through blk, a section of the type block that has no outbound",
		proxySection("a", "\toption detour 'b'\n") + proxySection("b", "\toption detour 'a'\n"):                                              "connects round in a loop: ",
		proxySection("a", "\toption detour 'b'\n") + proxySection("b", "\toption detour 'c'\n") + proxySection("c", "\toption detour 'a'\n"): " → ",
	} {
		_, err := buildDetour(settings, "")
		require.ErrorContains(t, err, message, settings)
	}
	_, err := buildDetour(proxySection("a", "\toption detour 'b'\n")+proxySection("b", "\toption detour 'a'\n"), "")
	require.Regexp(t, `section [ab] connects round in a loop: ([ab]) → [ab] → ([ab])$`, err.Error())

	result, err := buildDetour("\nconfig section 'awg'\n\toption connection_type 'vpn'\n\toption interface 'awg0'\n\toption detour 'main'\n"+proxySection("main", ""), "")
	require.NoError(t, err)
	require.Contains(t, result.Warnings, "section awg: the connection through main is left out: only a proxy section connects through another")
	require.Equal(t, "", detoursOf(t, result)["awg-out"])
}

// A section only others connect through needs no list; tls.spoof through a proxy section is
// warned of, through a VPN section it works.
func TestDetourWarnings(t *testing.T) {
	transit := "\nconfig section 'entry'\n\toption connection_type 'proxy'\n\toption proxy_config_type 'url'\n\toption proxy_string 'socks5://203.0.113.16:1080'\n"
	result, err := buildDetour(proxySection("main", "\toption detour 'entry'\n")+transit, "")
	require.NoError(t, err)
	require.NotContains(t, strings.Join(result.Warnings, "\n"), "has no list")
	result, err = buildDetour(proxySection("main", "")+transit, "")
	require.NoError(t, err)
	require.Contains(t, result.Warnings, "section entry has no list: no route rule for it")

	spoofed := `{"type": "vless", "server": "203.0.113.15", "server_port": 443, "uuid": "00000000-0000-0000-0000-000000000000", "tls": {"enabled": true, "server_name": "example.com", "spoof": "www.example.org"}}`
	result, err = buildDetour(proxySection("main", "\toption detour 'entry'\n")+transit, spoofed)
	require.NoError(t, err)
	require.Contains(t, strings.Join(result.Warnings, "\n"), "proxy link of main: tls.spoof does not work through the proxy section entry")
	result, err = buildDetour(proxySection("main", "\toption detour 'awg'\n")+"\nconfig section 'awg'\n\toption connection_type 'vpn'\n\toption interface 'awg0'\n", spoofed)
	require.NoError(t, err)
	require.NotContains(t, strings.Join(result.Warnings, "\n"), "tls.spoof")
}
