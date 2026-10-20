// Package sockets tells who listens where: the sockets of the system that wait for
// connections (TCP) or datagrams (UDP), read from /proc/net, with the process that holds
// each, found by the socket's inode among the descriptors of /proc/<pid>/fd. It needs no
// program of the system (netstat, ss).
package sockets

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Listener is a socket that waits for connections or datagrams.
type Listener struct {
	// Protocol is "tcp" or "udp".
	Protocol string
	Address  netip.AddrPort
	// PID and Program are the process that holds the socket: none when it is not found
	// (a socket of the kernel, or a process that went away meanwhile).
	PID     int
	Program string
}

// String names the listener as a person reads it: "dnsmasq (pid 7600) on 0.0.0.0:53".
func (l Listener) String() string {
	return l.who() + " on " + l.Address.String()
}

func (l Listener) who() string {
	if l.PID == 0 {
		return "a socket without a process"
	}
	return l.Program + " (pid " + strconv.Itoa(l.PID) + ")"
}

// Describe names the processes that hold listeners, each once with the addresses it
// listens on, whatever the protocol: "dnsmasq (pid 7600) on 0.0.0.0:53 and [::]:53".
func Describe(listeners []Listener) string {
	var order []string
	addresses := make(map[string][]string)
	for _, listener := range listeners {
		who := listener.who()
		if _, seen := addresses[who]; !seen {
			order = append(order, who)
		}
		address := listener.Address.String()
		if !slices.Contains(addresses[who], address) {
			addresses[who] = append(addresses[who], address)
		}
	}
	parts := make([]string, 0, len(order))
	for _, who := range order {
		parts = append(parts, who+" on "+strings.Join(addresses[who], " and "))
	}
	return strings.Join(parts, "; ")
}

// List returns the listeners of the system. Root is put before /proc ("/" on a router; a
// test gives a directory with files of the same shape).
func List(root string) ([]Listener, error) {
	var listeners []Listener
	inodes := make(map[uint64][]int)
	for _, file := range []struct {
		name     string
		protocol string
		v6       bool
	}{{"tcp", "tcp", false}, {"tcp6", "tcp", true}, {"udp", "udp", false}, {"udp6", "udp", true}} {
		content, err := os.ReadFile(filepath.Join(root, "proc/net", file.name))
		if err != nil {
			// a kernel without IPv6 has no tcp6 and udp6
			continue
		}
		for _, entry := range parseTable(content, file.protocol, file.v6) {
			listeners = append(listeners, entry.listener)
			inodes[entry.inode] = append(inodes[entry.inode], len(listeners)-1)
		}
	}
	if len(listeners) == 0 {
		return nil, nil
	}
	owners(root, inodes, listeners)
	return listeners, nil
}

type entry struct {
	listener Listener
	inode    uint64
}

// parseTable reads /proc/net/{tcp,udp}[6]: the TCP sockets in LISTEN, the UDP sockets bound
// and not connected. An address is the bytes of the kernel written as host-order words in
// hexadecimal, a word for IPv4 and four for IPv6; the port is in host order already.
func parseTable(content []byte, protocol string, v6 bool) []entry {
	var entries []entry
	scanner := bufio.NewScanner(bytes.NewReader(content))
	first := true
	for scanner.Scan() {
		if first {
			// the header line
			first = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 {
			continue
		}
		state := fields[3]
		if protocol == "tcp" && state != "0A" {
			continue
		}
		if protocol == "udp" && !strings.HasSuffix(fields[2], ":0000") {
			// a connected UDP socket is a client, not a listener
			continue
		}
		local, ok := parseAddress(fields[1], v6)
		if !ok {
			continue
		}
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, entry{Listener{Protocol: protocol, Address: local}, inode})
	}
	return entries
}

func parseAddress(text string, v6 bool) (netip.AddrPort, bool) {
	hexAddress, hexPort, found := strings.Cut(text, ":")
	if !found {
		return netip.AddrPort{}, false
	}
	raw, err := hex.DecodeString(hexAddress)
	if err != nil || (len(raw) != 4 && len(raw) != 16) || (len(raw) == 16) != v6 {
		return netip.AddrPort{}, false
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	// each word was printed as a number of the host's byte order: written back in that
	// order, its bytes are the address as the network has it
	address := make([]byte, len(raw))
	for offset := 0; offset < len(raw); offset += 4 {
		binary.NativeEndian.PutUint32(address[offset:], binary.BigEndian.Uint32(raw[offset:]))
	}
	ip, _ := netip.AddrFromSlice(address)
	return netip.AddrPortFrom(ip.Unmap(), uint16(port)), true
}

// owners finds the process of each socket: a descriptor of /proc/<pid>/fd that links to
// socket:[<inode>].
func owners(root string, inodes map[uint64][]int, listeners []Listener) {
	processes, err := os.ReadDir(filepath.Join(root, "proc"))
	if err != nil {
		return
	}
	left := len(inodes)
	for _, process := range processes {
		pid, err := strconv.Atoi(process.Name())
		if err != nil {
			continue
		}
		directory := filepath.Join(root, "proc", process.Name(), "fd")
		descriptors, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		var program string
		for _, descriptor := range descriptors {
			target, err := os.Readlink(filepath.Join(directory, descriptor.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			inode, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]"), 10, 64)
			if err != nil {
				continue
			}
			indexes, found := inodes[inode]
			if !found {
				continue
			}
			if program == "" {
				comm, _ := os.ReadFile(filepath.Join(root, "proc", process.Name(), "comm"))
				program = strings.TrimSpace(string(comm))
			}
			for _, index := range indexes {
				listeners[index].PID = pid
				listeners[index].Program = program
			}
			delete(inodes, inode)
			left--
			if left == 0 {
				return
			}
		}
	}
}

// Holding returns the listeners that take a protocol's port at an address: one bound to
// that address, and one bound to every address of the system (0.0.0.0, or :: for IPv6,
// which takes IPv4 too unless it is IPv6 only, which /proc/net does not tell).
func Holding(listeners []Listener, protocol string, at netip.AddrPort) []Listener {
	var holding []Listener
	for _, listener := range listeners {
		if listener.Protocol != protocol || listener.Address.Port() != at.Port() {
			continue
		}
		address := listener.Address.Addr()
		switch {
		case address == at.Addr():
		case address.IsUnspecified() && (address.Is4() == at.Addr().Is4() || address.Is6()):
		default:
			continue
		}
		holding = append(holding, listener)
	}
	return holding
}
