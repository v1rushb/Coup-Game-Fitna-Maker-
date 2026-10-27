# UX_PLAN: pacing, focus, card tracker, animations, soundtrack

This update makes the game easier to follow. The look stays the same: same palette, card art, boot
sequence and logo. The rules don't change. Every engine test must still pass unchanged.

## What exists today (the starting point)

- **Server** (`server/room.go`): every applied intent sends redacted `events`, then `state`, to
  each member. A reaction window's timer starts the moment the state goes out. Bots act after a
  random 0.8–2.5 s think delay.
- **Client** (`client/game.go`, `client/table.go`): it applies each frame the moment it arrives.
  Effects are short timestamp-based flashes. When a bot acts quickly, several turns can scroll
  past in under a second.
- **Audio** (`audio/`): one in-game song with Markov branching, a menu song, and victory and
  defeat jingles. There's a single master volume.

## Protocol v2

`protocol.Version` goes from 1 to 2. v1 and v2 clients and hosts refuse each other with the
existing "update NEON//COUP" message.

| Change | Where | Why |
|---|---|---|
| `Settings.Speed` (`relaxed` (default), `normal`, `fast`, `instant`) | `protocol.go` | room speed, picked by the host; solo uses the player's setting |
| `Settings.WindowMS = 0` means "auto by speed" (12 s / 8 s / 5 s) | `protocol.go` | 12 s is the default reaction window at Relaxed |
| `State.OpensInMS`, `State.Speed`, `State.Paused` | `protocol.go` | clients and headless bots know when the decision opens, the current speed, and whether solo play is paused |
| new client→server `control` message `{pause?, skip_ms?, speed?}` | `protocol.go`, `room.go` | Space-to-skip, `P` to pause, and speed changes; only honored for the only human at the table |
| `protocol/pace.go`: `BeatMS(event, speed)`, `PresentMS(events, speed)` | new | **one** beat-length table shared by the server and the client, so they agree exactly |

`instant` gives every beat a length of 0. Tests, the 10k simulation and the network integration
test use it, so they run with presentation off. It isn't offered in the UI.

## 1. Pacing

**Beat table** (Relaxed; Normal is 0.6×, Fast 0.3×) in `protocol/pace.go`:

| event | beat |
|---|---|
| action declared (announcement) | 1.5 s |
| challenge, block (reveal) | 2.0 s |
| claim proven / bluff caught (result) | 1.0 s |
| card redraw after a proven claim | 2.5 s |
| influence lost | 2.0 s |
| eliminated | 3.0 s |
| coins moving | 1.0 s |
| exchange draw / return | 1.5 s each |
| action blocked / voided | 1.2 s |
| turn start | 0.6 s |
| pass, window open, turn end, game start/over | 0 |

**Server (authoritative, no client acks):** every room tracks `presentEnd`, the time by which
every client will have finished playing what has been sent. Each batch of events moves it forward
by `PresentMS(batch)`. When a new decision comes up, it opens at `opensAt = presentEnd`, then:

- the reaction or turn timer runs from `opensAt` (deadline = `opensAt + window`);
- bots think from `opensAt` (think delays are scaled by the speed factor);
- intents that arrive before `opensAt` (from headless or modified clients) are **deferred**, then
  applied in arrival order once the decision opens. Nobody can act before everyone has seen the
  claim, and no client can hold the game up: the schedule depends only on the server clock.

**Solo controls**, honored only when the sender is the only human seated:

- `Space` skips the current beat, and the client sends `skip_ms` so the server shifts its
  presentation clock by the same amount.
- `P` (Shift+P, since `p` is Pass) pauses: the server stops the timer and freezes bots. On resume
  it shifts the deadline and the presentation clock by the paused time.

**Client presentation queue** (`client/present.go`): event, state, game-over and bot-chat frames
are queued in arrival order. Each event plays as a beat: announce (log line, SFX, banner) →
animate (sprites, counters) → settle (a small view patch: coins, face-up cards, deck, alive). A
state is applied only once the beats in front of it have finished, so the UI never jumps ahead.
Timer deadlines use the frame's arrival time, so the countdown stays honest. If more than one
state is waiting, beats play at 4× until the client has caught up.

**Tests:**
- a room-level test showing that a reaction window's `opensAt` is at least the announcement beat
  after the declaration, and that the deadline is `opensAt + window`;
- a wire test showing that no bot reacts, and no deferred human challenge is applied, before the
  announcement beat ends;
- a pause/skip retiming test;
- client queue tests: a state is withheld until its beats finish, Space skips a beat, catch-up
  works.

## 2. Focus

- **Action banner**: a large boxed banner over the middle of the table, shown during the
  announcement beat, e.g. `VEX ▸ CLAIMS DUKE ▸ TAX +3¢`. A targeted action names the target in
  the banner, gives the actor a bright border and the target a red `◀ TARGET` border.
- **Context bar** (the first of the two bottom lines): one sentence covering what's happening,
  what *you* can do, and what each choice risks, plus a draining bar and the seconds left. For
  example: `VEX claims Duke to take 3 coins. [C] Challenge (if VEX has a Duke, you lose an
  influence) [P] Pass ███████░░ 9s`. The second line keeps the hotkeys.
- **Your turn**: the center and hand borders pulse for 2 s and a new `SFXYourTurn` plays. The
  hotkey line lists every action; unaffordable or illegal ones are dimmed with the reason, e.g.
  `Coup needs 7¢`, `must Coup at 10¢`, `no target with 3+¢`.
