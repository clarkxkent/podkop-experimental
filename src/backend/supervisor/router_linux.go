package supervisor

import (
	"os"
	"strings"

	"github.com/podkop-net/podkop-experimental/src/backend/dnsmasq"
	"github.com/podkop-net/podkop-experimental/src/backend/firewall"
	"github.com/podkop-net/podkop-experimental/src/backend/netcfg"
	"github.com/podkop-net/podkop-experimental/src/backend/sockets"
	"github.com/podkop-net/podkop-experimental/src/backend/uci"
	E "github.com/sagernet/sing/common/exceptions"
)

// Router is the System of an OpenWrt router.
type Router struct{}

func (Router) LANAddress() string {
	address, err := netcfg.LANAddress()
	if err != nil {
		return ""
	}
	return address.String()
}

func (Router) Devices(names []string) map[string]string { return netcfg.Devices(names) }

func (Router) Listening() []sockets.Listener { return sockets.Bound("/") }

func (Router) ApplyRouting() error { return netcfg.Apply() }

func (Router) AllowBridgedTraffic() []string { return netcfg.AllowBridgedTraffic() }

func (Router) RemoveRouting() error { return netcfg.Remove() }

func (Router) RoutingPresent() (bool, error) { return netcfg.Present() }

func (Router) FirewallPresent() (bool, error) { return firewall.Exists() }

func (Router) SetClashSecret(key string) error {
	// a line of uci batch a command: the key is quoted, and a key that would end the
	// line is refused
	if strings.ContainsAny(key, "\r\n") {
		return E.New("the key of the Clash API has a line break")
	}
	return dnsmasq.Router{}.Batch("set podkop.settings.yacd_secret_key=" + dnsmasq.Quote(key) + "\ncommit podkop\n")
}

func (Router) UpdateFirewall(previous *firewall.Plan, next *firewall.Plan) error {
	return firewall.Update(previous, next)
}

func (Router) RemoveFirewall() error { return firewall.Remove() }

func (Router) TakeOverDNS() (bool, error) { return dnsmasq.TakeOver(dnsmasq.Router{}) }

func (Router) RestoreDNS() (bool, error) {
	changed, err := dnsmasq.Restore(dnsmasq.Router{})
	markCleanShutdown()
	return changed, err
}

// SettingsPath is the UCI file of the backend.
var SettingsPath = "/etc/config/podkop"

// compat: clean-shutdown
// markCleanShutdown keeps a promise to the shell backend, which may be installed again in
// place of this one: it sets dnsmasq up at its start only when its settings say that the
// previous stop was a clean one. This backend reads the state of dnsmasq from dnsmasq's
// own settings and has no use for the flag; it only never leaves it saying "not clean"
// after it gave dnsmasq back.
func markCleanShutdown() {
	content, err := os.ReadFile(SettingsPath)
	if err != nil {
		return
	}
	parsed, err := uci.Parse(content)
	if err != nil || parsed.Section("settings").Get("shutdown_correctly") != "0" {
		return
	}
	_ = dnsmasq.Router{}.Batch("set podkop.settings.shutdown_correctly='1'\ncommit podkop\n")
}
