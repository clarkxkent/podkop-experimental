package diag

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The lines are those nft monitor trace wrote on the stand (rs-lab/pk08/diag) for each way a
// packet of a device check was lost, cut to what the reading needs.
var traceHooks = map[string]string{
	"inet PodkopTrace trace": "prerouting", "inet fw4 raw_prerouting": "prerouting", "inet PodkopTable mangle": "prerouting",
	"inet fw4 mangle_prerouting": "prerouting", "inet PodkopTable proxy": "prerouting", "inet fw4 dstnat": "prerouting",
	"inet fw4 prerouting": "prerouting", "inet foreign pre": "prerouting", "inet fw4 mangle_input": "input",
	"inet fw4 input": "input", "inet fw4 forward": "forward", "inet fw4 mangle_forward": "forward",
	"inet fw4 mangle_postrouting": "postrouting", "inet fw4 srcnat": "postrouting",
}

const (
	packetSYN = `iif "eth1" ether saddr 1a:a5:fb:fb:8e:ec ip saddr 192.168.78.10 ip daddr 198.18.0.4 ip protocol tcp tcp sport 50000 tcp dport 40001 tcp flags == syn tcp window 64240`
	packetACK = `iif "eth1" ether saddr 1a:a5:fb:fb:8e:ec ip saddr 192.168.78.10 ip daddr 198.18.0.4 ip protocol tcp tcp sport 50000 tcp dport 40001 tcp flags == ack tcp window 502`
	ourMark   = `iifname @interfaces ip daddr 198.18.0.0/15 meta l4proto tcp meta mark set 0x00100000 counter packets 5 bytes 350 (verdict continue)`
	ourProxy  = `meta mark & 0x00100000 == 0x00100000 meta l4proto tcp tproxy ip to 127.0.0.1:1602 counter packets 5 bytes 350 (verdict continue)`
	// a DNS question for the check name, to the router and to a server past it
	questionRouter = `iif "eth1" ether saddr 1a:a5:fb:fb:8e:ec ip saddr 192.168.78.10 ip daddr 192.168.78.1 ip dscp cs0 ip ecn not-ect ip ttl 64 ip id 4411 ip protocol udp ip length 82 udp sport 41234 udp dport 53 udp length 62 @th,64,96 0x7a3b01000001000000000000`
	questionPast   = `iif "eth1" ether saddr 1a:a5:fb:fb:8e:ec ip saddr 192.168.78.10 ip daddr 8.8.8.8 ip dscp cs0 ip ecn not-ect ip ttl 64 ip id 4412 ip protocol udp ip length 82 udp sport 41235 udp dport 53 udp length 62 @th,64,96 0x7a3c01000001000000000000`
)

func traceLines(id string, entries ...string) []string {
	var lines []string
	for _, entry := range entries {
		lines = append(lines, "trace id "+id+" "+entry)
	}
	return lines
}

// the start of every traced packet: the chain of the trace table and the rules of podkop
func start(id string, packet string, proxied bool) []string {
	lines := traceLines(id,
		"inet PodkopTrace trace packet: "+packet,
		"inet PodkopTrace trace rule ip daddr 198.18.0.0/15 tcp dport 40001 meta nftrace set 1 (verdict continue)",
		"inet PodkopTrace trace policy accept ",
		"inet fw4 raw_prerouting packet: "+packet, "inet fw4 raw_prerouting policy accept ",
		"inet PodkopTable mangle packet: "+packet, "inet PodkopTable mangle rule "+ourMark,
		"inet PodkopTable mangle policy accept meta mark 0x00100000 ")
	if proxied {
		lines = append(lines, traceLines(id, "inet PodkopTable proxy packet: "+packet, "inet PodkopTable proxy rule "+ourProxy,
			"inet PodkopTable proxy policy accept meta mark 0x00100000 ")...)
	}
	return lines
}

