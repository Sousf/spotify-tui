# spotify-tui

A terminal remote control for Spotify, written in Go with bubbletea.

It drives the Spotify Web API, so it does not play audio itself. Open Spotify
on your phone, desktop, or a speaker and this controls that device. Playback
control needs Spotify Premium; browsing works on any account.

## Install

```
brew install Sousf/tap/spotify-tui
```

The tap-qualified name matters: homebrew-core has an unrelated, retired
formula called `spotify-tui`. Prebuilt binaries for macOS and Linux are also
on the [releases page](https://github.com/Sousf/spotify-tui/releases), or
build from source with Go 1.27+:

```
go install github.com/Sousf/spotify-tui@latest
```

## Setup

Spotify requires each user to register their own app, which takes two minutes.

```
spotify-tui setup
```

The setup command prints the steps. In short:

1. Go to https://developer.spotify.com/dashboard and create an app.
2. Set the Redirect URI to `http://127.0.0.1:8888/callback` (must be exactly this, Spotify rejects `localhost`).
3. Enable the Web API, save, and paste the Client ID into the prompt.

The first run of `spotify-tui` opens a browser to authorize. The token is cached in
`~/.config/spotify-tui/token.json` and refreshed automatically after that.

## Playing on this machine

The TUI needs a device running Spotify to control. On Linux the lightest
option is `spotifyd`, a headless Spotify Connect player:

```
sudo pacman -S spotifyd            # Arch; other distros package it too
mkdir -p ~/.config/spotifyd
cat > ~/.config/spotifyd/spotifyd.conf <<EOF
[global]
device_name = "$(cat /etc/hostname)"
device_type = "computer"
backend = "pulseaudio"             # works with PipeWire via pipewire-pulse
bitrate = 320
cache_path = "$HOME/.cache/spotifyd"
use_mpris = true
dbus_type = "session"
EOF
spotifyd authenticate              # prints a URL, open it and approve
systemctl --user enable --now spotifyd.service
```

`spotify-tui status` should then list the machine as a device, and enter on a
track plays through it. Playback needs Premium.

## Keys

| Key | Action |
| --- | --- |
| `j` `k` `g` `G` `ctrl+d` `ctrl+u` | move |
| `tab` `h` `l` | switch between library and content |
| `enter` | open a playlist, album or artist, or play a track |
| `esc` `backspace` | back |
| `/` | search, then `1`-`4` or `[` `]` to switch result tabs |
| `space` | play / pause |
| `n` `p` | next / previous |
| `>` `<` | seek 10s |
| `+` `-` | volume |
| `s` `r` | shuffle / repeat |
| `d` `u` | devices / queue |
| `v` | visualiser of what Spotify is playing: bars, piano keys, piano roll, off |
| `a` | add selected track to queue |
| `f` `F` | save / unsave selected track |
| `e` `b` | jump to the selected track's artist or album |
| `R` | reload |
| `?` | help |
| `q` | quit |

## Visualiser

`v` shows a spectrum of the player's output; a second press switches to a
piano keyboard lighting the notes it hears, a third adds a roll above the
keys where each note leaves a trail that rises and fades over the last few
seconds, and a fourth closes it. On
PipeWire the capture records only the
Spotify stream: it finds the playback node with `pw-dump` and reads it with
`pw-record`, so a video in another window never shows up in the bars. It
recognises the stream by a property containing `spotify` or `librespot`,
which covers spotifyd, the official client and most third-party players. For
another player, set `SPOTIFY_TUI_VIZ_SOURCE` to a comma-separated list of
words to look for in the stream's application name, process binary or node
name. The pane says "waiting for Spotify to play" until the stream exists.

Without `pw-record` the visualiser falls back to `parec` on the default
sink's monitor, which mixes every application.

The keyboard view is a pitch tracker for the whole mix, not a transcription.
Each key has its own filter at the note's frequency, the strongest peaks are
kept, and the harmonics those notes would produce are discounted so a bass
note does not light the octaves above it. Frames without clear pitches, such
as a drum fill, light nothing. Solo instruments and sparse arrangements track
well; in a dense mix expect the bassline and the current chord, with vocals
flickering between neighbouring keys. Low notes below about C2 need a longer
window than the display latency allows and may spill onto a neighbour. The
keyboard shows as many octaves as fit the pane, centred on middle C, with wider keys on wider terminals.

## Limits of a development-mode app

Spotify restricts apps that are not approved for production. What you will
notice here:

- Playlists owned by other users return 403 when listing their tracks. You
  can still press enter on the empty page to play the whole playlist.
- The artist top-tracks endpoint is blocked, so an artist opens as their
  albums and singles.
- Only the account that created the app (plus up to 25 users you add under
  "User Management" in the dashboard) can log in.
- Creating the app needs a Premium account, and each account gets one
  development-mode app.

## Theming

Colours come from `~/.config/spotify-tui/theme.toml`, flat `key = "#rrggbb"`
lines. Any key left out keeps the built-in Spotify-green default, and the
running TUI re-reads the file within two seconds of a change, so a desktop
theme switcher can regenerate it and recolour an open session.

```
text     = "#dfe2f0"   # primary text
subtext  = "#aaadba"
overlay0 = "#696c78"   # dimmed text, headers, hints
surface0 = "#1a1c27"   # cursor row
surface1 = "#282a36"   # unfocused borders
surface2 = "#393b47"   # progress track
accent   = "#1464c4"   # focused border, playing track, progress
accent2  = "#c41c14"   # visualiser mid band
red      = "#cc7264"   # errors, visualiser top band
yellow   = "#d8a850"   # notices
```

## Troubleshooting

Every run writes `~/.local/state/spotify-tui/spotify-tui.log` (truncated on
start, so it holds the latest session). It records every failed API call with
Spotify's response body, page loads, play requests, and the device fallback.
`spotify-tui log` prints the path. Set `SPOTIFY_TUI_DEBUG=1` to also log
successful requests and key presses.

`spotify-tui status` prints the account, playlist count, devices, and what is
playing without starting the interface. Zero devices means no Spotify app is
running on the account; open one on a phone or computer and check again.

## Files

`~/.config/spotify-tui/config.json` holds the client ID and callback port.
`SPOTIFY_TUI_CLIENT_ID` overrides the file. `spotify-tui logout` deletes the
cached token.

## Development

```
go test ./...
SPOTIFY_TUI_DUMP=1 go test -run TestDump -v ./internal/ui   # print a frame with fake data
SPOTIFY_TUI_LIVE=1 go test -run 'TestLive' -v ./...         # hit the real API and sound server
```

Releases are cut by pushing a tag. GoReleaser builds the binaries, publishes
the GitHub release, and updates the formula in Sousf/homebrew-tap:

```
git tag v0.2.0 && git push --tags
```
