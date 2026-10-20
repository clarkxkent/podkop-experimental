// Package model holds the settings of the backend as its UCI file of the 0.7 format states
// them: one "settings" section and any number of "section" blocks. Values are kept the
// way the shell backend reads them, defaults included, so that the configuration built
// from them is the one that backend builds.
package model

import (
	"strings"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"

	"github.com/podkop-net/podkop-experimental/src/backend/uci"
	E "github.com/sagernet/sing/common/exceptions"
)

// Config is the whole file.
type Config struct {
	Settings Settings
	// Sections in the order of the file: the order decides the order of outbounds and of
	// route rules, and which section is the first with an outbound.
	Sections []Section
}

// Settings is the section "settings".
type Settings struct {
	DNSType            string // udp, dot, doh
	DNSServer          string // host[:port][/path]
	BootstrapDNSServer string
	DNSRewriteTTL      string // a number is expected; kept as written

	SourceNetworkInterfaces []string
	OutputNetworkInterface  string
	ExcludeNTP              bool
	RoutingExcludedIPs      []string

	EnableYACD          bool
	EnableYACDWANAccess bool
	YACDSecretKey       string
	EnableUTLSFallback  bool
	DisableQUIC         bool

	UpdateInterval               string
	DownloadListsViaProxy        bool
	DownloadListsViaProxySection string
	DontTouchDHCP                bool

	CachePath            string
	LogLevel             string
	ServiceListenAddress string
}

// Section is one "config section" block: where a part of the traffic goes and which lists
// name that part.
type Section struct {
	Name           string
	ConnectionType string // proxy, vpn, block, exclusion

	ProxyConfigType      string // url, selector, urltest, outbound
	ProxyString          string
	OutboundJSON         string
	SelectorProxyLinks   []string
	URLTestProxyLinks    []string
	URLTestFallbackLinks []string
	URLTestCheckInterval string
	URLTestTolerance     string
	URLTestTestingURL    string
	URLTestDownloadCheck string // default, off, custom
	URLTestDownloadURL   string
	EnableUDPOverTCP     bool

	Interface               string
	DomainResolverEnabled   bool
	DomainResolverDNSType   string
	DomainResolverDNSServer string

	CommunityLists     []string
	UserDomainListType string // disabled, dynamic, text
	UserDomains        string // the items joined by spaces
	UserDomainsText    string
	UserSubnetListType string
	UserSubnets        string
	UserSubnetsText    string
	LocalDomainLists   []string
	LocalSubnetLists   []string
	RemoteDomainLists  []string
	RemoteSubnetLists  []string

	FullyRoutedIPs          []string
	MixedProxyEnabled       bool
	MixedProxyPort          string
	ResolveRealIPForRouting bool
}

