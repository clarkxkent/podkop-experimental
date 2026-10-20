package diag

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/podkop-net/podkop-experimental/src/backend/uci"

	mDNS "github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// answering answers every question with one address.
func answering(writer mDNS.ResponseWriter, request *mDNS.Msg) {
	response := new(mDNS.Msg)
	response.SetReply(request)
	record, _ := mDNS.NewRR(request.Question[0].Name + " 60 IN A 192.0.2.1")
	response.Answer = []mDNS.RR{record}
	writer.WriteMsg(response)
}

func serveUDP(t *testing.T) string {
	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &mDNS.Server{PacketConn: connection, Handler: mDNS.HandlerFunc(answering)}
	go server.ActivateAndServe()
	t.Cleanup(func() { server.Shutdown() })
	return connection.LocalAddr().String()
}

// A server of each kind on this host, and a server that is not there.
func TestCheckDNS(t *testing.T) {
	udp := serveUDP(t)
	_, udpPort, _ := net.SplitHostPort(udp)

	// DNS over HTTPS and over TLS behind the certificate of a test server
	web := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/dns-query" && request.URL.Path != "/custom/path" {
			http.NotFound(writer, request)
			return
		}
		body, _ := io.ReadAll(request.Body)
		question := new(mDNS.Msg)
		if question.Unpack(body) != nil {
			http.Error(writer, "no DNS message", http.StatusBadRequest)
			return
		}
		response := new(mDNS.Msg)
		response.SetReply(question)
		packed, _ := response.Pack()
		writer.Header().Set("Content-Type", "application/dns-message")
		writer.Write(packed)
	}))
	web.EnableHTTP2 = true
	web.StartTLS()
	t.Cleanup(web.Close)
	_, httpsPort, _ := net.SplitHostPort(web.Listener.Addr().String())
	roots := x509.NewCertPool()
	roots.AddCert(web.Certificate())
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: web.TLS.Certificates})
	require.NoError(t, err)
	overTLS := &mDNS.Server{Listener: listener, Net: "tcp-tls", Handler: mDNS.HandlerFunc(answering)}
	go overTLS.ActivateAndServe()
	t.Cleanup(func() { overTLS.Shutdown() })
	_, tlsPort, _ := net.SplitHostPort(listener.Addr().String())

	// a port nothing listens on
	closed, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	_, closedPort, _ := net.SplitHostPort(closed.LocalAddr().String())
	closed.Close()

	trusting := &probe{roots: roots, localAddress: udp}
	for _, test := range []struct {
		settings DNSSettings
		probe    *probe
		want     DNS
	}{
		{DNSSettings{"udp", "127.0.0.1:" + udpPort, "127.0.0.1"}, trusting, DNS{ServerAnswers: true, RouterAnswers: true}},
		{DNSSettings{"udp", "127.0.0.1:" + closedPort, ""}, trusting, DNS{RouterAnswers: true}},
		{DNSSettings{"dot", "127.0.0.1:" + tlsPort, ""}, trusting, DNS{ServerAnswers: true, RouterAnswers: true}},
		{DNSSettings{"doh", "127.0.0.1:" + httpsPort, ""}, trusting, DNS{ServerAnswers: true, RouterAnswers: true}},
		{DNSSettings{"doh", "127.0.0.1:" + httpsPort + "/custom/path", ""}, trusting, DNS{ServerAnswers: true, RouterAnswers: true}},
		{DNSSettings{"doh", "127.0.0.1:" + httpsPort + "/no/such/path", ""}, trusting, DNS{RouterAnswers: true}},
		// the certificate of the server is not trusted: sing-box would not talk to it either
		{DNSSettings{"dot", "127.0.0.1:" + tlsPort, ""}, &probe{localAddress: udp}, DNS{RouterAnswers: true}},
		{DNSSettings{"doh", "127.0.0.1:" + httpsPort, ""}, &probe{localAddress: udp}, DNS{RouterAnswers: true}},
		// a kind of server the backend does not know, and no server at all
		{DNSSettings{"quic", "127.0.0.1:" + udpPort, ""}, trusting, DNS{RouterAnswers: true}},
		{DNSSettings{"udp", "", ""}, &probe{localAddress: "127.0.0.1:" + closedPort}, DNS{}},
	} {
		got := test.probe.check(context.Background(), test.settings, nil)
		want := test.want
		want.Type, want.Server, want.Bootstrap = test.settings.Type, test.settings.Server, test.settings.Bootstrap
		if test.settings.Bootstrap != "" {
			// the bootstrap server is asked at port 53, where this host has nothing
			want.BootstrapAnswers = got.BootstrapAnswers
		}
		require.Equal(t, want, *got, "%+v", test.settings)
	}
}

func TestMaskServer(t *testing.T) {
	t.Parallel()
	for server, want := range map[string]string{
		"1.1.1.1":                    "1.1.1.1",
		"dns.google/dns-query":       "dns.google/dns-query",
		"abc123.dns.nextdns.io":      "******.dns.nextdns.io",
		"dns.nextdns.io/abc123":      "dns.nextdns.io/******",
		"dns.nextdns.io/abc123/home": "dns.nextdns.io/***********",
		"dns.nextdns.io":             "dns.nextdns.io",
	} {
		require.Equal(t, want, maskServer(server))
	}
}

func TestDnsmasqAsksBackend(t *testing.T) {
	t.Parallel()
	for content, want := range map[string]bool{
		"config dnsmasq\n\tlist server '127.0.0.42'\n\toption noresolv '1'\n\toption cachesize '0'\n":                           true,
		"config dnsmasq\n\tlist server '9.9.9.9'\n\tlist server '127.0.0.42'\n\toption noresolv '1'\n\toption cachesize '0'\n":  true,
		"config dnsmasq\n\tlist server '127.0.0.42'\n\toption noresolv '1'\n\toption cachesize '150'\n":                         false,
		"config dnsmasq\n\tlist server '127.0.0.42'\n\toption noresolv '0'\n\toption cachesize '0'\n":                           false,
		"config dnsmasq\n\tlist server '127.0.0.4'\n\toption noresolv '1'\n\toption cachesize '0'\n":                            false,
		"config dnsmasq\n\toption noresolv '1'\n\toption cachesize '0'\n":                                                       false,
		"config dnsmasq\n\tlist server '127.0.0.42'\n\toption noresolv '1'\n\toption cachesize '0'\n\nconfig dnsmasq 'guest'\n": false,
		"config dhcp 'lan'\n\toption interface 'lan'\n":                                                                         true,
	} {
		parsed, err := uci.Parse([]byte(content))
		require.NoError(t, err)
		require.Equal(t, want, dnsmasqAsksBackend(parsed), content)
	}
	require.False(t, dnsmasqAsksBackend(nil))
}
