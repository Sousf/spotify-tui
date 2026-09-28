package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/zmb3/spotify/v2"
)

type itemKind int

const (
	kindTrack itemKind = iota
	kindAlbum
	kindArtist
	kindPlaylist
	kindDevice
	kindLibrary // fixed entries in the left pane
)

// item is one row in a list. Every kind of Spotify object is flattened into
// the same shape so the list renderer only has to know about columns.
type item struct {
	kind  itemKind
	id    spotify.ID
	uri   spotify.URI
	title string
	sub   string // artist, owner, device type ...
	extra string // album, track count ...
	dur   time.Duration
	// artist ID for tracks, so the artist page can be reached from a track
	artistID spotify.ID
	albumID  spotify.ID
	active   bool // device is active
	header   bool // non-selectable section label
}

type pageKind int

const (
	pageTracks pageKind = iota
	pageAlbums
	pageArtists
	pagePlaylists
	pageDevices
	pageQueue
	pageSearch
)

// loader fetches more rows for a page, starting at offset.
type loader func(offset int) tea.Cmd

// page is a scrollable list in the content pane with lazy loading.
type page struct {
	kind    pageKind
	title   string
	context spotify.URI // playback context for tracks, empty for search results
	items   []item
	cursor  int
	offset  int // first visible row
	total   int // total rows on the server, 0 if unknown
	loading bool
	loadErr error
	load    loader
	after   string // cursor for cursor-paginated endpoints
	// search pages hold four result sets and a tab selector
	searchTab     int
	searchResults [4][]item
	query         string
}

func (p *page) hasMore() bool {
	return p.load != nil && (p.total == 0 || len(p.items) < p.total)
}

func (p *page) selected() *item {
	if p == nil || len(p.items) == 0 || p.cursor >= len(p.items) {
		return nil
	}
	return &p.items[p.cursor]
}

func (p *page) move(delta, height int) {
	if len(p.items) == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for n := delta; n != 0; n -= step {
		next := p.cursor + step
		for next >= 0 && next < len(p.items) && p.items[next].header {
			next += step
		}
		if next < 0 || next >= len(p.items) {
			break
		}
		p.cursor = next
	}
	p.clamp(height)
}

func (p *page) clamp(height int) {
	if height < 1 {
		height = 1
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+height {
		p.offset = p.cursor - height + 1
	}
	if p.offset < 0 {
		p.offset = 0
	}
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d / time.Minute)
	s := int(d%time.Minute) / int(time.Second)
	if m >= 60 {
		return fmt.Sprintf("%d:%02d:%02d", m/60, m%60, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func artistNames(as []spotify.SimpleArtist) string {
	names := make([]string, 0, len(as))
	for _, a := range as {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

func trackItem(t spotify.FullTrack) item {
	it := item{
		kind:    kindTrack,
		id:      t.ID,
		uri:     t.URI,
		title:   t.Name,
		sub:     artistNames(t.Artists),
		extra:   t.Album.Name,
		dur:     t.TimeDuration(),
		albumID: t.Album.ID,
	}
	if len(t.Artists) > 0 {
		it.artistID = t.Artists[0].ID
	}
	return it
}

func simpleTrackItem(t spotify.SimpleTrack, album spotify.SimpleAlbum) item {
	it := item{
		kind:    kindTrack,
		id:      t.ID,
		uri:     t.URI,
		title:   t.Name,
		sub:     artistNames(t.Artists),
		extra:   album.Name,
		dur:     t.TimeDuration(),
		albumID: album.ID,
	}
	if len(t.Artists) > 0 {
		it.artistID = t.Artists[0].ID
	}
	return it
}

func albumItem(a spotify.SimpleAlbum) item {
	year := a.ReleaseDate
	if len(year) > 4 {
		year = year[:4]
	}
	it := item{
		kind:  kindAlbum,
		id:    a.ID,
		uri:   a.URI,
		title: a.Name,
		sub:   artistNames(a.Artists),
		extra: year,
	}
	if len(a.Artists) > 0 {
		it.artistID = a.Artists[0].ID
	}
	return it
}

func artistItem(a spotify.FullArtist) item {
	return item{
		kind:     kindArtist,
		id:       a.ID,
		uri:      a.URI,
		title:    a.Name,
		sub:      fmt.Sprintf("%d followers", a.Followers.Count),
		artistID: a.ID,
	}
}

func playlistItem(p spotify.SimplePlaylist) item {
	return item{
		kind:  kindPlaylist,
		id:    p.ID,
		uri:   p.URI,
		title: p.Name,
		sub:   p.Owner.DisplayName,
	}
}

func deviceItem(d spotify.PlayerDevice) item {
	return item{
		kind:   kindDevice,
		id:     d.ID,
		title:  d.Name,
		sub:    d.Type,
		extra:  fmt.Sprintf("vol %d%%", d.Volume),
		active: d.Active,
	}
}

// pad fits s into exactly w cells, truncating with an ellipsis or padding
// with spaces, so columns line up regardless of wide characters.
func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if runewidth.StringWidth(s) > w {
		if w <= 1 {
			return runewidth.Truncate(s, w, "")
		}
		return runewidth.Truncate(s, w, "…")
	}
	return runewidth.FillRight(s, w)
}
