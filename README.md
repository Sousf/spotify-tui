# spotify-tui

A terminal remote control for Spotify, written in Go with bubbletea.

It drives the Spotify Web API, so it does not play audio itself. Open Spotify
on your phone, desktop, or a speaker and this controls that device. Playback
control needs Spotify Premium; browsing works on any account.

## Setup

Spotify requires each user to register their own app, which takes two minutes.

```
go build -o spotify-tui .
./spotify-tui setup
```

The setup command prints the steps. In short:

1. Go to https://developer.spotify.com/dashboard and create an app.
2. Set the Redirect URI to `http://127.0.0.1:8888/callback` (must be exactly this, Spotify rejects `localhost`).
3. Enable the Web API, save, and paste the Client ID into the prompt.

The first run opens a browser to authorize. The token is cached in
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
| `v` | audio visualiser (reads the PipeWire output monitor via `parec`) |
| `a` | add selected track to queue |
| `f` `F` | save / unsave selected track |
| `e` `b` | jump to the selected track's artist or album |
| `R` | reload |
| `?` | help |
| `q` | quit |

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
```
