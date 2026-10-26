package clock

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/uci"
)

// sysntpd is the router's NTP service as the clock check sees it; a test has its own.
type sysntpd struct {
	// ubus runs "ubus -t 2 call <object> <method> <message>"
	ubus func(ctx context.Context, object string, method string, message string) ([]byte, error)
	// systemPath is the UCI file with the section timeserver "ntp"
	systemPath string
	// initScript is the init script of the service
	initScript string
	run        func(ctx context.Context, name string, arguments ...string) error
}

var defaultSysntpd = sysntpd{
	ubus: func(ctx context.Context, object string, method string, message string) ([]byte, error) {
		return exec.CommandContext(ctx, "ubus", "-t", "2", "call", object, method, message).Output()
	},
	systemPath: "/etc/config/system",
	initScript: "/etc/init.d/sysntpd",
	run: func(ctx context.Context, name string, arguments ...string) error {
		return exec.CommandContext(ctx, name, arguments...).Run()
	},
}

// Servers are the NTP servers sysntpd asks: the -p arguments of its running instance (the
// servers of the settings and those DHCP gave), or the servers of the settings when it does
// not run.
func Servers() []string {
	return defaultSysntpd.servers(context.Background())
}

// Enabled tells whether sysntpd is meant to run: its init script is there and enabled, and
// its settings do not turn it off.
func Enabled() bool {
	return defaultSysntpd.enabled(context.Background())
}

// ServerNames are the names among Servers: the DNS of sing-box answers them with their own
// addresses, never from FakeIP (plan.Facts.NTPServerNames).
func ServerNames() []string {
	var names []string
	for _, server := range Servers() {
		if _, err := netip.ParseAddr(server); err != nil {
			names = append(names, server)
		}
	}
	return names
}

func (s sysntpd) servers(ctx context.Context) []string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if output, err := s.ubus(ctx, "service", "list", `{"name":"sysntpd"}`); err == nil {
		var services map[string]struct {
			Instances map[string]struct {
				Running bool     `json:"running"`
				Command []string `json:"command"`
			} `json:"instances"`
		}
		if json.Unmarshal(output, &services) == nil {
			var servers []string
			for _, instance := range services["sysntpd"].Instances {
				for index := 0; index+1 < len(instance.Command); index++ {
					if instance.Command[index] == "-p" && !slices.Contains(servers, instance.Command[index+1]) {
						servers = append(servers, instance.Command[index+1])
					}
				}
			}
			if len(servers) > 0 {
				return servers
			}
		}
	}
	timeserver := s.timeserver()
	if timeserver == nil {
		return nil
	}
	return timeserver.List("server")
}

func (s sysntpd) timeserver() *uci.Section {
	content, err := os.ReadFile(s.systemPath)
	if err != nil {
		return nil
	}
	settings, err := uci.Parse(content)
	if err != nil {
		return nil
	}
	return settings.Section("ntp")
}

// enabled tells whether sysntpd is meant to run: its init script is there and enabled, and
// the settings do not turn it off. One the user turned off is left alone.
func (s sysntpd) enabled(ctx context.Context) bool {
	if _, err := os.Stat(s.initScript); err != nil {
		return false
	}
	if s.run(ctx, s.initScript, "enabled") != nil {
		return false
	}
	if timeserver := s.timeserver(); timeserver != nil && !timeserver.Bool("enabled", true) {
		return false
	}
	return true
}

// restart restarts sysntpd: it resolves its servers anew and asks at once.
func (s sysntpd) restart(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return s.run(ctx, s.initScript, "restart")
}
