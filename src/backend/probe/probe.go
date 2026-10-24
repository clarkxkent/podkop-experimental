// Package probe is how the backend learns that a device reaches sing-box the way traffic
// is meant to: through a FakeIP answer of the DNS and the transparent proxy inbound.
//
// A name is made up for the question, a label under a domain every name of which is
// answered from FakeIP and none of which exists. Whoever is checked opens a connection to
// it. When the DNS question came to sing-box the name has a FakeIP address, and when the
// connection came through the firewall mark, the routing table and the inbound, sing-box
// routes it: a Watcher, told of every connection sing-box routes, remembers the label. The
// configuration sends such connections to a port of this host where they are accepted and
// closed, so nothing of a check leaves the router and no server outside is needed. Who
// asked sees an error of the connection either way; the answer is asked of the backend.
package probe

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/podkop-net/podkop-experimental/src/backend/engine"
)

const (
	// Suffix is the domain of the check names.
	Suffix = "fakeipchecker.podkop.net"
	// DeadEndAddress and DeadEndPort are where the configuration sends the connections to
	// check names: ServeDeadEnd accepts them there and closes them.
	DeadEndAddress = "127.0.0.1"
	DeadEndPort    = 1603

	// how many labels are remembered, and for how long
	labelsKept = 256
	labelAge   = 2 * time.Minute
	// what a connection of this host to a check name may take
	knockTimeout = 2 * time.Second
)

// Name returns the check name of a label.
func Name(label string) string {
	return label + "." + Suffix
}

// ValidLabel tells whether a text can be the label of a check name: 8 to 40 lower-case
// letters and digits.
func ValidLabel(label string) bool {
	if len(label) < 8 || len(label) > 40 {
		return false
	}
	for _, character := range label {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

// NewLabel makes a label nobody has used.
func NewLabel() string {
	random := make([]byte, 10)
	_, _ = rand.Read(random)
	return hex.EncodeToString(random)
}

// labelOf returns the label of a check name, and nothing for any other name.
func labelOf(name string) string {
	label, isCheckName := strings.CutSuffix(strings.ToLower(strings.TrimSuffix(name, ".")), "."+Suffix)
	if !isCheckName || !ValidLabel(label) {
		return ""
	}
	return label
}

// Arrival is a connection to a check name that sing-box routed.
type Arrival struct {
	// Inbound is the tag of the inbound the connection came in through.
	Inbound string
	// Source is who opened it.
	Source netip.Addr
	// Address is the address it was opened to: the FakeIP address of the name when FakeIP
	// is set.
	Address netip.Addr
	FakeIP  bool
	At      time.Time
}

// Watcher remembers the check names connections were opened to. Its Note is a hook of
// the instances of sing-box (engine.ConnectionHook): every connection sing-box routes is
// told to it.
type Watcher struct {
	access   sync.Mutex
	arrivals map[string]Arrival
	order    []string
	// changed is closed when a label is added
	changed chan struct{}
}

// NewWatcher makes a watcher that has seen nothing.
func NewWatcher() *Watcher {
	return &Watcher{arrivals: make(map[string]Arrival), changed: make(chan struct{})}
}

// Note remembers a connection to a check name; any other connection is passed over.
func (w *Watcher) Note(connection engine.Connection) {
	// the name a rule replaced, the name the address was given for, the name sniffed
	label := labelOf(connection.ReplacedName)
	if label == "" {
		label = labelOf(connection.Name)
	}
	if label == "" {
		label = labelOf(connection.SniffedName)
	}
	if label == "" {
		return
	}
	arrival := Arrival{Inbound: connection.Inbound, Source: connection.Source, FakeIP: connection.FakeIP, Address: connection.Address, At: time.Now()}
	w.access.Lock()
	defer w.access.Unlock()
	if _, seen := w.arrivals[label]; seen {
		// a browser tries again when its first connection is closed: the first one counts
		return
	}
	for len(w.order) > 0 && (len(w.order) >= labelsKept || arrival.At.Sub(w.arrivals[w.order[0]].At) > labelAge) {
		delete(w.arrivals, w.order[0])
		w.order = w.order[1:]
	}
	w.arrivals[label] = arrival
	w.order = append(w.order, label)
	close(w.changed)
	w.changed = make(chan struct{})
}

// Arrived returns the connection that was seen for a label.
func (w *Watcher) Arrived(label string) (Arrival, bool) {
	w.access.Lock()
	defer w.access.Unlock()
	arrival, seen := w.arrivals[label]
	return arrival, seen
}

// Wait returns the connection seen for a label, waiting for it until the context ends.
// The connection may have come before the call: who checks and who asks do not have to
// agree on an order.
func (w *Watcher) Wait(ctx context.Context, label string) (Arrival, bool) {
	for {
		w.access.Lock()
		arrival, seen := w.arrivals[label]
		changed := w.changed
		w.access.Unlock()
		if seen {
			return arrival, true
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return Arrival{}, false
		}
	}
}

// deadEndRetry is how long ServeDeadEnd waits before it tries an address it could not
// take again.
var deadEndRetry = 10 * time.Second

// deadEndListen is where ServeDeadEnd listens; a test takes a port of its own.
var deadEndListen = net.JoinHostPort(DeadEndAddress, strconv.Itoa(DeadEndPort))

// ServeDeadEnd accepts the connections the configuration sends to the dead end and closes
// them, until the context ends. Without it sing-box would log a refused connection for
// every check. The address may not be there when the backend starts (the loopback is down
// while the network restarts), and the listener may break (no file left to accept with):
// it is listened on again every deadEndRetry until that works, and say is told the first
// failure and the end of it.
func ServeDeadEnd(ctx context.Context, say func(level string, message string)) {
	failed := false
	for ctx.Err() == nil {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", deadEndListen)
		if err == nil {
			if failed {
				failed = false
				say("info", "the connections of the FakeIP checks end at "+deadEndListen+" again")
			}
			err = serveDeadEnd(ctx, listener)
			if ctx.Err() != nil {
				return
			}
		}
		if !failed {
			failed = true
			say("warn", "the connections of the FakeIP checks have no place to end, tried again every "+deadEndRetry.String()+": "+err.Error())
		}
		select {
		case <-ctx.Done():
		case <-time.After(deadEndRetry):
		}
	}
}

// serveDeadEnd closes what comes in until the context ends or the listener breaks; it
// tells what broke it.
func serveDeadEnd(ctx context.Context, listener net.Listener) error {
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	defer listener.Close()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return err
		}
		_ = connection.Close()
	}
}

// Knock opens a connection to the check name of a label the way a program of this host
// would, with its resolver and its routing, starts a TLS handshake as a browser does, and
// waits until the other side closes the connection. It returns the address the name was
// resolved to; a name that is not answered from FakeIP does not exist, and that is the
// error then.
func Knock(ctx context.Context, label string) (netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, knockTimeout)
	defer cancel()
	name := Name(label)
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", name)
	if err != nil {
		return netip.Addr{}, err
	}
	address := addresses[0].Unmap()
	connection, err := (&net.Dialer{}).DialContext(ctx, "tcp", netip.AddrPortFrom(address, 443).String())
	if err != nil {
		return address, err
	}
	defer connection.Close()
	// sing-box routes a connection once it has seen its first bytes; one that sends
	// nothing is routed only after the wait for them, 300 ms. The handshake sends them
	// at once and ends when the dead end closes the connection.
	_ = tls.Client(connection, &tls.Config{ServerName: name}).HandshakeContext(ctx)
	return address, nil
}
