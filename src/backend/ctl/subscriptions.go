package ctl

import (
	"strconv"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/api"
)

// subscriptionLines tell what is known of each subscription, a line each: its servers and
// when and how its list came, or why it did not.
func subscriptionLines(states []api.SubscriptionState, now time.Time) []string {
	var lines []string
	for _, state := range states {
		line := state.Name + ": "
		switch {
		case state.Changed == nil && state.Error != "":
			line = "❌ " + line + "no list: " + state.Error
		case state.Changed == nil:
			line = "⏳ " + line + "the list is not downloaded yet"
		default:
			line += strconv.Itoa(state.Servers) + " servers, " + strconv.Itoa(state.Usable) + " can be used; taken " + takenAgo(now.Sub(*state.Changed))
			if state.Route != "" {
				line += " (" + state.Route + ")"
			}
			if state.Error != "" {
				line = "⚠️ " + line + "; the last download failed: " + state.Error
			} else {
				line = "✅ " + line
			}
		}
		lines = append(lines, line)
	}
	return lines
}

// takenAgo tells how long ago a list was taken, roughly.
func takenAgo(passed time.Duration) string {
	if passed < time.Minute {
		return "just now"
	}
	return ago(passed) + " ago"
}
