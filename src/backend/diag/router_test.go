package diag

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The configuration dnsmasq runs with is the file of its command line and the files of its
// conf-dir lines: the servers every name goes to are those without a domain.
func TestDnsmasqForwards(t *testing.T) {
	root := t.TempDir()
	procRoot = root
	defer func() { procRoot = "/" }()
	write := func(name string, content string) {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("proc/7600/cmdline", "/usr/sbin/dnsmasq\x00-C\x00/var/etc/dnsmasq.conf.cfg01411c\x00-k\x00-x\x00/var/run/dnsmasq/dnsmasq.cfg01411c.pid\x00")
	write("var/etc/dnsmasq.conf.cfg01411c", "domain-needed\nserver=127.0.0.42\nno-resolv\nserver=/lan/\nconf-dir=/tmp/dnsmasq.cfg01411c.d\n")
	write("tmp/dnsmasq.cfg01411c.d/other.conf", "server=127.0.0.1#5053\nserver=/example.org/192.0.2.1\n")
	servers, noResolv, read := dnsmasqForwards(7600)
	require.True(t, read)
	require.True(t, noResolv)
	require.Equal(t, []string{"127.0.0.42", "127.0.0.1#5053"}, servers)
	_, _, read = dnsmasqForwards(7601)
	require.False(t, read)
}
