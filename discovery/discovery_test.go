package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMessageParsing(t *testing.T) {
	if r, ok := parseFind("NEONCOUP/1 FIND NEON-7K2Q"); !ok || r != "NEON-7K2Q" {
		t.Fatal(r, ok)
	}
	for _, bad := range []string{"", "NEONCOUP/2 FIND X", "NEONCOUP/1 HERE X 1", "NEONCOUP/1 FIND"} {
		if _, ok := parseFind(bad); ok {
			t.Errorf("parseFind(%q) accepted", bad)
		}
	}
	if r, p, ok := parseHere("NEONCOUP/1 HERE NEON-7K2Q 7777"); !ok || r != "NEON-7K2Q" || p != 7777 {
		t.Fatal(r, p, ok)
	}
	for _, bad := range []string{"NEONCOUP/1 HERE X 0", "NEONCOUP/1 HERE X 99999", "NEONCOUP/1 HERE X port"} {
		if _, _, ok := parseHere(bad); ok {
			t.Errorf("parseHere(%q) accepted", bad)
		}
	}
}

func TestFindRoomOnThisMachine(t *testing.T) {
	r, err := Listen(func(room string) (int, bool) { return 7777, room == "NEON-TEST" })
	if err != nil {
		t.Skipf("multicast unavailable here: %v", err)
	}
	defer r.Close()
	// A second host on the same machine must coexist (SO_REUSEADDR).
	r2, err := Listen(func(room string) (int, bool) { return 8888, room == "NEON-OTHR" })
	if err != nil {
		t.Fatalf("second responder: %v", err)
	}
	defer r2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addr, err := Find(ctx, "NEON-TEST")
	if errors.Is(err, ErrNotFound) {
		t.Skip("no multicast/broadcast route in this environment")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(addr, ":7777") {
		t.Fatalf("addr %s", addr)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if addr, err := Find(ctx2, "NEON-OTHR"); err != nil || !strings.HasSuffix(addr, ":8888") {
		t.Fatalf("second room: %s %v", addr, err)
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel3()
	if _, err := Find(ctx3, "NEON-NONE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown room: %v", err)
	}
}
