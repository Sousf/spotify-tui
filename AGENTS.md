# AGENTS.md

spotify-tui is a terminal remote control for Spotify, written in Go (see
`go.mod` for the version) with bubbletea. It drives the Spotify Web API and
does not play audio itself.

## Build and test

    go build ./...
    go vet ./...
    gofmt -l .          # must print nothing; CI fails otherwise
    go test -race ./... # what CI runs

`TestLive` (`SPOTIFY_TUI_LIVE=1`) hits the real Spotify API and sound server,
and `TestDump` (`SPOTIFY_TUI_DUMP=1`) prints frames for eyeballing. Both skip
by default and need a logged-in account or a human; do not enable them in CI.

## Layout

- `main.go`: entry point and subcommands (`setup`, `login`, `status`,
  `logout`, `log`, `version`).
- `internal/auth`: Spotify OAuth PKCE flow and token storage.
- `internal/logging`: per-session log file.
- `internal/ui`: bubbletea model, views and Web API loaders.
- `internal/viz`: audio capture via `parec` and FFT for the visualiser.

## Do not touch

- `.github/workflows/release.yml` and `.goreleaser.yaml`: tag-triggered
  release that publishes GitHub releases and pushes to `Sousf/homebrew-tap`
  with the `HOMEBREW_TAP_DEPLOY_KEY` secret.
- Do not push tags; pushing a `v*` tag cuts a release.
- The CI job in `.github/workflows/ci.yml` is named `test`, which the default
  branch ruleset requires as a status check. Renaming it blocks merges.
- Builds are `CGO_ENABLED=0`; do not add cgo dependencies (the visualiser
  shells out to `parec` instead of linking libpulse for this reason).
