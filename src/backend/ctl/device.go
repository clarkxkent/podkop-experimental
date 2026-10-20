package ctl

import (
	"net/netip"
	"strconv"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/api"
	"github.com/podkop-net/podkop-experimental/src/backend/diag"
	"github.com/podkop-net/podkop-experimental/src/backend/firewall"
	"github.com/podkop-net/podkop-experimental/src/backend/probe"
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

// run traces the packets of the check while it waits for a connection of the device, and
// tells what came: its DNS question, its connection, and when either did not come, what the
// kernel did to its DNS questions or to the packets of its connections. It returns the
// address of the device: the one its connection came from, or its traced packets did.
func (d *deviceCheck) run() ([]string, netip.Addr) {
	trace, traceErr := diag.StartTrace(d.port, d.label)
	result, err := d.probe(d.label, deviceWait)
	var traced []string
	if trace != nil {
		traced = trace.Stop()
	}
	if err != nil {
		return []string{"❌ The backend did not answer: " + err.Error()}, netip.Addr{}
	}
	var hooks map[string]string
	if traceErr == nil {
		hooks, _ = firewall.ChainHooks()
	}
	// stories tells the traced packets of one kind, or why there are none
	stories := func(questions bool, none string) []string {
		if traceErr != nil {
			return []string{"   they could not be traced: " + traceErr.Error()}
		}
		told := diag.TraceStories(traced, hooks, questions)
		if len(told) == 0 {
			return []string{"   " + none}
		}
		for index := range told {
			told[index] = "   " + told[index]
		}
		return told
	}
	name := probe.Name(d.label)
	if !result.DNS {
		lines := []string{"❌ The DNS question of this device for " + name + " did not come to sing-box. What the kernel did to its DNS questions:"}
		lines = append(lines, stories(true, "none of them passed the router over UDP port 53 while the check ran: the device resolves names some other way (DNS over HTTPS of the browser, say)")...)
		return lines, diag.TraceSource(traced, hooks)
	}
	lines := []string{"✅ The DNS question of this device came to sing-box: " + name + " is " + result.Address + " (FakeIP)"}
	if result.Connection {
		client, _ := netip.ParseAddr(result.Client)
		return append(lines, "✅ Its connection came to sing-box through the transparent proxy, from "+result.Client), client
	}
	lines = append(lines, "❌ Its connections to "+result.Address+":"+strconv.Itoa(int(d.port))+" did not come to sing-box. What the kernel did to their packets:")
	lines = append(lines, stories(false, "none of them came to the router while the check ran")...)
	return lines, diag.TraceSource(traced, hooks)
}
