package clock

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The hook's files: the last event, the time of the last sync; the sign of dnsmasq alone
// tells a sync without its time.
func TestReadState(t *testing.T) {
	directory := t.TempDir()
	paths := statePaths{filepath.Join(directory, "event"), filepath.Join(directory, "synced"), filepath.Join(directory, "dnsmasqsec")}
	require.Equal(t, State{}, paths.read())

	require.NoError(t, os.WriteFile(paths.event, []byte("1793000000 unsync - 16\n"), 0o644))
	state := paths.read()
	require.False(t, state.Synced)
	require.Equal(t, Event{At: time.Unix(1793000000, 0), Action: "unsync", Stratum: 16}, state.Event)

	require.NoError(t, os.WriteFile(paths.dnsmasq, []byte("ntpd says time is valid\n"), 0o644))
	state = paths.read()
	require.True(t, state.Synced)
	require.True(t, state.LastSync.IsZero())

	require.NoError(t, os.WriteFile(paths.synced, nil, 0o644))
	at := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	require.NoError(t, os.Chtimes(paths.synced, at, at))
	require.NoError(t, os.WriteFile(paths.event, []byte("1793000600 periodic -0.002 2\n"), 0o644))
	state = paths.read()
	require.True(t, state.Synced)
	require.True(t, at.Equal(state.LastSync))
	require.Equal(t, Event{At: time.Unix(1793000600, 0), Action: "periodic", Offset: "-0.002", Stratum: 2}, state.Event)

	require.Equal(t, Event{}, parseEvent("garbage"))
}

const sysntpdService = `{"sysntpd": {"instances": {"ntpd": {"running": true, "command": ["/usr/sbin/ntpd", "-n", "-N", "-S", "/usr/sbin/ntpd-hotplug", "-p", "0.openwrt.pool.ntp.org", "-p", "1.openwrt.pool.ntp.org", "-p", "10.0.0.123", "-p", "0.openwrt.pool.ntp.org"]}}}}`

const systemSettings = `
config system
	option hostname 'OpenWrt'

config timeserver 'ntp'
	option enabled '1'
	list server '2.openwrt.pool.ntp.org'
	list server '3.openwrt.pool.ntp.org'
`

// fakeSysntpd is sysntpd with what ubus answers, the settings and the init script given,
// and the commands run told.
func fakeSysntpd(t *testing.T, service string, settings string) (sysntpd, *[]string) {
	directory := t.TempDir()
	system := filepath.Join(directory, "system")
	require.NoError(t, os.WriteFile(system, []byte(settings), 0o644))
	initScript := filepath.Join(directory, "sysntpd")
	require.NoError(t, os.WriteFile(initScript, []byte("#!/bin/sh\n"), 0o755))
	var access sync.Mutex
	var commands []string
	return sysntpd{
		ubus: func(ctx context.Context, object string, method string, message string) ([]byte, error) {
			if service == "" {
				return nil, errors.New("no ubus")
			}
			return []byte(service), nil
		},
		systemPath: system,
		initScript: initScript,
		run: func(ctx context.Context, name string, arguments ...string) error {
			access.Lock()
			defer access.Unlock()
			commands = append(commands, strings.Join(arguments, " "))
			if strings.Contains(settings, "disabled-script") && arguments[0] == "enabled" {
				return errors.New("exit status 1")
			}
			return nil
		},
	}, &commands
}

// The servers are those of the running ntpd, each once, or those of the settings when it
// does not run; it is off when the settings or its init script say so.
func TestSysntpdServers(t *testing.T) {
	ctx := context.Background()
	service, _ := fakeSysntpd(t, sysntpdService, systemSettings)
	require.Equal(t, []string{"0.openwrt.pool.ntp.org", "1.openwrt.pool.ntp.org", "10.0.0.123"}, service.servers(ctx))
	require.True(t, service.enabled(ctx))

	service, _ = fakeSysntpd(t, "", systemSettings)
	require.Equal(t, []string{"2.openwrt.pool.ntp.org", "3.openwrt.pool.ntp.org"}, service.servers(ctx))
	service, _ = fakeSysntpd(t, `{}`, systemSettings)
	require.Equal(t, []string{"2.openwrt.pool.ntp.org", "3.openwrt.pool.ntp.org"}, service.servers(ctx))

	service, _ = fakeSysntpd(t, sysntpdService, strings.Replace(systemSettings, "option enabled '1'", "option enabled '0'", 1))
	require.False(t, service.enabled(ctx))
	service, _ = fakeSysntpd(t, sysntpdService, systemSettings+"# disabled-script\n")
	require.False(t, service.enabled(ctx))
	service.initScript = filepath.Join(t.TempDir(), "none")
	require.False(t, service.enabled(ctx))
}

