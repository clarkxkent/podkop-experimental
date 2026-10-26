package ctl

import (
	"context"
	"math"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/clock"
	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
)

// clockFacts are what the lines of the clock read of the router; a test has its own.
type clockFacts struct {
	state   func() clock.State
	enabled func() bool
	servers func() []string
	// lookup resolves a name as sysntpd does, by the DNS of the router
	lookup func(ctx context.Context, name string) ([]netip.Addr, error)
	// ask asks NTP servers for the time and sets nothing
	ask func(ctx context.Context, addresses []string) (clock.Result, error)
	now func() time.Time
}

var routerClockFacts = clockFacts{
	state:   clock.ReadState,
	enabled: clock.Enabled,
	servers: clock.Servers,
	lookup: func(ctx context.Context, name string) ([]netip.Addr, error) {
		return net.DefaultResolver.LookupNetIP(ctx, "ip4", name)
	},
	ask: func(ctx context.Context, addresses []string) (clock.Result, error) {
		// a threshold no offset reaches: the clock is not set
		return clock.Sync(ctx, addresses, time.Duration(math.MaxInt64), func(time.Time) error { return nil })
	},
	now: time.Now,
}

// clockAskWait bounds the question to the NTP servers.
const clockAskWait = 3 * time.Second

// clockLines tell whether the router's NTP service has set the clock since the boot, as its
// hook and that of dnsmasq tell (clock.State), and when it has not, what keeps it: the
// service off, its names not resolved or resolved to FakeIP addresses, no NTP server
// answering on UDP 123.
func clockLines(ctx context.Context, facts clockFacts) []string {
	state := facts.state()
	if state.Synced {
		line := "✅ The router's NTP service (sysntpd) sets the clock"
		if !state.LastSync.IsZero() {
			line += ", last " + ago(facts.now().Sub(state.LastSync)) + " ago"
		}
		lines := []string{line}
		if state.Event.Action == "unsync" && !state.Event.At.Before(state.LastSync.Truncate(time.Second)) {
			lines = append(lines, "⚠️ It lost every NTP server "+ago(facts.now().Sub(state.Event.At))+" ago")
		}
		return lines
	}
	lines := []string{"❌ The router's NTP service (sysntpd) has not set the clock since the boot: TLS and REALITY refuse a wrong clock"}
	if !facts.enabled() {
		return append(lines, "⚠️ sysntpd is off: System → System → Time Synchronization")
	}
	servers := facts.servers()
	if len(servers) == 0 {
		return append(lines, "❌ sysntpd has no NTP server")
	}
	var addresses []string
	for _, server := range servers {
		if address, err := netip.ParseAddr(server); err == nil {
			addresses = append(addresses, net.JoinHostPort(address.String(), "123"))
			continue
		}
		found, err := facts.lookup(ctx, server)
		switch {
		case err != nil || len(found) == 0:
			lines = append(lines, "❌ "+server+" is not resolved by the DNS of the router")
		case dataplane.FakeIPPrefix.Contains(found[0]):
			lines = append(lines, "❌ "+server+" resolves to a FakeIP address: its NTP goes through the proxy")
		default:
			addresses = append(addresses, net.JoinHostPort(found[0].String(), "123"))
		}
	}
	if len(addresses) == 0 {
		return lines
	}
	ctx, cancel := context.WithTimeout(ctx, clockAskWait)
	defer cancel()
	result, err := facts.ask(ctx, addresses)
	if err != nil {
		return append(lines, "❌ No NTP server answers on UDP 123: "+strings.Join(servers, ", "))
	}
	way := "behind"
	offset := result.Offset
	if offset < 0 {
		way, offset = "ahead", -offset
	}
	return append(lines, "⚠️ The NTP server "+result.Server+" answers: the clock is "+offset.Round(time.Second).String()+" "+way)
}

// ago writes a duration as minutes, hours or days.
func ago(duration time.Duration) string {
	switch {
	case duration < time.Hour:
		return strconv.Itoa(int(duration/time.Minute)) + " min"
	case duration < 48*time.Hour:
		return strconv.Itoa(int(duration/time.Hour)) + " h"
	default:
		return strconv.Itoa(int(duration/(24*time.Hour))) + " days"
	}
}
