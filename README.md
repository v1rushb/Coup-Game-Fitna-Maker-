# NEON//COUP

**A bluffing game for the terminal.** *Coup*'s rules in an 8-bit cyberpunk skin: ANSI art, a
chiptune soundtrack synthesized in code, LAN and internet multiplayer, SSH play with nothing to
install, and bots that read your bluffs.

```text
█▄  █ █▀▀▀ ▄▀▀▄ █▄  █    ▄▀    ▄▀ ▄▀▀▀ ▄▀▀▄ █  █ █▀▀▄
█▀▄ █ █▄▄  █  █ █▀▄ █   ▄▀    ▄▀  █    █  █ █  █ █▄▄▀
█ ▀▄█ █    █  █ █ ▀▄█  ▄▀    ▄▀   █    █  █ █  █ █
▀   ▀ ▀▀▀▀  ▀▀  ▀   ▀ ▀     ▀      ▀▀▀  ▀▀   ▀▀  ▀
```

- **Exact rules**, Magistrate variant included, in a deterministic, fully tested engine
  (10,000-game bot simulation, fuzzing, every edge case).
- **One command and a room code** to join: LAN discovery first, then a relay for players behind NAT.
- **Looks and sounds the part**: original card art, neon palette with 256/16-color and `NO_COLOR`
  fallbacks, `--ascii`, CRT scanlines, card flips, glitches, and an NES-style synth.
- **Bots at three levels**, from honest Script Kiddies to Black ICE, which tracks what every
  opponent probably holds.
- **Easy to follow**: every move plays out as a beat you can read (Relaxed, Normal or Fast), a
  bottom bar spells out your choices and what they risk, and a card tracker in the middle counts
  what's still unseen.