// certificateAuthority makes a CA and a certificate of it for 127.0.0.1 valid from notBefore
// to notAfter.
func certificateAuthority(t *testing.T, notBefore time.Time, notAfter time.Time) (*x509.CertPool, tls.Certificate) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "lab CA"},
		NotBefore: notBefore.Add(-365 * 24 * time.Hour), NotAfter: notAfter.Add(365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: notBefore, NotAfter: notAfter, IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature,
	}, ca, &key.PublicKey, caKey)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return roots, tls.Certificate{Certificate: [][]byte{leafDER}, PrivateKey: key}
}

// a DoH server whose answers tell the time date (whatever the request asks)
func dohServer(t *testing.T, certificate tls.Certificate, date time.Time) string {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Date", date.UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusBadRequest)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server.Listener.Addr().String()
}

// The time of an HTTPS answer is taken when the certificate is valid at that time: a
// certificate that has long expired by the clock of the router is fine when the server
// tells a time it was valid at; a time it was not valid at, or a certificate of another
// name or of an unknown authority, is refused.
func TestHTTPSTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	past := time.Now().Add(-90 * 24 * time.Hour)
	roots, certificate := certificateAuthority(t, past.Add(-24*time.Hour), past.Add(24*time.Hour))

	address := dohServer(t, certificate, past)
	told, err := HTTPSTime(ctx, address, "127.0.0.1", "/dns-query", roots)
	require.NoError(t, err)
	require.WithinDuration(t, past, told, time.Second)

	// valid at the time the router has, not at the time the server tells
	address = dohServer(t, certificate, time.Now())
	_, err = HTTPSTime(ctx, address, "127.0.0.1", "/dns-query", roots)
	require.ErrorContains(t, err, "the certificate at the time the server tells")

	address = dohServer(t, certificate, past)
	_, err = HTTPSTime(ctx, address, "dns.lab.example", "/dns-query", roots)
	require.Error(t, err)
	_, err = HTTPSTime(ctx, address, "127.0.0.1", "/dns-query", x509.NewCertPool())
	require.Error(t, err)
}

func TestSplitDoH(t *testing.T) {
	for server, want := range map[string][3]string{
		"1.1.1.1":                           {"1.1.1.1", "443", "/dns-query"},
		"9.9.9.9:5053/custom":               {"9.9.9.9", "5053", "/custom"},
		"https://dns.lab.example/dns-query": {"dns.lab.example", "443", "/dns-query"},
		"dns.lab.example:8443":              {"dns.lab.example", "8443", "/dns-query"},
	} {
		host, port, path := splitDoH(server)
		require.Equal(t, want, [3]string{host, port, path}, server)
	}
}

// keeperFixture is a Keeper with the router faked: the state of sysntpd, its service, the
// clock, the DNS. ntpServer is the address of an NTP server ("127.0.0.1:port") its names
// resolve to.
type keeperFixture struct {
	keeper   *Keeper
	commands *[]string
	setTo    *time.Time
	lines    *[]string
	synced   *bool
}

func newKeeperFixture(t *testing.T, ntpServer string, routerDNS func(name string) []netip.Addr) keeperFixture {
	host, port, err := net.SplitHostPort(ntpServer)
	require.NoError(t, err)
	service, commands := fakeSysntpd(t, `{"sysntpd": {"instances": {"ntpd": {"command": ["/usr/sbin/ntpd", "-p", "pool.lab.example"]}}}}`, systemSettings)
	var access sync.Mutex
	setTo, lines, synced := new(time.Time), new([]string), new(bool)
	keeper := &Keeper{
		Log: func(level string, message string) {
			access.Lock()
			defer access.Unlock()
			*lines = append(*lines, level+" "+message)
		},
		UplinkDNS: func() []string { return []string{"192.0.2.53"} },
		state: func() State {
			access.Lock()
			defer access.Unlock()
			return State{Synced: *synced}
		},
		sysntpd: service,
		set: func(now time.Time) error {
			access.Lock()
			defer access.Unlock()
			*setTo = now
			return nil
		},
		lookup: func(ctx context.Context, server string, name string) ([]netip.Addr, error) {
			if server == "" {
				return routerDNS(name), nil
			}
			if server == "192.0.2.53" && name == "pool.lab.example" {
				return []netip.Addr{netip.MustParseAddr(host)}, nil
			}
			return nil, errors.New("no such name")
		},
		ntpPort:   port,
		askWait:   time.Second,
		waitDNS:   300 * time.Millisecond,
		tryDNS:    100 * time.Millisecond,
		checkGap:  time.Hour,
		restartAt: time.Hour,
	}
	return keeperFixture{keeper, commands, setTo, lines, synced}
}

