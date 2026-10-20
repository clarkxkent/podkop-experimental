package diag

import (
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/itdoginfo/podkop/src/backend/firewall"
	"github.com/stretchr/testify/require"
)

// questionOf writes a DNS question for name, type A.
func questionOf(name string) []byte {
	message := []byte{0x7a, 0x3b, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	start := 0
	for index := 0; index <= len(name); index++ {
		if index == len(name) || name[index] == '.' {
			message = append(message, byte(index-start))
			message = append(message, name[start:index]...)
			start = index + 1
		}
	}
	return append(message, 0, 0x00, 0x01, 0x00, 0x01)
}

// A trace with the nft of the system: the DNS questions for the check name are told, from
// the moment the trace is armed, and those for other names are not. A question sent to an
// address of this host passes prerouting on lo; a table with an input chain stands for the
// one of the firewall.
func TestTraceOfQuestions(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("not root")
	}
	if _, err := exec.LookPath("nft"); err != nil {
		t.Skip("no nft program")
	}
	output, err := exec.Command("nft", "add table inet pdktest; add chain inet pdktest input { type filter hook input priority 0; policy accept; }").CombinedOutput()
	require.NoError(t, err, string(output))
	defer exec.Command("nft", "delete", "table", "inet", "pdktest").Run()
	label := "6e1d0c3b2a19f8e7d6c5"
	trace, err := StartTrace(40001, label)
	require.NoError(t, err)
	stopped := false
	defer func() {
		if !stopped {
			trace.Stop()
		}
	}()
	// the listener of nft starts while the table is added: give it a moment
	time.Sleep(300 * time.Millisecond)
	// a socket not connected: the refusal of one question is no error of the next
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer conn.Close()
	server := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 53}
	for _, name := range []string{label + ".fakeipchecker.podkop.net", "0123456789abcdef.fakeipchecker.podkop.net", label + ".fakeipchecker.podkop.net"} {
		_, err = conn.WriteTo(questionOf(name), server)
		require.NoError(t, err)
	}
	time.Sleep(500 * time.Millisecond)
	lines := trace.Stop()
	stopped = true
	hooks, err := firewall.ChainHooks()
	require.NoError(t, err)
	require.Equal(t, []string{"to 127.0.0.1 (2): reached the router (hook input)"}, TraceStories(lines, hooks, true), "%q", lines)
	require.Empty(t, TraceStories(lines, hooks, false))
	require.Equal(t, "127.0.0.1", TraceSource(lines, hooks).String())
}
