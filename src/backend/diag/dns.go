// Package diag holds the checks the backend runs for its user interface.
package diag

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
	"github.com/podkop-net/podkop-experimental/src/backend/plan"
	"github.com/podkop-net/podkop-experimental/src/backend/uci"

	mDNS "github.com/miekg/dns"
)

const (
	// probeName is the name every probe asks for, the one the shell backend asks for.
	probeName = "google.com."
	// probeTimeout is what the shell backend gives its DNS tool: one try of two seconds.
	probeTimeout = 2 * time.Second
)

// DNS is what the check of the DNS found.
type DNS struct {
	// Type, Server and Bootstrap are those of the settings; an account in the address of
	// the server is masked.
	Type      string
	Server    string
	Bootstrap string
	// ServerAnswers, RouterAnswers and BootstrapAnswers tell who answered: the server of
	// the settings, the resolver of the router, the bootstrap server.
	ServerAnswers    bool
	RouterAnswers    bool
	BootstrapAnswers bool
	// DHCPAsksBackend tells whether dnsmasq is set up to ask the backend.
	DHCPAsksBackend bool
}

// DNSSettings is what the check needs of the settings, as written there.
type DNSSettings struct {
	Type      string // udp, dot, doh
	Server    string // host[:port][/path]
	Bootstrap string
}

// probe is the settings of the probes; a test replaces the trusted certificates and the
// ports the local resolver is at.
type probe struct {
	roots        *x509.CertPool
	localAddress string
}

// CheckDNS asks the DNS server of the settings, the resolver of the router and the
// bootstrap server for a name, each from the router itself and not through the proxy, and
// tells whether dnsmasq is set up to ask the backend. The three questions are asked at
// once: the check takes the time of the slowest.
//
// Where it differs from the shell backend, which runs dig: a port in the setting is used
// (dig is given "host:port" as a name and fails), and the certificate of a DoT or DoH
// server is verified as sing-box verifies it (dig does not look at it).
func CheckDNS(ctx context.Context, settings DNSSettings, dhcp *uci.Package) *DNS {
	return (&probe{localAddress: "127.0.0.1:53"}).check(ctx, settings, dhcp)
}

func (p *probe) check(ctx context.Context, settings DNSSettings, dhcp *uci.Package) *DNS {
	report := &DNS{
		Type:            settings.Type,
		Server:          maskServer(settings.Server),
		Bootstrap:       settings.Bootstrap,
		DHCPAsksBackend: dnsmasqAsksBackend(dhcp),
	}
	results := make(chan func(), 3)
	go func() {
		ok := p.upstream(ctx, settings.Type, settings.Server)
		results <- func() { report.ServerAnswers = ok }
	}()
	go func() {
		ok := p.plain(ctx, p.localAddress)
		results <- func() { report.RouterAnswers = ok }
	}()
	go func() {
		ok := settings.Bootstrap != "" && p.plain(ctx, net.JoinHostPort(settings.Bootstrap, "53"))
		results <- func() { report.BootstrapAnswers = ok }
	}()
	for range 3 {
		(<-results)()
	}
	return report
}

func (p *probe) upstream(ctx context.Context, serverType string, server string) bool {
	host, port, path := plan.SplitServer(server)
	if host == "" {
		return false
	}
	switch serverType {
	case "udp":
		return p.plain(ctx, net.JoinHostPort(host, orDefault(port, "53")))
	case "dot":
		return p.overTLS(ctx, host, orDefault(port, "853"))
	case "doh":
		return p.overHTTPS(ctx, host, orDefault(port, "443"), orDefault(path, "/dns-query"))
	}
	return false
}

func orDefault(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func question() *mDNS.Msg {
	message := new(mDNS.Msg)
	message.SetQuestion(probeName, mDNS.TypeA)
	return message
}

// plain asks over UDP. Any answer counts, a refusal included: the server is there.
func (p *probe) plain(ctx context.Context, address string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	client := &mDNS.Client{Net: "udp", Timeout: probeTimeout}
	_, _, err := client.ExchangeContext(ctx, question(), address)
	return err == nil
}

func (p *probe) overTLS(ctx context.Context, host string, port string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	client := &mDNS.Client{Net: "tcp-tls", Timeout: probeTimeout, TLSConfig: &tls.Config{ServerName: host, RootCAs: p.roots}}
	_, _, err := client.ExchangeContext(ctx, question(), net.JoinHostPort(host, port))
	return err == nil
}

func (p *probe) overHTTPS(ctx context.Context, host string, port string, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	packed, err := question().Pack()
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+net.JoinHostPort(host, port)+path, bytes.NewReader(packed))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/dns-message")
	request.Header.Set("Accept", "application/dns-message")
	transport := &http.Transport{ForceAttemptHTTP2: true, DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: p.roots}}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65535))
	if err != nil {
		return false
	}
	return new(mDNS.Msg).Unpack(body) == nil
}

// maskServer hides the identifier of a NextDNS account, which is a part of the server's
// name or path: the report is shown and copied into bug reports.
func maskServer(server string) string {
	const suffix = ".dns.nextdns.io"
	const prefix = "dns.nextdns.io/"
	if strings.HasSuffix(server, suffix) {
		identifier, _, _ := strings.Cut(server, ".")
		return strings.Repeat("*", len(identifier)) + suffix
	}
	if rest, found := strings.CutPrefix(server, prefix); found {
		return prefix + strings.Repeat("*", len(rest))
	}
	return server
}

// dnsmasqAsksBackend tells whether every dnsmasq of the settings sends its questions to
// the backend and keeps no cache of its own. With no dnsmasq at all the answer is yes, as
// the shell backend has it.
func dnsmasqAsksBackend(dhcp *uci.Package) bool {
	if dhcp == nil {
		return false
	}
	for _, section := range dhcp.SectionsOfType("dnsmasq") {
		asks := false
		for _, server := range strings.Fields(section.Get("server")) {
			if server == dataplane.DNSAddress {
				asks = true
				break
			}
		}
		if section.Get("cachesize") != "0" || section.Get("noresolv") != "1" || !asks {
			return false
		}
	}
	return true
}
