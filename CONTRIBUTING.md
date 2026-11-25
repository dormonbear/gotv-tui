# Contributing

Thanks for helping out. Bug reports, relay URLs that work for other events and
pull requests are all welcome.

## Development

Requires Go 1.27+.

```sh
go run .                         # source list
go run . https://cstv.fragnet.net/a
go vet ./... && go test ./...    # what CI runs (plus gofmt and a cross-compile)
```

Code layout:

| Package | What it does |
|---|---|
| `internal/gotv` | CSTV relay -> match state and game-log events (demoinfocs-golang) |
| `internal/playback` | re-times the 1/s fragments into smooth 16 fps playback |
| `internal/radar` | Valve radar images, braille rendering, zoom |
| `internal/ui` | Bubble Tea screens: source list and match view |
| `internal/feeds` | saved relay list and probing |
| `internal/update` | `gotv-tui update` self-update |
| `internal/record` | `record` / `replay`: saves relay fragments, serves them back as a local relay |

Keep changes small and focused; add a test for any non-trivial logic. Live behaviour
(the map, the camera) is easiest to check against a running relay, so please say in
the PR what you tried it against.

## Pull requests

- `main` is protected: changes go through a pull request and the `ci` check must pass.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
  subjects (and the PR title, if you squash):
  `feat: ...`, `fix: ...`, `perf: ...`, `docs: ...`, `refactor: ...`, `test: ...`, `chore: ...`.

## Releases

Releases are automatic. Every merge to `main` runs the release workflow, which looks at
the commit subjects since the last tag:

| Commits since last release | Next version |
|---|---|
| any `feat!:` / `BREAKING CHANGE` | minor before 1.0, major after |
| any `feat:` | minor (`0.2.0` -> `0.3.0`) |
| only `fix:` / `perf:` | patch (`0.2.0` -> `0.2.1`) |
| anything else | no release |

It then tests, cross-compiles macOS / Linux / Windows binaries and publishes them as a
GitHub release, which `install.sh` and `gotv-tui update` pick up.
