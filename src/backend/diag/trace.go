package diag

import (
	"bufio"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/itdoginfo/podkop/src/backend/dataplane"
	"github.com/itdoginfo/podkop/src/backend/firewall"
	E "github.com/sagernet/sing/common/exceptions"
)

// Trace is the trace of the packets of a check a device makes (firewall.ArmTrace), heard
// through "nft monitor trace". The nft program comes with firewall4 (nftables-json), so
// every router podkop runs on has it.
type Trace struct {
	command *exec.Cmd
	access  sync.Mutex
	lines   []string
	done    chan struct{}
}

// traceLinesKept bounds what a trace keeps: a check makes a few packets a second, each a
// few dozen lines.
const traceLinesKept = 20000

// StartTrace listens to the traces of the kernel and arms the trace of the TCP packets to
// the FakeIP range and a port. The listener is there first: a packet traced before it would
// be told to nobody.
func StartTrace(port uint16) (*Trace, error) {
	path, err := exec.LookPath("nft")
	if err != nil {
		return nil, E.New("no nft program")
	}
	command := exec.Command(path, "monitor", "trace")
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = command.Start()
	if err != nil {
		return nil, E.Cause(err, "start nft monitor trace")
	}
	trace := &Trace{command: command, done: make(chan struct{})}
	go func() {
		defer close(trace.done)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			trace.access.Lock()
			if len(trace.lines) < traceLinesKept {
				trace.lines = append(trace.lines, scanner.Text())
			}
			trace.access.Unlock()
		}
	}()
	err = firewall.ArmTrace(port)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, err
	}
	return trace, nil
}

// Stop removes the trace table, ends the listener and returns what it heard.
func (t *Trace) Stop() []string {
	_ = firewall.DisarmTrace()
	_ = t.command.Process.Kill()
	_ = t.command.Wait()
	<-t.done
	t.access.Lock()
	defer t.access.Unlock()
	return t.lines
}

// tracedPacket is what a trace tells of one packet of a check.
type tracedPacket struct {
	flags string
	// hooks are the hooks of the base chains it passed, in order
	hooks []string
	// marked: the rule of podkop gave it the mark; proxied: the transparent proxy rule of
	// podkop took it
	marked, proxied bool
	// steps are what the rules of other tables and of podkop did to it, in their order
	steps []string
	// dropped is the rule (or the policy of a chain) that dropped or rejected it
	dropped string
}

// traceEvent is one line of nft monitor trace: "trace id <id> <family> <table> <chain>
// packet: <fields>", "... rule <rule> (verdict <verdict>)", "... verdict <verdict> ...",
// "... policy <verdict> ...".
type traceEvent struct {
	id, chain, kind, text, verdict string
}

var counters = regexp.MustCompile(` ?counter packets [0-9]+ bytes [0-9]+`)

func parseTraceLine(line string) (traceEvent, bool) {
	fields := strings.SplitN(line, " ", 7)
	if len(fields) < 7 || fields[0] != "trace" || fields[1] != "id" {
		return traceEvent{}, false
	}
	event := traceEvent{id: fields[2], chain: fields[3] + " " + fields[4] + " " + fields[5]}
	rest := fields[6]
	switch {
	case strings.HasPrefix(rest, "packet: "):
		event.kind, event.text = "packet", strings.TrimPrefix(rest, "packet: ")
	case strings.HasPrefix(rest, "rule "):
		event.kind = "rule"
		text := strings.TrimPrefix(rest, "rule ")
		if open := strings.LastIndex(text, " (verdict "); open >= 0 && strings.HasSuffix(text, ")") {
			event.verdict = strings.TrimSuffix(text[open+len(" (verdict "):], ")")
			text = text[:open]
		}
		// the counters of a rule say nothing of this packet
		event.text = counters.ReplaceAllString(text, "")
	case strings.HasPrefix(rest, "verdict "), strings.HasPrefix(rest, "policy "):
		kind, after, _ := strings.Cut(rest, " ")
		event.kind = kind
		event.verdict, _, _ = strings.Cut(after, " ")
	default:
		return traceEvent{}, false
	}
	return event, true
}

