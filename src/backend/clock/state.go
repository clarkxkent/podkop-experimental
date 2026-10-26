package clock

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// The files of the hook of the package, /etc/hotplug.d/ntp/60-podkop, which procd runs
// at every event of sysntpd. /var/state is made at every boot by /etc/init.d/boot, in RAM:
// its files tell of this boot only.
const (
	// EventPath holds the last event: "<unix time> <action> <offset> <stratum>", "-" for
	// what sysntpd did not tell.
	EventPath = "/var/state/podkop-ntp"
	// SyncedPath is touched whenever sysntpd tells the clock is set (an event with a
	// stratum below 16, or a step).
	SyncedPath = "/var/state/podkop-ntp-synced"
	// MarkPath is a file of /etc the hook keeps as new as the last time the clock was set,
	// at most an hour behind: sysfixtime sets the clock at boot to the newest file of /etc.
	MarkPath = "/etc/podkop-time"
	// dnsmasqSyncedPath is made by the hook of dnsmasq at the first time sysntpd tells the
	// clock is set after the boot: the sign before the hook of the package was there.
	dnsmasqSyncedPath = "/var/state/dnsmasqsec"
)

// State is what sysntpd told of the clock since the boot.
type State struct {
	// Synced tells that sysntpd has set the clock since the boot.
	Synced bool
	// LastSync is when it last told so, by the hook of the package: zero when only the
	// sign of dnsmasq tells it.
	LastSync time.Time
	// Event is the last event; Action is empty when there was none.
	Event Event
}

// Event is an event of sysntpd as the hook kept it.
type Event struct {
	At time.Time
	// Action is step, stratum, periodic or unsync.
	Action string
	// Offset is the offset sysntpd measured, in seconds, as it wrote it.
	Offset string
	// Stratum is that of the server it follows; 16 when none.
	Stratum int
}

// statePaths are the files ReadState reads; a test has its own.
type statePaths struct {
	event, synced, dnsmasq string
}

var defaultStatePaths = statePaths{EventPath, SyncedPath, dnsmasqSyncedPath}

// ReadState reads what the hooks left: a few small files, no process and no network.
func ReadState() State {
	return defaultStatePaths.read()
}

func (p statePaths) read() State {
	var state State
	if info, err := os.Stat(p.synced); err == nil {
		state.Synced, state.LastSync = true, info.ModTime()
	} else if _, err := os.Stat(p.dnsmasq); err == nil {
		state.Synced = true
	}
	content, err := os.ReadFile(p.event)
	if err == nil {
		state.Event = parseEvent(string(content))
	}
	return state
}

func parseEvent(line string) Event {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Event{}
	}
	seconds, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return Event{}
	}
	event := Event{At: time.Unix(seconds, 0), Action: fields[1], Stratum: 16}
	if len(fields) > 2 && fields[2] != "-" {
		event.Offset = fields[2]
	}
	if len(fields) > 3 {
		if stratum, err := strconv.Atoi(fields[3]); err == nil {
			event.Stratum = stratum
		}
	}
	return event
}
