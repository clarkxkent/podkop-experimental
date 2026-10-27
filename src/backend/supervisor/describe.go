package supervisor

import (
	"strconv"
	"strings"

	"github.com/podkop-net/podkop-experimental/src/backend/firewall"
)

// describeConfig tells in a line what a configuration that starts is: its sections with what
// each sends traffic through, its lists, its DNS and the interfaces the LAN comes in on.
func describeConfig(next *candidate) string {
	var sections []string
	for _, section := range next.settings.Sections {
		what := section.ConnectionType
		switch section.ConnectionType {
		case "proxy":
			switch section.ProxyConfigType {
			case "url":
				scheme, _, found := strings.Cut(strings.TrimSpace(section.ProxyString), "://")
				what = "proxy"
				if found && scheme != "" && !strings.ContainsAny(scheme, " \n") {
					what = scheme
				}
			case "urltest":
				what = "urltest of " + strconv.Itoa(len(section.URLTestProxyLinks))
			case "selector":
				what = "selector of " + strconv.Itoa(len(section.SelectorProxyLinks))
			case "outbound":
				what = "outbound JSON"
			}
		case "vpn":
			what = "interface " + section.Interface
		case "group":
			what = "group " + strings.Join(section.Groups, ", ")
		}
		if section.ConnectionType == "proxy" && section.Detour != "" {
			what += " via " + section.Detour
		}
		sections = append(sections, section.Name+" ("+what+")")
	}
	downloaded, files := 0, 0
	for _, ruleSet := range next.result.RuleSets {
		if ruleSet.URL != "" {
			downloaded++
		} else {
			files++
		}
	}
	settings := &next.settings.Settings
	return "configuration: sections " + strings.Join(sections, ", ") +
		"; " + strconv.Itoa(len(next.result.RuleSets)) + " lists (" + strconv.Itoa(downloaded) + " downloaded, " + strconv.Itoa(files) + " from files)" +
		"; DNS " + settings.DNSType + " " + settings.DNSServer +
		"; the LAN on " + strings.Join(settings.SourceNetworkInterfaces, ", ")
}

// describeFirewall tells what the sets of the firewall send to the proxy by address.
func describeFirewall(plan *firewall.Plan) string {
	text := "firewall: " + strconv.Itoa(len(plan.Subnets)) + " address ranges of the lists go to the proxy by address"
	if len(plan.Groups) > 0 {
		count := 0
		for _, group := range plan.Groups {
			count += len(group.Ranges)
		}
		text += ", " + strconv.Itoa(count) + " more for some ports only (" + strconv.Itoa(len(plan.Groups)) + " groups of ports)"
	}
	return text
}

// tellFirewall logs what the sets of the firewall hold when that changed.
func (s *Supervisor) tellFirewall(plan *firewall.Plan) {
	if plan == nil {
		return
	}
	text := describeFirewall(plan)
	if text != s.firewallTold {
		s.firewallTold = text
		s.log("info", text)
	}
}