func join(parts ...[]string) []string {
	var lines []string
	for _, part := range parts {
		lines = append(lines, part...)
	}
	return lines
}

func TestTraceStories(t *testing.T) {
	for _, test := range []struct {
		name  string
		lines []string
		want  []string
	}{{
		"to the router",
		join(start("a1", packetSYN, true), traceLines("a1", "inet fw4 input packet: "+packetSYN,
			`inet fw4 accept_from_lan rule iifname "eth1" accept comment "!fw4: accept lan IPv4/IPv6 traffic" (verdict accept)`)),
		[]string{"SYN (1): marked by podkop → taken by the transparent proxy rule of podkop → reached the router (hook input)"},
	}, {
		"a mark of another table between the marking and the transparent proxy",
		join(start("b1", packetSYN, false), traceLines("b1", "inet foreign pre packet: "+packetSYN,
			"inet foreign pre rule ip daddr 198.18.0.0/15 meta mark set 0x00000001 (verdict continue)",
			"inet PodkopTable proxy packet: "+packetSYN, "inet PodkopTable proxy policy accept meta mark 0x00000001 ",
			"inet fw4 forward packet: "+packetSYN,
			`inet fw4 accept_to_wan rule oifname "eth2" accept comment "!fw4: accept wan IPv4/IPv6 traffic" (verdict accept)`)),
		[]string{"SYN (1): marked by podkop → its mark replaced by inet foreign pre: ip daddr 198.18.0.0/15 meta mark set 0x00000001 → not taken by the transparent proxy rule of podkop → routed past the router (hook forward)"},
	}, {
		"a mark of another table set before the one of podkop is no matter",
		join(traceLines("c1", "inet PodkopTrace trace packet: "+packetSYN, "inet foreign pre packet: "+packetSYN,
			"inet foreign pre rule ip daddr 198.18.0.0/15 meta mark set 0x00200000 (verdict continue)",
			"inet PodkopTable mangle packet: "+packetSYN, "inet PodkopTable mangle rule "+ourMark,
			"inet PodkopTable proxy packet: "+packetSYN, "inet PodkopTable proxy rule "+ourProxy, "inet fw4 input packet: "+packetSYN)),
		[]string{"SYN (1): marked by podkop → taken by the transparent proxy rule of podkop → reached the router (hook input)"},
	}, {
		"a mark replaced after the transparent proxy: lost after the routing",
		join(start("d1", packetSYN, true), traceLines("d1", "inet fw4 prerouting packet: "+packetSYN, "inet foreign pre packet: "+packetSYN,
			"inet foreign pre rule ip daddr 198.18.0.0/15 meta mark set 0x00000001 (verdict continue)")),
		[]string{"SYN (1): marked by podkop → taken by the transparent proxy rule of podkop → its mark replaced by inet foreign pre: ip daddr 198.18.0.0/15 meta mark set 0x00000001 → lost after the routing: neither hook input nor forward"},
	}, {
		"dropped by another table",
		join(traceLines("e1", "inet PodkopTrace trace packet: "+packetSYN, "inet foreign pre packet: "+packetSYN,
			"inet foreign pre rule ip daddr 198.18.0.0/15 drop (verdict drop)")),
		[]string{"SYN (1): not marked by podkop → dropped by inet foreign pre: ip daddr 198.18.0.0/15 drop"},
	}, {
		"a zone of fw4 that rejects input",
		join(start("f1", packetSYN, true), traceLines("f1", "inet fw4 input packet: "+packetSYN,
			"inet fw4 input_lan rule jump reject_from_lan (verdict jump reject_from_lan)",
			`inet fw4 handle_reject rule meta l4proto tcp reject with tcp reset comment "!fw4: Reject TCP traffic" (verdict drop)`)),
		[]string{`SYN (1): marked by podkop → taken by the transparent proxy rule of podkop → dropped by inet fw4 handle_reject: meta l4proto tcp reject with tcp reset comment "!fw4: Reject TCP traffic"`},
	}, {
		// the SYN came to sing-box; the ACK was taken by another transparent proxy, which
		// set its own mark: the connection never came up. Two packets with one trace id.
		"another transparent proxy takes a later packet",
		join(start("g1", packetSYN, true), traceLines("g1", "inet fw4 input packet: "+packetSYN),
			traceLines("g1", "inet PodkopTrace trace packet: "+packetACK, "inet PodkopTable mangle packet: "+packetACK, "inet PodkopTable mangle rule "+ourMark,
				"inet foreign pre packet: "+packetACK,
				"inet foreign pre rule ip daddr 198.18.0.0/15 meta l4proto tcp tproxy ip to 127.0.0.1:7777 meta mark set 0x00000077 accept (verdict accept)",
				"inet fw4 prerouting packet: "+packetACK)),
		[]string{
			"SYN (1): marked by podkop → taken by the transparent proxy rule of podkop → reached the router (hook input)",
			"ACK (1): marked by podkop → taken by inet foreign pre: ip daddr 198.18.0.0/15 meta l4proto tcp tproxy ip to 127.0.0.1:7777 meta mark set 0x00000077 accept → not taken by the transparent proxy rule of podkop → lost after the routing: neither hook input nor forward",
		},
	}, {
		"what is no line of a trace is passed over; the same story of two packets is told once",
		join([]string{"", "garbage", "trace id x"}, start("h1", packetSYN, true), traceLines("h1", "inet fw4 input packet: "+packetSYN),
			start("h2", packetSYN, true), traceLines("h2", "inet fw4 input packet: "+packetSYN)),
		[]string{"SYN (2): marked by podkop → taken by the transparent proxy rule of podkop → reached the router (hook input)"},
	}} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, TraceStories(test.lines, traceHooks, false))
		})
	}
}

