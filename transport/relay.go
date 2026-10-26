package transport

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/v1rushb/neoncoup/protocol"
)

// Relay link framing. After a JSON register/registered handshake, the host
// and relay exchange binary messages: op (1 byte), stream id (4 bytes, big
// endian), payload. The payload of OpData is one protocol frame, which the
// relay never inspects.
const (
	OpOpen  byte = 1 // relay -> host: a player connected (payload: remote address)
	OpData  byte = 2 // both ways: a frame for stream id
	OpClose byte = 3 // both ways: the stream ended
)

// RelayRegister is the host's first message to a relay. Key is a secret the
// host picks once per room; the relay keeps the room code reserved for that
// key for a while after the link drops, so nobody else can take it over
// while the host re-registers.
type RelayRegister struct {
	Proto int    `json:"proto"`
	Room  string `json:"room"`
	Key   string `json:"key"`
}

// EncodeMux builds a relay link message.
func EncodeMux(op byte, id uint32, payload []byte) []byte {
	b := make([]byte, 5+len(payload))
	b[0] = op
	binary.BigEndian.PutUint32(b[1:5], id)
	copy(b[5:], payload)
	return b
}

// DecodeMux splits a relay link message.
func DecodeMux(b []byte) (op byte, id uint32, payload []byte, err error) {
	if len(b) < 5 {
		return 0, 0, nil, errors.New("relay: short link message")
	}
	return b[0], binary.BigEndian.Uint32(b[1:5]), b[5:], nil
}

// RelayHost is a host's registration with a relay. Every player who joins
// the room through the relay arrives as a Conn from Accept.
type RelayHost struct {
	ws      *websocket.Conn
	room    string
	accept  chan Conn
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	streams map[uint32]*muxConn
	err     error
}

// DialRelayHost registers room with the relay at relayURL, owned by key.
func DialRelayHost(ctx context.Context, relayURL, room, key string) (*RelayHost, error) {
	u, err := WSURL(strings.TrimRight(relayURL, "/")+"/host", "/host")
	if err != nil {
		return nil, err
	}
	ws, resp, err := websocket.Dial(ctx, u, nil)
	closeBody(resp)
	if err != nil {
		return nil, fmt.Errorf("relay unreachable: %w", err)
	}
	ws.SetReadLimit(protocol.MaxFrame + 16)
	reg, _ := protocol.Encode("register", RelayRegister{Proto: protocol.Version, Room: room, Key: key})
	if err := ws.Write(ctx, websocket.MessageText, reg); err != nil {
		_ = ws.CloseNow()
		return nil, err
	}
	_, reply, err := ws.Read(ctx)
	if err != nil {
		_ = ws.CloseNow()
		return nil, fmt.Errorf("relay handshake: %w", err)
	}
	env, err := protocol.Decode(reply)
	if err != nil {
		_ = ws.CloseNow()
		return nil, err
	}
	if env.Type != "registered" {
		var rej protocol.Reject
		_ = json.Unmarshal(env.Data, &rej)
		_ = ws.CloseNow()
		return nil, fmt.Errorf("relay refused the room: %s", rej.Reason)
	}
	h := &RelayHost{ws: ws, room: room, accept: make(chan Conn, 16), done: make(chan struct{}), streams: map[uint32]*muxConn{}}
	go h.readLoop()
	go func() {
		// Keep the link alive through NAT timeouts.
		w := &wsConn{c: ws}
		w.keepalive(KeepaliveInterval)
		<-h.done
		w.cancel()
	}()
	return h, nil
}

func (h *RelayHost) readLoop() {
	defer h.shutdown(errors.New("relay link closed"))
	ctx := context.Background()
	for {
		typ, data, err := h.ws.Read(ctx)
		if err != nil {
			h.shutdown(err)
			return
		}
		if typ != websocket.MessageBinary {
			continue
		}
		op, id, payload, err := DecodeMux(data)
		if err != nil {
			continue
		}
		switch op {
		case OpOpen:
			c := &muxConn{h: h, id: id, in: make(chan []byte, 256), done: make(chan struct{}), addr: "relay:" + string(payload)}
			h.mu.Lock()
			h.streams[id] = c
			h.mu.Unlock()
			select {
			case h.accept <- c:
			case <-h.done:
				return
			}
		case OpData:
			h.mu.Lock()
			c := h.streams[id]
			h.mu.Unlock()
			if c == nil {
				continue
			}
			select {
			case c.in <- append([]byte(nil), payload...):
			default:
				// A player flooding us gets cut off instead of stalling the link.
				c.closeLocal()
				_ = h.send(OpClose, id, nil)
			}
		case OpClose:
			h.mu.Lock()
			c := h.streams[id]
			delete(h.streams, id)
			h.mu.Unlock()
			if c != nil {
				c.closeLocal()
			}
		}
	}
}

// send writes to the shared link. It deliberately has no timeout: with
// coder/websocket a write timeout closes the whole connection, which would
// drop every relayed player at once. A dead link is caught by keepalive.
func (h *RelayHost) send(op byte, id uint32, payload []byte) error {
	select {
	case <-h.done:
		return ErrClosed
	default:
	}
	return h.ws.Write(context.Background(), websocket.MessageBinary, EncodeMux(op, id, payload))
}

func (h *RelayHost) shutdown(err error) {
	h.once.Do(func() {
		h.mu.Lock()
		h.err = err
		streams := h.streams
		h.streams = map[uint32]*muxConn{}
		h.mu.Unlock()
		close(h.done)
		for _, c := range streams {
			c.closeLocal()
		}
		_ = h.ws.Close(websocket.StatusNormalClosure, "host closing")
	})
}

// Accept waits for the next relayed player.
func (h *RelayHost) Accept(ctx context.Context) (Conn, error) {
	select {
	case c := <-h.accept:
		return c, nil
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return nil, fmt.Errorf("%w: %w", ErrClosed, h.err)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Done is closed when the relay link drops.
func (h *RelayHost) Done() <-chan struct{} { return h.done }

// Close unregisters from the relay.
func (h *RelayHost) Close() { h.shutdown(errors.New("closed")) }

// muxConn is one relayed player stream.
type muxConn struct {
	h    *RelayHost
	id   uint32
	in   chan []byte
	done chan struct{}
	once sync.Once
	addr string
}

func (c *muxConn) Send(ctx context.Context, frame []byte) error {
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	if err := c.h.send(OpData, c.id, frame); err != nil {
		return fmt.Errorf("%w: %w", ErrClosed, err)
	}
	return nil
}

func (c *muxConn) Recv(ctx context.Context) ([]byte, error) {
	select {
	case f := <-c.in:
		return f, nil
	case <-c.done:
		select {
		case f := <-c.in:
			return f, nil
		default:
			return nil, ErrClosed
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *muxConn) closeLocal() { c.once.Do(func() { close(c.done) }) }

func (c *muxConn) Close() error {
	c.h.mu.Lock()
	_, live := c.h.streams[c.id]
	delete(c.h.streams, c.id)
	c.h.mu.Unlock()
	if live {
		// Close may be called from a room's goroutine; never block it on
		// the shared link.
		go func() { _ = c.h.send(OpClose, c.id, nil) }()
	}
	c.closeLocal()
	return nil
}

func (c *muxConn) Addr() string { return c.addr }
