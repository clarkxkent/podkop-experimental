package probe

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/dataplane"
	"github.com/podkop-net/podkop-experimental/src/backend/engine"

	"github.com/stretchr/testify/require"
)

// The readiness name is under the suffix, so a configuration with check names answers it
// from FakeIP whatever its own rule, and it is no check name a page could knock with.
func TestReadinessName(t *testing.T) {
	require.True(t, strings.HasSuffix(dataplane.ReadinessName, "."+Suffix), dataplane.ReadinessName)
	require.Empty(t, labelOf(dataplane.ReadinessName))
}

func TestLabels(t *testing.T) {
	label := NewLabel()
	require.True(t, ValidLabel(label), label)
	require.NotEqual(t, label, NewLabel())
	require.Equal(t, label, labelOf(Name(label)))
	require.Equal(t, label, labelOf(Name(label)+"."), "the name as a DNS question spells it")
	require.Equal(t, "0a1b2c3d4e5f", labelOf("0A1B2C3D4E5F.FakeIPChecker.Podkop.NET"))
	for _, name := range []string{"", Suffix, "www.podkop.net", "0a1b2c3d4e5f.example.com", "short." + Suffix, "two.labels0." + Suffix, "dash-inside." + Suffix, "0a1b2c3d4e5f.x" + Suffix} {
		require.Empty(t, labelOf(name), name)
	}
	for _, label := range []string{"", "1234567", "UPPERCASE0", "with space", "0123456789012345678901234567890123456789x"} {
		require.False(t, ValidLabel(label), label)
	}
}

// through the transparent proxy with a FakeIP address, as sing-box tells of it once a
// rule has sent the connection to the dead end
func arrived(label string, source string) engine.Connection {
	return engine.Connection{
		Inbound: "tproxy-in", Source: netip.MustParseAddrPort(source).Addr(), FakeIP: true,
		Address:      netip.MustParseAddr("198.18.0.7"),
		ReplacedName: Name(label),
	}
}

func note(watcher *Watcher, connection engine.Connection) {
	watcher.Note(connection)
}

func TestWatcher(t *testing.T) {
	watcher := NewWatcher()
	_, seen := watcher.Arrived("0a1b2c3d4e5f")
	require.False(t, seen)

	// other connections pass unseen
	watcher.Note(engine.Connection{Name: "example.com"})
	require.Empty(t, watcher.arrivals)

	watcher.Note(arrived("0a1b2c3d4e5f", "192.168.1.23:50000"))
	arrival, seen := watcher.Arrived("0a1b2c3d4e5f")
	require.True(t, seen)
	require.Equal(t, "tproxy-in", arrival.Inbound)
	require.Equal(t, netip.MustParseAddr("192.168.1.23"), arrival.Source)
	require.Equal(t, netip.MustParseAddr("198.18.0.7"), arrival.Address)
	require.True(t, arrival.FakeIP)

	// the second connection to a name, a browser that tries again, changes nothing
	watcher.Note(arrived("0a1b2c3d4e5f", "192.168.1.99:50001"))
	arrival, _ = watcher.Arrived("0a1b2c3d4e5f")
	require.Equal(t, netip.MustParseAddr("192.168.1.23"), arrival.Source)

	// the name of the address, and the name as it was sniffed, where no address was given
	// for it
	watcher.Note(engine.Connection{Inbound: "mixed-in", Name: Name("named0name0")})
	_, seen = watcher.Arrived("named0name0")
	require.True(t, seen)
	watcher.Note(engine.Connection{Inbound: "mixed-in", SniffedName: Name("sniffed0name")})
	arrival, seen = watcher.Arrived("sniffed0name")
	require.True(t, seen)
	require.Equal(t, "mixed-in", arrival.Inbound)
	require.False(t, arrival.FakeIP)
}

func TestWait(t *testing.T) {
	watcher := NewWatcher()
	// what came before the question
	note(watcher, arrived("came0before0", "192.168.1.23:50000"))
	_, seen := watcher.Wait(context.Background(), "came0before0")
	require.True(t, seen)
	// what comes while the question waits
	time.AfterFunc(100*time.Millisecond, func() { note(watcher, arrived("comes0later00", "192.168.1.23:50000")) })
	began := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, seen = watcher.Wait(ctx, "comes0later00")
	require.True(t, seen)
	require.Less(t, time.Since(began), 2*time.Second)
	// what never comes
	ctx, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	_, seen = watcher.Wait(ctx, "never0comes00")
	require.False(t, seen)
}

func TestLabelsAreForgotten(t *testing.T) {
	watcher := NewWatcher()
	for number := 0; number < labelsKept+10; number++ {
		note(watcher, arrived("label0number"+strconv.Itoa(number), "192.168.1.23:50000"))
	}
	require.Len(t, watcher.arrivals, labelsKept)
	require.Len(t, watcher.order, labelsKept)
	_, seen := watcher.Arrived("label0number0")
	require.False(t, seen)
	_, seen = watcher.Arrived("label0number" + strconv.Itoa(labelsKept+9))
	require.True(t, seen)
}

func TestDeadEnd(t *testing.T) {
	address := net.JoinHostPort(DeadEndAddress, strconv.Itoa(DeadEndPort))
	// the address held by another listener at the start: tried again until it is free
	held, err := net.Listen("tcp", address)
	require.NoError(t, err)
	deadEndRetry = 50 * time.Millisecond
	defer func() { deadEndRetry = 10 * time.Second }()
	var said []string
	var saidAccess sync.Mutex
	say := func(level string, message string) {
		saidAccess.Lock()
		defer saidAccess.Unlock()
		said = append(said, level+" "+message)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ServeDeadEnd(ctx, say)
		close(done)
	}()
	require.Eventually(t, func() bool {
		saidAccess.Lock()
		defer saidAccess.Unlock()
		return len(said) == 1
	}, 5*time.Second, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	held.Close()
	var connection net.Conn
	require.Eventually(t, func() bool {
		var err error
		connection, err = net.DialTimeout("tcp", address, time.Second)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond)
	// accepted and closed: the reader sees the end at once
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = connection.Read(make([]byte, 1))
	require.Error(t, err)
	require.False(t, errors.Is(err, os.ErrDeadlineExceeded), "the connection was left open")
	connection.Close()
	// the failure told once, while it lasted, and its end
	saidAccess.Lock()
	require.Len(t, said, 2, "%v", said)
	require.Contains(t, said[0], "warn the connections of the FakeIP checks have no place to end")
	require.Equal(t, "info the connections of the FakeIP checks end at "+address+" again", said[1])
	saidAccess.Unlock()
	cancel()
	<-done
	_, err = net.DialTimeout("tcp", address, time.Second)
	require.Error(t, err, "the dead end is still served")
}
