# TODO

What is deliberately not done (or only partly done). Nothing here is silently stubbed in the code.

## Rules
- **Official 2-player draft setup** (each player gets one of each character, keeps one, and is
  dealt a second from a third set; the court deck is 3 cards), as an opt-in variant. The default
  2-player game uses the normal deal plus the 1-coin rule. See PLAN.md, "Rules decisions".

## Multiplayer
- **No public relay is operated.** Players behind NAT need someone to run `neoncoup relay` on a
  VPS (Dockerfile and systemd unit are in the repo) and pass `--relay wss://...` or set it in
  settings. There is deliberately no hard-coded default relay URL.
- **LAN discovery is IPv4 only** (multicast 239.255.77.77 plus broadcast). IPv6-only LANs must
  join by address.
- **The hosting process is the server.** When the player who ran `neoncoup host` quits, the
  room ends for everyone. Inside a room, the lobby host role migrates, but the process doesn't.
  `neoncoup host --headless` or `serve-ssh` give a room that outlives any one player.
- **No kick/ban** in the lobby.
- **Room codes are not secret capabilities**: 4 characters from a 31-character alphabet. Fine
  for casual play; a public relay with many rooms would want longer codes or a join password.
  (Taking over a registered or recently dropped room on a relay is prevented by the host's room
  key; guessing a live code to join it is not.)

## Bots
- Every bot at a table shares one difficulty. Mixed tables (one Black ICE, two Fixers) would need
  a per-seat level in the room settings, the protocol and the setup screens.

## SSH mode
- Stats, replays and settings are kept per session in memory only. There are no per-user
  accounts.

## Platforms
- Audio output on macOS and Windows (oto) is verified by cross-compiling and by the
  device-independent synth tests only; it has not been played on real macOS/Windows hardware
  in this repo's CI.
- No mouse support; the game is keyboard-only by design.
