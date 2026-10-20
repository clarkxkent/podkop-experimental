package model

import (
	"testing"

	"github.com/itdoginfo/podkop/src/backend/uci"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	parsed, err := uci.Parse([]byte("config settings 'settings'\n\toption dns_server ''\n\nconfig section 'main'\n\toption connection_type 'proxy'\n\tlist selector_proxy_links 'a://1'\n\tlist selector_proxy_links 'b://2 c://3'\n"))
	require.NoError(t, err)
	config, err := Load(parsed)
	require.NoError(t, err)
	require.Equal(t, "doh", config.Settings.DNSType)
	require.Equal(t, "1.1.1.1", config.Settings.DNSServer)
	require.Equal(t, "77.88.8.8", config.Settings.BootstrapDNSServer)
	require.Equal(t, "60", config.Settings.DNSRewriteTTL)
	require.Equal(t, "1d", config.Settings.UpdateInterval)
	require.Equal(t, "warn", config.Settings.LogLevel)
	require.Equal(t, "/var/run/podkop/cache.db", config.Settings.CachePath)
	require.Len(t, config.Sections, 1)
	section := config.Sections[0]
	require.Equal(t, []string{"a://1", "b://2", "c://3"}, section.SelectorProxyLinks)
	require.Equal(t, "3m", section.URLTestCheckInterval)
	require.Equal(t, "50", section.URLTestTolerance)
	require.Equal(t, "default", section.URLTestDownloadCheck)
	require.Equal(t, "disabled", section.UserDomainListType)
	require.False(t, section.HasEnabledLists())
	require.True(t, section.HasOutbound())
}

func TestLoadNoSettings(t *testing.T) {
	t.Parallel()
	parsed, err := uci.Parse([]byte("config section 'main'\n"))
	require.NoError(t, err)
	_, err = Load(parsed)
	require.Error(t, err)
	// a section of another type named settings is no settings
	parsed, err = uci.Parse([]byte("config section 'settings'\n\toption connection_type 'proxy'\n"))
	require.NoError(t, err)
	_, err = Load(parsed)
	require.Error(t, err)
}

func TestValues(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"1.2.3.4", "0.0.0.0", "255.255.255.255", "10.0.0.1"} {
		require.True(t, IsIPv4(text), text)
	}
	for _, text := range []string{"", "1.2.3", "1.2.3.4.5", "256.1.1.1", "01.2.3.4", "1.2.3.4.", "a.b.c.d", "1.2.3.4/24", "1234"} {
		require.False(t, IsIPv4(text), text)
	}
	for _, text := range []string{"1.2.3.0/24", "0.0.0.0/0", "1.2.3.4/32", "1.2.3.4/9", "1.2.3.4/19", "1.2.3.4/29"} {
		require.True(t, IsIPv4CIDR(text), text)
	}
	for _, text := range []string{"1.2.3.4", "1.2.3.4/33", "1.2.3.4/", "1.2.3.4/08", "1.2.3.4/123", "1.2.3/24"} {
		require.False(t, IsIPv4CIDR(text), text)
	}
	for _, text := range []string{"example.com", ".example.com", "a", "xn--p1ai", "a-b.c1", "1.2.3.4"} {
		require.True(t, IsDomainSuffix(text), text)
	}
	for _, text := range []string{"", ".", "Example.com", "a..b", "-a.com", "a-.com", "a.com/x", "a b", "..a"} {
		require.False(t, IsDomainSuffix(text), text)
	}
}

func TestIsOne(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"1", " 1", "1 ", "01", "+1", "001"} {
		require.True(t, isOne(text), text)
	}
	for _, text := range []string{"", "0", "2", "11", "on", "true", "1x", "-1", "10", "+"} {
		require.False(t, isOne(text), text)
	}
}

// The cache file is never in a directory of the sing-box package: the places podkop 0.7
// offered are taken to the backend's own.
func TestCachePathMoved(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"/tmp/sing-box/cache.db":       "/var/run/podkop/cache.db",
		"/usr/share/sing-box/cache.db": "/usr/share/podkop/cache.db",
		"/mnt/usb/cache.db":            "/mnt/usb/cache.db",
	} {
		parsed, err := uci.Parse([]byte("config settings 'settings'\n\toption cache_path '" + path + "'\n\nconfig section 'main'\n\toption connection_type 'proxy'\n\toption proxy_string 'socks5://203.0.113.15:1080'\n"))
		require.NoError(t, err)
		config, err := Load(parsed)
		require.NoError(t, err)
		require.Equal(t, want, config.Settings.CachePath, path)
	}
}
