# NEON//COUP — Plan

A terminal take on *Coup* with an 8-bit cyberpunk look: authoritative Go server, Bubble Tea
client, chiptune audio synthesized in code, LAN/relay/SSH multiplayer and bots.

## Tech stack (and the choices made)

| Concern      | Choice                                                         | Notes |
|--------------|----------------------------------------------------------------|-------|
| Language     | Go 1.27 (latest stable on this machine)                        | one binary per platform, `CGO_ENABLED=0`; the Linux build is fully static |
| TUI          | `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2` | v2 is the current stable major. It downsamples truecolor to 256/16 colors per output (and honors `NO_COLOR`), which SSH mode needs because every session has its own profile. Bubbles' `textinput` drives every text field, with cursor blink off so idle fields never wake the UI. |
| SSH          | `charm.land/wish/v2`                                           | pairs with Bubble Tea v2 |
| WebSockets   | `github.com/coder/websocket`                                   | maintained successor of nhooyr/websocket, context-aware, no deps |
| Audio        | `github.com/ebitengine/oto/v3` on macOS and Windows; on Linux, `github.com/jfreymuth/pulse` with an `aplay` pipe fallback | **Deviation, explained below.** |
| LAN discovery| stdlib UDP multicast (239.255.77.77:47777) and broadcast       | mDNS-style query/answer, no extra dependency. Several hosts can share a machine because `ListenMulticastUDP` sets `SO_REUSEADDR`. |
| CLI          | stdlib `flag` with hand-rolled subcommands                     | avoids a cobra dependency for ~10 commands |

