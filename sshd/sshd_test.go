package sshd

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func freeAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestSSHSessionShowsTheMenu(t *testing.T) {
	addr := freeAddr(t)
	s, err := New(Options{Addr: addr, HostKeyPath: filepath.Join(t.TempDir(), "key"), Version: "test", Instant: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx) }()
	defer func() {
		cancel()
		<-done
	}()

	var client *gossh.Client
	for range 50 {
		client, err = gossh.Dial("tcp", addr, &gossh.ClientConfig{
			User:            "neo",
			Auth:            []gossh.AuthMethod{gossh.Password("")},
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Timeout:         2 * time.Second,
		})
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.RequestPty("xterm-256color", 34, 110, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	out := &syncBuf{}
	sess.Stdout = out
	in, _ := sess.StdinPipe()
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	// Skip the boot sequence and wait for the menu.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(out.String(), "SOLO") {
		_, _ = in.Write([]byte("x"))
		time.Sleep(200 * time.Millisecond)
	}
	got := out.String()
	if !strings.Contains(got, "SOLO") || !strings.Contains(got, "NEO") {
		t.Fatalf("menu not shown over ssh; got %d bytes", len(got))
	}
	if strings.Contains(got, "REPLAYS") {
		t.Fatal("local-only menu items shown over ssh")
	}
	_, _ = in.Write([]byte{3}) // ctrl+c
}