---

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [How to play](#how-to-play)
- [Reading the table](#reading-the-table)
- [Multiplayer](#multiplayer)
- [Running a relay](#running-a-relay)
- [Bots](#bots)
- [Audio](#audio)
- [Configuration](#configuration)
- [Screens](#screens)
- [Development](#development)

## Install

You need a terminal of at least **100×30**. Truecolor looks best, but 256/16 colors and
`NO_COLOR` all work.

**From source** (Go 1.27+):

```sh
go install github.com/v1rushb/neoncoup/cmd/neoncoup@latest
# or, from a checkout:
make build            # ./neoncoup, a static binary
make dist             # dist/neoncoup-{linux,darwin,windows}-{amd64,arm64}
```

Every build is `CGO_ENABLED=0`. The Linux binary is fully static, with no glibc, ALSA or Pulse
libraries needed. macOS and Windows binaries need nothing beyond the OS.

**Docker** (handy for the relay):

```sh
docker build -t neoncoup .
docker run --rm -it neoncoup solo        # play in a container
docker run -d -p 8080:8080 neoncoup      # default command: the relay
```

## Quick start

```sh
neoncoup                 # boot sequence, then the main menu
neoncoup solo            # straight into a game against 3 bots
neoncoup tutorial        # learn the con in a scripted game
neoncoup host            # open a room; prints the room code and join command
neoncoup join NEON-7K2Q  # join it (LAN first, then relay)
```

## Commands

Global flags, accepted by every command:

| flag | meaning |
|------|---------|
| `--ascii` | plain ASCII instead of Unicode box drawing and block art |
| `--no-audio` | no sound at all |
| `--name NAME` | your nickname (saved in the config otherwise) |
| `--config DIR` | config directory (default: OS config dir; env `NEONCOUP_HOME`) |
| `--skip-boot` | skip the BIOS boot sequence |
| `--crt` | CRT scanlines (toggle in game with `s`) |

### `neoncoup` (main menu)
Solo run, host, join, tutorial, replays, stats and settings.

### `neoncoup solo [--bots N] [--difficulty easy|normal|hard] [--speed relaxed|normal|fast] [--magistrate] [--window SECONDS]`
A private game against 1–5 bots. Defaults come from your config (3 Fixers at Relaxed speed, with
the reaction window set by the speed: 12 s at Relaxed, 8 s at Normal, 5 s at Fast).

### `neoncoup tutorial`
A rigged three-player game in which a coach walks you through Tax, Steal, calling a bluff, blocking,
a Coup and challenging an Assassin, then hands you free play.

### `neoncoup host [flags]`
Opens a room and prints the room code with the exact join commands, then drops you into its lobby.

| flag | default | meaning |
|------|---------|---------|
| `--port N` | 7777 | TCP port for LAN/direct joins (`0` picks one) |
| `--relay URL` | config / `$NEONCOUP_RELAY` | register the room with a relay so players behind NAT can join |
| `--seats N` | 4 | seats at the table, 2–6 |
| `--no-bots` | off | leave empty seats empty instead of filling them with bots |
| `--difficulty L` | normal | bot difficulty |
| `--speed S` | relaxed | presentation speed for everyone at the table: `relaxed`, `normal`, `fast` |
| `--window S` | by speed | challenge/block window in seconds (12 at Relaxed, 8 at Normal, 5 at Fast) |
| `--turn S` | 60 | turn timer; `0` turns it off |
| `--hold S` | 60 | how long a dropped player's seat is held before a bot takes over |
| `--magistrate` | off | Magistrate variant (recommended for 5+ players) |
| `--headless` | off | dedicated room: no local player and no UI. The first person to join hosts the table. |

```text
$ neoncoup host --relay wss://relay.example.com

  NEON//COUP room open: NEON-7K2Q

  same network: neoncoup join NEON-7K2Q
  direct:       neoncoup join 192.168.1.20:7777
  internet:     neoncoup join NEON-7K2Q --relay wss://relay.example.com
```

### `neoncoup join CODE|HOST:PORT [--relay URL] [--spectate]`
- `neoncoup join NEON-7K2Q`: asks the LAN (UDP multicast + broadcast, about 1.5 s), then the
  relay if one is configured.
- `neoncoup join 192.168.1.20:7777` (or `ws://…`/`wss://…`): connects directly.
- `--spectate`: watch without a seat; you never see a hidden card.

### `neoncoup relay [--addr :8080] [--max-rooms 1000] [--max-players 16]`
A small matchmaking/forwarding service for a VPS. Hosts register their room code and players
connect through it. The relay forwards frames without reading them; all game logic stays on the
host. See [Running a relay](#running-a-relay).

### `neoncoup serve-ssh [--addr :2222] [--host-key PATH] [--http :7777]`
Players run `ssh -p 2222 your.host` and get the full game with nothing to install, and no audio.
Your SSH username becomes your nickname. SSH players can host rooms and join each other by code.
`--http` also exposes those rooms over WebSocket (with LAN discovery) so app users can join them.
Set `NEONCOUP_ASCII=1` in the SSH environment (`ssh -o SetEnv=NEONCOUP_ASCII=1 …`) for ASCII mode.

### `neoncoup replay [FILE]`
Without a file it lists saved replays. With one it opens the step-through viewer with every hand
face up (←/→ step, ↑/↓ ±10, space play/pause, +/- speed).

### `neoncoup stats` · `neoncoup version`
Your lifetime record, and the build and protocol version.

## How to play

Everyone holds two face-down cards (their **influence**) and coins. On your turn, take one
action. Character actions may be **bluffs**: you can claim any character, held or not. Anyone
may **challenge** a claim. If the claimant can show the card, the challenger loses an influence
(and the claimant swaps the shown card for a fresh one). If not, the claimant loses one and the
action fails. Some actions can be **blocked** by claiming a character, and blocks can be
challenged too. Lose both cards and you're out. The last runner standing wins.

| key | action | effect | claim | blocked by |
|-----|--------|--------|-------|------------|
| `1` | Income | +1¢ | — | — |
| `2` | Foreign Aid | +2¢ | — | Duke (anyone) |
| `3` | Coup | pay 7¢, target loses an influence. Unblockable. **Forced at 10¢.** | — | — |
| `4` | Tax | +3¢ | Duke | Magistrate (variant) |
| `5` | Assassinate | pay 3¢, target loses an influence | Assassin | Contessa (target) |
| `6` | Steal | take 2¢ (1 if they only have 1) | Captain | Captain or Ambassador (target) |
| `7` | Exchange | draw 2, keep any mix of hand + drawn | Ambassador | — |
| `8` | Audit (variant) | a target with 3+¢ pays 2¢ to the treasury | Magistrate | — |

When a window opens: `c` challenge, `b` block (then `1`/`2` to pick the card if there's a choice),
`p` pass. Windows open once the claim has been announced, run on a timer (12 s at Relaxed by
default) and close early once everyone has passed. When challenged, or when you must lose an
influence, press `1` or `2` to pick the card to flip. Flipping the claimed card proves the claim;
flipping any other card concedes, as the rulebook allows.

| key | does |
|-----|------|
| `tab` | inspect a player: claims, actions and face-up cards this game (tab again for the next) |
| `space` | skip the current beat (solo) |
| `P` | pause: bots and timers freeze (solo; Shift+P, since `p` passes) |
| `esc` | cancel a choice, or open settings (pauses solo play) |
| `n` / `m` | next music track / mute |
| `pgup` `pgdn` (or `[` `]`) | scroll the netlog |
| `enter` / `t` | chat / taunts |
| `?` / `s` / `q` | help / CRT scanlines / leave |

Key reminders on the table: actions you can back up show in **green**, bluffs in **magenta with
a `~`**. Actions you can't take are dimmed, and the bottom bar says why ("Coup needs 7¢").

Rules notes (Assassin refunds, double losses, eliminated players' coins, the Magistrate) are
written up in [PLAN.md](PLAN.md#rules-decisions-and-rulebook-cross-check), including where this
implementation had to interpret the rulebook.

## Reading the table

**Pacing.** Every event plays out as a *beat*: it's announced, animated, then settles, and the
table never jumps ahead while a beat is playing. At Relaxed (the default) an action announcement
takes 1.5 s, a challenge or block 2 s, a lost influence 2 s, a card redraw 2.5 s and an
elimination 3 s; Normal is about 0.6× that and Fast 0.3×. Bots think on the same clock. The host
holds each reaction window until the announcement in front of it has played, using its own clock
rather than waiting on clients, so nobody loses reaction time to an animation and a slow client
can't stall the table. In solo, `space` skips a beat and `P` pauses.

**Focus.**
- *Banner*: a large banner over the table announces each move, e.g. `VEX ▸ CLAIMS DUKE ▸ TAX
  +3¢`. A target's panel turns red with `◀ TARGET`, and panels outside the beat dim.
- *Context bar*: the line above the hotkeys says what's happening, what you can do and what each
  choice risks, with a draining countdown: `VEX claims Duke to take 3 coins. [C] Challenge (if VEX
  has a Duke, you lose an influence) [P] Pass ███████░░ 9s`.
- *Your turn*: the border pulses and a sound plays.
- *Active player*: the mover's panel always wears a `▶` and a heavy border.

**Card tracker.** The middle of the table shows every character as a mini card:
- `REVEALED x/3` counts copies face up anywhere.
- `UNSEEN y` counts copies that aren't face up and aren't in your hand, so they could be in a
  rival's hand or the deck.
- At `UNSEEN 0` the card is stamped `▓ NO COPIES LEFT ▓` in red: any claim of it is a guaranteed
  bluff, and the context bar says so.
- The deck and treasury sit beside the row.
- On narrower terminals the cards become tiles (`DUKE R1/3 U2`).

**Animations.**
- Coins fly between the treasury and players (or between players on a steal) while the counters
  tick.
- A proven card goes back into the deck, the deck shuffles, and a fresh card comes back face down;
  only its owner sees it flip.
- Exchanged cards fly in and out face down.
- A lost card flips and glitches.
- An eliminated runner's panel tears and greys to `[ DISCONNECTED ]`.
- Opponents only ever see card backs move. *Reduce motion* in settings swaps all of this for still
  frames.

**History.**
- *Claim badges*: each rival's panel shows what they've claimed this game (`DUKE×2 CAPT×1`).
- *Netlog*: color-coded with an icon per event type, and adds a one-line `LAST ROUND // …` summary
  when your turn starts.
- *Inspect*: `tab` opens a player's full history.
- *Post-game screen*: replays the game to reveal every hand, each player's bluffs that got away
  and were caught, and a timeline of key moments.

**Hints.** The first time each situation comes up (your first challenge window, block, exchange
and so on) a short tip explains it. Hints can be turned off in settings, and turning them back on
shows them all again.

## Multiplayer

- **LAN**: `neoncoup host` answers discovery queries for its room code, so `neoncoup join
  NEON-XXXX` finds it on the same network. Several hosts can run on one machine.
- **Direct**: `neoncoup join HOST:PORT` for any reachable host (forward the port to play over
  the internet without a relay).
- **Relay**: for players behind NAT. The host registers the code with a relay (`--relay`) and
  players reach it through the relay's `/join/CODE`.
- **SSH**: `neoncoup serve-ssh` turns a server into a lobby anyone with `ssh` can use.

**Lobby.** Nicknames (sanitized, deduplicated), ready-up, spectators, and host settings (`v`
cycles the speed everyone plays at): seats,
bot fill and difficulty, window and turn timers, Magistrate, seat hold.

**Reconnects.** Every player gets a rejoin token, which is also saved to disk. If your link
drops, the client reconnects on its own. If it crashes, run the same `join` command again. Your
seat is held for 60 s (configurable), then a Fixer bot plays it until you come back, at which
point you take it over again.

**Security.** The host is authoritative: clients send intents, and the host validates every one
against the engine and sends each player a view redacted for their seat. Another player's hidden
card never crosses the wire. Mismatched protocol versions are rejected with a clear message.
This release speaks protocol v2 (pacing and solo controls), so every player and every host,
including a dedicated server such as `host --headless`, needs to run the same release.
Input is rate-limited, and nicknames and chat are stripped of ANSI/OSC escape sequences, control
characters and bidi overrides.

## Running a relay

The relay is stateless and tiny (a 13 MB distroless image).

**Docker**

```sh
docker build -t neoncoup .
docker run -d --name neoncoup-relay --restart unless-stopped -p 8080:8080 neoncoup
```

**systemd** (sandboxed; see [`deploy/neoncoup-relay.service`](deploy/neoncoup-relay.service))

```sh
sudo install -m 0755 neoncoup /usr/local/bin/neoncoup
sudo cp deploy/neoncoup-relay.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now neoncoup-relay
```

**TLS.** Put it behind a reverse proxy so players can use `wss://`. With Caddy
([`deploy/Caddyfile`](deploy/Caddyfile)):

```text
relay.example.com {
	reverse_proxy 127.0.0.1:8080
}
```

The relay keeps a dropped host's room code reserved for that host for two minutes, and caps rooms
per IP, players per room, and per-player traffic.

Then: `neoncoup host --relay wss://relay.example.com`, and players run `neoncoup join NEON-XXXX
--relay wss://relay.example.com` (or set the relay once in Settings). `GET /` shows status and
`GET /healthz` returns `ok`. There is also a unit for the SSH server:
[`deploy/neoncoup-ssh.service`](deploy/neoncoup-ssh.service).

## Bots

| level | name | style |
|-------|------|-------|
| easy | **Script Kiddie** | mostly honest, random legal moves, rarely challenges |
| normal | **Fixer** | claims what it holds, bluffs now and then, challenges when card counting says a claim is impossible or unlikely |
| hard | **Black ICE** | keeps a probability distribution over every opponent's hidden cards (face-up cards, claims, blocks made and *not* made, proofs), weighs expected value, bluffs where you can't easily disprove it, targets the leader, avoids becoming the obvious Coup target, plays the endgame properly |

Bots have cyberpunk handles (VEX, NULLBYTE, GHOSTWIRE, DAEMON JANE…), personalities, the
occasional taunt, and a human-like 0.8–2.5 s think time shown with a spinner. They use exactly
the same redacted view as human clients (the bot API never receives the game state), so they can
fill online seats. In 3-way games over fixed seeds, Black ICE wins about 50%, Fixer 33% and
Script Kiddie 17%.

## Audio

All music and effects are synthesized live by an NES-style chip: two pulse channels (12.5/25/50%
duty, band-limited), a 32-step triangle, a 15-bit LFSR noise channel and the console's nonlinear
mixer. Everything it plays is an original composition:

- a moody menu theme;
- an in-game playlist of four tracks, each 2–2½ minutes long with its own sections:
  - **Neon Protocol** (the original theme): hook, climb, breakdown and countermelody sections that
    branch randomly.
  - **Rain on Chrome** (90 BPM, C minor): moody arpeggios and sparse hats, the calm opener.
  - **Overclock** (150 BPM, D minor): galloping triangle bass, a punchy lead and busy drums.
  - **Neon Protocol (Chrome Rain Remix)** (120 BPM, A minor): the theme laced with Rain's
    arpeggios and Overclock's riff, plus a new half-time section built on the hook.
- a tension stinger when a challenge window opens on you, which ducks the music underneath it;
- victory and defeat jingles;
- effects for coins, card flips, challenges, blocks, your turn and the elimination glitch.

The playlist plays in shuffled order and never plays the same track twice in a row. Tracks
crossfade over 3 s, and a corner toast shows `♪ NOW PLAYING // …`. When the game gets tense
(anyone down to one influence or holding 7+ coins, or only two runners left) the playlist favors
Overclock next and the current track gains an extra layer of hats. `n` skips a track.

Audio runs on its own goroutine and the UI never blocks on it. Master, music and SFX volumes and
mute (`m`) persist.
With `--no-audio`, over SSH, or when no audio device is found, the game runs silently and shows
a muted icon. On Linux it talks to PulseAudio/PipeWire directly, with `aplay` as a fallback. On
macOS and Windows it uses oto. `make wav` renders every song and effect to WAV files.

## Configuration

Settings live in the OS config directory (`~/.config/neoncoup` on Linux, `~/Library/Application
Support/neoncoup` on macOS, `%AppData%\neoncoup` on Windows), or `$NEONCOUP_HOME`:

| file | contents |
|------|----------|
| `config.json` | name, master/music/SFX volume, mute, speed, hints, palette, Reduce motion, CRT, ASCII, relay URL, timers, bot defaults, port, rejoin tokens |
| `stats.json` | games, wins, streaks, bluffs (and how many were caught), challenges (and how many were right), blocks, coins, coups |
| `replays/*.json` | one per finished game: seed, config, seats and every intent. The engine is deterministic, so that reproduces the game exactly. |
| `neoncoup.log` | host/server logs from TUI sessions (kept off the screen) |

`$NEONCOUP_RELAY` sets a default relay. `NO_COLOR` disables color.

**Accessibility.** Settings (`esc` in game, or Settings in the menu) has three palettes: the neon
default, a colorblind-safe one (Okabe–Ito-based: vermillion and bluish green instead of red and
green) and high contrast (black, white and full color). *Reduce motion* turns off sliding cards,
glitches, pulses and screen jolts. Over SSH the palette stays on neon, because one server process
hosts every session.

## Screens

Text captures of every screen, generated from the real renderer by `make screenshots` (colors
stripped). Click to expand.

<!-- screens:begin -->
<details>
<summary><b>Boot: fake BIOS POST with a glitching logo</b></summary>

```text

   NEONTEK BIOS v4.20  (C) 2087 NEONTEK SYSTEMS
   CPU: QUANTUM-8 @ 6.66 THz ................... OK
   MEMORY TEST: 655360K ........................ OK
   VECTORCORE GPU .............................. OK
   NEURAL LINK ................................. CALIBRATED
   MOUNTING /dev/court (15 cards) .............. OK
   SEEDING ENTROPY POOL ........................ OK
   CHIPTUNE DRIVER (2xPULSE TRI NOISE) ......... NO DEVICE (muted)
   FIREWALL .................................... DISABLED (it's that kind of night)
   JACKING IN ..........


   █▄  █ █▀▀▀ ▄▀▀▄ █▄  █    ▄▀    ▄▀ ▄▀▀▀ ▄▀▀▄ █  █ █▀▀▄
   █▀▄ █ █▄▄  █  █ █▀▄ █   ▄▀    ▄▀  █    █  █ █  █ █▄▄▀
   █ ▀▄█ █    █  █ █ ▀▄█  ▄▀    ▄▀   █    █  █ █  █ █
   ▀   ▀ ▀▀▀▀  ▀▀  ▀   ▀ ▀     ▀      ▀▀▀  ▀▀   ▀▀  ▀
```

</details>

<details>
<summary><b>Main menu</b></summary>

```text





                       █▄  █ █▀▀▀ ▄▀▀▄ █▄  █    ▄▀    ▄▀ ▄▀▀▀ ▄▀▀▄ █  █ █▀▀▄
                       █▀▄ █ █▄▄  █  █ █▀▄ █   ▄▀    ▄▀  █    █  █ █  █ █▄▄▀
                       █ ▀▄█ █    █  █ █ ▀▄█  ▄▀    ▄▀   █    █  █ █  █ █
                       ▀   ▀ ▀▀▀▀  ▀▀  ▀   ▀ ▀     ▀      ▀▀▀  ▀▀   ▀▀  ▀

                             a bluffing game for the terminal // v1.0.0


                  ▶ 1  SOLO RUN     you vs bots, right now
                    2  HOST ROOM    open a room for LAN or internet friends
                    3  JOIN ROOM    room code (NEON-XXXX) or host:port
                    4  TUTORIAL     a scripted run that teaches the con
                    5  REPLAYS      step through saved games
                    6  STATS        your record: wins, bluffs, challenges
                    7  SETTINGS     name, audio, CRT, ASCII
                    8  QUIT         jack out


                runner: RUNNER   ↑↓ select · enter go · ? help · m mute ♪✖ no audio
```

</details>

<details>
<summary><b>Solo setup</b></summary>

```text







              ╭──────────────────────────────────────────────────────────────────────╮
              │                                                                      │
              │   SOLO RUN                                                           │
              │   a private grid, you against the machine                            │
              │                                                                      │
              │   ▶ Bots            ◀ 3 ▶                                            │
              │     Difficulty      ◀ FIXER (normal) ▶                               │
              │     Magistrate      ◀ OFF ▶                                          │
              │     Speed           ◀ RELAXED ▶                                      │
              │     Reaction window ◀ AUTO (12s) ▶                                   │
              │                                                                      │
              │   1 to 5 opponents. Magistrate is recommended for 5+ players.        │
              │                                                                      │
              │   ↑↓ field · ←→ change · enter to jack in · esc back                 │
              │                                                                      │
              ╰──────────────────────────────────────────────────────────────────────╯
```

</details>

<details>
<summary><b>Host setup</b></summary>

```text




              ╭──────────────────────────────────────────────────────────────────────╮
              │                                                                      │
              │   HOST ROOM                                                          │
              │   you run the table; friends join with a room code                   │
              │                                                                      │
              │   ▶ Seats           ◀ 4 ▶                                            │
              │     Fill with bots  ◀ ON ▶                                           │
              │     Bot difficulty  ◀ FIXER (normal) ▶                               │
              │     Speed           ◀ RELAXED ▶                                      │
              │     Reaction window ◀ AUTO (12s) ▶                                   │
              │     Turn timer      ◀ 60s ▶                                          │
              │     Magistrate      ◀ OFF ▶                                          │
              │     Seat hold       ◀ 60s ▶                                          │
              │     Port            7777                                             │
              │     Relay           none (LAN + direct only)                         │
              │                                                                      │
              │   Total seats at the table, humans and bots.                         │
              │                                                                      │
              │   ↑↓ field · ←→ change · enter to open the room · esc back           │
              │                                                                      │
              ╰──────────────────────────────────────────────────────────────────────╯
```

</details>

<details>
<summary><b>Join</b></summary>

```text








              ╭──────────────────────────────────────────────────────────────────────╮
              │                                                                      │
              │   JOIN ROOM                                                          │
              │   room codes are found on your LAN first, then through the relay     │
              │                                                                      │
              │   > NEON-7K2Q                                                        │
              │                                                                      │
              │   joining as a player  (tab to switch)                               │
              │   relay: none configured (settings)                                  │
              │                                                                      │
              │   enter connect · esc back                                           │
              │                                                                      │
              ╰──────────────────────────────────────────────────────────────────────╯
```

</details>

<details>
<summary><b>Lobby (host view)</b></summary>

```text
      LOBBY     NEON-7K2Q
      same network: neoncoup join NEON-7K2Q
      direct:       neoncoup join 192.168.1.20:7777
      internet:     neoncoup join NEON-7K2Q --relay wss://relay.example.com

      ╭────────────────────────────────────────────╮  ╭──────────────────────────────────────╮
      │ RUNNERS                                    │  │ TABLE RULES                          │
      │                                            │  │                                      │
      │ ● RUNNER             READY [HOST] (you)    │  │ seats        4                       │
      │ ● TRINITY            READY                 │  │ bot fill     ON (normal)             │
      │ ● CASE               NOT READY             │  │ speed        RELAXED                 │
      │ ○ bot #1             FIXER (NORMAL)        │  │ window       AUTO (12s)              │
      │                                            │  │ turn timer   60s                     │
      │ 1 spectator(s) watching                    │  │ magistrate   OFF                     │
      ╰────────────────────────────────────────────╯  │ seat hold    60s                     │
                                                      ╰──────────────────────────────────────╯

      ╭──────────────────────────────────────────────────────────────────────────────────────╮
      │ // TRINITY jacked in                                                                 │
      │ TRINITY: ready when you are                                                          │
      │ // MOLLY is watching                                                                 │
      │ tab to chat                                                                          │
      │                                                                                      │
      │                                                                                      │
      │                                                                                      │
      │                                                                                      │
      │                                                                                      │
      ╰──────────────────────────────────────────────────────────────────────────────────────╯

      enter START · +/- seats · f bot fill · d difficulty · v speed
```

</details>

<details open>
<summary><b>Game table, 120x40: an assassination aimed at you, challenge window open</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 9  ▶ GHOSTWIRE                                  ♪✖ no audio  ? help  q leave
         ╔════════════════════════════════╗╭────────────────────────────────╮          ╭───────────────────────────────╮
         ║ ▶ GHOSTWIRE               HARD ║│ ⠙ NULLBYTE                EASY │          │ NETLOG                        │
         ║ ¢1                          ●● ║│ ¢2 ✖ASSN                    ●○ │          │ ✖ NULLBYTE'S TAX IS VOIDED    │
         ║ ASSN×1 CAPT×1                  ║│ DUKE×1                         │          │ ── TURN 6 · VEX ──            │
         ║ claims ASSASSIN                ║│ thinking...                    │          │ ▸ [VEX] SKIMS A CREDIT //     │
         ╚════════════════════════════════╝╰────────────────────────────────╯          │   INCOME +1¢                  │
                  ╔═════════════════════════════════════════════════╗                  │ ── TURN 7 · YOU ──            │
                  ║ TREASURY 35¢ · DECK 5                           ║                  │ LAST ROUND // YOU tax ·       │
                  ║ > GHOSTWIRE → ASSASSINATE YOU (claims ASSASSIN) ║                  │   MOTH foreign aid (blocked   │
                  ║ CHALLENGE WINDOW OPEN                           ║                  │   by YOU) · GHOSTWIRE         │
                  ║ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░░░░  5.2s   ║                  │   steal→VEX (blocked by VEX)  │
╭────────────────╮║                                                 ║╭────────────────╮│   (bluff caught) +2¢ ·        │
│ ⠙ MOTH         │║ COURT TRACKER // revealed: face up · unseen: a… ║│ ⠙ VEX          ││   NULLBYTE tax (bluff caught) │
│ ¢3          ●● │║  DUKE           ASSASSIN       CAPTAIN          ║│ ¢2 ✖CONT    ●○ ││   · VEX income                │
│ DUKE×1         │║ REVEALED 0/3   REVEALED 1/3   REVEALED 0/3      ║│ AMBS×1         ││ ▸ [YOU] FLASHES A CAPTAIN     │
│ thinking...    │║ UNSEEN 2       UNSEEN 2       UNSEEN 2          ║│ thinking...    ││   BADGE AT [MOTH] // STEAL 2¢ │
│                │║                                                 ║│                ││ ¢ [YOU] LIFTS 2¢ FROM         │
│                │║  AMBASSADOR     CONTESSA                        ║│                ││   [MOTH]                      │
╰────────────────╯║ REVEALED 0/3   REVEALED 1/3                     ║╰────────────────╯│ ── TURN 8 · MOTH ──           │
                  ║ UNSEEN 3       UNSEEN 2                         ║                  │ ▸ [MOTH] BROADCASTS DUKE      │
                  ║                                                 ║                  │   SIGNAL // TAX +3¢           │
                  ║                                                 ║                  │ ¢ +3¢ → [MOTH]                │
                  ║                                                 ║                  │ ── TURN 9 · GHOSTWIRE ──      │
                  ║                                                 ║                  │ ▸ [GHOSTWIRE] RUNS ASSASSIN   │
                  ╚═════════════════════════════════════════════════╝                  │   PROTOCOL ON [YOU] // -3¢    │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                   │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 7                                              │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║  coup armed (7)                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║                                                   │ [VEX] your firewall is made … │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  you hold                                         │ // TRINITY lost the uplink. … │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║  DUKE + CAPTAIN                                   │ [CASE] nobody believes that … │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │ enter: chat · t: taunt        │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 GHOSTWIRE claims Assassin to assassinate you.  [C] Challenge (wrong: you lose 1)  [P] Pass  ██████░░░ 6s
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Game table at the minimum size, 100x30</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 9  ▶ GHOSTWIRE              ♪✖ no audio  ? help  q leave
 ╔════════════════════════════════╗╭────────────────────────────────╮ ╭────────────────────────────╮
 ║ ▶ GHOSTWIRE               HARD ║│ ⠙ NULLBYTE                EASY │ │ NETLOG                     │
 ║ ¢1                          ●● ║│ ¢2 ✖ASSN                    ●○ │ │ ▸ [YOU] FLASHES A          │
 ║ ASSN×1 CAPT×1                  ║│ DUKE×1                         │ │   CAPTAIN BADGE AT [MOTH]  │
 ║ claims ASSASSIN                ║│ thinking...                    │ │   // STEAL 2¢              │
 ╚════════════════════════════════╝╰────────────────────────────────╯ │ ¢ [YOU] LIFTS 2¢ FROM      │
╭────────────────╮╔════════════════════════════════╗╭────────────────╮│   [MOTH]                   │
│ ⠙ MOTH         │║ TREASURY 35¢ · DECK 5          ║│ ⠙ VEX          ││ ── TURN 8 · MOTH ──        │
│ ¢3          ●● │║ > GHOSTWIRE → ASSASSINATE YOU  ║│ ¢2 ✖CONT    ●○ ││ ▸ [MOTH] BROADCASTS DUKE   │
│ DUKE×1         │║ (claims ASSASSIN)              ║│ AMBS×1         ││   SIGNAL // TAX +3¢        │
│ thinking...    │║ CHALLENGE WINDOW OPEN          ║│ thinking...    ││ ¢ +3¢ → [MOTH]             │
│                │║ DUKE R0/3 U2  ASSN R1/3 U2     ║│                ││ ── TURN 9 · GHOSTWIRE ──   │
│                │║ CAPT R0/3 U2  AMBS R0/3 U3     ║│                ││ ▸ [GHOSTWIRE] RUNS         │
╰────────────────╯║ CONT R1/3 U2                   ║╰────────────────╯│   ASSASSIN PROTOCOL ON     │
                  ╚════════════════════════════════╝                  │   [YOU] // -3¢             │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                  │                            │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 7                             │                            │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                    ╰────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║  coup armed (7)                  ╭────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║                                  │ [VEX] your firewall is ma… │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  you hold                        │ // TRINITY lost the uplin… │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║  DUKE + CAPTAIN                  │ [CASE] nobody believes th… │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                  │ enter: chat · t: taunt     │
║────────────────║║────────────────║                                  │                            │
║ TAX +3¢        ║║ STEAL 2¢       ║                                  │                            │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                  │                            │
╚════════════════╝╚════════════════╝                                  ╰────────────────────────────╯
 GHOSTWIRE claims Assassin to assassinate you.  [C] Challenge  [P] Pass  ██████░░░ 6s
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>You called a bluff: waiting for the claimant to flip a card</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13  ▶ MOTH                                      ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢1                          ●● ││ ¢3 ✖ASSN                    ●○ │          │ ✖ [YOU] LOSES INFLUENCE //    │
         │ ASSN×1 CAPT×1                  ││ DUKE×1                         │          │   CAPTAIN EXPOSED             │
         │                                ││                                │          │ ── TURN 10 · NULLBYTE ──      │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [NULLBYTE] SKIMS A CREDIT   │
                  ╔═════════════════════════════════════════════════╗                  │   // INCOME +1¢               │
                  ║ TREASURY 33¢ · DECK 5                           ║                  │ ── TURN 11 · VEX ──           │
                  ║ > MOTH → TAX (claims DUKE)                      ║                  │ ▸ [VEX] SKIMS A CREDIT //     │
                  ║ YOU CHALLENGES MOTH's DUKE                      ║                  │   INCOME +1¢                  │
                  ║ PROVE IT OR LOSE IT                             ║                  │ ── TURN 12 · YOU ──           │
╔════════════════╗║                                                 ║╭────────────────╮│ LAST ROUND // YOU             │
║ ▶ MOTH         ║║ COURT TRACKER // revealed: face up · unseen: a… ║│   VEX          ││   steal→MOTH +2¢ · MOTH tax · │
║ ¢3          ●● ║║  DUKE           ASSASSIN       CAPTAIN          ║│ ¢3 ✖CONT    ●○ ││   GHOSTWIRE assassinate→YOU · │
║ DUKE×2         ║║ REVEALED 0/3   REVEALED 1/3   REVEALED 1/3      ║│ AMBS×1         ││   NULLBYTE income · VEX       │
║ CHALLENGED! pr ║║ UNSEEN 2       UNSEEN 2       UNSEEN 2          ║│                ││   income                      │
║                ║║                                                 ║│                ││ ▸ [YOU] OPENS AN AMBASSADOR   │
║                ║║  AMBASSADOR     CONTESSA                        ║│                ││   CHANNEL // EXCHANGE         │
╚════════════════╝║ REVEALED 0/3   REVEALED 1/3                     ║╰────────────────╯│ ⇄ (you draw DUKE +            │
                  ║ UNSEEN 3       UNSEEN 2                         ║                  │   ASSASSIN)                   │
                  ║                                                 ║                  │ ⇄ [YOU] RETURNS 2 CARDS       │
                  ║                                                 ║                  │ ── TURN 13 · MOTH ──          │
                  ║                                                 ║                  │ ▸ [MOTH] BROADCASTS DUKE      │
                  ║                                                 ║                  │   SIGNAL // TAX +3¢           │
                  ╚═════════════════════════════════════════════════╝                  │ ! [YOU] CALLS [MOTH]'S DUKE   │
╔════════════════╗┌────────────────┐  RUNNER  seat 2                                   │   A LIE                       │
║ DUKE          ◆║│ CAPTAIN       ▲│  ¢ 7                                              │                               │
║     ▄▄▄▄▄▄     ║│═══▄▄   ▄▄▄▄▄▄  │  influence ●○                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║│  ▐██▌ █▀▀▀▀▀▀█ │  coup armed (7)                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║│   ▀█▄▐▓▓▓▓▓▓▓▌ │                                                   │ [VEX] your firewall is made … │
║   █   ▄▄   █ ║ ║│     ▀█▄▄▄▄▄▄▄█ │  you hold                                         │ // TRINITY lost the uplink. … │
║   ▀█▄═══▄▄█▀ ║ ║│  ▄▓▓▓█▓▓▓▓▓▓▓█▄│  DUKE                                             │ [CASE] nobody believes that … │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║│  █▓▓▓█▓▓┃┃▓▓▓██│                                                   │ enter: chat · t: taunt        │
║────────────────║│────────────────│                                                   │                               │
║ TAX +3¢        ║│ ✖ FLATLINED    │                                                   │                               │
║ BLOCKS FOR.AID ║│                │                                                   │                               │
╚════════════════╝└────────────────┘                                                   ╰───────────────────────────────╯
 You challenged MOTH's Duke. MOTH must flip a card: the Duke proves it. ⠙
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Losing an influence: pick the card to flip</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 9  ▶ GHOSTWIRE                                  ♪✖ no audio  ? help  q leave
         ╔════════════════════════════════╗╭────────────────────────────────╮          ╭───────────────────────────────╮
         ║ ▶ GHOSTWIRE               HARD ║│   NULLBYTE                EASY │          │ NETLOG                        │
         ║ ¢1                          ●● ║│ ¢2 ✖ASSN                    ●○ │          │ ✖ NULLBYTE'S TAX IS VOIDED    │
         ║ ASSN×1 CAPT×1                  ║│ DUKE×1                         │          │ ── TURN 6 · VEX ──            │
         ║ claims ASSASSIN                ║│                                │          │ ▸ [VEX] SKIMS A CREDIT //     │
         ╚════════════════════════════════╝╰────────────────────────────────╯          │   INCOME +1¢                  │
                  ╔═════════════════════════════════════════════════╗                  │ ── TURN 7 · YOU ──            │
                  ║ TREASURY 35¢ · DECK 5                           ║                  │ LAST ROUND // YOU tax ·       │
                  ║ > GHOSTWIRE → ASSASSINATE YOU (claims ASSASSIN) ║                  │   MOTH foreign aid (blocked   │
                  ║ YOU LOSES AN INFLUENCE                          ║                  │   by YOU) · GHOSTWIRE         │
                  ║                                                 ║                  │   steal→VEX (blocked by VEX)  │
╭────────────────╮║ COURT TRACKER // revealed: face up · unseen: a… ║╭────────────────╮│   (bluff caught) +2¢ ·        │
│   MOTH         │║  DUKE           ASSASSIN       CAPTAIN          ║│   VEX          ││   NULLBYTE tax (bluff caught) │
│ ¢3          ●● │║ REVEALED 0/3   REVEALED 1/3   REVEALED 0/3      ║│ ¢2 ✖CONT    ●○ ││   · VEX income                │
│ DUKE×1         │║ UNSEEN 2       UNSEEN 2       UNSEEN 2          ║│ AMBS×1         ││ ▸ [YOU] FLASHES A CAPTAIN     │
│                │║                                                 ║│                ││   BADGE AT [MOTH] // STEAL 2¢ │
│                │║  AMBASSADOR     CONTESSA                        ║│                ││ ¢ [YOU] LIFTS 2¢ FROM         │
│                │║ REVEALED 0/3   REVEALED 1/3                     ║│                ││   [MOTH]                      │
╰────────────────╯║ UNSEEN 3       UNSEEN 2                         ║╰────────────────╯│ ── TURN 8 · MOTH ──           │
                  ║                                                 ║                  │ ▸ [MOTH] BROADCASTS DUKE      │
                  ║                                                 ║                  │   SIGNAL // TAX +3¢           │
                  ║                                                 ║                  │ ¢ +3¢ → [MOTH]                │
                  ║                                                 ║                  │ ── TURN 9 · GHOSTWIRE ──      │
                  ║                                                 ║                  │ ▸ [GHOSTWIRE] RUNS ASSASSIN   │
                  ╚═════════════════════════════════════════════════╝                  │   PROTOCOL ON [YOU] // -3¢    │
╔[1]═════════════╗╔[2]═════════════╗  RUNNER  seat 2                                   │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 7                                              │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║  coup armed (7)                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║                                                   │ [VEX] your firewall is made … │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  you hold                                         │ // TRINITY lost the uplink. … │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║  DUKE + CAPTAIN                                   │ [CASE] nobody believes that … │
║╭─────────────────────────────────────────────────────────╮                           │ enter: chat · t: taunt        │
║│ HINT Pick the card to give up. It stays face up for the │                           │                               │
║│ rest of the game.                                       │                           │                               │
║│ (hints: on/off in settings, esc)                        │                           │                               │
╚╰─────────────────────────────────────────────────────────╯                           ╰───────────────────────────────╯
 You lose an influence. Pick the card to give up; it stays face up for everyone.
 [1] DUKE  [2] CAPTAIN
```

</details>

<details>
<summary><b>Ambassador exchange: pick what to keep</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 12   ▶ YOUR TURN                                ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢1                          ●● ││ ¢3 ✖ASSN                    ●○ │          │ ── TURN 8 · MOTH ──           │
         │ ASSN×1 CAPT×1                  ││ DUKE×1                         │          │ ▸ [MOTH] BROADCASTS DUKE      │
         │                                ││                                │          │   SIGNAL // TAX +3¢           │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ¢ +3¢ → [MOTH]                │
                  ╔═════════════════════════════════════════════════╗                  │ ── TURN 9 · GHOSTWIRE ──      │
                  ║ TREASURY 33¢ · DECK 3                           ║                  │ ▸ [GHOSTWIRE] RUNS ASSASSIN   │
                  ║ > YOU → EXCHANGE (claims AMBASSADOR)            ║                  │   PROTOCOL ON [YOU] // -3¢    │
                  ║ YOU IS SWAPPING CARDS                           ║                  │ ✖ [YOU] LOSES INFLUENCE //    │
                  ║                                                 ║                  │   CAPTAIN EXPOSED             │
╭────────────────╮║ COURT TRACKER // revealed: face up · unseen: a… ║╭────────────────╮│ ── TURN 10 · NULLBYTE ──      │
│   MOTH         │║  DUKE           ASSASSIN       CAPTAIN          ║│   VEX          ││ ▸ [NULLBYTE] SKIMS A CREDIT   │
│ ¢3          ●● │║ REVEALED 0/3   REVEALED 1/3   REVEALED 1/3      ║│ ¢3 ✖CONT    ●○ ││   // INCOME +1¢               │
│ DUKE×1         │║ UNSEEN 1       UNSEEN 1       UNSEEN 2          ║│ AMBS×1         ││ ── TURN 11 · VEX ──           │
│                │║                                                 ║│                ││ ▸ [VEX] SKIMS A CREDIT //     │
│                │║  AMBASSADOR     CONTESSA                        ║│                ││   INCOME +1¢                  │
│                │║ REVEALED 0/3   REVEALED 1/3                     ║│                ││ ── TURN 12 · YOU ──           │
╰────────────────╯║ UNSEEN 3       UNSEEN 2                         ║╰────────────────╯│ LAST ROUND // YOU             │
                  ║                                                 ║                  │   steal→MOTH +2¢ · MOTH tax · │
                  ║                                                 ║                  │   GHOSTWIRE assassinate→YOU · │
                  ║                                                 ║                  │   NULLBYTE income · VEX       │
                  ║                                                 ║                  │   income                      │
                  ║                                                 ║                  │ ▸ [YOU] OPENS AN AMBASSADOR   │
                  ╚═════════════════════════════════════════════════╝                  │   CHANNEL // EXCHANGE         │
╔[1]═════════════╗╔[2]═════════════╗╔[3]═════════════╗  EXCHANGE                       │ ⇄ (you draw DUKE +            │
║ DUKE          ◆║║ DUKE          ◆║║ ASSASSIN      ╳║  keep 1 of 3                    │   ASSASSIN)                   │
║     ▄▄▄▄▄▄     ║║     ▄▄▄▄▄▄     ║║      ▄▄▄▄     ╱║  selected 1/1                   ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║   ▄█▀▀▀▀▀▀█▄   ║║    ▄█▓▓▓▓█▄  ╱ ║                                 ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   █  ▄▄ (◉)█═╗ ║║   █▓▀▀▀▀▀▀▓█╱  ║  1-3 toggle                     │ [VEX] your firewall is made … │
║   █   ▄▄   █ ║ ║║   █   ▄▄   █ ║ ║║   █ ══════ ▐▌  ║  enter confirm                  │ // TRINITY lost the uplink. … │
║   ▀█▄═══▄▄█▀ ║ ║║   ▀█▄═══▄▄█▀ ║ ║║   ▀█▄    ▄█▀▌  ║                                 │ [CASE] nobody believes that … │
║╭──────────────────────────────────────────────────────────╮                          │ enter: chat · t: taunt        │
║│ HINT Exchange: choose which cards to keep (1-4, then     │                          │                               │
║│ enter). Nobody else sees what you drew or returned.      │                          │                               │
║│ (hints: on/off in settings, esc)                         │                          │                               │
╚╰──────────────────────────────────────────────────────────╯                          ╰───────────────────────────────╯
 EXCHANGE: keep 1 of 3. The rest go back into the deck. Only you see these.
 1-3 toggle · enter confirm · esc clear
```

</details>

<details>
<summary><b>Your elimination (the screen tears apart, in color)</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13  ▶ MOTH                                      ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢4                          ●● ││ ¢4                          ●● │          │ ▸ [YOU] SKIMS A CREDIT //     │
         │ no claims yet                  ││ no claims yet                  │          │   INCOME +1¢                  │
         │                                ││                                │          │ ── TURN 8 · MOTH ──           │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [MOTH] SKIMS A CREDIT //    │
                  ╔═════════════════════════════════════════════════╗                  │   INCOME +1¢                  │
                  ║ TREASURY 28¢ · DECK 5                           ║                  │ ── TURN 9 · GHOSTWIRE ──      │
                  ║ > MOTH is choosing a move                       ║                  │ ▸ [GHOSTWIRE] SKIMS A         │
                  ║                                                 ║                  │   CREDIT // INCOME +1¢        │
                  ║ COURT TRACKER // revealed: face up · unseen: a… ║                  │ ── TURN 10 · NULLBYTE ──      │
╔════════════════╗║  DUKE           ASSASSIN       CAPTAIN          ║╭────────────────╮│ ▸ [NULLBYTE] SKIMS A CREDIT   │
║ ▶ MOTH         ║║ REVEALED 0/3   REVEALED 0/3   REVEALED 0/3      ║│   VEX          ││   // INCOME +1¢               │
║ ¢4          ●● ║║ UNSEEN 2       UNSEEN 3       UNSEEN 2          ║│ ¢5          ●● ││ ── TURN 11 · VEX ──           │
║ no claims yet  ║║                                                 ║│ no claims yet  ││ ▸ [VEX] SKIMS A CREDIT //     │
║ thinking...    ║║  AMBASSADOR     CONTESSA                        ║│                ││   INCOME +1¢                  │
║                ║║ REVEALED 0/3   REVEALED 0/3                     ║│                ││ ── TURN 12 · YOU ──           │
║                ║║ UNSEEN 3       UNSEEN 3                                           ││ LAST ROUND // YOU income ·    │
╚════════════════╝║                                F L A T L I * E $      ────────────╯│   MOTH income · GHOSTWIRE     │
                  ║                                                                    │   income · NULLBYTE income ·  │
                  ║                                                 ║                  │   VEX income                  │
                  ║                                                 ║                  │ ▸ [YOU] SKIMS A CREDIT //     │
                  ║                                                 ║                  │   INCOME +1¢                  │
                  ║                                                 ║                  │ ── TURN 13 · MOTH ──          │
                  ╚═════════════════════════════════════════════════╝                  │                               │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                   │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 5                                              │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
 ║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                   ╭───────────────────────────────
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
   ║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
   ║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 MOTH is choosing a move ⠋
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Help overlay (?)</b></summary>

```text
        ╭─────────────────────────────────────────────────────────────────────────────────╮
        │                                                                                 │
        │  CONTROLS                                                                       │
        │                                                                                 │
        │   1-8     actions: income, aid, coup, tax, assassinate, steal, exchange, audit  │
        │   c       challenge the claim on the table                                      │
        │   b       block (then pick the card if there's a choice)                        │
        │   p       pass                                                                  │
        │   1 / 2   pick a card when losing influence or proving a claim                  │
        │   tab     inspect a player: claims, history, face-up cards                      │
        │   space   skip the current beat (solo)    P  pause (solo)                       │
        │   enter   chat    t  taunt menu    pgup/pgdn  scroll the log                    │
        │   m       mute    n  next track    s  CRT scanlines                             │
        │   esc     cancel a choice, or open settings    q  leave                         │
        │   ?       this help                                                             │
        │                                                                                 │
        │  RULES IN ONE BREATH                                                            │
        │  Everyone has two hidden cards (influence) and coins. On your turn take         │
        │  one action. Any character action may be a bluff: anyone can challenge.         │
        │  A failed challenge costs the challenger a card; a caught bluff costs           │
        │  the bluffer a card. 7 coins buys a Coup; at 10 you must Coup. Lose             │
        │  both cards and you're out. Last runner standing wins.                          │
        │                                                                                 │
        │  READING THE TABLE                                                              │
        │  The bottom bar says what's happening, what you can do and what it              │
        │  risks. The tracker in the middle counts each character: REVEALED is            │
        │  face up, UNSEEN could be in a rival's hand or the deck. UNSEEN 0 means         │
        │  any claim of it is a bluff. Badges on a rival's panel show what                │
        │  they've claimed this game.                                                     │
        │                                                                                 │
```

</details>

<details>
<summary><b>Victory</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13  ▶ MOTH                                      ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢4        ╔════════════════════════════════════════════════════════════════════════════╗IMS A CREDIT //     │
         │ no claims ║                                                                            ║1¢                  │
         │           ║       SYSTEM SEIZED                                                        ║· MOTH ──           │
         ╰───────────║                                                                            ║KIMS A CREDIT //    │
                  ╔══║    winner: RUNNER   turns: 13                                              ║1¢                  │
                  ║ T║    your challenges: 2 (2 won)   blocks: 1   coins earned: 17   coups: 2    ║· GHOSTWIRE ──      │
                  ║ >║                                                                            ║RE] SKIMS A         │
                  ║  ║    THE CARDS // every hand, face up                                        ║/ INCOME +1¢        │
                  ║ C║    VEX          DUKE        CONTESSA    bluffs: 2 got away, 0 caught       ║ · NULLBYTE ──      │
╔════════════════╗║  ║    YOU          CONTESSA    CAPTAIN     played it straight                 ║E] SKIMS A CREDIT   │
║ ▶ MOTH         ║║ R║    MOTH         AMBASSADOR  CAPTAIN     played it straight                 ║E +1¢               │
║ ¢4          ●● ║║ U║    GHOSTWIRE    CAPTAIN     CONTESSA    played it straight                 ║ · VEX ──           │
║ no claims yet  ║║  ║    NULLBYTE     DUKE        ASSASSIN    played it straight                 ║IMS A CREDIT //     │
║ thinking...    ║║  ║                                                                            ║1¢                  │
║                ║║ R║    KEY MOMENTS                                                             ║ · YOU ──           │
║                ║║ U║    T6  VEX assassinated your CAPTAIN                                       ║ // YOU income ·    │
╚════════════════╝║  ║    T7  MOTH called your DUKE: it was real                                  ║ome · GHOSTWIRE     │
                  ║  ║    T8  NULLBYTE called MOTH's DUKE: it was real                            ║ NULLBYTE income ·  │
                  ║  ║    T9  VEX called MOTH's AMBASSADOR: it was real                           ║me                  │
                  ║  ║    T12 YOU couped MOTH                                                     ║IMS A CREDIT //     │
                  ║  ║    T12 MOTH flatlined                                                      ║1¢                  │
                  ║  ║    T22 NULLBYTE couped GHOSTWIRE                                           ║ · MOTH ──          │
                  ╚══║    T23 GHOSTWIRE called VEX's ASSASSIN: it was real                        ║                    │
╔════════════════╗╔══║    T23 GHOSTWIRE flatlined                                                 ║                    │
║ DUKE          ◆║║ C║    T34 NULLBYTE couped VEX                                                 ║                    │
║     ▄▄▄▄▄▄     ║║══║    T34 VEX flatlined                                                       ║────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ║    T39 YOU couped NULLBYTE                                                 ║────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║  ║    T39 NULLBYTE flatlined                                                  ║                    │
║   █   ▄▄   █ ║ ║║  ║    T39 YOU seized the city                                                 ║t · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ║    replay saved: ~/.config/neoncoup/replays/20870101-000000-7K2Q.json      ║                    │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  ║                                                                            ║                    │
║────────────────║║──║    [r] rematch  [v] replay  [enter] lobby  [q] leave                       ║                    │
║ TAX +3¢        ║║ S║                                                                            ║                    │
║ BLOCKS FOR.AID ║║ B╚════════════════════════════════════════════════════════════════════════════╝                    │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 MOTH is choosing a move ⠋
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Defeat</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13  ▶ MOTH                                      ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢4        ╔════════════════════════════════════════════════════════════════════════════╗IMS A CREDIT //     │
         │ no claims ║                                                                            ║1¢                  │
         │           ║       CONNECTION TERMINATED                                                ║· MOTH ──           │
         ╰───────────║                                                                            ║KIMS A CREDIT //    │
                  ╔══║    winner: GHOSTWIRE   turns: 13                                           ║1¢                  │
                  ║ T║    your challenges: 2 (2 won)   blocks: 1   coins earned: 17   coups: 2    ║· GHOSTWIRE ──      │
                  ║ >║                                                                            ║RE] SKIMS A         │
                  ║  ║    THE CARDS // every hand, face up                                        ║/ INCOME +1¢        │
                  ║ C║    VEX          DUKE        CONTESSA    bluffs: 2 got away, 0 caught       ║ · NULLBYTE ──      │
╔════════════════╗║  ║    YOU          CONTESSA    CAPTAIN     played it straight                 ║E] SKIMS A CREDIT   │
║ ▶ MOTH         ║║ R║    MOTH         AMBASSADOR  CAPTAIN     played it straight                 ║E +1¢               │
║ ¢4          ●● ║║ U║    GHOSTWIRE    CAPTAIN     CONTESSA    played it straight                 ║ · VEX ──           │
║ no claims yet  ║║  ║    NULLBYTE     DUKE        ASSASSIN    played it straight                 ║IMS A CREDIT //     │
║ thinking...    ║║  ║                                                                            ║1¢                  │
║                ║║ R║    KEY MOMENTS                                                             ║ · YOU ──           │
║                ║║ U║    T6  VEX assassinated your CAPTAIN                                       ║ // YOU income ·    │
╚════════════════╝║  ║    T7  MOTH called your DUKE: it was real                                  ║ome · GHOSTWIRE     │
                  ║  ║    T8  NULLBYTE called MOTH's DUKE: it was real                            ║ NULLBYTE income ·  │
                  ║  ║    T9  VEX called MOTH's AMBASSADOR: it was real                           ║me                  │
                  ║  ║    T12 YOU couped MOTH                                                     ║IMS A CREDIT //     │
                  ║  ║    T12 MOTH flatlined                                                      ║1¢                  │
                  ║  ║    T22 NULLBYTE couped GHOSTWIRE                                           ║ · MOTH ──          │
                  ╚══║    T23 GHOSTWIRE called VEX's ASSASSIN: it was real                        ║                    │
╔════════════════╗╔══║    T23 GHOSTWIRE flatlined                                                 ║                    │
║ DUKE          ◆║║ C║    T34 NULLBYTE couped VEX                                                 ║                    │
║     ▄▄▄▄▄▄     ║║══║    T34 VEX flatlined                                                       ║────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ║    T39 YOU couped NULLBYTE                                                 ║────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║  ║    T39 NULLBYTE flatlined                                                  ║                    │
║   █   ▄▄   █ ║ ║║  ║    T39 YOU seized the city                                                 ║t · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ║    replay saved: ~/.config/neoncoup/replays/20870101-000000-7K2Q.json      ║                    │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  ║                                                                            ║                    │
║────────────────║║──║    [r] rematch  [v] replay  [enter] lobby  [q] leave                       ║                    │
║ TAX +3¢        ║║ S║                                                                            ║                    │
║ BLOCKS FOR.AID ║║ B╚════════════════════════════════════════════════════════════════════════════╝                    │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 MOTH is choosing a move ⠋
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Stats</b></summary>

```text


               ╭────────────────────────────────────────────────────────────────────╮
               │                                                                    │
               │   RUNNER RECORD // RUNNER                                          │
               │                                                                    │
               │   games played         3                                           │
               │   wins                 2   ████████████████████░░░░░░░░░░  66.7%   │
               │   win streak           1 / 1 best                                  │
               │                                                                    │
               │   bluffs               19 (5 caught)                               │
               │   bluff success        ██████████████████████░░░░░░░░  73.7%       │
               │   challenges           11 (7 won)                                  │
               │   challenge accuracy   ███████████████████░░░░░░░░░░░  63.6%       │
               │                                                                    │
               │   blocks               8                                           │
               │   coins earned         123                                         │
               │   coups launched       6                                           │
               │   times flatlined      1                                           │
               │   longest game         41 turns                                    │
               │                                                                    │
               │   wins by mode: solo 2/2 · host 0/1                                │
               │   last run: 2026-10-27 00:19                                       │
               │                                                                    │
               │   esc back                                                         │
               │                                                                    │
               ╰────────────────────────────────────────────────────────────────────╯
```

</details>

<details>
<summary><b>Taunt menu (t)</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13  ▶ MOTH                                      ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢4                          ●● ││ ¢4                          ●● │          │ ▸ [YOU] SKIMS A CREDIT //     │
         │ no claims yet                  ││ no claims yet                  │          │   INCOME +1¢                  │
         │                                ││                                │          │ ── TURN 8 · MOTH ──           │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [MOTH] SKIMS A CREDIT //    │
                  ╔═════════════════════════════════════════════════╗                  │   INCOME +1¢                  │
                  ║ TREASURY 28¢ · DECK 5                           ║                  │ ── TURN 9 · GHOSTWIRE ──      │
                  ║ > MOTH is choosing a move                       ║                  │ ▸ [GHOSTWIRE] SKIMS A         │
                  ║                                                 ║                  │   CREDIT // INCOME +1¢        │
                  ║ COURT TRACKER // revealed: face up · unseen: a… ║                  │ ── TURN 10 · NULLBYTE ──      │
╔════════════════╗║  DUKE           ASSAS╭───────────────────────────────────╮────────╮│ ▸ [NULLBYTE] SKIMS A CREDIT   │
║ ▶ MOTH         ║║ REVEALED 0/3   REVEAL│                                   │        ││   // INCOME +1¢               │
║ ¢4          ●● ║║ UNSEEN 2       UNSEEN│  TAUNT                            │     ●● ││ ── TURN 11 · VEX ──           │
║ no claims yet  ║║                      │                                   │ms yet  ││ ▸ [VEX] SKIMS A CREDIT //     │
║ thinking...    ║║  AMBASSADOR     CONTE│  [1] gg, choom                    │        ││   INCOME +1¢                  │
║                ║║ REVEALED 0/3   REVEAL│  [2] nice bluff... or was it?     │        ││ ── TURN 12 · YOU ──           │
║                ║║ UNSEEN 3       UNSEEN│  [3] i have the Duke. obviously.  │        ││ LAST ROUND // YOU income ·    │
╚════════════════╝║                      │  [4] your firewall is showing     │────────╯│   MOTH income · GHOSTWIRE     │
                  ║                      │  [5] say goodbye to your credits  │         │   income · NULLBYTE income ·  │
                  ║                      │  [6] challenge me. i dare you.    │         │   VEX income                  │
                  ║                      │  [7] pure static                  │         │ ▸ [YOU] SKIMS A CREDIT //     │
                  ║                      │  [8] see you in the next run      │         │   INCOME +1¢                  │
                  ║                      │                                   │         │ ── TURN 13 · MOTH ──          │
                  ╚══════════════════════│  any other key to close           │         │                               │
╔════════════════╗╔════════════════╗  RUN│                                   │         │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 5╰───────────────────────────────────╯         │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │                               │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 MOTH is choosing a move ⠋
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>--ascii mode</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 3  > MOTH                                       ~x no audio  ? help  q leave
         +--------------------------------++--------------------------------+          +-------------------------------+
         | . GHOSTWIRE               HARD || . NULLBYTE                EASY |          | NETLOG                        |
         | c2                          oo || c2                          oo |          | // GAME 3 // 5 RUNNERS //     |
         | no claims yet                  || no claims yet                  |          |   JACKED IN                   |
         | thinking...                    || thinking...                    |          | -- TURN 1 . VEX --            |
         +--------------------------------++--------------------------------+          | > [VEX] SKIMS A CREDIT //     |
                  +-------------------------------------------------+                  |   INCOME +1c                  |
                  | TREASURY 36c . DECK 5                           |                  | -- TURN 2 . YOU --            |
                  | > MOTH -> STEAL YOU (claims CAPTAIN)            |                  | LAST ROUND // VEX income      |
                  | CHALLENGE WINDOW OPEN                           |                  | > [YOU] BROADCASTS DUKE       |
                  | #########################..............  5.2s   |                  |   SIGNAL // TAX +3c           |
+----------------+|                                                 |+----------------+| c +3c > [YOU]                 |
| > MOTH         || COURT TRACKER // revealed: face up . unseen: a~ || . VEX          || -- TURN 3 . MOTH --           |
| c2          oo ||  DUKE           ASSASSIN       CAPTAIN          || c3          oo || > [MOTH] FLASHES A CAPTAIN    |
| CAPTx1         || REVEALED 0/3   REVEALED 0/3   REVEALED 0/3      || no claims yet  ||   BADGE AT [YOU] // STEAL 2c  |
| claims CAPTAIN || UNSEEN 2       UNSEEN 3       UNSEEN 2          || thinking...    ||                               |
|                ||                                                 ||                ||                               |
|                ||  AMBASSADOR     CONTESSA                        ||                ||                               |
+----------------+| REVEALED 0/3   REVEALED 0/3                     |+----------------+|                               |
                  | UNSEEN 3       UNSEEN 3                         |                  |                               |
                  |                                                 |                  |                               |
                  |                                                 |                  |                               |
                  |                                                 |                  |                               |
                  |                                                 |                  |                               |
                  +-------------------------------------------------+                  |                               |
+----------------++----------------+  RUNNER  seat 2                                   |                               |
| DUKE          *|| CAPTAIN       ^|  c 5                                              |                               |
|     ______     ||===__   ______  |  influence oo                                     +-------------------------------+
|   _#""""""#_   ||  ]##[ #""""""# |                                                   +-------------------------------+
|   #  __ (@)#=+ ||   "#_]%%%%%%%[ |  you hold                                         |                               |
|+--------------------------------------------------------+                            | enter: chat . t: taunt        |
|| HINT Someone claimed a character. Challenge (c) if you |                            |                               |
|| think they're bluffing: if they can show it, YOU       |                            |                               |
|| lose an influence. The tracker's UNSEEN 0 means a      |                            |                               |
|| sure bluff.                                            |                            |                               |
|| (hints: on/off in settings, esc)                       |                            |                               |
++--------------------------------------------------------+                            +-------------------------------+
 MOTH claims Captain to steal 2 coins from you.  [C] Challenge (wrong: you lose 1)  [P] Pass  ######... 6s
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Terminal too small</b></summary>

```text








                    ╭─────────────────────────────────────╮
                    │                                     │
                    │  NEON//COUP needs at least 100x30.  │
                    │  This window is 80x24.              │
                    │                                     │
                    │  Make it bigger, or zoom out.       │
                    │                                     │
                    ╰─────────────────────────────────────╯
```

</details>

<details>
<summary><b>Replay viewer: every hand face up</b></summary>

```text
 REPLAY  2087-01-01 02:00  step 88/133  turn 20                                                     paused
TREASURY 31¢ · DECK 5                                                         ╭────────────────────────────────────────╮
> VEX: INCOME                                                                 │ NETLOG                                 │
                                                                              │ [MOTH] RESHUFFLES AND DRAWS A FRESH C… │
VEX               ┌──────────┐┌──────────┐                                    │ [VEX] LOSES INFLUENCE // CONTESSA EXP… │
¢3                │ ASSASSIN╳││ CONTESSA✧│                                    │ BLOCK HOLDS // GHOSTWIRE'S STEAL FIZZ… │
                  │  ▄█▓▓█▄ ╱││   ▐◆  ◆▌ │                                    │ ── TURN 10 · NULLBYTE ──               │
                  │  █ ════▐▌││ ▐░▒▓░▒▓░▌│                                    │ [NULLBYTE] BROADCASTS DUKE SIGNAL // … │
                  │KILL 3¢   ││✖ LOST    │                                    │ +3¢ → [NULLBYTE]                       │
                  │-1 INFL   ││          │                                    │ ── TURN 11 · VEX ──                    │
                  └──────────┘└──────────┘                                    │ [VEX] SKIMS A CREDIT // INCOME +1¢     │
▶ RUNNER          ┌──────────┐┌──────────┐                                    │ ── TURN 12 · RUNNER ──                 │
¢1                │ CONTESSA✧││ CAPTAIN ▲│                                    │ [RUNNER] LAUNCHES A COUP ON [MOTH] //… │
                  │   ▐◆  ◆▌ ││══▄ █▀▀▀▀█│                                    │ [MOTH] LOSES INFLUENCE // CAPTAIN EXP… │
                  │ ▐░▒▓░▒▓░▌││ ▐█▌▐▓▓▓▓▌│                                    │ [MOTH] FLATLINED // OUT OF THE GAME (… │
                  │BLK KILL  ││✖ LOST    │                                    │ ── TURN 13 · GHOSTWIRE ──              │
                  │          ││          │                                    │ [GHOSTWIRE] FLASHES A CAPTAIN BADGE A… │
                  └──────────┘└──────────┘                                    │ [GHOSTWIRE] LIFTS 2¢ FROM [VEX]        │
MOTH              ┌──────────┐┌──────────┐                                    │ ── TURN 14 · NULLBYTE ──               │
FLATLINED         │ AMBASSAD◎││ CAPTAIN ▲│                                    │ [NULLBYTE] BROADCASTS DUKE SIGNAL // … │
                  │  ░▒▓▀▀▓▒░││══▄ █▀▀▀▀█│                                    │ +3¢ → [NULLBYTE]                       │
                  │  ▐═[▓▓]═▌││ ▐█▌▐▓▓▓▓▌│                                    │ ── TURN 15 · VEX ──                    │
                  │✖ LOST    ││✖ LOST    │                                    │ [VEX] SKIMS A CREDIT // INCOME +1¢     │
                  │          ││          │                                    │ ── TURN 16 · RUNNER ──                 │
                  └──────────┘└──────────┘                                    │ [RUNNER] OPENS AN AMBASSADOR CHANNEL … │
GHOSTWIRE         ┌──────────┐┌──────────┐                                    │ [RUNNER] PULLS 2 CARDS FROM THE COURT  │
¢6                │ CAPTAIN ▲││ CONTESSA✧│                                    │ [RUNNER] RETURNS 2 CARDS               │
                  │══▄ █▀▀▀▀█││   ▐◆  ◆▌ │                                    │ ── TURN 17 · GHOSTWIRE ──              │
                  │ ▐█▌▐▓▓▓▓▌││ ▐░▒▓░▒▓░▌│                                    │ [GHOSTWIRE] FLASHES A CAPTAIN BADGE A… │
                  │STEAL 2¢  ││BLK KILL  │                                    │ [GHOSTWIRE] LIFTS 2¢ FROM [NULLBYTE]   │
                  │BLK STEAL ││          │                                    │ ── TURN 18 · NULLBYTE ──               │
                  └──────────┘└──────────┘                                    │ [NULLBYTE] BROADCASTS DUKE SIGNAL // … │
NULLBYTE          ┌──────────┐┌──────────┐                                    │ +3¢ → [NULLBYTE]                       │
¢9                │ DUKE    ◆││ ASSASSIN╳│                                    │ ── TURN 19 · VEX ──                    │
                  │ ▄█▀▀(◉)═╗││  ▄█▓▓█▄ ╱│                                    │ [VEX] SKIMS A CREDIT // INCOME +1¢     │
                  │ ▓▓█[¢]█▓▓││  █ ════▐▌│                                    │ ── TURN 20 · RUNNER ──                 │
                  │TAX +3¢   ││✖ LOST    │                                    ╰────────────────────────────────────────╯
←/→ step · ↑/↓ ±10 · space play/pause · +/- speed · home/end · esc back
```

</details>

<details>
<summary><b>Tutorial intro</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 1  ▶ VEX                                        ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │ ⠋ GHOSTWIRE               HARD ││ ⠋ NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢2                          ●● ││ ¢2                          ●● │          │ // GAME 3 // 5 RUNNERS //     │
         │ no claims yet                  ││ no claims yet                  │          │   JACKED IN                   │
         │ thinking...                    ││ thinking...                    │          │ ── TURN 1 · VEX ──            │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [VEX] BROADCASTS DUKE       │
                  ╔═════════════════════════════════════════════════╗                  │   SIGNAL // TAX +3¢           │
                  ║ TREASURY 40¢ · DECK 5                           ║                  │                               │
                  ║ > VEX → TAX (claims DUKE)                       ║                  │                               │
                  ║ CHALLENGE WINDOW OPEN                           ║                  │                               │
                  ║ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░░░░░░░  5.2s   ║                  │                               │
╭────────────────╮║╔════════════════════════════════════════════════════════════════════════════════╗                  │
│ ⠋ MOTH         │║║                                                                                ║                  │
│ ¢2          ●● │║║   WELCOME TO THE CON                                                           ║                  │
│ no claims yet  │║║                                                                                ║                  │
│ thinking...    │║║   Every runner holds 2 face-down cards: their INFLUENCE.                       ║                  │
│                │║║   Lose both and you're out. Last runner standing takes the city.               ║                  │
│                │║║                                                                                ║                  │
╰────────────────╯║║   On your turn you take one action. Some actions belong to a character:        ║                  │
                  ║║   you may claim ANY character, whether you hold it or not.                     ║                  │
                  ║║   Others can CHALLENGE the claim, or BLOCK it with a character of their own.   ║                  │
                  ║║                                                                                ║                  │
                  ║║   Your cards are at the bottom. The log is on the right.                       ║                  │
                  ║║                                                                                ║                  │
                  ╚║   press enter to start                                                         ║                  │
╔════════════════╗╔║                                                                                ║                  │
║ DUKE          ◆║║╚════════════════════════════════════════════════════════════════════════════════╝                  │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │                               │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 VEX claims Duke to take 3 coins.  [C] Challenge (if VEX has a Duke, you lose an influence)  [P] Pass  ██████░░░ 6s
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Tutorial coach</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 1  ▶ VEX                                        ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │ ⠋ GHOSTWIRE               HARD ││ ⠋ NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢2                          ●● ││ ¢2                          ●● │          │ // GAME 3 // 5 RUNNERS //     │
         │ no claims yet                  ││ no claims yet                  │          │   JACKED IN                   │
         │ thinking...                    ││ thinking...                    │          │ ── TURN 1 · VEX ──            │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [VEX] BROADCASTS DUKE       │
  ╭────────────────────────────────────────────────────────────────────────────╮       │   SIGNAL // TAX +3¢           │
  │  COACH // SMELL THE BLUFF                                                  │       │                               │
  │  VEX claims DUKE. MOTH just showed confidence in a Duke and you hold one.  │       │                               │
  │  VEX never acted like a Duke before. Call it: press c to CHALLENGE.        │       │                               │
  ╰────────────────────────────────────────────────────────────────────────────╯       │                               │
╭────────────────╮║                                                 ║╔════════════════╗│                               │
│ ⠋ MOTH         │║ COURT TRACKER // revealed: face up · unseen: a… ║║ ▶ VEX          ║│                               │
│ ¢2          ●● │║  DUKE           ASSASSIN       CAPTAIN          ║║ ¢2          ●● ║│                               │
│ no claims yet  │║ REVEALED 0/3   REVEALED 0/3   REVEALED 0/3      ║║ DUKE×1         ║│                               │
│ thinking...    │║ UNSEEN 2       UNSEEN 3       UNSEEN 2          ║║ claims DUKE    ║│                               │
│                │║                                                 ║║                ║│                               │
│                │║  AMBASSADOR     CONTESSA                        ║║                ║│                               │
╰────────────────╯║ REVEALED 0/3   REVEALED 0/3                     ║╚════════════════╝│                               │
                  ║ UNSEEN 3       UNSEEN 3                         ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ╚═════════════════════════════════════════════════╝                  │                               │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                   │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 2                                              │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │                               │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 VEX claims Duke to take 3 coins.  [C] Challenge (if VEX has a Duke, you lose an influence)  [P] Pass  ██████░░░ 6s
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>A move being announced: banner, target marked, the rest dimmed</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 1  ▶ VEX                                        ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢2                          ●● ││ ¢2                          ●● │          │ // GAME 3 // 5 RUNNERS //     │
         │ no claims yet                  ││ no claims yet                  │          │   JACKED IN                   │
         │                                ││                                │          │ ── TURN 1 · VEX ──            │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [VEX] FLASHES A CAPTAIN     │
                  ╔═════════════════════════════════════════════════╗                  │   BADGE AT [MOTH] // STEAL 2¢ │
                  ║ TREASURY 40¢ · DECK 5                           ║                  │                               │
                  ║ > VEX → STEAL MOTH (claims CAPTAIN)             ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║ COURT TRACKER // revealed: face up · unseen: a… ║                  │                               │
╭────────────────╮║  DUKE           ASSASSIN       CAPTAIN          ║╔════════════════╗│                               │
│   MOTH         │║ REVEALED 0/3   REVEALED 0/3   REVEALED 0/3      ║║ ▶ VEX          ║│                               │
│ ¢2          ●● │║╔══════════════════════════════════════════════╗ ║║ ¢2          ●● ║│                               │
│ no claims yet  │║║                                              ║ ║║ CAPT×1         ║│                               │
│ ◀ TARGET       │║║    VEX ▸ CLAIMS CAPTAIN ▸ STEAL 2¢ ▸ MOTH    ║ ║║ claims CAPTAIN ║│                               │
│                │║║                                              ║ ║║                ║│                               │
│                │║╚══════════════════════════════════════════════╝ ║║                ║│                               │
╰────────────────╯║                                                 ║╚════════════════╝│                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ║                                                 ║                  │                               │
                  ╚═════════════════════════════════════════════════╝                  │                               │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                   │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 2                                              │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │                               │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 VEX ▸ CLAIMS CAPTAIN ▸ STEAL 2¢ ▸ MOTH
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>The card tracker at 160x45, after a bluff was caught</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 2   ▶ YOUR TURN                                                                         ♪✖ no audio  ? help  q leave
                        ╭────────────────────────────────╮╭────────────────────────────────╮                        ╭──────────────────────────────────────────╮
                        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │                        │ NETLOG                                   │
                        │ ¢2                          ●● ││ ¢2                          ●● │                        │ // GAME 3 // 5 RUNNERS // JACKED IN      │
                        │ no claims yet                  ││ no claims yet                  │                        │ ── TURN 1 · VEX ──                       │
                        │                                ││                                │                        │ ▸ [VEX] BROADCASTS DUKE SIGNAL // TAX    │
                        ╰────────────────────────────────╯╰────────────────────────────────╯                        │   +3¢                                    │
                   ╔════════════════════════════════════════════════════════════════════════════╗                   │ ! [YOU] CALLS [VEX]'S DUKE A LIE         │
                   ║ > YOUR TURN · choose an action below                                       ║                   │ ✖ [VEX] WAS BLUFFING // FLIPS ASSASSIN   │
                   ║                                                                            ║                   │ ✖ [VEX] LOSES INFLUENCE // ASSASSIN      │
                   ║ COURT TRACKER // revealed: face up · unseen: anywhere but your hand        ║                   │   EXPOSED                                │
                   ║ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   ╭────╮  ║                   │ ✖ VEX'S TAX IS VOIDED                    │
                   ║ │ DUKE    ◆│ │ ASSASSIN╳│ │ CAPTAIN ▲│ │ AMBASSAD◎│ │ CONTESSA✧│  ╭│▒░▒░│  ║                   │ ── TURN 2 · YOU ──                       │
                   ║ │ ▄█▀▀(◉)═╗│ │  ▄█▓▓█▄ ╱│ │══▄ █▀▀▀▀█│ │  ░▒▓▀▀▓▒░│ │   ▐◆  ◆▌ │  ││░▒░▒│  ║                   │ LAST ROUND // VEX tax (bluff caught)     │
                   ║ │ ▓▓█[¢]█▓▓│ │  █ ════▐▌│ │ ▐█▌▐▓▓▓▓▌│ │  ▐═[▓▓]═▌│ │ ▐░▒▓░▒▓░▌│  ╰╰────╯  ║                   │                                          │
╭─────────────────╮║ │TAX +3¢   │ │KILL 3¢   │ │STEAL 2¢  │ │SWAP 2    │ │BLK KILL  │ DECK   5  ║╭─────────────────╮│                                          │
│   MOTH          │║ │BLK AID   │ │-1 INFL   │ │BLK STEAL │ │BLK STEAL │ │          │           ║│   VEX      HARD ││                                          │
│ ¢2           ●● │║ └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘ TREASURY  ║│ ¢2 ✖ASSN     ●○ ││                                          │
│ no claims yet   │║ REVEALED 0/3 REVEALED 1/3 REVEALED 0/3 REVEALED 0/3 REVEALED 0/3      40¢  ║│ DUKE×1          ││                                          │
│                 │║ UNSEEN 2     UNSEEN 2     UNSEEN 2     UNSEEN 3     UNSEEN 3               ║│                 ││                                          │
│                 │║                                                                            ║│                 ││                                          │
│                 │║                                                                            ║│                 ││                                          │
╰─────────────────╯║                                                                            ║╰─────────────────╯│                                          │
                   ║                                                                            ║                   │                                          │
                   ║                                                                            ║                   │                                          │
                   ║                                                                            ║                   │                                          │
                   ║                                                                            ║                   │                                          │
                   ║                                                                            ║                   │                                          │
                   ║                                                                            ║                   │                                          │
                   ║                                                                            ║                   │                                          │
                   ╚════════════════════════════════════════════════════════════════════════════╝                   │                                          │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                                                │                                          │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 2                                                                           │                                          │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                                                  ╰──────────────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                                                ╭──────────────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                                                      │                                          │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                                                │ enter: chat · t: taunt                   │
║╭─────────────────────────────────────────────────────────╮                                                        │                                          │
║│ HINT Your turn: pick an action with 1-8. Green ones you │                                                        │                                          │
║│ can back up; magenta~ ones would be bluffs. Anyone      │                                                        │                                          │
║│ may challenge a claim.                                  │                                                        │                                          │
║│ (hints: on/off in settings, esc)                        │                                                        │                                          │
╚╰─────────────────────────────────────────────────────────╯                                                        ╰──────────────────────────────────────────╯
 YOUR TURN with 2¢. Green: you hold it. Magenta~: a bluff. Coup needs 7¢ · Assassinate needs 3¢
 [1]INCOME  [2]AID  [3]COUP  [4]TAX  [5]ASSASSIN  [6]STEAL  [7]SWAP~  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Inspect (tab): one player's claims and history</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 2   ▶ YOUR TURN                                 ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢2                          ●● ││ ¢2                          ●● │          │ // GAME 3 // 5 RUNNERS //     │
         │ no claims yet                  ││ no claims yet                  │          │   JACKED IN                   │
         │                                ││                                │          │ ── TURN 1 · VEX ──            │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [VEX] BROADCASTS DUKE       │
                  ╔═════════════════════════════════════════════════╗                  │   SIGNAL // TAX +3¢           │
                  ║ TREASURY 40¢ · DECK 5                           ║                  │ ! [YOU] CALLS [VEX]'S DUKE    │
                  ║ > YOUR TURN · choose an action below            ║                  │   A LIE                       │
                  ║                                                 ║                  │ ✖ [VEX] WAS BLUFFING //       │
                  ║ COURT TRACKER // revealed: face up · unseen: a… ║                  │   FLIPS ASSASSIN              │
╭────────────────╮║  DUKE           ASSAS╭───────────────────────────────────╮────────╮│ ✖ [VEX] LOSES INFLUENCE //    │
│   MOTH         │║ REVEALED 0/3   REVEAL│                                   │        ││   ASSASSIN EXPOSED            │
│ ¢2          ●● │║ UNSEEN 2       UNSEEN│   INSPECT // VEX  HARD bot        │N    ●○ ││ ✖ VEX'S TAX IS VOIDED         │
│ no claims yet  │║                      │   2¢ · influence 1                │        ││ ── TURN 2 · YOU ──            │
│                │║  AMBASSADOR     CONTE│   revealed  ASSASSIN              │        ││ LAST ROUND // VEX tax         │
│                │║ REVEALED 0/3   REVEAL│   claims    DUKE×1                │        ││   (bluff caught)              │
│                │║ UNSEEN 3       UNSEEN│                                   │        ││                               │
╰────────────────╯║                      │   HISTORY                         │────────╯│                               │
                  ║                      │   T1  TAX (claims DUKE)           │         │                               │
                  ║                      │   T1  CHALLENGED by YOU           │         │                               │
                  ║                      │       ✖ CAUGHT BLUFFING           │         │                               │
                  ║                      │   T1  LOST ASSASSIN               │         │                               │
                  ║                      │                                   │         │                               │
                  ╚══════════════════════│   tab: next player · esc: close   │         │                               │
╔════════════════╗╔════════════════╗  RUN│                                   │         │                               │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 2╰───────────────────────────────────╯         │                               │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                     ╰───────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                   ╭───────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │                               │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 YOUR TURN with 2¢. Green: you hold it. Magenta~: a bluff. Coup needs 7¢ · Assassinate needs 3¢
 [1]INCOME  [2]AID  [3]COUP  [4]TAX  [5]ASSASSIN  [6]STEAL  [7]SWAP~  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Settings overlay (esc)</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 2   ▶ YOUR TURN                                 ♪✖ no audio  ? help  q leave
         ╭────────────────────────────────╮╭────────────────────────────────╮          ╭───────────────────────────────╮
         │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │          │ NETLOG                        │
         │ ¢2                          ●● ││ ¢2                          ●● │          │ // GAME 3 // 5 RUNNERS //     │
         │ no claims yet                  ││ no claims yet                  │          │   JACKED IN                   │
         │                                ││                                │          │ ── TURN 1 · VEX ──            │
         ╰────────────────────────────────╯╰────────────────────────────────╯          │ ▸ [VEX] BROADCASTS DUKE       │
                  ╔═════════════════════════════════════════════════╗                  │   SIGNAL // TAX +3¢           │
                  ║ TREASURY 40¢ · DECK 5                           ║                  │ ! [YOU] CALLS [VEX]'S DUKE    │
                  ║ > YOUR TURN · choose an action below            ║                  │   A LIE                       │
                  ║     ╭──────────────────────────────────────────────────────────────────────╮ WAS BLUFFING //       │
                  ║ COUR│                                                                      │ ASSASSIN              │
╭────────────────╮║  DUK│   SETTINGS                                                           │ LOSES INFLUENCE //    │
│   MOTH         │║ REVE│   saved to this session only                                         │SIN EXPOSED            │
│ ¢2          ●● │║ UNSE│                                                                      │ TAX IS VOIDED         │
│ no claims yet  │║     │   ▶ Speed           ◀ RELAXED  (the host's choice) ▶                 │ 2 · YOU ──            │
│                │║  AMB│     Hints           ◀ ON ▶                                           │UND // VEX tax         │
│                │║ REVE│     Reduce motion   ◀ OFF ▶                                          │f caught)              │
│                │║ UNSE│     Palette         ◀ NEON ▶                                         │                       │
╰────────────────╯║     │     Master volume   ◀ 60% ▶                                          │                       │
                  ║     │     Music volume    ◀ 80% ▶                                          │                       │
                  ║     │     SFX volume      ◀ 100% ▶                                         │                       │
                  ║     │     Muted           ◀ OFF ▶                                          │                       │
                  ║     │     CRT scanlines   ◀ OFF ▶                                          │                       │
                  ║     │                                                                      │                       │
                  ╚═════│   The host picks the speed for everyone at an online table.          │                       │
╔════════════════╗╔═════│                                                                      │                       │
║ DUKE          ◆║║ CAPT│   ↑↓ field · ←→ change · enter to close · esc back                   │                       │
║     ▄▄▄▄▄▄     ║║═══▄▄│                                                                      │───────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██╰──────────────────────────────────────────────────────────────────────╯───────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                         │                               │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                   │ enter: chat · t: taunt        │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                   │                               │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                   │                               │
║────────────────║║────────────────║                                                   │                               │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                   │                               │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                   │                               │
╚════════════════╝╚════════════════╝                                                   ╰───────────────────────────────╯
 YOUR TURN with 2¢. Green: you hold it. Magenta~: a bluff. Coup needs 7¢ · Assassinate needs 3¢
 [1]INCOME  [2]AID  [3]COUP  [4]TAX  [5]ASSASSIN  [6]STEAL  [7]SWAP~  [t]TAUNT  [?]HELP
```

</details>

<!-- screens:end -->

## Development

```sh
make test         # everything, including the 10,000-game simulation (about a minute)
make test-short   # quick pass (500-game simulation)
make race         # race detector
make lint         # golangci-lint (config in .golangci.yml)
make screenshots  # regenerate docs/screens
make wav          # export the soundtrack
```

Layout:

```text
cmd/neoncoup   CLI                         engine/     rules state machine (pure, seeded)
bot/           three AI levels             protocol/   versioned JSON messages, sanitizing
server/        authoritative rooms         transport/  pipe, WebSocket, relay mux
discovery/     LAN find/answer             relay/      NAT relay
sshd/          SSH front end               config/     settings, stats, replays
client/        Bubble Tea UI (+ art, theme, headless)
audio/         synth, sequencer, songs, SFX
```

Highlights of the test suite:

- every rule and edge case from the brief has its own engine test;
- 10,000 bot-vs-bot games with random seeds, mixed difficulties, player counts and the
  Magistrate variant, asserting coin conservation and card accounting after **every** intent,
  termination, and no panics;
- a WebSocket server plus 3 headless clients playing a full game over the network;
- whole games and the full tutorial driven through real keystrokes in a headless Bubble Tea
  program, with every frame checked at five terminal sizes (and for pure ASCII in `--ascii`
  mode);
- LAN discovery, relay forwarding, SSH sessions, automatic reconnects, and device-free audio tests.

Design notes, the protocol, the rules interpretations and deliberate deviations are in
[PLAN.md](PLAN.md); open items are in [TODO.md](TODO.md).

---

*Coup* was designed by Rikki Tahta. NEON//COUP is an unofficial fan implementation of its rules;
all card art, music and text here are original.
