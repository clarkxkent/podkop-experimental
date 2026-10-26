// Package clock helps the router's NTP service, sysntpd (BusyBox ntpd), to set the clock;
// it does not keep the clock itself. Most routers have no clock of their own: one boots with
// the time of the newest file of /etc (sysfixtime), which may be weeks behind, and TLS and
// REALITY refuse what such a clock says. sysntpd finds its servers by name through dnsmasq,
// which after a power loss points at sing-box before it is there, and with DoH or DoT the
// names need the TLS the clock breaks; it notices that the DNS works only at its next try,
// minutes later, and logs nothing.
//
// What is done, and only while sysntpd has not set the clock since the boot:
//
//   - A: the servers of sysntpd itself are asked once, their names resolved by the DNS
//     servers of the uplinks directly, and a clock more than Threshold off is set;
//   - C: when none of them answers, the time of the HTTPS answer of the DoH server of the
//     settings, its certificate verified at that time;
//   - B: once sing-box answers DNS, sysntpd is restarted, as LuCI's "Sync with NTP-Server"
//     does: it asks at once instead of at its next try.
//
// The hook of the package (/etc/hotplug.d/ntp/60-podkop) tells what sysntpd did (State)
// and keeps a file of /etc as new as the last time sysntpd set the clock, at most an hour
// behind, so that sysfixtime boots with the time of the last power loss.
package clock

import (
	"context"
	"time"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/ntp"
)

// Threshold is how far off the clock may be before it is set: closer than this it is the
// business of the router's NTP service, which adjusts it smoothly.
const Threshold = time.Minute

// Result is what the check came to.
type Result struct {
	// Server answered first.
	Server string
	// Offset is how far the clock was behind (positive) or ahead (negative).
	Offset time.Duration
	// Set tells whether the clock was set.
	Set bool
}

// Sync asks every server at once and takes the first valid answer. A clock off by more than
// threshold is set with set (ntp.SetSystemTime when nil). Without a valid answer before the
// context ends nothing is changed and the error says why.
func Sync(ctx context.Context, servers []string, threshold time.Duration, set func(time.Time) error) (Result, error) {
	if set == nil {
		set = ntp.SetSystemTime
	}
	type answer struct {
		server   string
		response *ntp.Response
		err      error
	}
	answers := make(chan answer, len(servers))
	for _, server := range servers {
		go func(server string) {
			address := M.ParseSocksaddr(server)
			if address.Port == 0 {
				address.Port = 123
			}
			response, err := ntp.Exchange(ctx, N.SystemDialer, address)
			if err == nil {
				err = response.Validate()
			}
			answers <- answer{server, response, err}
		}(server)
	}
	var problems []error
	for range servers {
		select {
		case <-ctx.Done():
			return Result{}, E.Cause(ctx.Err(), "no NTP server answered (", len(problems), " failed)")
		case got := <-answers:
			if got.err != nil {
				problems = append(problems, E.Cause(got.err, got.server))
				continue
			}
			result := Result{Server: got.server, Offset: got.response.ClockOffset}
			if result.Offset > threshold || result.Offset < -threshold {
				err := set(time.Now().Add(result.Offset))
				if err != nil {
					return result, E.Cause(err, "set the clock")
				}
				result.Set = true
			}
			return result, nil
		}
	}
	return Result{}, E.Errors(problems...)
}