**Audio deviation (Linux only).** oto v3 is cgo-free everywhere, but on Linux its ALSA fallback
loads `libasound` through purego. That links the dynamic loader in and makes the "static" binary
depend on glibc (`libdl`, `libpthread`, `libc`), which breaks the single-static-binary goal.
oto's first choice on Linux is the pure-Go PulseAudio client `jfreymuth/pulse`, so the Linux
build talks to that client directly (PulseAudio or PipeWire's Pulse server). ALSA-only systems
fall back to piping PCM into `aplay`, and with neither, the game runs muted with the muted icon.
macOS and Windows use oto unchanged. `go build -tags neoncoup_oto` restores oto on Linux for
anyone who prefers it over a static binary.

## Architecture

```
cmd/neoncoup      CLI: solo | tutorial | host | join | relay | serve-ssh | replay | stats | version
engine/           pure rules state machine. No I/O, no clock, seeded PCG RNG, deterministic.
bot/              AI players. Input: engine.PlayerView + redacted engine.Event. Output: engine.Intent.
protocol/         versioned wire messages (JSON envelopes), ANSI stripping, rate limiter, room codes.
server/           authoritative rooms: lobby, seats, timers, reconnect holds, bot seats, idle close.
transport/        Conn interface: in-memory pipe, WebSocket, relay mux (host side of the relay link).
discovery/        LAN UDP multicast/broadcast query and answer.
relay/            NAT-traversal forwarding relay; game logic stays on the host.
sshd/             wish server: one shared hub, a TUI per SSH session, no audio.
config/           OS config dir: settings, rejoin tokens, stats, replays.
client/           Bubble Tea app: boot, menu, setup forms, lobby, table, overlays, tutorial,
                  replay viewer, stats. LocalBackend runs solo/host servers in-process.
  art/            original card art and logo, Unicode and ASCII.
  theme/          palette, styles, ASCII glyph map.
  headless/       a UI-less network client driven by a bot (tests, filling remote seats).
audio/            NES-style synth (2 pulse, triangle, noise), sequencer, songs, SFX, device output.
deploy/           systemd units, Caddyfile; Dockerfile at the root.
```

Data flow: `client` sends an **intent**. The `server` validates it with `engine.Apply`, then sends
every member a **state** (`engine.PlayerView` redacted for that seat) and the new **events**, also
redacted per seat. Hidden cards of other players never leave the server: `PlayerView` has no field
that can hold another seat's face-down cards, and `Event.Redact(seat)` strips private payloads
(replacement draws, exchange draws).

Bots implement `bot.Bot` (`Observe(engine.Event)`, `Decide(engine.PlayerView) engine.Intent`).
They never get an `*engine.Game`, so the compiler rules out access to hidden state. Bot seats in
online games run through the same redaction path as remote humans.

### Turn state machine

`engine.Phase`. The waiting phases are the ones where `Awaiting()` is non-empty.

```
TurnStart ──declare──▶ ActionDeclared (transient)
   │                       ├─ Income/Coup ─────────────────────────────▶ Resolve
   │                       ├─ ForeignAid ──────────────▶ BlockWindow
   │                       └─ character action ─▶ ChallengeWindow
ChallengeWindow ── all pass ─▶ BlockWindow (if blockable) / Resolve
               └─ challenge ─▶ ProveClaim ─ proven ─▶ LoseInfluence(challenger) ─▶ BlockWindow/Resolve
                                          └ failed ─▶ (revealed card lost) ─▶ TurnEnd (action fails, Assassin refunded)
BlockWindow ── all pass ─▶ Resolve
            └─ block ───▶ BlockChallengeWindow ── all pass ─▶ TurnEnd (blocked)
                                               └─ challenge ─▶ ProveClaim ─ proven ─▶ LoseInfluence(challenger) ─▶ TurnEnd (blocked)
                                                                           └ failed ─▶ (blocker loses card) ─▶ Resolve
Resolve ─▶ effects; Coup/Assassinate ─▶ LoseInfluence(target); Exchange ─▶ Exchange
LoseInfluence / Exchange ─▶ TurnEnd (transient) ─▶ next living player's TurnStart, or GameOver
```

`ProveClaim` is the one phase not in the brief. The rulebook says a challenged player who *can't
or doesn't want to* prove the claim loses the challenge. So the claimant picks which face-down
card to flip: the claimed character proves the claim, any other card is the influence lost. The
engine decides automatically only when a player has a single face-down card, which everyone
already knows. A player holding two identical cards is still asked, because skipping the choice
would tell the table what the other card is (this applies to `LoseInfluence` too).

The engine has no clock. The server owns timers and, when a timer runs out, submits `Pass` for
everyone still pending, or the engine's `DefaultIntent` for a decision.

## Protocol (v1)

> Protocol v2 (the UX update) adds the room speed, presentation timing in `state`
> (`opens_in_ms`, `speed`, `paused`), the solo `control` message and host-side deferral of
> early intents. See [UX_PLAN.md](UX_PLAN.md#protocol-v2). The tables below describe v1, which
> v2 extends.

Every frame is a JSON envelope: `{"t": "<type>", "d": {...}}`. The first client frame must be
`hello`. A `proto` mismatch is answered with `reject` (code `version`) and the connection closes.

Client → server

| type       | payload | notes |
|------------|---------|-------|
| `hello`    | `proto, client, name, token?, room?, spectate` | `token` reclaims a held seat |
| `ready`    | `ready` | lobby ready-up |
| `settings` | `max_players, fill_bots, bot_level, window_ms, turn_ms, magistrate, reconnect_s` | host only |
| `start`    | — | host only. Needs ≥2 seats and every human ready. |
| `intent`   | `ver, intent{kind, action, target, claim, card, keep}` | `ver` must equal the current state version; stale intents are refused |
| `chat`     | `text` | sanitized, length-capped, rate-limited |
| `emote`    | `id` | quick taunts |
| `leave`    | — | |
| `ping`     | `t` | |

Server → client

| type        | payload | notes |
|-------------|---------|-------|
| `welcome`   | `proto, you, token, room, host` | |
| `reject`    | `code, reason` | `version`, `full`, `not_found`, `bad_hello`, `closed` |
| `lobby`     | `room, members[], settings, host_id` | |
| `game_start`| `seats[{seat,name,bot,level}], you_seat, settings` | |
| `state`     | `ver, view(PlayerView), deadline_ms, window_ms` | redacted per member |
| `events`    | `events[]` | redacted per member, used for narration and animations |
| `chat`      | `from, name, text, system` | |
| `emote`     | `from, name, id` | |
| `error`     | `code, msg` | non-fatal (illegal intent, rate limit) |
| `game_over` | `winner, names, replay` | the replay holds the seed and intents, safe to send once the game is over |
| `pong`      | `t` | |

Relay link (host ↔ relay, `/host`): `register{proto, room, key}` → `registered` | `reject`, then
`open{cid}`, `data{cid, frame}`, `close{cid}` in both directions. Players connect to
`/join/<ROOM>` and speak the normal protocol; the relay forwards frames without reading them.

The `key` is a secret the host picks once per room. After a host's link drops, the relay keeps
the code reserved for that key for two minutes, so nobody can take the room over while the host
re-registers. The relay caps rooms per IP, players per room, and frames and bytes per player.
It drops players who stay silent for 10 s after connecting. Writes to a host link have no
timeout: with coder/websocket a write timeout closes the whole connection, which would let one
player cut off a room.

LAN discovery: the joiner sends `NEONCOUP/1 FIND <ROOM>` to 239.255.77.77:47777 and to the
broadcast address, and the host answers `NEONCOUP/1 HERE <ROOM> <port>` by unicast.

## Bots

| Level  | Name          | Behavior |
|--------|---------------|-----------|
| easy   | Script Kiddie | mostly honest, random legal moves, rare challenges (impossible claims only, plus about 4% noise) |
| normal | Fixer         | claims what it holds, bluffs Duke/Contessa sometimes, counts cards (own hand + face-up cards) to challenge impossible or unlikely claims |
| hard   | Black ICE     | per-opponent probability of each character: a hypergeometric prior over the unseen cards, updated by likelihood ratios for claims, blocks, blocks not made and failed proofs (reset on exchange or replacement). Expected-value action choice, an adaptive estimate of each opponent's challenge rate, targets the leader, avoids sitting on a coin pile, endgame rules (a winning Coup, forced Contessa bluffs on the last card, steals that knock a rival out of Coup range). |

Each bot gets a cyberpunk handle and personality (bluff/suspicion/aggression multipliers, taunt
lines) and a 0.8–2.5 s think delay with a spinner in online and solo games. Simulations run with
no delay.

Measured over fixed seeds (bot tests): in three-way games Black ICE wins about 50%, Fixer 33% and
Script Kiddie 17%. One-on-one, Black ICE beats Fixer about 73% of the time and Fixer beats Script
Kiddie about 78%.

**Stalemates.** Coin-neutral loops are possible within the rules (Steal ping-pong, or the
Magistrate's Audit against a player who keeps taking Foreign Aid), and early bots could get stuck
in them. Black ICE now presses harder when nobody has lost influence for 24 turns (more
challenges, braver bluffs, no value in pure-denial Audits). As a backstop, bots switch to
Income/Coup after turn 200, which always ends the game. In the 10,000-game simulation 7 games
reach that point. Humans are not restricted.

## Rules decisions and rulebook cross-check

Checked against the official Indie Boards & Cards rulebook text (via rulespal.com/coup/rulebook).

Matches the brief:
- Challenged action fails and its cost is refunded ("any coins paid as the cost of the action are
  returned"); a blocked action keeps its cost spent.
- Winning a challenge: return the card, reshuffle, draw a random replacement.
- Start your turn with 10+ coins and you must Coup.
- 2 players: the starting player begins with 1 coin.

**Discrepancies and gaps, flagged rather than guessed:**
1. **Official 2-player setup is a draft** (each player gets one of each character, keeps one, and
   is dealt a second from a third set; the court deck is 3 cards). The brief asks only for the
   1-coin rule, so the engine uses the normal deal for 2 players. The draft is listed in TODO.md
   as an optional variant.
2. **Declining to prove**: the rulebook allows a challenged player to lose on purpose. Handled by
   the `ProveClaim` phase above.
3. **Eliminated players' coins** go back to the Treasury (rulebook). The brief doesn't say; the
   engine follows the rulebook and logs it.
4. **Starting player**: the rulebook says the last game's winner starts. The first game in a room
   picks a random seat (seeded); rematches in the same room start with the previous winner.
5. **Treasury size**: 50 coins, as in the box. Gains are capped at what the Treasury holds. The
   rulebook doesn't cover an empty Treasury and in practice it never runs dry.
6. **Steal from a player with 0 coins** is legal and takes nothing. The rulebook doesn't forbid it,
   and it is a legal Captain bluff.
7. **Block after losing a challenge**: if the target of an assassination challenges the Assassin,
   loses, and still has a card, the block window still opens and they may claim Contessa. Read
   literally, the rulebook order (challenge, then counteraction) allows it. The brief's
   double-loss case is the version where they don't block; both are tested.
8. **Magistrate** isn't in the official rules. Interpretation: Audit is a Magistrate claim
   (challengeable, not blockable) against a target with ≥3 coins, who pays 2 coins to the
   Treasury. Tax becomes blockable by *any* other player claiming Magistrate (like Duke against
   Foreign Aid), and that block can be challenged. The deck is 18 cards.
9. **Timeouts**: the rulebook has none. Windows (default 8 s) auto-pass whoever hasn't answered.
   Decisions (choosing an action, a card to lose, the exchange, proving a claim) use a separate
   turn timer (default 60 s online, off in solo) and fall back to `DefaultIntent`: Income (or a
   forced Coup on the next living seat), the first card, keeping the current hand, or proving
   when possible.

## Milestones

All seven are implemented. A repository hook blocks commits by the assistant, so each milestone
ended with the full test suite and a suggested commit message for the maintainer instead of a
commit.

1. **Engine + tests**: every rule and edge case above, JSON-stable enums, redaction.
2. **Bots + simulation**: three levels; 10,000 random-seed games asserting coin conservation,
   card accounting, termination and no panics.
3. **Solo TUI**: boot, menu, solo setup, table, overlays, log narration, results.
4. **Host/join + LAN discovery**: WebSocket transport, lobby, ready-up, spectators, settings.
5. **Relay, reconnects, SSH**: relay with Dockerfile and systemd unit, token rejoin with bot
   takeover, automatic client reconnect, wish server.
6. **Audio**: synth, sequencer, songs, SFX, ducking, persisted volume and mute.
7. **Polish**: card flips, coin tickers, elimination glitch, screen jolt, CRT scanlines,
   tutorial, replays, stats, README with text screenshots, golangci-lint clean.

## Testing

- `engine`: table tests per action, challenge, block and edge case; invariant checks after
  every transition (coins sum to 50, cards sum to the deck size, a waiting phase always waits on
  someone); 3,000 random-play games; replay determinism; a fuzz target.
- `bot`: 10,000-game simulation across mixed difficulties, player counts and the Magistrate
  variant (`-short` runs 500), plus a strength-ordering test.
- `server`: lobby/settings rules, stale and illegal intents, rate limits, sanitizing, spectator
  redaction, window timeouts, idle close, disconnect → hold → bot takeover → token rejoin, and an
  integration test where a WebSocket server and 3 headless clients play a full game.
- `client`: full solo games and the whole tutorial driven through real keystrokes in a headless
  Bubble Tea program. Every frame is checked at five terminal sizes for exact dimensions, and
  for pure ASCII in `--ascii` mode. Also LAN/direct/relay joins and automatic reconnect.
- `relay`, `discovery`, `sshd`: forwarding and capacity, multicast find, an SSH session with a
  PTY that renders the menu.
- `audio`: tuning, sequencer timing, song variation, ducking, mute, no clipping or NaN,
  determinism, non-blocking commands; no device needed. `make wav` exports everything.
