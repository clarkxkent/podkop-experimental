// Package podkop is the backend of podkop: the settings of podkop (UCI) made into a running
// sing-box, with the firewall, the routing and the DNS of the LAN around it. sing-box runs
// inside this process: podkop-engine, the sing-box of podkop, is a module it builds with.
package podkop

// Version is the version of the backend, which the user interface shows as the version of
// podkop; the version of sing-box is apart (constant.Version). A build sets it with
// -X github.com/podkop-net/podkop-experimental/src/backend.Version=...
var Version = "0.8.0-dev"