func (f keeperFixture) restarts() int {
	count := 0
	for _, command := range *f.commands {
		if command == "restart" {
			count++
		}
	}
	return count
}

// A: a clock two hours behind is set from the NTP server of sysntpd, its name resolved by
// the DNS server of the uplink; B: sysntpd is restarted once the DNS of the router answers
// its name, and not again within the gap. A synced clock is left alone.
func TestKeeperFromNTP(t *testing.T) {
	routerDNS := func(name string) []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.0.2.123")} }
	fixture := newKeeperFixture(t, fakeServer(t, 2*time.Hour), routerDNS)
	fixture.keeper.AtStart(context.Background())
	require.WithinDuration(t, time.Now().Add(2*time.Hour), *fixture.setTo, 2*time.Second)
	require.Len(t, *fixture.lines, 1)
	require.Contains(t, (*fixture.lines)[0], "warn the clock was 2h0m0s behind; it is set from the NTP server 127.0.0.1:")
	require.Zero(t, fixture.restarts(), "sysntpd restarted before sing-box runs")

	fixture.keeper.run.Lock()
	fixture.keeper.check(context.Background(), true)
	fixture.keeper.run.Unlock()
	require.Equal(t, 1, fixture.restarts())
	require.Len(t, *fixture.lines, 2, "A ran again within the gap")
	fixture.keeper.run.Lock()
	fixture.keeper.check(context.Background(), true)
	fixture.keeper.run.Unlock()
	require.Equal(t, 1, fixture.restarts(), "restarted again within the gap")

	*fixture.synced = true
	*fixture.setTo = time.Time{}
	fixture.keeper.due.Store(true)
	fixture.keeper.restarted = time.Time{}
	fixture.keeper.run.Lock()
	fixture.keeper.check(context.Background(), true)
	fixture.keeper.run.Unlock()
	require.True(t, fixture.setTo.IsZero())
	require.Equal(t, 1, fixture.restarts())
}

// C: no NTP server answers (UDP 123 closed), the time of the HTTPS answer of the DoH
// server of the settings sets a clock months behind. B waits for the DNS of the router:
// a name answered from FakeIP is not an answer, and sysntpd is not restarted.
func TestKeeperFromHTTPS(t *testing.T) {
	future := time.Now().Add(90 * 24 * time.Hour)
	roots, certificate := certificateAuthority(t, future.Add(-24*time.Hour), future.Add(24*time.Hour))
	doh := dohServer(t, certificate, future)
	silent, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer silent.Close()
	routerDNS := func(name string) []netip.Addr { return []netip.Addr{netip.MustParseAddr("198.18.0.7")} }
	fixture := newKeeperFixture(t, silent.LocalAddr().String(), routerDNS)
	fixture.keeper.roots = roots
	fixture.keeper.DoH = func() string { return doh + "/dns-query" }
	fixture.keeper.run.Lock()
	fixture.keeper.check(context.Background(), true)
	fixture.keeper.run.Unlock()
	require.WithinDuration(t, future, *fixture.setTo, 2*time.Second)
	require.Contains(t, strings.Join(*fixture.lines, "\n"), "it is set from the HTTPS answer of the DoH server 127.0.0.1 (no NTP server of sysntpd answered)")
	require.Zero(t, fixture.restarts(), "restarted while the DNS answers from FakeIP")

	// no DoH in the settings: what failed is told
	fixture.keeper.DoH = nil
	fixture.keeper.due.Store(true)
	fixture.keeper.run.Lock()
	fixture.keeper.check(context.Background(), false)
	fixture.keeper.run.Unlock()
	last := (*fixture.lines)[len(*fixture.lines)-1]
	require.Contains(t, last, "info the clock is not checked: no NTP server of sysntpd answered")
	require.Contains(t, last, "the DNS server of the settings is not DoH")
}

// sysntpd turned off by the user: nothing is asked, set or restarted, and that is told once.
func TestKeeperSysntpdOff(t *testing.T) {
	fixture := newKeeperFixture(t, fakeServer(t, 2*time.Hour), func(string) []netip.Addr { return nil })
	service, commands := fakeSysntpd(t, sysntpdService, strings.Replace(systemSettings, "option enabled '1'", "option enabled '0'", 1))
	fixture.keeper.sysntpd, fixture.commands = service, commands
	fixture.keeper.AtStart(context.Background())
	fixture.keeper.AtStart(context.Background())
	require.True(t, fixture.setTo.IsZero())
	require.Equal(t, []string{"info the clock is not checked: the NTP service of the router (sysntpd) is off"}, *fixture.lines)
	require.Equal(t, []string{"enabled", "enabled"}, *fixture.commands)
}