// The DNS questions of a check are told apart from the packets of its connection: what
// became of them, and the addresses they were sent to.
func TestTraceQuestions(t *testing.T) {
	question := func(id string, packet string, entries ...string) []string {
		return traceLines(id, append([]string{"inet PodkopTrace trace packet: " + packet,
			"inet PodkopTrace trace rule udp dport 53 @th,160,128 0x14366530 meta nftrace set 1 (verdict continue)",
			"inet fw4 raw_prerouting packet: " + packet, "inet PodkopTable mangle packet: " + packet}, entries...)...)
	}
	for _, test := range []struct {
		name  string
		lines []string
		want  []string
	}{{
		"to the DNS server of the router",
		join(question("q1", questionRouter, "inet fw4 input packet: "+questionRouter,
			`inet fw4 accept_from_lan rule iifname "eth1" accept comment "!fw4: accept lan IPv4/IPv6 traffic" (verdict accept)`),
			question("q2", questionRouter, "inet fw4 input packet: "+questionRouter)),
		[]string{"to 192.168.78.1 (2): reached the router (hook input)"},
	}, {
		"a zone of fw4 that rejects input",
		question("q3", questionRouter, "inet fw4 input packet: "+questionRouter,
			"inet fw4 input_lan rule jump reject_from_lan (verdict jump reject_from_lan)",
			`inet fw4 handle_reject rule reject comment "!fw4: Reject any other traffic" (verdict drop)`),
		[]string{`to 192.168.78.1 (1): dropped by inet fw4 handle_reject: reject comment "!fw4: Reject any other traffic"`},
	}, {
		"sent to another server by a rule of fw4",
		question("q4", questionRouter, "inet fw4 dstnat packet: "+questionRouter,
			`inet fw4 dstnat_lan rule meta nfproto ipv4 udp dport 53 counter packets 3 bytes 180 redirect to :5353 comment "!fw4: Intercept-DNS" (verdict accept)`,
			"inet fw4 input packet: "+questionRouter),
		[]string{`to 192.168.78.1 (1): sent elsewhere by inet fw4 dstnat_lan: meta nfproto ipv4 udp dport 53 redirect to :5353 comment "!fw4: Intercept-DNS" → reached the router (hook input)`},
	}, {
		// as fw4 writes them: the redirect, then the rule of the zone that accepts what was
		// redirected, which a later question of the same flow passes alone
		"a later question of a flow sent to another server",
		join(question("q9", questionRouter, "inet fw4 dstnat packet: "+questionRouter,
			`inet fw4 dstnat_lan rule meta nfproto ipv4 udp dport 53 redirect to :5353 comment "!fw4: Intercept-DNS" (verdict accept)`,
			"inet fw4 input packet: "+questionRouter, `inet fw4 input_lan rule ct status dnat accept comment "!fw4: Accept port redirections" (verdict accept)`),
			question("q10", questionRouter, "inet fw4 input packet: "+questionRouter, `inet fw4 input_lan rule ct status dnat accept comment "!fw4: Accept port redirections" (verdict accept)`)),
		[]string{
			`to 192.168.78.1 (1): sent elsewhere by inet fw4 dstnat_lan: meta nfproto ipv4 udp dport 53 redirect to :5353 comment "!fw4: Intercept-DNS" → reached the router (hook input)`,
			`to 192.168.78.1 (1): sent elsewhere as the first question of its flow was (inet fw4 input_lan: ct status dnat accept comment "!fw4: Accept port redirections") → reached the router (hook input)`,
		},
	}, {
		"to a server past the router, while a connection of the check is traced too",
		join(question("q5", questionPast, "inet fw4 forward packet: "+questionPast), start("q6", packetSYN, true),
			traceLines("q6", "inet fw4 input packet: "+packetSYN)),
		[]string{"to 8.8.8.8 (1): routed past the router (hook forward)"},
	}} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, TraceStories(test.lines, traceHooks, true))
		})
	}
	require.Empty(t, TraceStories(start("q7", packetSYN, true), traceHooks, true))
	require.Equal(t, netip.MustParseAddr("192.168.78.10"), TraceSource(question("q8", questionPast), traceHooks))
	require.False(t, TraceSource([]string{"garbage"}, traceHooks).IsValid())
}

