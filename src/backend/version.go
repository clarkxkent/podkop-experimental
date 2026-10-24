// Package podkop is the backend of podkop: the settings of podkop (UCI) made into a running
// sing-box, with the firewall, the routing and the DNS of the LAN around it. sing-box runs
// inside this process: podkop-engine, the sing-box of podkop, is a module it builds with.
package podkop

import "strings"

// Version is the version of the backend, which the user interface shows as the version of
// podkop; the version of sing-box is apart (constant.Version). A build sets it with
// -X github.com/podkop-net/podkop-experimental/src/backend.Version=...
var Version = "0.8.0-dev"

// EngineRelease is the release of podkop-engine whose sing-box is built in (r16): the
// commit go.mod takes podkop-engine at is that release's. A build sets it with
// -X github.com/podkop-net/podkop-experimental/src/backend.EngineRelease=...
var EngineRelease = ""

// Engine tells what the sing-box inside is, for a person: its version, the release of
// podkop-engine and the build (slim or full), and the commit of podkop-engine, from the
// version sing-box reports ("1.14.3-pdk-ae09bd696dff"):
// "sing-box 1.14.3, podkop-engine r16, slim (ae09bd696dff)".
func Engine(singBoxVersion string) string {
	base, commit, _ := strings.Cut(singBoxVersion, "-pdk-")
	text := "sing-box " + base
	if EngineRelease != "" {
		text += ", podkop-engine " + EngineRelease
	}
	text += ", " + Build
	if commit != "" {
		text += " (" + commit + ")"
	}
	return text
}
