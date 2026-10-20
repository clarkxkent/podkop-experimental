package ctl

import (
	"net/netip"
	"strconv"
	"time"

	"github.com/itdoginfo/podkop/src/backend/api"
	"github.com/itdoginfo/podkop/src/backend/diag"
	"github.com/itdoginfo/podkop/src/backend/firewall"
	"github.com/itdoginfo/podkop/src/backend/probe"
)

// deviceCheck is the check of the device that asks for global_check (the browser of the
// page): the label of a check name and the port it opens connections to while the report
// is made.
type deviceCheck struct {
	label string
	port  uint16
	probe func(label string, wait time.Duration) (*api.ProbeResult, error)
}

// deviceWait is how long the report waits for a connection of the device: the page opens
// one about every 400 ms until the report comes.
const deviceWait = 5 * time.Second

// run traces the TCP packets to the FakeIP range and the port of the check while it waits
// for a connection of the device, and tells what came: its DNS question, its connection,
// and when the connection did not come, what the kernel did to its packets. It returns the
// address of the device when the connection came.
func (d *deviceCheck) run() ([]string, netip.Addr) {
	trace, traceErr := diag.StartTrace(d.port)
	result, err := d.probe(d.label, deviceWait)
	var traced []string
	if trace != nil {
		traced = trace.Stop()
	}
	if err != nil {
		return []string{"❌ The backend did not answer: " + err.Error()}, netip.Addr{}
	}
	name := probe.Name(d.label)
	if !result.DNS {
		return []string{"❌ The DNS question of this device for " + name + " did not come to sing-box"}, netip.Addr{}
	}
	lines := []string{"✅ The DNS question of this device came to sing-box: " + name + " is " + result.Address + " (FakeIP)"}
	client, _ := netip.ParseAddr(result.Client)
	if result.Connection {
		return append(lines, "✅ Its connection came to sing-box through the transparent proxy, from "+result.Client), client
	}
	lines = append(lines, "❌ Its connections to "+result.Address+":"+strconv.Itoa(int(d.port))+" did not come to sing-box. What the kernel did to their packets:")
	if traceErr != nil {
		return append(lines, "   they could not be traced: "+traceErr.Error()), client
	}
	hooks, _ := firewall.ChainHooks()
	stories := diag.TraceStories(traced, hooks)
	if len(stories) == 0 {
		lines = append(lines, "   none of them came to the router while the check ran")
	}
	for _, story := range stories {
		lines = append(lines, "   "+story)
	}
	return lines, client
}
