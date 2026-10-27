package subscription

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	t.Parallel()
	store := Store{Dir: filepath.Join(t.TempDir(), "subscriptions")}
	_, found := store.Load("prov1", "https://a.example/sub/1")
	require.False(t, found)
	servers, err := Parse([]byte("vless://u1@a.example:443#A\nbad://b#B\n"), fakeDecode)
	require.NoError(t, err)
	saved := &Saved{Address: AddressKey("https://a.example/sub/1"), Changed: time.Unix(1700000000, 0).UTC(), Route: "direct", Servers: servers}
	require.NoError(t, store.Save("prov1", saved))
	info, err := os.Stat(filepath.Join(store.Dir, "prov1.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	loaded, found := store.Load("prov1", "https://a.example/sub/1")
	require.True(t, found)
	require.Equal(t, saved.Servers[0].Name, loaded.Servers[0].Name)
	require.True(t, SameServers(servers, loaded.Servers))
	// another address: the list is not of it
	_, found = store.Load("prov1", "https://a.example/sub/2")
	require.False(t, found)
	require.NoError(t, store.Save("prov2", saved))
	store.Keep([]string{"prov2"})
	_, found = store.Load("prov1", "https://a.example/sub/1")
	require.False(t, found)
	_, found = store.Load("prov2", "https://a.example/sub/1")
	require.True(t, found)
}

func TestCheck(t *testing.T) {
	t.Parallel()
	servers, err := Parse([]byte("vless://u1@a.example:443#A\nvmess://u2@b.example:443#B\nbad://c#C\n"), fakeDecode)
	require.NoError(t, err)
	var configurations []string
	checkAll := func(configuration []byte) error {
		configurations = append(configurations, string(configuration))
		if strings.Contains(string(configuration), "vmess") {
			return errors.New("outbounds[1]: unknown outbound type: vmess")
		}
		return nil
	}
	checkOne := func(link string) error {
		if strings.HasPrefix(link, "vmess") {
			return errors.New("vmess is not included in this build, install podkop-engine-full")
		}
		return nil
	}
	verdicts := Check(servers, nil, checkAll, checkOne)
	require.Len(t, configurations, 1)
	require.Contains(t, configurations[0], `{"tag":"check-0","server":"a.example"`)
	require.True(t, servers[0].Usable())
	require.Equal(t, "vmess is not included in this build", servers[1].Reason)
	require.False(t, servers[2].Usable())
	require.Len(t, verdicts, 2)
	// known servers are not built again
	again, err := Parse([]byte("vless://u1@a.example:443#A2\nvmess://u2@b.example:443#B2\n"), fakeDecode)
	require.NoError(t, err)
	Check(again, verdicts, checkAll, checkOne)
	require.Len(t, configurations, 1)
	require.Equal(t, "vmess is not included in this build", again[1].Reason)
}