func TestRedirects(t *testing.T) {
	for text, want := range map[string]bool{
		`meta nfproto ipv4 udp dport 53 redirect to :5353 comment "!fw4: Intercept-DNS"`: true,
		`udp dport 53 redirect`:                                          true,
		`udp dport 53 dnat ip to 192.168.1.2:53`:                         true,
		`udp dport 53 dnat to 192.168.1.2`:                               true,
		`meta l4proto udp tproxy ip6 to [::1]:7777`:                      true,
		`ct status dnat accept comment "!fw4: Accept port redirections"`: false,
		`jump redirect_lan`:                                              false,
	} {
		require.Equal(t, want, redirects.MatchString(text), text)
	}
}

func TestTCPFlags(t *testing.T) {
	require.Equal(t, "syn", tcpFlags(packetSYN))
	require.Equal(t, "fin | ack", tcpFlags("ip protocol tcp tcp sport 1 tcp dport 2 tcp flags == fin | ack tcp window 9"))
	// nft 1.1.1 of OpenWrt 24.10 writes a set of several flags as a number
	require.Equal(t, "psh | ack", tcpFlags("ip protocol tcp tcp sport 1 tcp dport 2 tcp flags == 0x18 tcp window 9"))
	require.Equal(t, "fin | ack", tcpFlags("ip protocol tcp tcp sport 1 tcp dport 2 tcp flags == 0x11 tcp window 9"))
	require.Equal(t, "tcp", tcpFlags("ip protocol udp udp sport 53"))
	require.True(t, strings.HasPrefix(packetACK, "iif"))
	require.Equal(t, netip.MustParseAddr("fd00::10"), packetAddress("iif \"eth1\" ip6 saddr fd00::10 ip6 daddr fd00::1 ip6 nexthdr udp", "saddr"))
}