// packetsOfTrace reads the packets of a trace. A packet begins where it enters the chain of
// the trace table; the lines of packets traced at once may be mixed, and are told apart by
// their trace id. hooks names the hook of each base chain ("family table chain").
func packetsOfTrace(lines []string, hooks map[string]string) []*tracedPacket {
	traceChain := "inet " + firewall.TraceTable + " trace"
	ourTable := "inet " + dataplane.FirewallTable + " "
	markOfPodkop := "meta mark set 0x" + strconv.FormatUint(dataplane.ProxyMark+0x100000000, 16)[1:]
	current := make(map[string]*tracedPacket)
	var packets []*tracedPacket
	for _, line := range lines {
		event, ok := parseTraceLine(line)
		if !ok {
			continue
		}
		if event.kind == "packet" && event.chain == traceChain {
			packet := &tracedPacket{flags: tcpFlags(event.text)}
			current[event.id] = packet
			packets = append(packets, packet)
			continue
		}
		packet := current[event.id]
		if packet == nil || event.chain == traceChain {
			continue
		}
		ours := strings.HasPrefix(event.chain, ourTable)
		switch event.kind {
		case "packet":
			if hook, known := hooks[event.chain]; known && (len(packet.hooks) == 0 || packet.hooks[len(packet.hooks)-1] != hook) {
				packet.hooks = append(packet.hooks, hook)
			}
		case "rule":
			switch {
			case ours && strings.Contains(event.text, markOfPodkop) && !packet.marked:
				packet.marked = true
				packet.steps = append(packet.steps, "marked by podkop")
			case ours && strings.Contains(event.text, " tproxy ") && !packet.proxied:
				packet.proxied = true
				packet.steps = append(packet.steps, "taken by the transparent proxy rule of podkop")
			case !ours && strings.Contains(event.text, " tproxy "):
				// a mark it sets is told with it
				packet.steps = append(packet.steps, "taken by "+event.chain+": "+event.text)
			case !ours && strings.Contains(event.text, "mark set ") && packet.marked:
				// a mark set before the one of podkop is replaced by it: no matter
				packet.steps = append(packet.steps, "its mark replaced by "+event.chain+": "+event.text)
			}
			if (event.verdict == "drop" || event.verdict == "reject") && packet.dropped == "" {
				packet.dropped = event.chain + ": " + event.text
			}
		case "policy":
			if event.verdict == "drop" && packet.dropped == "" {
				packet.dropped = event.chain + ": the policy of the chain"
			}
		}
	}
	return packets
}

// tcpFlags reads the TCP flags of the fields of a traced packet: "syn", "ack", "psh | ack".
func tcpFlags(fields string) string {
	_, after, found := strings.Cut(fields, "tcp flags == ")
	if !found {
		return "tcp"
	}
	// the flags run up to the next field of the packet
	if end := strings.Index(after, " tcp "); end >= 0 {
		after = after[:end]
	}
	return strings.TrimSpace(after)
}

// story tells what became of a packet, as facts in their order.
func (p *tracedPacket) story() string {
	var steps []string
	if !p.marked {
		steps = append(steps, "not marked by podkop")
	}
	steps = append(steps, p.steps...)
	if p.marked && !p.proxied {
		steps = append(steps, "not taken by the transparent proxy rule of podkop")
	}
	switch {
	case p.dropped != "":
		steps = append(steps, "dropped by "+p.dropped)
	case contains(p.hooks, "forward"):
		steps = append(steps, "routed past the router (hook forward)")
	case contains(p.hooks, "input"):
		steps = append(steps, "reached the router (hook input)")
	default:
		steps = append(steps, "lost after the routing: neither hook input nor forward")
	}
	return strings.Join(steps, " → ")
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// TraceStories tells what became of the packets of a trace: each different story once,
// with the TCP flags of the packets it is the story of, the first few.
func TraceStories(lines []string, hooks map[string]string) []string {
	type told struct {
		flags []string
		count int
	}
	stories := make(map[string]*told)
	var order []string
	for _, packet := range packetsOfTrace(lines, hooks) {
		story := packet.story()
		if stories[story] == nil {
			stories[story] = &told{}
			order = append(order, story)
		}
		entry := stories[story]
		entry.count++
		if !contains(entry.flags, packet.flags) {
			entry.flags = append(entry.flags, packet.flags)
		}
	}
	var result []string
	for index, story := range order {
		if index == 4 {
			result = append(result, strconv.Itoa(len(order)-index)+" more ways")
			break
		}
		entry := stories[story]
		result = append(result, strings.ToUpper(strings.Join(entry.flags, ", "))+" ("+strconv.Itoa(entry.count)+"): "+story)
	}
	return result
}
