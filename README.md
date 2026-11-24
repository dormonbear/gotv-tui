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

## Run

Requires Node.js 22+ (or build the binary with Bun).

```sh
npm install
npm start                                   # pick a source from the list
npx tsx src/main.tsx https://cstv.fragnet.net/a   # or open a relay directly
npx tsx src/main.tsx 'playcast "https://cstv.fragnet.net/a"'
```

Single-file binary (Bun):

```sh
npm run build     # -> dist/gotv-tui
```

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