- **Highlighting**: during a beat, opponent panels that aren't involved are dimmed. The active
  player always has a `▶` marker and a magenta border, and the top bar names whose turn it is.

## 3. Center: card tracker

`client/tracker.go`:
- **Counts** (pure function, unit-tested with and without the Magistrate): `REVEALED x/3` is the
  number of copies face-up anywhere. `UNSEEN y` is `3 − revealed − copies in your hand`. When
  `UNSEEN` is 0, the card gets a red `▓ NO COPIES LEFT ▓` stamp.
- **Card row**: the existing 12×7 compact card art (the "about 10×7" scale-down) in a row, with
  the counts under each card. Beside the row are the deck, with a small stacked-card graphic, and
  the treasury.
- **Narrow fallback**: when the row doesn't fit (100×30, and 6 cards with side panels), one-line
  tiles (`DUKE R1/3 U2`), packed several per line.
- **Placement**: the tracker takes over the center terminal, below a short headline of what's
  happening.

## 4. Animations

`client/sprites.go`: while drawing, the table records an anchor point for each seat, the deck and
the treasury. Beats draw sprites over the frame with the compositor:
- **coins**: `¢` glyphs travel between the treasury and a player, or between players on Steal,
  while the counters tick;
- **card redraw**: the revealed card pops up face-up, slides into the deck, the deck shuffles,
  and a card **back** slides to the owner. Only the owner then sees their own hand slot flip
  face-up;
- **exchange**: two backs slide from the deck to the player, the picker shows every option side
  by side, then the returned backs slide back and the deck shuffles;
- **influence loss**: the card flips face-up at the player's panel, glitches, then settles into
  their revealed row;
- **elimination**: the panel glitches out, then greys to `[ DISCONNECTED ]`.

Opponents only ever see card backs moving. A face is drawn only when the event made it public
(a proven or lost card), or in your own hand. Sprites follow the speed setting (they're drawn
from beat progress), skip with Space, and become static frames under Reduced Motion. Frame rate
stays capped at 20 fps, and nothing ticks when idle.

## 5. Music

`audio/songs2.go` adds three original pattern-data tracks, each with a fixed `Order` lasting
1.5–3 minutes:
- **Rain on Chrome**: 90 BPM, C minor, arpeggios, sparse hats;
- **Overclock**: 150 BPM, driving triangle bass, punchy lead;
- **Neon Remix**: the original theme moved to A minor at 120 BPM, with Rain and Overclock motifs
  and a new section built on the original hook.

The original theme gets a capped length (a pattern budget), so it ends too.

**Playlist** (in the synth, on the audio goroutine):
- shuffled rotation of the 4 in-game tracks, never the same track twice in a row;
- 3 s crossfade between two music chips;
- `NextTrack()` for `N`.

**Intensity**: `SetIntensity(tense)` is sent by the client when any player is at 1 influence or
7+ coins, or only 2 players remain. While tense, Overclock is preferred as the next track, and
soft hats fill the empty noise steps of the current one. The existing stinger and ducking stay.

**Notices**: the Player gains `Notices()`, a channel of "now playing" titles. The client shows a
corner toast, `♪ NOW PLAYING // RAIN ON CHROME`.

**Volumes**: master, music and SFX volumes are separate and saved to the config.

## 6. Other enhancements

- **Claim history badges** on opponent panels (`DUKE×2 CAPT×1`), from declared actions and
  blocks.
- **Inspect mode** (`Tab`): an overlay cycling through the players, with each one's claims,
  actions and revealed cards.
- **Event log**: an icon and color per event type, PgUp/PgDn scrollback (kept), and a one-line
  "last round" summary when your turn starts.
- **Post-game screen**: the client re-runs the replay (the engine is deterministic) to reveal
  every hand. It shows a timeline of key moments, and per-player successful and caught bluffs.
- **First-game hints**: a tooltip the first time each situation comes up (challenge window, block
  window, being targeted, losing influence, exchange, 7+ coins, forced coup). Seen hints are
  saved; hints can be turned off in settings.
- **Accessibility**: `palette` = neon (default), colorblind-safe (Okabe–Ito based) or
  high-contrast; Reduced Motion is extended to cover sprites and pulses. The palette is
  process-wide, so SSH sessions keep the default one (noted in TODO.md).
- **Settings overlay** (`Esc` in game): speed (solo; read-only online), music, SFX and master
  volume, mute, hints, palette, Reduced Motion, CRT. Opening it pauses solo play. The main-menu
  Settings screen gets the same fields. Everything is saved to `config.json`.

**Config migration**: configs written before this update keep their saved `window_ms` (8000).
The new `config_version` field marks pre-update files, and on load their window resets to auto,
so Relaxed's 12 s default applies.

## Tests (done-when)

- The Section 1 timing tests above, for both the server and the client queue.
- Tracker count tests, including the Magistrate variant and every-copy-in-hand cases.
- Playlist tests: 4 tracks, no back-to-back repeats over many picks, a crossfade with a smooth
  gain ramp and no clicks, every track between 1.5 and 3 minutes, and the intensity preference.
- Golden snapshot tests of the table at 100×30 and 160×45 (`go test ./client -run Snapshot
  -update` rewrites them).
- `go test ./...`, the 10k-game simulation and the network integration test, all with
  presentation off.

## Commits

The repository's commit hook blocks commits made by the assistant. Each section ends with a
suggested commit message for you to commit yourself.
