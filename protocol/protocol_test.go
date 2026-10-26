package protocol

import (
	"strings"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/engine"
)

func TestStripANSI(t *testing.T) {
	cases := map[string]string{
		"plain":                          "plain",
		"\x1b[31mred\x1b[0m":             "red",
		"\x1b[2J\x1b[Hwipe":              "wipe",
		"\x1b]0;title\x07after":          "after",
		"\x1b]8;;http://x\x1b\\link":     "link",
		"bell\x07":                       "bell",
		"\x1bPdcs stuff\x1b\\ok":         "ok",
		"\x1b(Bcharset":                  "charset",
		"\u009b31mc1csi":                 "c1csi",
		"tab\there":                      "tab here",
		"null\x00byte":                   "nullbyte",
		"rtl\u202eoverride":              "rtloverride",
		"del\x7f":                        "del",
		"ünïcødé ✓":                      "ünïcødé ✓",
		"\x1b[38;2;255;0;255mtrue\x1b[m": "true",
		"broken\xffutf8":                 "brokenutf8",
		"trailing esc\x1b":               "trailing esc",
	}
	for in, want := range cases {
		if got := StripANSI(in); got != want {
			t.Errorf("StripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanName(t *testing.T) {
	if got := CleanName("  \x1b[5mVEX\x1b[0m  "); got != "VEX" {
		t.Errorf("got %q", got)
	}
	if got := CleanName("[SYSTEM] admin"); got != "SYSTEM admin" {
		t.Errorf("brackets kept: %q", got)
	}
	if got := CleanName(strings.Repeat("x", 40)); len(got) != MaxNameLen {
		t.Errorf("not truncated: %d", len(got))
	}
	if got := CleanName("\x1b[2J"); got != "" {
		t.Errorf("escape-only name became %q", got)
	}
}

func TestCleanText(t *testing.T) {
	got := CleanText("hi\n\n  there\x1b[1A", MaxChatLen)
	if got != "hi there" {
		t.Errorf("got %q", got)
	}
	if n := len([]rune(CleanText(strings.Repeat("é", 500), MaxChatLen))); n != MaxChatLen {
		t.Errorf("len %d", n)
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, 3)
	now := time.Unix(0, 0)
	for i := range 3 {
		if !l.Allow(now) {
			t.Fatalf("burst %d refused", i)
		}
	}
	if l.Allow(now) {
		t.Fatal("over burst allowed")
	}
	if !l.Allow(now.Add(600 * time.Millisecond)) {
		t.Fatal("refill failed")
	}
	if l.Allow(now.Add(700 * time.Millisecond)) {
		t.Fatal("refilled too fast")
	}
}

func TestRoomCodes(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		c := NewRoomCode()
		if _, ok := ParseRoomCode(c); !ok {
			t.Fatalf("generated code %q does not parse", c)
		}
		seen[c] = true
	}
	if len(seen) < 190 {
		t.Fatal("codes collide too often")
	}
	for in, want := range map[string]string{"neon-7k2q": "NEON-7K2Q", "7K2Q": "NEON-7K2Q", "NEON7K2Q": "NEON-7K2Q"} {
		if got, ok := ParseRoomCode(in); !ok || got != want {
			t.Errorf("ParseRoomCode(%q) = %q, %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "NEON-", "NEON-0000", "NEON-7K2", "localhost:7777", "NEON-7K2QQ"} {
		if _, ok := ParseRoomCode(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	frame := MustEncode(TIntent, IntentMsg{Ver: 9, Intent: engine.BlockIntent(engine.Contessa)})
	env, err := Decode(frame)
	if err != nil || env.Type != TIntent {
		t.Fatal(env, err)
	}
	var m IntentMsg
	if err := env.Into(&m); err != nil {
		t.Fatal(err)
	}
	if m.Ver != 9 || m.Intent.Claim != engine.Contessa || m.Intent.Kind != engine.IntentBlock {
		t.Fatalf("%+v", m)
	}
	if _, err := Decode([]byte(`{"d":{}}`)); err == nil {
		t.Fatal("typeless frame accepted")
	}
	if _, err := Decode([]byte(`nope`)); err == nil {
		t.Fatal("junk accepted")
	}
	if _, err := Decode(make([]byte, MaxFrame+1)); err == nil {
		t.Fatal("oversized frame accepted")
	}
}

func TestSettingsClamp(t *testing.T) {
	s := Settings{MaxPlayers: 99, WindowMS: 1, TurnMS: 5, ReconnectS: 0, BotLevel: "god"}.Clamp()
	if s.MaxPlayers != engine.MaxPlayers || s.WindowMS != 2000 || s.TurnMS != 10000 || s.ReconnectS != 5 || s.BotLevel != "normal" {
		t.Fatalf("%+v", s)
	}
	if (Settings{TurnMS: 0}).Clamp().TurnMS != 0 {
		t.Fatal("turn timer 0 means off")
	}
}
