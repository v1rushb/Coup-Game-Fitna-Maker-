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

---

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [How to play](#how-to-play)
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

### `neoncoup solo [--bots N] [--difficulty easy|normal|hard] [--magistrate] [--window SECONDS]`
A private game against 1–5 bots. Defaults come from your config (3 Fixers, 8-second windows).

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
| `--window S` | 8 | challenge/block window in seconds |
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
`p` pass. Windows run on a timer (8 s by default) and close early once everyone has passed. When
challenged, or when you must lose an influence, press `1` or `2` to pick the card to flip. Flipping
the claimed card proves the claim; flipping any other card concedes, as the rulebook allows.

Other keys: `?` help · `m` mute · `s` CRT scanlines · `t` taunts · `enter` chat ·
`[`/`]` scroll the log · `q` leave.

Key reminders on the table: actions you can back up show in **green**, bluffs in **magenta with
a `~`**. The center terminal's *court intel* counts face-up cards and the ones in your hand. A
character struck through there has every copy accounted for, so any claim of it is a lie.

Rules notes (Assassin refunds, double losses, eliminated players' coins, the Magistrate) are
written up in [PLAN.md](PLAN.md#rules-decisions-and-rulebook-cross-check), including where this
implementation had to interpret the rulebook.

## Multiplayer

- **LAN**: `neoncoup host` answers discovery queries for its room code, so `neoncoup join
  NEON-XXXX` finds it on the same network. Several hosts can run on one machine.
- **Direct**: `neoncoup join HOST:PORT` for any reachable host (forward the port to play over
  the internet without a relay).
- **Relay**: for players behind NAT. The host registers the code with a relay (`--relay`) and
  players reach it through the relay's `/join/CODE`.
- **SSH**: `neoncoup serve-ssh` turns a server into a lobby anyone with `ssh` can use.

**Lobby.** Nicknames (sanitized, deduplicated), ready-up, spectators, and host settings: seats,
bot fill and difficulty, window and turn timers, Magistrate, seat hold.

**Reconnects.** Every player gets a rejoin token, which is also saved to disk. If your link
drops, the client reconnects on its own. If it crashes, run the same `join` command again. Your
seat is held for 60 s (configurable), then a Fixer bot plays it until you come back, at which
point you take it over again.

**Security.** The host is authoritative: clients send intents, and the host validates every one
against the engine and sends each player a view redacted for their seat. Another player's hidden
card never crosses the wire. Mismatched protocol versions are rejected with a clear message.
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
mixer. It plays original compositions:

- a moody menu theme;
- a driving in-game loop built from four sections (hook, climb, breakdown, countermelody) that
  branch randomly so it doesn't get repetitive;
- a tension stinger when a challenge window opens on you, which ducks the music underneath it;
- victory and defeat jingles;
- effects for coins, card flips, challenges, blocks and the elimination glitch.

Audio runs on its own goroutine and the UI never blocks on it. Volume and mute (`m`) persist.
With `--no-audio`, over SSH, or when no audio device is found, the game runs silently and shows
a muted icon. On Linux it talks to PulseAudio/PipeWire directly, with `aplay` as a fallback. On
macOS and Windows it uses oto. `make wav` renders every song and effect to WAV files.

## Configuration

Settings live in the OS config directory (`~/.config/neoncoup` on Linux, `~/Library/Application
Support/neoncoup` on macOS, `%AppData%\neoncoup` on Windows), or `$NEONCOUP_HOME`:

| file | contents |
|------|----------|
| `config.json` | name, volume, mute, CRT, ASCII, relay URL, timers, bot defaults, port, rejoin tokens |
| `stats.json` | games, wins, streaks, bluffs (and how many were caught), challenges (and how many were right), blocks, coins, coups |
| `replays/*.json` | one per finished game: seed, config, seats and every intent. The engine is deterministic, so that reproduces the game exactly. |
| `neoncoup.log` | host/server logs from TUI sessions (kept off the screen) |

`$NEONCOUP_RELAY` sets a default relay. `NO_COLOR` disables color.

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
              │     Reaction window ◀ 8s ▶                                           │
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
              │     Reaction window ◀ 8s ▶                                           │
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
      │ ● CASE               NOT READY             │  │ window       8s                      │
      │ ○ bot #1             FIXER (NORMAL)        │  │ turn timer   60s                     │
      │                                            │  │ magistrate   OFF                     │
      │ 1 spectator(s) watching                    │  │ seat hold    60s                     │
      ╰────────────────────────────────────────────╯  ╰──────────────────────────────────────╯

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

      enter START · +/- seats · f bot fill · d difficulty
      w window · t turn timer · g magistrate · tab chat · q leave
```

</details>

<details open>
<summary><b>Game table, 120x40: an assassination aimed at you, challenge window open</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 9                                               ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │ ▶ GHOSTWIRE               HARD ││ ⠙ NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢1                          ●● ││ ¢2                          ●○ │        │ ── TURN 5 · NULLBYTE ──          │
        │ no cards exposed               ││ ✖ASSASN                        │        │ > [NULLBYTE] BROADCASTS DUKE     │
        │ claims ASSASSIN                ││ thinking...                    │        │   SIGNAL // TAX +3¢              │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ ! [MOTH] CALLS [NULLBYTE]'S      │
                        ╔══════════════════════════════════╗                        │   DUKE A LIE                     │
                        ║ TREASURY 35¢ · DECK 5 · TURN 9   ║                        │ [NULLBYTE] WAS BLUFFING //       │
                        ║ ──────────────────────────────── ║                        │   FLIPS ASSASSIN                 │
                        ║ > GHOSTWIRE → ASSASSINATE YOU    ║                        │ ✖ [NULLBYTE] LOSES INFLUENCE     │
                        ║ (claims ASSASSIN)                ║                        │   // ASSASSIN EXPOSED            │
╭──────────────────────╮║                                  ║╭──────────────────────╮│ NULLBYTE'S TAX IS VOIDED         │
│ ⠙ MOTH        NORMAL │║ CHALLENGE WINDOW OPEN            ║│ ⠙ VEX           HARD ││ ── TURN 6 · VEX ──               │
│ ¢3                ●● │║ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░  5.2s   ║│ ¢2                ●○ ││ > [VEX] SKIMS A CREDIT //        │
│ no cards exposed     │║ waiting on: VEX, YOU, MOTH,      ║│ ✖CONTSA              ││   INCOME +1¢                     │
│ thinking...          │║ NULLBYTE                         ║│ thinking...          ││ ── TURN 7 · YOU ──               │
│                      │║                                  ║│                      ││ > [YOU] FLASHES A CAPTAIN        │
│                      │║ COURT INTEL // face-up cards     ║│                      ││   BADGE AT [MOTH] // STEAL 2¢    │
╰──────────────────────╯║ DUKE    0/3+1  ASSASN  1/3       ║╰──────────────────────╯│ [YOU] LIFTS 2¢ FROM [MOTH]       │
                        ║ CAPTAIN 0/3+1  AMBSDR  0/3       ║                        │ ── TURN 8 · MOTH ──              │
                        ║ CONTSA  1/3                      ║                        │ > [MOTH] BROADCASTS DUKE         │
                        ║ (+n = in your hand)              ║                        │   SIGNAL // TAX +3¢              │
                        ║                                  ║                        │ +3¢ → [MOTH]                     │
                        ║                                  ║                        │ ── TURN 9 · GHOSTWIRE ──         │
                        ╚══════════════════════════════════╝                        │ > [GHOSTWIRE] RUNS ASSASSIN      │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                │   PROTOCOL ON [YOU] // -3¢       │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 7                                           │                                  │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║  coup armed (7)                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║                                                │ [VEX] your firewall is made of … │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  you hold                                      │ // TRINITY lost the uplink. Sea… │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║  DUKE + CAPTAIN                                │ [CASE] nobody believes that Duke │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │ enter: chat · t: taunt           │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 GHOSTWIRE claims ASSASSIN for ASSASSINATE. call the bluff?
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Game table at the minimum size, 100x30</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 9                           ♪✖ no audio  ? help  q leave
 ╭────────────────────────────────╮╭────────────────────────────────╮ ╭────────────────────────────╮
 │ ▶ GHOSTWIRE               HARD ││ ⠙ NULLBYTE                EASY │ │ NETLOG                     │
 │ ¢1                          ●● ││ ¢2                          ●○ │ │ ── TURN 7 · YOU ──         │
 │ no cards exposed               ││ ✖ASSASN                        │ │ > [YOU] FLASHES A          │
 │ claims ASSASSIN                ││ thinking...                    │ │   CAPTAIN BADGE AT [MOTH]  │
 ╰────────────────────────────────╯╰────────────────────────────────╯ │   // STEAL 2¢              │
╭────────────────╮╔════════════════════════════════╗╭────────────────╮│ [YOU] LIFTS 2¢ FROM        │
│ ⠙ MOTH         │║ TREASURY 35¢ · DECK 5 · TURN 9 ║│ ⠙ VEX          ││   [MOTH]                   │
│ ¢3          ●● │║ > GHOSTWIRE → ASSASSINATE YOU  ║│ ¢2          ●○ ││ ── TURN 8 · MOTH ──        │
│ none exposed   │║ (claims ASSASSIN)              ║│ ✖CONTSA        ││ > [MOTH] BROADCASTS DUKE   │
│ thinking...    │║ CHALLENGE WINDOW OPEN          ║│ thinking...    ││   SIGNAL // TAX +3¢        │
│                │║ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░  5.2s   ║│                ││ +3¢ → [MOTH]               │
│                │║ waiting on: VEX, YOU, MOTH,    ║│                ││ ── TURN 9 · GHOSTWIRE ──   │
╰────────────────╯║ NULLBYTE                       ║╰────────────────╯│ > [GHOSTWIRE] RUNS         │
                  ╚════════════════════════════════╝                  │   ASSASSIN PROTOCOL ON     │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                  │   [YOU] // -3¢             │
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
 GHOSTWIRE claims ASSASSIN for ASSASSINATE. call the bluff?
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>You called a bluff: waiting for the claimant to flip a card</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13                                              ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢1                          ●● ││ ¢3                          ●○ │        │ ── TURN 8 · MOTH ──              │
        │ no cards exposed               ││ ✖ASSASN                        │        │ > [MOTH] BROADCASTS DUKE         │
        │                                ││                                │        │   SIGNAL // TAX +3¢              │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ +3¢ → [MOTH]                     │
                        ╔══════════════════════════════════╗                        │ ── TURN 9 · GHOSTWIRE ──         │
                        ║ TREASURY 33¢ · DECK 5 · TURN 13  ║                        │ > [GHOSTWIRE] RUNS ASSASSIN      │
                        ║ ──────────────────────────────── ║                        │   PROTOCOL ON [YOU] // -3¢       │
                        ║ > MOTH → TAX (claims DUKE)       ║                        │ ✖ [YOU] LOSES INFLUENCE //       │
                        ║ YOU CHALLENGES MOTH's DUKE       ║                        │   CAPTAIN EXPOSED                │
╭──────────────────────╮║                                  ║╭──────────────────────╮│ ── TURN 10 · NULLBYTE ──         │
│ ⠙ MOTH        NORMAL │║ PROVE IT OR LOSE IT              ║│   VEX           HARD ││ > [NULLBYTE] SKIMS A CREDIT //   │
│ ¢3                ●● │║ waiting on: MOTH                 ║│ ¢3                ●○ ││   INCOME +1¢                     │
│ no cards exposed     │║                                  ║│ ✖CONTSA              ││ ── TURN 11 · VEX ──              │
│ CHALLENGED! proving. │║ COURT INTEL // face-up cards     ║│                      ││ > [VEX] SKIMS A CREDIT //        │
│                      │║ DUKE    0/3+1  ASSASN  1/3       ║│                      ││   INCOME +1¢                     │
│                      │║ CAPTAIN 1/3    AMBSDR  0/3       ║│                      ││ ── TURN 12 · YOU ──              │
╰──────────────────────╯║ CONTSA  1/3                      ║╰──────────────────────╯│ > [YOU] OPENS AN AMBASSADOR      │
                        ║ (+n = in your hand)              ║                        │   CHANNEL // EXCHANGE            │
                        ║                                  ║                        │ (you draw DUKE + ASSASSIN)       │
                        ║                                  ║                        │ [YOU] RETURNS 2 CARDS            │
                        ║                                  ║                        │ ── TURN 13 · MOTH ──             │
                        ║                                  ║                        │ > [MOTH] BROADCASTS DUKE         │
                        ╚══════════════════════════════════╝                        │   SIGNAL // TAX +3¢              │
╔════════════════╗┌────────────────┐  RUNNER  seat 2                                │ ! [YOU] CALLS [MOTH]'S DUKE A    │
║ DUKE          ◆║│ CAPTAIN       ▲│  ¢ 7                                           │   LIE                            │
║     ▄▄▄▄▄▄     ║│═══▄▄   ▄▄▄▄▄▄  │  influence ●○                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║│  ▐██▌ █▀▀▀▀▀▀█ │  coup armed (7)                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║│   ▀█▄▐▓▓▓▓▓▓▓▌ │                                                │ [VEX] your firewall is made of … │
║   █   ▄▄   █ ║ ║│     ▀█▄▄▄▄▄▄▄█ │  you hold                                      │ // TRINITY lost the uplink. Sea… │
║   ▀█▄═══▄▄█▀ ║ ║│  ▄▓▓▓█▓▓▓▓▓▓▓█▄│  DUKE                                          │ [CASE] nobody believes that Duke │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║│  █▓▓▓█▓▓┃┃▓▓▓██│                                                │ enter: chat · t: taunt           │
║────────────────║│────────────────│                                                │                                  │
║ TAX +3¢        ║│ ✖ FLATLINED    │                                                │                                  │
║ BLOCKS FOR.AID ║│                │                                                │                                  │
╚════════════════╝└────────────────┘                                                ╰──────────────────────────────────╯
 waiting on MOTH ⠙
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Losing an influence: pick the card to flip</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 9                                               ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │ ▶ GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢1                          ●● ││ ¢2                          ●○ │        │ ── TURN 5 · NULLBYTE ──          │
        │ no cards exposed               ││ ✖ASSASN                        │        │ > [NULLBYTE] BROADCASTS DUKE     │
        │ claims ASSASSIN                ││                                │        │   SIGNAL // TAX +3¢              │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ ! [MOTH] CALLS [NULLBYTE]'S      │
                        ╔══════════════════════════════════╗                        │   DUKE A LIE                     │
                        ║ TREASURY 35¢ · DECK 5 · TURN 9   ║                        │ [NULLBYTE] WAS BLUFFING //       │
                        ║ ──────────────────────────────── ║                        │   FLIPS ASSASSIN                 │
                        ║ > GHOSTWIRE → ASSASSINATE YOU    ║                        │ ✖ [NULLBYTE] LOSES INFLUENCE     │
                        ║ (claims ASSASSIN)                ║                        │   // ASSASSIN EXPOSED            │
╭──────────────────────╮║                                  ║╭──────────────────────╮│ NULLBYTE'S TAX IS VOIDED         │
│   MOTH        NORMAL │║ YOU LOSES AN INFLUENCE           ║│   VEX           HARD ││ ── TURN 6 · VEX ──               │
│ ¢3                ●● │║ waiting on: YOU                  ║│ ¢2                ●○ ││ > [VEX] SKIMS A CREDIT //        │
│ no cards exposed     │║                                  ║│ ✖CONTSA              ││   INCOME +1¢                     │
│                      │║ COURT INTEL // face-up cards     ║│                      ││ ── TURN 7 · YOU ──               │
│                      │║ DUKE    0/3+1  ASSASN  1/3       ║│                      ││ > [YOU] FLASHES A CAPTAIN        │
│                      │║ CAPTAIN 0/3+1  AMBSDR  0/3       ║│                      ││   BADGE AT [MOTH] // STEAL 2¢    │
╰──────────────────────╯║ CONTSA  1/3                      ║╰──────────────────────╯│ [YOU] LIFTS 2¢ FROM [MOTH]       │
                        ║ (+n = in your hand)              ║                        │ ── TURN 8 · MOTH ──              │
                        ║                                  ║                        │ > [MOTH] BROADCASTS DUKE         │
                        ║                                  ║                        │   SIGNAL // TAX +3¢              │
                        ║                                  ║                        │ +3¢ → [MOTH]                     │
                        ║                                  ║                        │ ── TURN 9 · GHOSTWIRE ──         │
                        ╚══════════════════════════════════╝                        │ > [GHOSTWIRE] RUNS ASSASSIN      │
╔[1]═════════════╗╔[2]═════════════╗  RUNNER  seat 2                                │   PROTOCOL ON [YOU] // -3¢       │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 7                                           │                                  │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║  coup armed (7)                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║                                                │ [VEX] your firewall is made of … │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  you hold                                      │ // TRINITY lost the uplink. Sea… │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║  DUKE + CAPTAIN                                │ [CASE] nobody believes that Duke │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │ enter: chat · t: taunt           │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 LOSE AN INFLUENCE: pick the card to flip
 [1] DUKE  [2] CAPTAIN
```

</details>

<details>
<summary><b>Ambassador exchange: pick what to keep</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 12                                              ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢1                          ●● ││ ¢3                          ●○ │        │ > [VEX] SKIMS A CREDIT //        │
        │ no cards exposed               ││ ✖ASSASN                        │        │   INCOME +1¢                     │
        │                                ││                                │        │ ── TURN 7 · YOU ──               │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ > [YOU] FLASHES A CAPTAIN        │
                        ╔══════════════════════════════════╗                        │   BADGE AT [MOTH] // STEAL 2¢    │
                        ║ TREASURY 33¢ · DECK 3 · TURN 12  ║                        │ [YOU] LIFTS 2¢ FROM [MOTH]       │
                        ║ ──────────────────────────────── ║                        │ ── TURN 8 · MOTH ──              │
                        ║ > YOU → EXCHANGE (claims         ║                        │ > [MOTH] BROADCASTS DUKE         │
                        ║ AMBASSADOR)                      ║                        │   SIGNAL // TAX +3¢              │
╭──────────────────────╮║                                  ║╭──────────────────────╮│ +3¢ → [MOTH]                     │
│   MOTH        NORMAL │║ YOU IS SWAPPING CARDS            ║│   VEX           HARD ││ ── TURN 9 · GHOSTWIRE ──         │
│ ¢3                ●● │║ waiting on: YOU                  ║│ ¢3                ●○ ││ > [GHOSTWIRE] RUNS ASSASSIN      │
│ no cards exposed     │║                                  ║│ ✖CONTSA              ││   PROTOCOL ON [YOU] // -3¢       │
│                      │║ COURT INTEL // face-up cards     ║│                      ││ ✖ [YOU] LOSES INFLUENCE //       │
│                      │║ DUKE    0/3+1  ASSASN  1/3       ║│                      ││   CAPTAIN EXPOSED                │
│                      │║ CAPTAIN 1/3    AMBSDR  0/3       ║│                      ││ ── TURN 10 · NULLBYTE ──         │
╰──────────────────────╯║ CONTSA  1/3                      ║╰──────────────────────╯│ > [NULLBYTE] SKIMS A CREDIT //   │
                        ║ (+n = in your hand)              ║                        │   INCOME +1¢                     │
                        ║                                  ║                        │ ── TURN 11 · VEX ──              │
                        ║                                  ║                        │ > [VEX] SKIMS A CREDIT //        │
                        ║                                  ║                        │   INCOME +1¢                     │
                        ║                                  ║                        │ ── TURN 12 · YOU ──              │
                        ╚══════════════════════════════════╝                        │ > [YOU] OPENS AN AMBASSADOR      │
╔[1]═════════════╗╔[2]═════════════╗╔[3]═════════════╗  EXCHANGE                    │   CHANNEL // EXCHANGE            │
║ DUKE          ◆║║ DUKE          ◆║║ ASSASSIN      ╳║  keep 1 of 3                 │ (you draw DUKE + ASSASSIN)       │
║     ▄▄▄▄▄▄     ║║     ▄▄▄▄▄▄     ║║      ▄▄▄▄     ╱║  selected 1/1                ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║   ▄█▀▀▀▀▀▀█▄   ║║    ▄█▓▓▓▓█▄  ╱ ║                              ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   █  ▄▄ (◉)█═╗ ║║   █▓▀▀▀▀▀▀▓█╱  ║  1-3 toggle                  │ [VEX] your firewall is made of … │
║   █   ▄▄   █ ║ ║║   █   ▄▄   █ ║ ║║   █ ══════ ▐▌  ║  enter confirm               │ // TRINITY lost the uplink. Sea… │
║   ▀█▄═══▄▄█▀ ║ ║║   ▀█▄═══▄▄█▀ ║ ║║   ▀█▄    ▄█▀▌  ║                              │ [CASE] nobody believes that Duke │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  ▄▓▓▓▓▓▓▓▓▓▓▄  ║                              │ enter: chat · t: taunt           │
║────────────────║║────────────────║║────────────────║                              │                                  │
║ TAX +3¢        ║║ TAX +3¢        ║║ PAY 3¢: STRIKE ║                              │                                  │
║ ✓ KEEP         ║║ BLOCKS FOR.AID ║║ -1 INFLUENCE   ║                              │                                  │
╚════════════════╝╚════════════════╝╚════════════════╝                              ╰──────────────────────────────────╯
 EXCHANGE: keep 1. selected: DUKE
 1-3 toggle · enter confirm · esc clear
```

</details>

<details>
<summary><b>Your elimination (the screen tears apart, in color)</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13                                              ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢4                          ●● ││ ¢4                          ●● │        │ ── TURN 5 · NULLBYTE ──          │
        │ no cards exposed               ││ no cards exposed               │        │ > [NULLBYTE] SKIMS A CREDIT //   │
        │                                ││                                │        │   INCOME +1¢                     │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ ── TURN 6 · VEX ──               │
                        ╔══════════════════════════════════╗                        │ > [VEX] SKIMS A CREDIT //        │
                        ║ TREASURY 28¢ · DECK 5 · TURN 13  ║                        │   INCOME +1¢                     │
                        ║ ──────────────────────────────── ║                        │ ── TURN 7 · YOU ──               │
                        ║ > MOTH is choosing a move        ║                        │ > [YOU] SKIMS A CREDIT //        │
                        ║                                  ║                        │   INCOME +1¢                     │
╭──────────────────────╮║ waiting on: MOTH                 ║╭──────────────────────╮│ ── TURN 8 · MOTH ──              │
│ ⠋ MOTH        NORMAL │║                                  ║│   VEX           HARD ││ > [MOTH] SKIMS A CREDIT //       │
│ ¢4                ●● │║ COURT INTEL // face-up cards     ║│ ¢5                ●● ││   INCOME +1¢                     │
│ no cards exposed     │║ DUKE    0/3+1  ASSASN  0/3       ║│ no cards exposed     ││ ── TURN 9 · GHOSTWIRE ──         │
│ thinking...          │║ CAPTAIN 0/3+1  AMBSDR  0/3       ║│                      ││ > [GHOSTWIRE] SKIMS A CREDIT     │
│                      │║ CONTSA  0/3                      ║│                      ││   // INCOME +1¢                  │
│                      │║ (+n = in your hand)                                      ││ ── TURN 10 · NULLBYTE ──         │
╰──────────────────────╯║                          F L A T L I * E $      ─────────╯│ > [NULLBYTE] SKIMS A CREDIT //   │
                        ║                                                           │   INCOME +1¢                     │
                        ║                                  ║                        │ ── TURN 11 · VEX ──              │
                        ║                                  ║                        │ > [VEX] SKIMS A CREDIT //        │
                        ║                                  ║                        │   INCOME +1¢                     │
                        ║                                  ║                        │ ── TURN 12 · YOU ──              │
                        ╚══════════════════════════════════╝                        │ > [YOU] SKIMS A CREDIT //        │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                │   INCOME +1¢                     │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 5                                           │ ── TURN 13 · MOTH ──             │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
 ║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                ╭──────────────────────────────────
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                      │                                  │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                │ enter: chat · t: taunt           │
   ║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │                                  │
║────────────────║║────────────────║                                                │                                  │
   ║ TAX +3¢        ║║ STEAL 2¢       ║                                                │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 waiting on MOTH ⠋
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
        │   enter   chat    t  taunt menu                                                 │
        │   m       mute    s  CRT scanlines                                              │
        │   esc     cancel a choice    q  leave                                           │
        │   ?       this help                                                             │
        │                                                                                 │
        │  RULES IN ONE BREATH                                                            │
        │  Everyone has two hidden cards (influence) and coins. On your turn take         │
        │  one action. Any character action may be a bluff: anyone can challenge.         │
        │  A failed challenge costs the challenger a card; a caught bluff costs           │
        │  the bluffer a card. 7 coins buys a Coup; at 10 you must Coup. Lose             │
        │  both cards and you're out. Last runner standing wins.                          │
        │                                                                                 │
        │  any key to close                                                               │
        │                                                                                 │
        ╰─────────────────────────────────────────────────────────────────────────────────╯
```

</details>

<details>
<summary><b>Victory</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13                                              ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢4                          ●● ││ ¢4                          ●● │        │ ── TURN 5 · NULLBYTE ──          │
        │ no cards exposed               ││ no cards exposed               │        │ > [NULLBYTE] SKIMS A CREDIT //   │
        │                                ││                                │        │   INCOME +1¢                     │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ ── TURN 6 · VEX ──               │
                        ╔══════════════════════════════════╗                        │ > [VEX] SKIMS A CREDIT //        │
                        ║ TREASURY 28¢ · DECK 5 · TURN 13  ║                        │   INCOME +1¢                     │
                        ║ ──────────────────────────────── ║                        │ ── TURN 7 · YOU ──               │
                        ║ > MOTH is choosing a move        ║                        │ > [YOU] SKIMS A CREDIT //        │
                        ║                                  ║                        │   INCOME +1¢                     │
╭──────────────────────╮║ waiting on: MOTH                 ║╭──────────────────────╮│ ── TURN 8 · MOTH ──              │
│ ⠋ MOTH        NORMAL │║                                  ║│   VEX           HARD ││ > [MOTH] SKIMS A CREDIT //       │
│ ¢4                ●●╔══════════════════════════════════════════════════════════════════════════╗                     │
│ no cards exposed    ║                                                                          ║GHOSTWIRE ──         │
│ thinking...         ║       SYSTEM SEIZED                                                      ║] SKIMS A CREDIT     │
│                     ║                                                                          ║+1¢                  │
│                     ║    winner: RUNNER   turns: 13                                            ║ NULLBYTE ──         │
╰─────────────────────║    your bluffs: 3 (1 caught)   challenges: 2 (2 won)                     ║ SKIMS A CREDIT //   │
                      ║    blocks: 1   coins earned: 17   coups: 2                               ║                     │
                      ║    replay saved: ~/.config/neoncoup/replays/20870101-000000-7K2Q.json    ║ VEX ──              │
                      ║                                                                          ║S A CREDIT //        │
                      ║    [r] rematch  [v] replay  [enter] lobby  [q] leave                     ║                     │
                      ║                                                                          ║ YOU ──              │
                      ╚══════════════════════════════════════════════════════════════════════════╝S A CREDIT //        │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                │   INCOME +1¢                     │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 5                                           │ ── TURN 13 · MOTH ──             │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                      │                                  │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                │ enter: chat · t: taunt           │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                │                                  │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │                                  │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 waiting on MOTH ⠋
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Defeat</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13                                              ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢4                          ●● ││ ¢4                          ●● │        │ ── TURN 5 · NULLBYTE ──          │
        │ no cards exposed               ││ no cards exposed               │        │ > [NULLBYTE] SKIMS A CREDIT //   │
        │                                ││                                │        │   INCOME +1¢                     │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ ── TURN 6 · VEX ──               │
                        ╔══════════════════════════════════╗                        │ > [VEX] SKIMS A CREDIT //        │
                        ║ TREASURY 28¢ · DECK 5 · TURN 13  ║                        │   INCOME +1¢                     │
                        ║ ──────────────────────────────── ║                        │ ── TURN 7 · YOU ──               │
                        ║ > MOTH is choosing a move        ║                        │ > [YOU] SKIMS A CREDIT //        │
                        ║                                  ║                        │   INCOME +1¢                     │
╭──────────────────────╮║ waiting on: MOTH                 ║╭──────────────────────╮│ ── TURN 8 · MOTH ──              │
│ ⠋ MOTH        NORMAL │║                                  ║│   VEX           HARD ││ > [MOTH] SKIMS A CREDIT //       │
│ ¢4                ●●╔══════════════════════════════════════════════════════════════════════════╗                     │
│ no cards exposed    ║                                                                          ║GHOSTWIRE ──         │
│ thinking...         ║       CONNECTION TERMINATED                                              ║] SKIMS A CREDIT     │
│                     ║                                                                          ║+1¢                  │
│                     ║    winner: GHOSTWIRE   turns: 13                                         ║ NULLBYTE ──         │
╰─────────────────────║    your bluffs: 3 (1 caught)   challenges: 2 (2 won)                     ║ SKIMS A CREDIT //   │
                      ║    blocks: 1   coins earned: 17   coups: 2                               ║                     │
                      ║    replay saved: ~/.config/neoncoup/replays/20870101-000000-7K2Q.json    ║ VEX ──              │
                      ║                                                                          ║S A CREDIT //        │
                      ║    [r] rematch  [v] replay  [enter] lobby  [q] leave                     ║                     │
                      ║                                                                          ║ YOU ──              │
                      ╚══════════════════════════════════════════════════════════════════════════╝S A CREDIT //        │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                │   INCOME +1¢                     │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 5                                           │ ── TURN 13 · MOTH ──             │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                      │                                  │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                │ enter: chat · t: taunt           │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                │                                  │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │                                  │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 waiting on MOTH ⠋
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
               │   last run: 2026-10-26 14:36                                       │
               │                                                                    │
               │   esc back                                                         │
               │                                                                    │
               ╰────────────────────────────────────────────────────────────────────╯
```

</details>

<details>
<summary><b>Taunt menu (t)</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 13                                              ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │   GHOSTWIRE               HARD ││   NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢4                          ●● ││ ¢4                          ●● │        │ ── TURN 5 · NULLBYTE ──          │
        │ no cards exposed               ││ no cards exposed               │        │ > [NULLBYTE] SKIMS A CREDIT //   │
        │                                ││                                │        │   INCOME +1¢                     │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ ── TURN 6 · VEX ──               │
                        ╔══════════════════════════════════╗                        │ > [VEX] SKIMS A CREDIT //        │
                        ║ TREASURY 28¢ · DECK 5 · TURN 13  ║                        │   INCOME +1¢                     │
                        ║ ──────────────────────────────── ║                        │ ── TURN 7 · YOU ──               │
                        ║ > MOTH is choosing a move        ║                        │ > [YOU] SKIMS A CREDIT //        │
                        ║                                  ║                        │   INCOME +1¢                     │
╭──────────────────────╮║ waiting on: MOT╭───────────────────────────────────╮─────╮│ ── TURN 8 · MOTH ──              │
│ ⠋ MOTH        NORMAL │║                │                                   │HARD ││ > [MOTH] SKIMS A CREDIT //       │
│ ¢4                ●● │║ COURT INTEL // │  TAUNT                            │  ●● ││   INCOME +1¢                     │
│ no cards exposed     │║ DUKE    0/3+1  │                                   │     ││ ── TURN 9 · GHOSTWIRE ──         │
│ thinking...          │║ CAPTAIN 0/3+1  │  [1] gg, choom                    │     ││ > [GHOSTWIRE] SKIMS A CREDIT     │
│                      │║ CONTSA  0/3    │  [2] nice bluff... or was it?     │     ││   // INCOME +1¢                  │
│                      │║ (+n = in your h│  [3] i have the Duke. obviously.  │     ││ ── TURN 10 · NULLBYTE ──         │
╰──────────────────────╯║                │  [4] your firewall is showing     │─────╯│ > [NULLBYTE] SKIMS A CREDIT //   │
                        ║                │  [5] say goodbye to your credits  │      │   INCOME +1¢                     │
                        ║                │  [6] challenge me. i dare you.    │      │ ── TURN 11 · VEX ──              │
                        ║                │  [7] pure static                  │      │ > [VEX] SKIMS A CREDIT //        │
                        ║                │  [8] see you in the next run      │      │   INCOME +1¢                     │
                        ║                │                                   │      │ ── TURN 12 · YOU ──              │
                        ╚════════════════│  any other key to close           │      │ > [YOU] SKIMS A CREDIT //        │
╔════════════════╗╔════════════════╗  RUN│                                   │      │   INCOME +1¢                     │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 5╰───────────────────────────────────╯      │ ── TURN 13 · MOTH ──             │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                      │                                  │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                │ enter: chat · t: taunt           │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                │                                  │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │                                  │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 waiting on MOTH ⠋
 [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>--ascii mode</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 3                                               ~x no audio  ? help  q leave
        +--------------------------------++--------------------------------+        +----------------------------------+
        | . GHOSTWIRE               HARD || . NULLBYTE                EASY |        | NETLOG                           |
        | c2                          oo || c2                          oo |        | // GAME 3 // 5 RUNNERS //        |
        | no cards exposed               || no cards exposed               |        |   JACKED IN                      |
        | thinking...                    || thinking...                    |        | -- TURN 1 . VEX --               |
        +--------------------------------++--------------------------------+        | > [VEX] SKIMS A CREDIT //        |
                        +----------------------------------+                        |   INCOME +1c                     |
                        | TREASURY 36c . DECK 5 . TURN 3   |                        | -- TURN 2 . YOU --               |
                        | -------------------------------- |                        | > [YOU] BROADCASTS DUKE SIGNAL   |
                        | > MOTH -> STEAL YOU (claims      |                        |   // TAX +3c                     |
                        | CAPTAIN)                         |                        | +3c > [YOU]                      |
+----------------------+|                                  |+----------------------+| -- TURN 3 . MOTH --              |
| > MOTH        NORMAL || CHALLENGE WINDOW OPEN            || . VEX           HARD || > [MOTH] FLASHES A CAPTAIN       |
| c2                oo || ################........  5.2s   || c3                oo ||   BADGE AT [YOU] // STEAL 2c     |
| no cards exposed     || waiting on: VEX, YOU, GHOSTWIRE, || no cards exposed     ||                                  |
| claims CAPTAIN       || NULLBYTE                         || thinking...          ||                                  |
|                      ||                                  ||                      ||                                  |
|                      || COURT INTEL // face-up cards     ||                      ||                                  |
+----------------------+| DUKE    0/3+1  ASSASN  0/3       |+----------------------+|                                  |
                        | CAPTAIN 0/3+1  AMBSDR  0/3       |                        |                                  |
                        | CONTSA  0/3                      |                        |                                  |
                        | (+n = in your hand)              |                        |                                  |
                        |                                  |                        |                                  |
                        |                                  |                        |                                  |
                        +----------------------------------+                        |                                  |
+----------------++----------------+  RUNNER  seat 2                                |                                  |
| DUKE          *|| CAPTAIN       ^|  c 5                                           |                                  |
|     ______     ||===__   ______  |  influence oo                                  +----------------------------------+
|   _#""""""#_   ||  ]##[ #""""""# |                                                +----------------------------------+
|   #  __ (@)#=+ ||   "#_]%%%%%%%[ |  you hold                                      |                                  |
|   #   __   # | ||     "#_______# |  DUKE + CAPTAIN                                | enter: chat . t: taunt           |
|   "#_===__#" | ||  _%%%#%%%%%%%#_|                                                |                                  |
|  _%%%#[c]#%%%_ ||  #%%%#%%||%%%##|                                                |                                  |
|----------------||----------------|                                                |                                  |
| TAX +3c        || STEAL 2c       |                                                |                                  |
| BLOCKS FOR.AID || BLOCKS STEAL   |                                                |                                  |
+----------------++----------------+                                                +----------------------------------+
 MOTH claims CAPTAIN for STEAL. call the bluff?
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
 REPLAY  2087-01-01 02:00  step 48/72  turn 10                                                      paused
TREASURY 27¢ · DECK 5                                                         ╭────────────────────────────────────────╮
> VEX: passes                                                                 │ NETLOG                                 │
  on the table: NULLBYTE → TAX (claims DUKE)                                  │   +3¢ → [RUNNER]                       │
                                                                              │ ── TURN 3 · MOTH ──                    │
VEX               ┌──────────┐┌──────────┐                                    │ > [MOTH] BROADCASTS DUKE SIGNAL // TA… │
¢0                │ ASSASSIN╳││ CONTESSA✧│                                    │   +3¢ → [MOTH]                         │
                  │  ▄█▓▓█▄ ╱││   ▐◆  ◆▌ │                                    │ ── TURN 4 · GHOSTWIRE ──               │
                  │  █ ════▐▌││ ▐░▒▓░▒▓░▌│                                    │ > [GHOSTWIRE] FLASHES A CAPTAIN BADGE… │
                  │KILL 3¢   ││BLK KILL  │                                    │ ! [RUNNER] CALLS [GHOSTWIRE]'S CAPTAI… │
                  │-1 INFL   ││          │                                    │   [GHOSTWIRE] SHOWS CAPTAIN // CLAIM … │
                  └──────────┘└──────────┘                                    │   [GHOSTWIRE] RESHUFFLES AND DRAWS A … │
RUNNER            ┌──────────┐┌──────────┐                                    │ ✖ [RUNNER] LOSES INFLUENCE // CAPTAIN… │
¢6                │ DUKE    ◆││ CAPTAIN ▲│                                    │   [GHOSTWIRE] LIFTS 2¢ FROM [RUNNER]   │
                  │ ▄█▀▀(◉)═╗││══▄ █▀▀▀▀█│                                    │ ── TURN 5 · NULLBYTE ──                │
                  │ ▓▓█[¢]█▓▓││ ▐█▌▐▓▓▓▓▌│                                    │ > [NULLBYTE] OPENS AN AMBASSADOR CHAN… │
                  │TAX +3¢   ││✖ LOST    │                                    │   [NULLBYTE] PULLS 2 CARDS FROM THE C… │
                  │BLK AID   ││          │                                    │   [NULLBYTE] RETURNS 2 CARDS           │
                  └──────────┘└──────────┘                                    │ ── TURN 6 · VEX ──                     │
MOTH              ┌──────────┐┌──────────┐                                    │ > [VEX] RUNS ASSASSIN PROTOCOL ON [MO… │
¢8                │ AMBASSAD◎││ ASSASSIN╳│                                    │ ✖ [MOTH] LOSES INFLUENCE // AMBASSADO… │
                  │  ░▒▓▀▀▓▒░││  ▄█▓▓█▄ ╱│                                    │ ── TURN 7 · RUNNER ──                  │
                  │  ▐═[▓▓]═▌││  █ ════▐▌│                                    │ > [RUNNER] BROADCASTS DUKE SIGNAL // … │
                  │✖ LOST    ││KILL 3¢   │                                    │   +3¢ → [RUNNER]                       │
                  │          ││-1 INFL   │                                    │ ── TURN 8 · MOTH ──                    │
                  └──────────┘└──────────┘                                    │ > [MOTH] BROADCASTS DUKE SIGNAL // TA… │
GHOSTWIRE         ┌──────────┐┌──────────┐                                    │ ! [NULLBYTE] CALLS [MOTH]'S DUKE A LIE │
¢7                │ CONTESSA✧││ CONTESSA✧│                                    │   [MOTH] SHOWS DUKE // CLAIM VERIFIED  │
                  │   ▐◆  ◆▌ ││   ▐◆  ◆▌ │                                    │   [MOTH] RESHUFFLES AND DRAWS A FRESH… │
                  │ ▐░▒▓░▒▓░▌││ ▐░▒▓░▒▓░▌│                                    │ ✖ [NULLBYTE] LOSES INFLUENCE // CAPTA… │
                  │BLK KILL  ││BLK KILL  │                                    │   +3¢ → [MOTH]                         │
                  │          ││          │                                    │ ── TURN 9 · GHOSTWIRE ──               │
                  └──────────┘└──────────┘                                    │ > [GHOSTWIRE] BROADCASTS DUKE SIGNAL … │
▶ NULLBYTE        ┌──────────┐┌──────────┐                                    │   +3¢ → [GHOSTWIRE]                    │
¢2                │ CAPTAIN ▲││ DUKE    ◆│                                    │ ── TURN 10 · NULLBYTE ──               │
                  │══▄ █▀▀▀▀█││ ▄█▀▀(◉)═╗│                                    │ > [NULLBYTE] BROADCASTS DUKE SIGNAL /… │
                  │ ▐█▌▐▓▓▓▓▌││ ▓▓█[¢]█▓▓│                                    ╰────────────────────────────────────────╯
←/→ step · ↑/↓ ±10 · space play/pause · +/- speed · home/end · esc back
```

</details>

<details>
<summary><b>Tutorial intro</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 1                                               ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │ ⠋ GHOSTWIRE               HARD ││ ⠋ NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢2                          ●● ││ ¢2                          ●● │        │ // GAME 3 // 5 RUNNERS //        │
        │ no cards exposed               ││ no cards exposed               │        │   JACKED IN                      │
        │ thinking...                    ││ thinking...                    │        │ ── TURN 1 · VEX ──               │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ > [VEX] BROADCASTS DUKE SIGNAL   │
                        ╔══════════════════════════════════╗                        │   // TAX +3¢                     │
                        ║ TREASURY 40¢ · DECK 5 · TURN 1   ║                        │                                  │
                        ║ ──────────────────────────────── ║                        │                                  │
                        ║ > VEX → TAX (claims DUKE)        ║                        │                                  │
                        ║                                  ║                        │                                  │
╭──────────────────╔════════════════════════════════════════════════════════════════════════════════╗                  │
│ ⠋ MOTH        NOR║                                                                                ║                  │
│ ¢2               ║   WELCOME TO THE CON                                                           ║                  │
│ no cards exposed ║                                                                                ║                  │
│ thinking...      ║   Every runner holds 2 face-down cards: their INFLUENCE.                       ║                  │
│                  ║   Lose both and you're out. Last runner standing takes the city.               ║                  │
│                  ║                                                                                ║                  │
╰──────────────────║   On your turn you take one action. Some actions belong to a character:        ║                  │
                   ║   you may claim ANY character, whether you hold it or not.                     ║                  │
                   ║   Others can CHALLENGE the claim, or BLOCK it with a character of their own.   ║                  │
                   ║                                                                                ║                  │
                   ║   Your cards are at the bottom. The log is on the right.                       ║                  │
                   ║                                                                                ║                  │
                   ║   press enter to start                                                         ║                  │
╔════════════════╗╔║                                                                                ║                  │
║ DUKE          ◆║║╚════════════════════════════════════════════════════════════════════════════════╝                  │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                      │                                  │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                │ enter: chat · t: taunt           │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                │                                  │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │                                  │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 VEX claims DUKE for TAX. call the bluff?
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
```

</details>

<details>
<summary><b>Tutorial coach</b></summary>

```text
 NEON//COUP   ROOM NEON-7K2Q  GAME 3  TURN 1                                               ♪✖ no audio  ? help  q leave
        ╭────────────────────────────────╮╭────────────────────────────────╮        ╭──────────────────────────────────╮
        │ ⠋ GHOSTWIRE               HARD ││ ⠋ NULLBYTE                EASY │        │ NETLOG                           │
        │ ¢2                          ●● ││ ¢2                          ●● │        │ // GAME 3 // 5 RUNNERS //        │
        │ no cards exposed               ││ no cards exposed               │        │   JACKED IN                      │
        │ thinking...                    ││ thinking...                    │        │ ── TURN 1 · VEX ──               │
        ╰────────────────────────────────╯╰────────────────────────────────╯        │ > [VEX] BROADCASTS DUKE SIGNAL   │
  ╭────────────────────────────────────────────────────────────────────────────╮    │   // TAX +3¢                     │
  │  COACH // SMELL THE BLUFF                                                  │    │                                  │
  │  VEX claims DUKE. MOTH just showed confidence in a Duke and you hold one.  │    │                                  │
  │  VEX never acted like a Duke before. Call it: press c to CHALLENGE.        │    │                                  │
  ╰────────────────────────────────────────────────────────────────────────────╯    │                                  │
╭──────────────────────╮║ CHALLENGE WINDOW OPEN            ║╭──────────────────────╮│                                  │
│ ⠋ MOTH        NORMAL │║ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░░░  5.2s   ║│ ▶ VEX           HARD ││                                  │
│ ¢2                ●● │║ waiting on: YOU, MOTH,           ║│ ¢2                ●● ││                                  │
│ no cards exposed     │║ GHOSTWIRE, NULLBYTE              ║│ no cards exposed     ││                                  │
│ thinking...          │║                                  ║│ claims DUKE          ││                                  │
│                      │║ COURT INTEL // face-up cards     ║│                      ││                                  │
│                      │║ DUKE    0/3+1  ASSASN  0/3       ║│                      ││                                  │
╰──────────────────────╯║ CAPTAIN 0/3+1  AMBSDR  0/3       ║╰──────────────────────╯│                                  │
                        ║ CONTSA  0/3                      ║                        │                                  │
                        ║ (+n = in your hand)              ║                        │                                  │
                        ║                                  ║                        │                                  │
                        ║                                  ║                        │                                  │
                        ║                                  ║                        │                                  │
                        ╚══════════════════════════════════╝                        │                                  │
╔════════════════╗╔════════════════╗  RUNNER  seat 2                                │                                  │
║ DUKE          ◆║║ CAPTAIN       ▲║  ¢ 2                                           │                                  │
║     ▄▄▄▄▄▄     ║║═══▄▄   ▄▄▄▄▄▄  ║  influence ●●                                  ╰──────────────────────────────────╯
║   ▄█▀▀▀▀▀▀█▄   ║║  ▐██▌ █▀▀▀▀▀▀█ ║                                                ╭──────────────────────────────────╮
║   █  ▄▄ (◉)█═╗ ║║   ▀█▄▐▓▓▓▓▓▓▓▌ ║  you hold                                      │                                  │
║   █   ▄▄   █ ║ ║║     ▀█▄▄▄▄▄▄▄█ ║  DUKE + CAPTAIN                                │ enter: chat · t: taunt           │
║   ▀█▄═══▄▄█▀ ║ ║║  ▄▓▓▓█▓▓▓▓▓▓▓█▄║                                                │                                  │
║  ▄▓▓▓█[¢]█▓▓▓▄ ║║  █▓▓▓█▓▓┃┃▓▓▓██║                                                │                                  │
║────────────────║║────────────────║                                                │                                  │
║ TAX +3¢        ║║ STEAL 2¢       ║                                                │                                  │
║ BLOCKS FOR.AID ║║ BLOCKS STEAL   ║                                                │                                  │
╚════════════════╝╚════════════════╝                                                ╰──────────────────────────────────╯
 VEX claims DUKE for TAX. call the bluff?
 [c]CHALLENGE  [p]PASS  [t]TAUNT  [?]HELP
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
