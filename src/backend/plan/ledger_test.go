package plan

import (
	"os"
	"testing"

	"github.com/podkop-net/podkop-experimental/src/backend/model"

	"github.com/stretchr/testify/require"
)

// The differences of ledger.go, one case each.
func TestFromShell(t *testing.T) {
	noFiles := Options{ReadFile: func(string) ([]byte, error) { return nil, os.ErrNotExist }}

	// the shell backend's download_detour becomes the detour of the one HTTP client of
	// the lists, named the default; independent_cache goes; the key of the settings goes
	// to the Clash API
	settings := &model.Config{Settings: model.Settings{UpdateInterval: "1d", YACDSecretKey: "k3y"}, Sections: []model.Section{{Name: "main",
		RemoteDomainLists: []string{"https://lists.example/a.srs"}}}}
	shell := []byte(`{"dns":{"final":"dns-server","independent_cache":true},"certificate":{},"endpoints":[],"route":{"rules":[{"action":"route","rule_set":"main-a-remote-domains-ruleset"}],"rule_set":[` +
		`{"type":"remote","tag":"main-a-remote-domains-ruleset","format":"binary","url":"https://lists.example/a.srs","download_detour":"main-out","update_interval":"1d"}],"final":"direct-out"},` +
		`"experimental":{"clash_api":{"external_controller":"192.168.1.1:9090"}}}`)
	converted, err := FromShell(shell, Shell{Settings: settings, Options: noFiles})
	require.NoError(t, err)
	require.Empty(t, converted.Dropped)
	require.Equal(t, `{
  "dns": {
    "final": "dns-server"
  },
  "certificate": {},
  "http_clients": [
    {
      "tag": "lists",
      "detour": "main-out"
    }
  ],
  "endpoints": [],
  "route": {
    "rules": [
      {
        "action": "route",
        "rule_set": "main-a-remote-domains-ruleset"
      }
    ],
    "rule_set": [
      {
        "type": "remote",
        "tag": "main-a-remote-domains-ruleset",
        "format": "binary",
        "url": "https://lists.example/a.srs",
        "update_interval": "1d"
      }
    ],
    "final": "direct-out",
    "default_http_client": "lists"
  },
  "experimental": {
    "clash_api": {
      "external_controller": "192.168.1.1:9090",
      "secret": "k3y"
    }
  }
}
`, string(converted.Config))

	// the one file of the plain lists of a section becomes a rule set a URL: those written
	// as text together, a rule set file of a query or an upper-case extension where it
	// is; the service proxy inbound and its rule go, a mixed inbound of a section of that
	// name stays
	settings = &model.Config{Settings: model.Settings{UpdateInterval: "1d"}, Sections: []model.Section{{Name: "main",
		RemoteDomainLists: []string{"https://a.example/x.srs", "https://a.example/one.lst", "https://b.example/q.json?raw=1", "https://b.example/dir/one.lst", "https://c.example/"}}}}
	shell = []byte(`{"certificate":{},"inbounds":[{"type":"mixed","tag":"service-mixed-in","listen":"127.0.0.1","listen_port":4534},{"type":"direct","tag":"dns-in"}],` +
		`"route":{"rules":[{"action":"route","inbound":"service-mixed-in","outbound":"main-out"},{"action":"route","rule_set":["main-x-remote-domains-ruleset","main-remote-domains-ruleset"]}],"rule_set":[` +
		`{"type":"remote","tag":"main-x-remote-domains-ruleset","format":"binary","url":"https://a.example/x.srs","update_interval":"1d"},` +
		`{"type":"local","tag":"main-remote-domains-ruleset","format":"source","path":"/tmp/sing-box/rulesets/main-remote-domains-ruleset.json"}]},` +
		`"dns":{"rules":[{"action":"route","server":"fakeip-server","rule_set":["main-x-remote-domains-ruleset","main-remote-domains-ruleset"]}]}}`)
	converted, err = FromShell(shell, Shell{Settings: settings, Options: noFiles})
	require.NoError(t, err)
	require.Equal(t, []string{"main-remote-domains-ruleset.json"}, converted.Dropped)
	text := string(converted.Config)
	require.NotContains(t, text, "service-mixed-in")
	require.NotContains(t, text, `"main-remote-domains-ruleset"`)
	want := []string{"main-x-remote-domains-ruleset", "main-one-text-domains-ruleset", "main-one-2-text-domains-ruleset", "main-list-text-domains-ruleset", "main-q-remote-domains-ruleset"}
	var tags []string
	for _, entry := range remoteOrder(listEntries("main", "domains", settings.Sections[0].RemoteDomainLists, false)) {
		tags = append(tags, entry.tag)
	}
	require.Equal(t, want, tags)
	require.Contains(t, text, `"url": "https://b.example/q.json?raw=1"`)
	require.Contains(t, text, `"format": "source"`)

	// the files of the router: a file each, one that cannot be read an empty file of the
	// backend
	settings = &model.Config{Settings: model.Settings{UpdateInterval: "1d"}, Sections: []model.Section{{Name: "main",
		LocalDomainLists: []string{"/etc/lists/good.lst", "/etc/lists/gone.lst"}}}}
	files := Options{ReadFile: func(path string) ([]byte, error) {
		if path == "/etc/lists/good.lst" {
			return []byte("good.example\n"), nil
		}
		return nil, os.ErrNotExist
	}}
	shell = []byte(`{"certificate":{},"route":{"rules":[{"action":"route","rule_set":"main-local-domains-ruleset"}],"rule_set":[` +
		`{"type":"local","tag":"main-local-domains-ruleset","format":"source","path":"/tmp/sing-box/rulesets/main-local-domains-ruleset.json"}]}}`)
	converted, err = FromShell(shell, Shell{Settings: settings, Options: files})
	require.NoError(t, err)
	require.Equal(t, []string{"main-local-domains-ruleset.json"}, converted.Dropped)
	require.Contains(t, converted.Added, "main-gone-local-domains-ruleset.json")
	text = string(converted.Config)
	require.Contains(t, text, `"path": "/etc/lists/good.lst"`)
	require.Contains(t, text, `"format": "text"`)
	require.Contains(t, text, `"path": "/var/run/podkop/rulesets/main-gone-local-domains-ruleset.json"`)
	require.Contains(t, text, `"main-good-local-domains-ruleset",`)

	// the files of the shell backend in the directories of the sing-box package are the
	// backend's in its own (item 8)
	shell = []byte(`{"route":{"rule_set":[{"type":"local","tag":"main-user-domains-ruleset","format":"source","path":"/tmp/sing-box/rulesets/main-user-domains-ruleset.json"}]},` +
		`"experimental":{"cache_file":{"enabled":true,"path":"/usr/share/sing-box/cache.db","store_fakeip":true}}}`)
	converted, err = FromShell(shell, Shell{Settings: &model.Config{}, Options: noFiles})
	require.NoError(t, err)
	text = string(converted.Config)
	require.Contains(t, text, `"path": "/var/run/podkop/rulesets/main-user-domains-ruleset.json"`)
	require.Contains(t, text, `"path": "/usr/share/podkop/cache.db"`)
	require.NotContains(t, text, "sing-box")
}
