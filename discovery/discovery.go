// Package discovery finds NEON//COUP rooms on the local network. A joiner
// multicasts (and broadcasts) "NEONCOUP/1 FIND <room>"; any host serving that
// room answers "NEONCOUP/1 HERE <room> <port>" by unicast. It is the same
// query/answer idea as mDNS, without the dependency.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Port and Group are the discovery rendezvous.
var (
	Port  = 47777
	Group = net.IPv4(239, 255, 77, 77)
)

const magic = "NEONCOUP/1"

// Lookup reports the TCP port serving a room, if this host has it.
type Lookup func(room string) (port int, ok bool)

// Responder answers discovery queries until closed.
type Responder struct {
	conn   *net.UDPConn
	lookup Lookup
	wg     sync.WaitGroup
}

// Listen starts answering queries. ListenMulticastUDP binds the port with
// SO_REUSEADDR, so several hosts on one machine can all answer; the socket
// also receives broadcast queries.
func Listen(lookup Lookup) (*Responder, error) {
	conn, err := net.ListenMulticastUDP("udp4", nil, &net.UDPAddr{IP: Group, Port: Port})
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	r := &Responder{conn: conn, lookup: lookup}
	r.wg.Add(1)
	go r.serve()
	return r, nil
}

func (r *Responder) serve() {
	defer r.wg.Done()
	buf := make([]byte, 512)
	for {
		n, from, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		room, ok := parseFind(string(buf[:n]))
		if !ok {
			continue
		}
		port, ok := r.lookup(room)
		if !ok {
			continue
		}
		reply := fmt.Sprintf("%s HERE %s %d", magic, room, port)
		// Answer from a fresh socket so the reply's source address is a
		// routable interface address, not the multicast group.
		if c, err := net.DialUDP("udp4", nil, from); err == nil {
			_, _ = c.Write([]byte(reply))
			_ = c.Close()
		}
	}
}

// Close stops the responder.
func (r *Responder) Close() error {
	err := r.conn.Close()
	r.wg.Wait()
	return err
}

func parseFind(s string) (string, bool) {
	f := strings.Fields(s)
	if len(f) != 3 || f[0] != magic || f[1] != "FIND" {
		return "", false
	}
	return f[2], true
}

func parseHere(s string) (room string, port int, ok bool) {
	f := strings.Fields(s)
	if len(f) != 4 || f[0] != magic || f[1] != "HERE" {
		return "", 0, false
	}
	p, err := strconv.Atoi(f[3])
	if err != nil || p <= 0 || p > 65535 {
		return "", 0, false
	}
	return f[2], p, true
}

// ErrNotFound means nobody on the LAN answered for the room.
var ErrNotFound = errors.New("room not found on the local network")

// Find asks the LAN for a room and returns the first "host:port" that
// answers, or ErrNotFound when ctx expires.
func Find(ctx context.Context, room string) (string, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return "", fmt.Errorf("discovery: %w", err)
	}
	defer func() { _ = conn.Close() }()
	query := []byte(fmt.Sprintf("%s FIND %s", magic, room))
	targets := append([]*net.UDPAddr{{IP: Group, Port: Port}, {IP: net.IPv4bcast, Port: Port}}, broadcastAddrs()...)
	send := func() {
		for _, t := range targets {
			_, _ = conn.WriteToUDP(query, t)
		}
	}
	found := make(chan string, 1)
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			r, port, ok := parseHere(string(buf[:n]))
			if ok && r == room {
				select {
				case found <- net.JoinHostPort(from.IP.String(), strconv.Itoa(port)):
				default:
				}
				return
			}
		}
	}()
	send()
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case addr := <-found:
			return addr, nil
		case <-ctx.Done():
			return "", ErrNotFound
		case <-t.C:
			send()
		}
	}
}

// broadcastAddrs lists the directed broadcast address of every IPv4 LAN.
func broadcastAddrs() []*net.UDPAddr {
	var out []*net.UDPAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			ip := n.IP.To4()
			mask := net.IP(n.Mask).To4()
			if mask == nil {
				continue
			}
			b := make(net.IP, 4)
			for i := range 4 {
				b[i] = ip[i] | ^mask[i]
			}
			out = append(out, &net.UDPAddr{IP: b, Port: Port})
		}
	}
	return out
}

// LocalIPs lists this machine's non-loopback IPv4 addresses, for printing
// direct-join instructions.
func LocalIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}
