package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/v1rushb/neoncoup/protocol"
)

// wsConn adapts a WebSocket to Conn. Text messages carry protocol frames.
type wsConn struct {
	c      *websocket.Conn
	addr   string
	once   sync.Once
	cancel context.CancelFunc
}

func newWS(c *websocket.Conn, addr string) *wsConn {
	c.SetReadLimit(protocol.MaxFrame)
	return &wsConn{c: c, addr: addr}
}

func (w *wsConn) Send(ctx context.Context, frame []byte) error {
	if err := w.c.Write(ctx, websocket.MessageText, frame); err != nil {
		return fmt.Errorf("%w: %w", ErrClosed, err)
	}
	return nil
}

func (w *wsConn) Recv(ctx context.Context) ([]byte, error) {
	typ, data, err := w.c.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: %w", ErrClosed, err)
	}
	if typ != websocket.MessageText {
		// The game protocol is text only; a binary frame is a broken or
		// hostile client.
		_ = w.c.Close(websocket.StatusUnsupportedData, "text frames only")
		return nil, fmt.Errorf("%w: binary frame", ErrClosed)
	}
	return data, nil
}

func (w *wsConn) Close() error {
	w.once.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		// A clean close waits for the peer's acknowledgement; don't block
		// the caller on a slow or vanished peer.
		go func() { _ = w.c.Close(websocket.StatusNormalClosure, "bye") }()
	})
	return nil
}

func (w *wsConn) Addr() string { return w.addr }

// keepalive pings the peer until the connection closes. A Recv loop must be
// running for pongs to be processed.
func (w *wsConn) keepalive(every time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, every)
				err := w.c.Ping(pctx)
				pcancel()
				if err != nil {
					_ = w.c.CloseNow()
					return
				}
			}
		}
	}()
}

// KeepaliveInterval is how often idle WebSocket connections are pinged.
var KeepaliveInterval = 15 * time.Second

// Accept upgrades an HTTP request to a WebSocket Conn.
func Accept(w http.ResponseWriter, r *http.Request) (Conn, error) {
	// Terminal clients send no Origin header and are accepted. Browsers do,
	// and the default same-origin check stops a web page from quietly
	// joining a LAN host's room.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return nil, err
	}
	ws := newWS(c, r.RemoteAddr)
	ws.keepalive(KeepaliveInterval)
	return ws, nil
}

// Dial connects to a host. addr may be "host:port", "ws://host:port/path" or
// "wss://...". A bare host:port gets the default /ws path.
func Dial(ctx context.Context, addr string) (Conn, error) {
	u, err := WSURL(addr, "/ws")
	if err != nil {
		return nil, err
	}
	c, resp, err := websocket.Dial(ctx, u, nil)
	closeBody(resp)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", u, err)
	}
	ws := newWS(c, u)
	ws.keepalive(KeepaliveInterval)
	return ws, nil
}

// closeBody releases a handshake response body, if there is one.
func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// WSURL normalizes an address into a ws:// or wss:// URL, adding defPath
// when the address has no path.
func WSURL(addr, defPath string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", errors.New("empty address")
	}
	switch {
	case strings.HasPrefix(addr, "http://"):
		addr = "ws://" + strings.TrimPrefix(addr, "http://")
	case strings.HasPrefix(addr, "https://"):
		addr = "wss://" + strings.TrimPrefix(addr, "https://")
	case !strings.HasPrefix(addr, "ws://") && !strings.HasPrefix(addr, "wss://"):
		addr = "ws://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("bad address %q: %w", addr, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("bad address %q: no host", addr)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = defPath
	}
	return u.String(), nil
}
