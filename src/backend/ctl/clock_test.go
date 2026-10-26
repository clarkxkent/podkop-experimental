package ctl

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/clock"

	"github.com/stretchr/testify/require"
)

// The clock lines: set by sysntpd and when; not set, with what keeps it: sysntpd off, a
// name not resolved or resolved from FakeIP, no NTP answer, or an answer with the offset.
func TestClockLines(t *testing.T) {
	now := time.Unix(1793000000, 0)
	facts := clockFacts{
		state:   func() clock.State { return clock.State{Synced: true, LastSync: now.Add(-7 * time.Minute)} },
		enabled: func() bool { return true },
		servers: func() []string { return []string{"0.pool.lab.example", "1.pool.lab.example", "192.0.2.123"} },
		lookup: func(ctx context.Context, name string) ([]netip.Addr, error) {
			switch name {
			case "0.pool.lab.example":
				return []netip.Addr{netip.MustParseAddr("198.18.0.9")}, nil
			case "1.pool.lab.example":
				return []netip.Addr{netip.MustParseAddr("192.0.2.124")}, nil
			}
			return nil, errors.New("no such name")
		},
		ask: func(ctx context.Context, addresses []string) (clock.Result, error) {
			require.Equal(t, []string{"192.0.2.124:123", "192.0.2.123:123"}, addresses)
			return clock.Result{Server: "192.0.2.124:123", Offset: 90 * time.Minute}, nil
		},
		now: func() time.Time { return now },
	}
	ctx := context.Background()
	require.Equal(t, []string{"✅ The router's NTP service (sysntpd) sets the clock, last 7 min ago"}, clockLines(ctx, facts))

	facts.state = func() clock.State {
		return clock.State{Synced: true, LastSync: now.Add(-3 * time.Hour), Event: clock.Event{At: now.Add(-2 * time.Hour), Action: "unsync", Stratum: 16}}
	}
	require.Equal(t, []string{"✅ The router's NTP service (sysntpd) sets the clock, last 3 h ago", "⚠️ It lost every NTP server 2 h ago"}, clockLines(ctx, facts))

	facts.state = func() clock.State { return clock.State{} }
	require.Equal(t, []string{
		"❌ The router's NTP service (sysntpd) has not set the clock since the boot: TLS and REALITY refuse a wrong clock",
		"❌ 0.pool.lab.example resolves to a FakeIP address: its NTP goes through the proxy",
		"⚠️ The NTP server 192.0.2.124:123 answers: the clock is 1h30m0s behind",
	}, clockLines(ctx, facts))

	facts.ask = func(ctx context.Context, addresses []string) (clock.Result, error) {
		return clock.Result{}, errors.New("timeout")
	}
	require.Equal(t, "❌ No NTP server answers on UDP 123: 0.pool.lab.example, 1.pool.lab.example, 192.0.2.123", clockLines(ctx, facts)[2])

	facts.enabled = func() bool { return false }
	require.Equal(t, []string{
		"❌ The router's NTP service (sysntpd) has not set the clock since the boot: TLS and REALITY refuse a wrong clock",
		"⚠️ sysntpd is off: System → System → Time Synchronization",
	}, clockLines(ctx, facts))
	require.Equal(t, "3 days", ago(80*time.Hour))
}
