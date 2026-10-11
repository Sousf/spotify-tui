# Record with: vhs demo.tape
# Needs vhs and ttyd, plus ffmpeg. It runs `spotify-tui demo`, which plays a
# made-up library and a built-in tune, so no account, token or player is
# involved and every take comes out the same.

Output demo.gif

Require go

Set Shell bash
Set FontSize 15
Set Width 1300
Set Height 760
Set Padding 20
Set Framerate 20
Set TypingSpeed 70ms
Set Theme "Catppuccin Mocha"

Hide
Type `export PS1='$ ' PATH="$(mktemp -d):$PATH" && go build -o "${PATH%%:*}/spotify-tui" . && clear`
Enter
Sleep 5s
Show

Type "spotify-tui demo"
Sleep 400ms
Enter
Sleep 2s

# Library: down to a playlist and open it.
Down@250ms 7
Sleep 500ms
Enter
Sleep 1500ms

# Play a track from it.
Down@200ms 4
Sleep 400ms
Enter
Sleep 2s

# Search, then walk the result tabs.
Type "/"
Sleep 300ms
Type "harbour"
Sleep 300ms
Enter
Sleep 1800ms
Type "2"
Sleep 1200ms
Type "3"
Sleep 1200ms
Type "4"
Sleep 1200ms
Escape
Sleep 600ms

# The visualiser: bars, piano keys, piano roll, off.
Type "v"
Sleep 3500ms
Type "v"
Sleep 3500ms
Type "v"
Sleep 5s
Type "v"
Sleep 800ms

Type "q"
Sleep 500ms
