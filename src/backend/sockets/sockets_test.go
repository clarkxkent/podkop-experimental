package sockets

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// hexAddress writes an address the way /proc/net does on this host: words of the host's
// byte order in hexadecimal.
func hexAddress(address netip.AddrPort) string {
	raw := address.Addr().AsSlice()
	var text strings.Builder
	for offset := 0; offset < len(raw); offset += 4 {
		fmt.Fprintf(&text, "%08X", binary.NativeEndian.Uint32(raw[offset:]))
	}
	return fmt.Sprintf("%s:%04X", text.String(), address.Port())
}

func line(local string, remote string, state string, inode int) string {
	return fmt.Sprintf("   0: %s %s %s 00000000:00000000 00:00000000 00000000     0        0 %d 1 0000000000000000 100 0 0 10 0",
		local, remote, state, inode)
}

func TestList(t *testing.T) {
	root := t.TempDir()
	header := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"
	write := func(name string, lines ...string) {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(append([]string{header}, lines...), "\n")+"\n"), 0o644))
	}
	any4 := hexAddress(netip.MustParseAddrPort("0.0.0.0:0"))
	any6 := hexAddress(netip.MustParseAddrPort("[::]:0"))
	write("proc/net/tcp",
		line(hexAddress(netip.MustParseAddrPort("0.0.0.0:53")), any4, "0A", 101),
		// a connection, not a listener
		line(hexAddress(netip.MustParseAddrPort("192.168.1.1:53")), hexAddress(netip.MustParseAddrPort("192.168.1.10:40000")), "01", 102),
		line(hexAddress(netip.MustParseAddrPort("127.0.0.1:1602")), any4, "0A", 103))
	write("proc/net/udp",
		line(hexAddress(netip.MustParseAddrPort("127.0.0.42:53")), any4, "07", 104),
		// a connected UDP socket is a client
		line(hexAddress(netip.MustParseAddrPort("192.168.1.1:5353")), hexAddress(netip.MustParseAddrPort("192.168.1.10:5353")), "01", 105))
	write("proc/net/udp6", line(hexAddress(netip.MustParseAddrPort("[::]:53")), any6, "07", 106))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "proc/7600/fd"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "proc/4388/fd"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "proc/7600/comm"), []byte("dnsmasq\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "proc/4388/comm"), []byte("podkop\n"), 0o644))
	for descriptor, target := range map[string]string{"proc/7600/fd/4": "socket:[101]", "proc/7600/fd/5": "socket:[106]", "proc/7600/fd/0": "/dev/null",
		"proc/4388/fd/7": "socket:[103]", "proc/4388/fd/8": "socket:[104]"} {
		require.NoError(t, os.Symlink(target, filepath.Join(root, descriptor)))
	}
	listeners, err := List(root)
	require.NoError(t, err)
	var named []string
	for _, listener := range listeners {
		named = append(named, listener.Protocol+" "+listener.String())
	}
	require.ElementsMatch(t, []string{
		"tcp dnsmasq (pid 7600) on 0.0.0.0:53", "tcp podkop (pid 4388) on 127.0.0.1:1602",
		"udp podkop (pid 4388) on 127.0.0.42:53", "udp dnsmasq (pid 7600) on [::]:53",
	}, named)

	// who takes 127.0.0.42:53: the wildcard listener of TCP; for UDP the socket on that
	// address and the IPv6 wildcard one
	tcp := Holding(listeners, "tcp", netip.MustParseAddrPort("127.0.0.42:53"))
	require.Len(t, tcp, 1)
	require.Equal(t, "dnsmasq", tcp[0].Program)
	udp := Holding(listeners, "udp", netip.MustParseAddrPort("127.0.0.42:53"))
	require.Len(t, udp, 2)
	require.Empty(t, Holding(listeners, "tcp", netip.MustParseAddrPort("127.0.0.1:1603")))
}

// The sockets of this test process are found with it, on a real /proc.
func TestListThisProcess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	at := netip.MustParseAddrPort(listener.Addr().String())
	listeners, err := List("/")
	require.NoError(t, err)
	holding := Holding(listeners, "tcp", at)
	require.Len(t, holding, 1)
	require.Equal(t, os.Getpid(), holding[0].PID)
}
