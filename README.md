# gotv-tui

Watch live Counter-Strike 2 matches in the terminal from a public CSTV (GOTV+)
relay: scoreboard, kill feed and a live radar with every player's position.

- Data comes straight from the CSTV broadcast that organizers publish for in-game
  viewing (`playcast "<url>"`), parsed with [demoinfocs-golang](https://github.com/markus-wa/demoinfocs-golang).
  No accounts, no scraping.
- The CSTV feed runs ~3 minutes behind real time; that delay is set by the organizer.
- Radar images and calibration are Valve's overview files, fetched on first use from
  [2mlml/cs2-radar-images](https://github.com/2mlml/cs2-radar-images) and cached in
  `~/.cache/gotv-tui/radar`. Two-level maps (Nuke, Train, Vertigo) show both levels.

## Map rendering

The radar is drawn as a real image where the terminal supports it, else as text:

| Terminal | Map |
|---|---|
| Ghostty, kitty | kitty graphics: radar uploaded once, markers as sprites, pixel-smooth zoom/drag |
| iTerm2, WezTerm | iTerm2 inline images: whole frame (map + markers), up to 10 fps |
| Windows Terminal, foot, Konsole | Sixel: whole frame, 255 colours, up to 10 fps |
| VS Code | opt in with `GOTV_TUI_GRAPHICS=iterm` (needs `terminal.integrated.enableImages`) |
| others (Terminal.app, Alacritty, ...) | braille outlines; more detail lines appear as you zoom in |

`GOTV_TUI_GRAPHICS=kitty|iterm|sixel|off` overrides the detection.

Opened in Terminal.app (e.g. by a double-click on macOS) or the old Windows console,
gotv-tui reopens itself in the best installed terminal: Ghostty, kitty, WezTerm or
iTerm2 on macOS, Windows Terminal on Windows. `GOTV_TUI_RELAUNCH=0` keeps it where it is.

## Install

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/dormonbear/gotv-tui/main/install.sh | sh
gotv-tui            # pick a source from the list
gotv-tui update     # update to the latest release
```

The script downloads the static binary (~4 MB download) for your platform from the latest
[release](https://github.com/dormonbear/gotv-tui/releases) into `~/.local/bin`
(override with `GOTV_TUI_INSTALL_DIR`). On Windows, download
`gotv-tui-windows-x64.exe` from the release page.

Open a relay directly:

```sh
gotv-tui https://cstv.fragnet.net/a
gotv-tui 'playcast "https://cstv.fragnet.net/a"'
```

## Run from source

Requires Go 1.27+.

```sh
go run .                        # or: go run . https://cstv.fragnet.net/a
go build -o gotv-tui .
```

Releases are automatic on merge to `main`, versioned from Conventional Commit
subjects; see [CONTRIBUTING.md](CONTRIBUTING.md).

## Keys

| Where | Keys |
|---|---|
| Source list | `↑/↓` select · `enter` open · `a` add · `e` edit · `d` delete · `r` refresh · `q` quit |
| Match | mouse wheel or `+`/`-` zoom map (around the pointer) · drag or `←↑↓→`/`hjkl` pan · `d` or click the switch: auto director (follows fights) · click a player (on the map or the scoreboard) to follow them, click again or elsewhere to stop · any manual move takes over · `0` whole map · `b` back to sources · `q` quit |

Sources are saved in `~/.config/gotv-tui/feeds.json` and start from a few presets
(PGL / BLAST / Challengermode relays published for past and current Majors). Each
row shows whether the relay is `live`, `stale` (match over), `waiting` for the next
broadcast, or `offline`.

## Development

```sh
go vet ./... && go test ./...
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).
