package engine

import (
	"testing"

	"github.com/sagernet/sing-box/option"

	"github.com/stretchr/testify/require"
)

// The ports of a rule as the firewall takes them: in order, those that overlap or touch
// as one range (an interval set of nftables refuses elements that overlap).
func TestPortsOnly(t *testing.T) {
	rule := &option.DefaultHeadlessRule{
		Network:   []string{"udp"},
		Port:      []uint16{8080, 3478, 65535, 3479},
		PortRange: []string{"8000:9000", ":100", "101:200", "60000:"},
	}
	network, ports, ok := portsOnly(rule)
	require.True(t, ok)
	require.Equal(t, "udp", network)
	require.Equal(t, []PortRange{{From: 0, To: 200}, {From: 3478, To: 3479}, {From: 8000, To: 9000}, {From: 60000, To: 65535}}, ports)

	_, ports, ok = portsOnly(&option.DefaultHeadlessRule{Network: []string{"tcp"}, Port: []uint16{443}})
	require.True(t, ok)
	require.Equal(t, []PortRange{{From: 443, To: 443}}, ports)
	_, _, ok = portsOnly(&option.DefaultHeadlessRule{Network: []string{"tcp"}, PortRange: []string{"9:1"}})
	require.False(t, ok)
}