// Load takes the settings out of a parsed file.
func Load(parsed *uci.Package) (*Config, error) {
	settings := parsed.Section("settings")
	if settings == nil || settings.Type != "settings" {
		// the page writes "config settings 'settings'"; a section of another type that
		// takes the name is no settings
		return nil, E.New("the file has no section \"settings\"")
	}
	get := func(section *uci.Section, name string, defaultValue string) string {
		// as the shell's config_get: an option set to nothing takes the default too
		if value := section.Get(name); value != "" {
			return value
		}
		return defaultValue
	}
	config := &Config{Settings: Settings{
		DNSType:                      get(settings, "dns_type", "doh"),
		DNSServer:                    get(settings, "dns_server", "1.1.1.1"),
		BootstrapDNSServer:           get(settings, "bootstrap_dns_server", "77.88.8.8"),
		DNSRewriteTTL:                get(settings, "dns_rewrite_ttl", "60"),
		SourceNetworkInterfaces:      list(settings, "source_network_interfaces"),
		OutputNetworkInterface:       settings.Get("output_network_interface"),
		ExcludeNTP:                   settings.Bool("exclude_ntp", false),
		RoutingExcludedIPs:           list(settings, "routing_excluded_ips"),
		EnableYACD:                   settings.Bool("enable_yacd", false),
		EnableYACDWANAccess:          settings.Bool("enable_yacd_wan_access", false),
		YACDSecretKey:                settings.Get("yacd_secret_key"),
		EnableUTLSFallback:           settings.Bool("enable_utls_fallback", false),
		DisableQUIC:                  settings.Bool("disable_quic", false),
		UpdateInterval:               get(settings, "update_interval", "1d"),
		DownloadListsViaProxy:        settings.Bool("download_lists_via_proxy", false),
		DownloadListsViaProxySection: settings.Get("download_lists_via_proxy_section"),
		DontTouchDHCP:                settings.Bool("dont_touch_dhcp", false),
		CachePath:                    CachePath(get(settings, "cache_path", DefaultCachePath)),
		LogLevel:                     get(settings, "log_level", "warn"),
		ServiceListenAddress:         settings.Get("service_listen_address"),
	}}
	for _, section := range parsed.SectionsOfType("section") {
		config.Sections = append(config.Sections, Section{
			Name:                    section.Name,
			ConnectionType:          section.Get("connection_type"),
			ProxyConfigType:         section.Get("proxy_config_type"),
			ProxyString:             section.Get("proxy_string"),
			OutboundJSON:            section.Get("outbound_json"),
			SelectorProxyLinks:      links(section, "selector_proxy_links"),
			URLTestProxyLinks:       links(section, "urltest_proxy_links"),
			URLTestFallbackLinks:    links(section, "urltest_fallback_links"),
			URLTestCheckInterval:    get(section, "urltest_check_interval", "3m"),
			URLTestTolerance:        get(section, "urltest_tolerance", "50"),
			URLTestTestingURL:       get(section, "urltest_testing_url", "https://www.gstatic.com/generate_204"),
			URLTestDownloadCheck:    get(section, "urltest_download_check", "default"),
			URLTestDownloadURL:      section.Get("urltest_download_url"),
			EnableUDPOverTCP:        section.Get("enable_udp_over_tcp") == "1",
			Interface:               section.Get("interface"),
			DomainResolverEnabled:   isOne(section.Get("domain_resolver_enabled")),
			DomainResolverDNSType:   section.Get("domain_resolver_dns_type"),
			DomainResolverDNSServer: section.Get("domain_resolver_dns_server"),
			CommunityLists:          list(section, "community_lists"),
			UserDomainListType:      get(section, "user_domain_list_type", "disabled"),
			UserDomains:             section.Get("user_domains"),
			UserDomainsText:         section.Get("user_domains_text"),
			UserSubnetListType:      get(section, "user_subnet_list_type", "disabled"),
			UserSubnets:             section.Get("user_subnets"),
			UserSubnetsText:         section.Get("user_subnets_text"),
			LocalDomainLists:        list(section, "local_domain_lists"),
			LocalSubnetLists:        list(section, "local_subnet_lists"),
			RemoteDomainLists:       list(section, "remote_domain_lists"),
			RemoteSubnetLists:       list(section, "remote_subnet_lists"),
			FullyRoutedIPs:          list(section, "fully_routed_ips"),
			MixedProxyEnabled:       section.Bool("mixed_proxy_enabled", false),
			MixedProxyPort:          section.Get("mixed_proxy_port"),
			ResolveRealIPForRouting: section.Bool("resolve_real_ip_for_routing", false),
		})
	}
	return config, nil
}

// list returns the items of a list option; none when all there is to it is empty items,
// which the shell backend takes for an option that is not set.
func list(section *uci.Section, name string) []string {
	if section.Get(name) == "" {
		return nil
	}
	return section.List(name)
}

// isOne tells whether the text is the number one for the shell's "-eq": spaces around it
// and zeros before it allowed. The shell backend reads one switch this way.
func isOne(text string) bool {
	text = strings.TrimPrefix(strings.Trim(text, " \t\n"), "+")
	if text == "" {
		return false
	}
	for index := 0; index < len(text)-1; index++ {
		if text[index] != '0' {
			return false
		}
	}
	return text[len(text)-1] == '1'
}

// links splits a list of proxy links the way the shell backend walks it: by white space,
// so a link written with a space in it is two words there as well.
func links(section *uci.Section, name string) []string {
	return strings.Fields(section.Get(name))
}

// HasEnabledLists tells whether the section names any traffic: without a list it has no
// route rule of its own.
func (s *Section) HasEnabledLists() bool {
	return len(s.CommunityLists) > 0 ||
		s.UserDomainListType != "disabled" ||
		s.UserSubnetListType != "disabled" ||
		len(s.LocalDomainLists) > 0 ||
		len(s.LocalSubnetLists) > 0 ||
		len(s.RemoteDomainLists) > 0 ||
		len(s.RemoteSubnetLists) > 0
}

// HasOutbound tells whether traffic of the section leaves through an outbound of its own.
func (s *Section) HasOutbound() bool {
	return s.ConnectionType == "proxy" || s.ConnectionType == "vpn"
}

// DefaultCachePath is the cache file of sing-box (FakeIP, rule sets) when the settings name
// none: in RAM, as the shell backend had it.
const DefaultCachePath = dataplane.RunDir + "/cache.db"

// movedDirs are the directories of the sing-box package that podkop 0.7 offered for the
// cache file, and where the backend has it instead.
var movedDirs = [][2]string{
	{"/tmp/sing-box/", dataplane.RunDir + "/"},
	{"/usr/share/sing-box/", dataplane.DataDir + "/"},
}

// CachePath is the cache file the settings name, a path in a directory of the sing-box
// package taken to the same name in the backend's own: settings kept from podkop 0.7 name
// those. The package moves the file there once (uci-defaults), so the FakeIP addresses
// handed out before stay.
func CachePath(path string) string {
	for _, moved := range movedDirs {
		if rest, found := strings.CutPrefix(path, moved[0]); found {
			return moved[1] + rest
		}
	}
	return path
}
