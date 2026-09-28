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
| `a` | add selected track to queue |
| `f` `F` | save / unsave selected track |
| `e` `A` `b` | jump to the selected track's artist, artist albums, or album |
| `R` | reload |
| `?` | help |
| `q` | quit |

## Files

`~/.config/spotify-tui/config.json` holds the client ID and callback port.
`SPOTIFY_TUI_CLIENT_ID` overrides the file. `spotify-tui logout` deletes the
cached token.

## Development

```
go test ./...
SPOTIFY_TUI_DUMP=1 go test -run TestDump -v ./internal/ui   # print a frame with fake data
```
