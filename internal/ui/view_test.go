package ui

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/zmb3/spotify/v2"

	"github.com/Sousf/spotify-tui/internal/viz"
)

// TestMain forces a colour profile so rendered frames contain the same
// escape sequences a real terminal would see. Width checks then catch code
// that measures styled strings incorrectly.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

func fakeModel(w, h int) Model {
	m := New(http.DefaultClient)
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = mm.(Model)
	var items []item
	for i := 0; i < 80; i++ {
		items = append(items, item{
			kind:  kindTrack,
			id:    spotify.ID("t" + string(rune('a'+i%26))),
			title: strings.Repeat("Song title ", 1+i%4),
			sub:   "Some Artist, Another Artist",
			extra: "An Album With A Fairly Long Name",
			dur:   3*time.Minute + time.Duration(i)*7*time.Second,
		})
	}
	mm, _ = m.Update(pageLoadedMsg{target: m.top(), items: items, total: 240})
	m = mm.(Model)
	mm, _ = m.Update(playlistsMsg{items: []item{
		{kind: kindPlaylist, id: "p1", uri: "spotify:playlist:p1", title: "Road trip 日本語", sub: "pete", extra: "12 tracks"},
		{kind: kindPlaylist, id: "p2", uri: "spotify:playlist:p2", title: "A playlist with a very long name that overflows", sub: "pete"},
	}})
	m = mm.(Model)
	st := &spotify.PlayerState{
		Device:       spotify.PlayerDevice{Name: "Living room", Volume: 42},
		ShuffleState: true,
		RepeatState:  "context",
	}
	st.Playing = true
	st.Progress = 61000
	st.Item = &spotify.FullTrack{}
	st.Item.ID = "tb"
	st.Item.Name = "Now Playing Song"
	st.Item.Duration = 200000
	st.Item.Artists = []spotify.SimpleArtist{{Name: "Artist"}}
	st.Item.Album.Name = "Album"
	mm, _ = m.Update(playerStateMsg{state: st})
	return mm.(Model)
}

func checkFrame(t *testing.T, name, out string, w, h int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Errorf("%s: got %d lines, want %d", name, len(lines), h)
	}
	for i, l := range lines {
		if lw := lipgloss.Width(l); lw != w {
			t.Errorf("%s: line %d is %d wide, want %d: %q", name, i, lw, w, l)
		}
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 15}, {200, 60}} {
		w, h := size[0], size[1]
		m := fakeModel(w, h)
		checkFrame(t, "tracks", m.View(), w, h)

		m.focus = focusContent
		m.showHelp = true
		checkFrame(t, "help", m.View(), w, h)
		m.showHelp = false

		m.vizMode = vizBars
		m.vizErr = nil
		n := m.vizBarCount()
		m.vizLevels = make([]float64, n)
		m.vizPeaks = make([]float64, n)
		for i := range m.vizLevels {
			m.vizLevels[i] = float64(i%7) / 6
			m.vizPeaks[i] = 1
		}
		m.viz = &viz.Capture{}
		checkFrame(t, "viz", m.View(), w, h)
		m.vizMode = vizKeys
		m.vizKeys = make([]float64, viz.Keys)
		for k := range m.vizKeys {
			m.vizKeys[k] = float64(k%4) / 3
		}
		checkFrame(t, "keys", m.View(), w, h)
		m.vizStatus = "waiting for Spotify to play…"
		checkFrame(t, "keys waiting", m.View(), w, h)
		m.vizStatus = ""
		m.viz = nil
		m.vizMode = vizOff

		mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		m = mm.(Model)
		checkFrame(t, "search input", m.View(), w, h)

		sp := &page{kind: pageSearch, title: "Search", query: "x"}
		m.stack = append(m.stack, sp)
		m.focus = focusContent
		mm, _ = m.Update(searchDoneMsg{target: sp, results: [4][]item{
			{{kind: kindTrack, title: "T"}},
			{{kind: kindArtist, title: "Ar", sub: "5 followers"}},
			{{kind: kindAlbum, title: "Al", sub: "Ar", extra: "1999"}},
			{{kind: kindPlaylist, title: "Pl", sub: "me", extra: "3 tracks"}},
		}})
		m = mm.(Model)
		for tab := 0; tab < 4; tab++ {
			m.setSearchTab(sp, tab)
			checkFrame(t, "search tab", m.View(), w, h)
		}
	}
}

