package protocol

import (
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Length limits for user-supplied text.
const (
	MaxNameLen = 16
	MaxChatLen = 200
)

// StripANSI removes terminal escape sequences (CSI, OSC, DCS and friends),
// C0/C1 control characters and invalid UTF-8. It is applied to every string
// a player can make another player's terminal print.
func StripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			i++
			continue
		}
		switch {
		case r == 0x1b: // ESC
			i += size
			i = skipEscape(s, i)
			continue
		case r == 0x9b: // C1 CSI
			i += size
			i = skipUntilFinal(s, i)
			continue
		case r == 0x9d || r == 0x90 || r == 0x9e || r == 0x9f || r == 0x98: // C1 OSC, DCS, PM, APC, SOS
			i += size
			i = skipUntilST(s, i)
			continue
		case r == '\t' || r == '\n':
			b.WriteRune(' ')
		case unicode.IsControl(r) || r == 0x7f:
			// drop
		case unicode.Is(unicode.Cf, r) && r != 0x200d:
			// format characters (bidi overrides etc.) can reorder text; drop
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// skipEscape skips the sequence following an ESC at s[i-1].
func skipEscape(s string, i int) int {
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[':
		return skipUntilFinal(s, i+1)
	case ']', 'P', '^', '_', 'X':
		return skipUntilST(s, i+1)
	default:
		// Two-character escape (ESC c, ESC 7, ...), possibly with
		// intermediates (ESC ( B).
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i < len(s) {
			i++
		}
		return i
	}
}

// skipUntilFinal skips CSI parameters up to and including the final byte.
func skipUntilFinal(s string, i int) int {
	for i < len(s) {
		c := s[i]
		i++
		if c >= 0x40 && c <= 0x7e {
			break
		}
	}
	return i
}

// skipUntilST skips a string sequence terminated by BEL, ESC \ or C1 ST.
func skipUntilST(s string, i int) int {
	for i < len(s) {
		if s[i] == 0x07 {
			return i + 1
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
		if r, size := utf8.DecodeRuneInString(s[i:]); r == 0x9c {
			return i + size
		}
		i++
	}
	return i
}

// CleanText strips escapes, collapses whitespace and truncates to limit runes.
func CleanText(s string, limit int) string {
	s = strings.Join(strings.Fields(StripANSI(s)), " ")
	if utf8.RuneCountInString(s) > limit {
		r := []rune(s)
		s = string(r[:limit])
	}
	return strings.TrimSpace(s)
}

// CleanName sanitizes a nickname. It may return "" if nothing usable is left.
func CleanName(s string) string {
	s = CleanText(s, MaxNameLen)
	// Brackets are used by the log's "[NAME]" narration.
	s = strings.Map(func(r rune) rune {
		if r == '[' || r == ']' {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// Limiter is a token bucket. It is safe for concurrent use.
type Limiter struct {
	mu     sync.Mutex
	rate   float64 // tokens per second
	burst  float64
	tokens float64
	last   time.Time
}

// NewLimiter allows rate events per second with bursts of burst.
func NewLimiter(rate float64, burst int) *Limiter {
	return &Limiter{rate: rate, burst: float64(burst), tokens: float64(burst)}
}

// Allow reports whether an event at now fits in the budget, consuming a token if so.
func (l *Limiter) Allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.last.IsZero() {
		l.tokens += now.Sub(l.last).Seconds() * l.rate
		if l.tokens > l.burst {
			l.tokens = l.burst
		}
	}
	l.last = now
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

// Room codes look like NEON-7K2Q. The alphabet skips 0/O and 1/I/L.
const (
	RoomPrefix   = "NEON-"
	roomAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	roomCodeLen  = 4
)

// NewRoomCode returns a random room code.
func NewRoomCode() string {
	var b strings.Builder
	b.WriteString(RoomPrefix)
	for range roomCodeLen {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(roomAlphabet))))
		if err != nil {
			panic(err)
		}
		b.WriteByte(roomAlphabet[n.Int64()])
	}
	return b.String()
}

// ParseRoomCode normalizes "neon-7k2q", "7K2Q" or "NEON7K2Q" to "NEON-7K2Q".
func ParseRoomCode(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "NEON")
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "/")
	if len(s) != roomCodeLen {
		return "", false
	}
	for _, c := range s {
		if !strings.ContainsRune(roomAlphabet, c) {
			return "", false
		}
	}
	return RoomPrefix + s, true
}

// NewToken returns a random URL-safe secret used for rejoining.
func NewToken() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 24)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic(err)
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}
