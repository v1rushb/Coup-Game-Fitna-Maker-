// Package transport moves protocol frames between clients and hosts. Every
// transport (in-memory pipe, WebSocket, relayed stream) implements Conn, so
// the server and client never care how they are connected.
package transport

import (
	"context"
	"errors"
	"sync"
)

// ErrClosed is returned by operations on a closed connection.
var ErrClosed = errors.New("transport: connection closed")

// Conn is a bidirectional, message-oriented connection.
type Conn interface {
	// Send writes one frame.
	Send(ctx context.Context, frame []byte) error
	// Recv blocks for the next frame.
	Recv(ctx context.Context) ([]byte, error)
	// Close closes both directions. It is safe to call more than once.
	Close() error
	// Addr describes the remote end, for logs.
	Addr() string
}

// pipeEnd is one side of an in-memory connection.
type pipeEnd struct {
	in, out chan []byte
	done    chan struct{}
	once    *sync.Once
	addr    string
}

// Pipe returns two connected in-memory Conns. Frames are copied, so callers
// may reuse buffers.
func Pipe() (Conn, Conn) {
	ab := make(chan []byte, 1024)
	ba := make(chan []byte, 1024)
	done := make(chan struct{})
	once := &sync.Once{}
	return &pipeEnd{in: ba, out: ab, done: done, once: once, addr: "pipe:a"},
		&pipeEnd{in: ab, out: ba, done: done, once: once, addr: "pipe:b"}
}

func (p *pipeEnd) Send(ctx context.Context, frame []byte) error {
	cp := append([]byte(nil), frame...)
	select {
	case <-p.done:
		return ErrClosed
	default:
	}
	select {
	case p.out <- cp:
		return nil
	case <-p.done:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *pipeEnd) Recv(ctx context.Context) ([]byte, error) {
	select {
	case f := <-p.in:
		return f, nil
	default:
	}
	select {
	case f := <-p.in:
		return f, nil
	case <-p.done:
		// Deliver anything already queued before reporting the close.
		select {
		case f := <-p.in:
			return f, nil
		default:
			return nil, ErrClosed
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *pipeEnd) Close() error {
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *pipeEnd) Addr() string { return p.addr }