func TestCursorPaging(t *testing.T) {
	m := fakeModel(100, 30)
	m.focus = focusContent
	h := m.listHeight()
	p := m.top()
	for i := 0; i < 50; i++ {
		p.move(1, h)
	}
	if p.cursor != 50 {
		t.Fatalf("cursor = %d, want 50", p.cursor)
	}
	if p.offset != 50-h+1 {
		t.Fatalf("offset = %d, want %d", p.offset, 50-h+1)
	}
	// Moving to the bottom should ask for another page.
	p.cursor = len(p.items) - 1
	if cmd := m.maybeLoadMore(p); cmd == nil {
		t.Fatal("expected a load command near the end of a partial page")
	}
	if !p.loading {
		t.Fatal("page should be marked loading")
	}
}

func TestLibrarySkipsHeaders(t *testing.T) {
	m := fakeModel(100, 30)
	lib := &m.library
	lib.cursor = 4 // Devices, just above the Playlists header
	lib.move(1, 20)
	if lib.items[lib.cursor].header {
		t.Fatal("cursor landed on a header row")
	}
	if lib.items[lib.cursor].kind != kindPlaylist {
		t.Fatalf("cursor on %q, want first playlist", lib.items[lib.cursor].title)
	}
	lib.move(-1, 20)
	if lib.items[lib.cursor].id != "devices" {
		t.Fatalf("cursor on %q, want devices", lib.items[lib.cursor].title)
	}
}

func TestFmtDur(t *testing.T) {
	cases := map[time.Duration]string{
		0:                             "0:00",
		61 * time.Second:              "1:01",
		3*time.Minute + 5*time.Second: "3:05",
		75 * time.Minute:              "1:15:00",
	}
	for d, want := range cases {
		if got := fmtDur(d); got != want {
			t.Errorf("fmtDur(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestColumnsFillWidth(t *testing.T) {
	m := fakeModel(100, 30)
	pages := []*page{
		{kind: pageTracks},
		{kind: pageAlbums},
		{kind: pageArtists},
		{kind: pagePlaylists},
		{kind: pageDevices},
		{kind: pageSearch, searchTab: 0},
		{kind: pageSearch, searchTab: 3},
	}
	it := item{title: "t", sub: "s", extra: "e", dur: time.Minute}
	for _, width := range []int{40, 61, 78, 133} {
		for _, p := range pages {
			c := m.columns(p, width)
			if got := lipgloss.Width(c.row(0, it, "")); got != width {
				t.Errorf("kind %d width %d: row is %d wide", p.kind, width, got)
			}
			if hdr := c.header(); hdr != "" && lipgloss.Width(hdr) != width {
				t.Errorf("kind %d width %d: header is %d wide", p.kind, width, lipgloss.Width(hdr))
			}
		}
	}
}

// TestStyledRowsKeepContent guards against width being measured with the
// colour escape codes included, which used to truncate styled rows.
func TestStyledRowsKeepContent(t *testing.T) {
	m := fakeModel(120, 30)
	m.focus = focusContent

	m.vizMode = vizBars
	n := m.vizBarCount()
	m.vizLevels = make([]float64, n)
	m.vizPeaks = make([]float64, n)
	for i := range m.vizLevels {
		m.vizLevels[i] = 1
	}
	m.viz = &viz.Capture{}
	lines := strings.Split(m.View(), "\n")
	// Rows 2..(list height) are bar rows; each must show every bar.
	for i := 2; i < 2+m.listHeight()-2; i++ {
		if got := strings.Count(lines[i], "█"); got != n {
			t.Fatalf("viz row %d has %d blocks, want %d: %q", i, got, n, lines[i])
		}
	}
	m.vizMode = vizOff
	m.viz = nil

	sp := &page{kind: pageSearch, title: "Search", query: "x"}
	m.stack = append(m.stack, sp)
	mm, _ := m.Update(searchDoneMsg{target: sp})
	m = mm.(Model)
	// The active tab is underlined, and lipgloss styles each word separately
	// for underlines, so compare the plain text.
	out := ansi.Strip(m.View())
	for _, label := range []string{"1 Tracks", "2 Artists", "3 Albums", "4 Playlists"} {
		if !strings.Contains(out, label) {
			t.Errorf("search tab %q missing from frame", label)
		}
	}
}
