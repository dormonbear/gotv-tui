# gotv-tui

Watch live Counter-Strike 2 matches in the terminal from a public CSTV (GOTV+)
relay: scoreboard, kill feed and a live radar with every player's position.

- Data comes straight from the CSTV broadcast that organizers publish for in-game
  viewing (`playcast "<url>"`), parsed with [cs2parser](https://github.com/osztenkurden/cs2parser).
  No accounts, no scraping.
- The CSTV feed runs ~3 minutes behind real time; that delay is set by the organizer.
- Radar images and calibration are Valve's overview files, fetched on first use from
  [2mlml/cs2-radar-images](https://github.com/2mlml/cs2-radar-images) and cached in
  `~/.cache/gotv-tui/radar`. Two-level maps (Nuke, Train, Vertigo) show both levels.

## Install

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/dormonbear/gotv-tui/main/install.sh | sh
gotv-tui            # pick a source from the list
gotv-tui update     # update to the latest release
```

The script downloads the single-file binary for your platform from the latest
[release](https://github.com/dormonbear/gotv-tui/releases) into `~/.local/bin`
(override with `GOTV_TUI_INSTALL_DIR`). On Windows, download
`gotv-tui-windows-x64.exe` from the release page.

Open a relay directly:

```sh
gotv-tui https://cstv.fragnet.net/a
gotv-tui 'playcast "https://cstv.fragnet.net/a"'
```

## Run from source

Requires Node.js 22+.

```sh
npm install
npm start
npm run build     # single-file binary for this machine (needs Bun) -> dist/gotv-tui
```

Releases: bump `version` in package.json, then push a matching tag (`v0.1.0`); the
release workflow builds macOS / Linux / Windows binaries and publishes them.

## Keys

| Where | Keys |
|---|---|
| Source list | `↑/↓` select · `enter` open · `a` add · `e` edit · `d` delete · `r` refresh · `q` quit |
| Match | `b` back to sources · `q` quit |

Sources are saved in `~/.config/gotv-tui/feeds.json` and start from a few presets
(PGL / BLAST / Challengermode relays published for past and current Majors). Each
row shows whether the relay is `live`, `stale` (match over), `waiting` for the next
broadcast, or `offline`.

## Development

```sh
npm test          # node:test via tsx
npm run typecheck
```
