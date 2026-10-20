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
	// sources are the interfaces podkop takes traffic from (source_network_interfaces)
	sources []string
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
	// told tells why the packets of one kind did not come to podkop, in a sentence, and
	// then what the kernel did to them, a line a fate
	told := func(questions bool, head string, none string) []string {
		if traceErr != nil {
			return []string{head + ": they could not be traced (" + traceErr.Error() + ")"}
		}
		stories := diag.TraceStories(traced, hooks, questions)
		if len(stories) == 0 {
			return []string{head + ": " + none}
		}
		lines := []string{head + ": " + diag.TraceVerdict(traced, hooks, questions, d.sources) + ". What the kernel did to them:"}
		for _, story := range stories {
			lines = append(lines, "   "+story)
		}
		return lines
	}
	name := probe.Name(d.label)
	if !result.DNS {
		lines := told(true, "❌ The DNS question of this device for "+name+" did not come to podkop",
			"none of its DNS questions passed the router while the check ran: the device resolves names another way (DNS over HTTPS in the browser, or a DNS server it reaches by an encrypted channel)")
		return lines, diag.TraceSource(traced, hooks)
	}
	lines := []string{"✅ The DNS question of this device came to podkop: " + name + " is " + result.Address + " (FakeIP)"}
	if result.Connection {
		client, _ := netip.ParseAddr(result.Client)
		return append(lines, "✅ Its connection came to podkop through the transparent proxy, from "+result.Client), client
	}
	lines = append(lines, told(false, "❌ Its connections to "+result.Address+":"+strconv.Itoa(int(d.port))+" did not come to podkop",
		"none of them came to the router while the check ran: the device does not send them through this router")...)
	return lines, diag.TraceSource(traced, hooks)
}
