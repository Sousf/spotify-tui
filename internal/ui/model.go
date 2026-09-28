// Package ui is the bubbletea program: a library sidebar, a content pane
// backed by a stack of pages, and a now-playing bar that polls the player.
package ui

import (
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/zmb3/spotify/v2"
)

type focus int

const (
	focusLibrary focus = iota
	focusContent
	focusSearch
)

const (
	sidebarWidth = 30
	bottomHeight = 3 // now playing (2 lines) + status line
	pollEvery    = 3 * time.Second
	loadAhead    = 15 // rows before the end at which the next page is requested
)

// Model is the root bubbletea model.
type Model struct {
	client *api

	width, height int
	focus         focus
	prevFocus     focus
	showHelp      bool

	library page
	stack   []*page

	state    *spotify.PlayerState
	stateAt  time.Time
	progress time.Duration
	country  string

	search textinput.Model

	status      string
	statusErr   bool
	statusUntil time.Time
}

// New builds the model from an authenticated HTTP client.
func New(hc *http.Client) Model {
	client := newAPI(hc)
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "search tracks, artists, albums, playlists"
	ti.CharLimit = 200

	m := Model{
		client: client,
		focus:  focusLibrary,
		search: ti,
	}
	m.library = page{kind: pagePlaylists, title: "Library", items: libraryEntries(nil)}
	m.openLibrary(m.library.items[0])
	return m
}

func libraryEntries(playlists []item) []item {
	items := []item{
		{kind: kindLibrary, id: "liked", title: "Liked Songs"},
		{kind: kindLibrary, id: "albums", title: "Albums"},
		{kind: kindLibrary, id: "artists", title: "Artists"},
		{kind: kindLibrary, id: "queue", title: "Queue"},
		{kind: kindLibrary, id: "devices", title: "Devices"},
		{kind: kindLibrary, header: true, title: "Playlists"},
	}
	return append(items, playlists...)
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		fetchUser(m.client),
		fetchPlaylists(m.client),
		fetchPlayerState(m.client),
		tick(),
		poll(pollEvery),
	}
	if top := m.top(); top != nil {
		cmds = append(cmds, m.initialLoad(top))
	}
	return tea.Batch(cmds...)
}

func (m *Model) top() *page {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

// listHeight is how many rows fit in the content list.
func (m Model) listHeight() int {
	h := m.height - bottomHeight - 2 // pane borders
	h -= 1                           // title row
	if top := m.top(); top != nil {
		switch top.kind {
		case pageTracks, pageQueue:
			h-- // column header
		case pageSearch:
			h -= 2 // input + tabs
			if top.searchTab == 0 {
				h--
			}
		}
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) libraryHeight() int {
	h := m.height - bottomHeight - 2 - 1
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) setStatus(text string, isErr bool) {
	m.status = text
	m.statusErr = isErr
	m.statusUntil = time.Now().Add(6 * time.Second)
}

func (m *Model) push(p *page) tea.Cmd {
	m.stack = append(m.stack, p)
	m.focus = focusContent
	return m.initialLoad(p)
}

func (m *Model) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

// initialLoad kicks off the first fetch for a page that has a loader.
func (m *Model) initialLoad(p *page) tea.Cmd {
	if p.load == nil || p.loading || len(p.items) > 0 {
		return nil
	}
	p.loading = true
	return p.load(0)
}

// maybeLoadMore requests the next page when the cursor nears the end.
func (m *Model) maybeLoadMore(p *page) tea.Cmd {
	if p == nil || p.loading || !p.hasMore() {
		return nil
	}
	if p.cursor < len(p.items)-loadAhead {
		return nil
	}
	p.loading = true
	return p.load(len(p.items))
}

// openLibrary replaces the page stack with the page for a sidebar entry.
func (m *Model) openLibrary(it item) tea.Cmd {
	c := m.client
	var p *page
	switch {
	case it.kind == kindPlaylist:
		p = &page{kind: pageTracks, title: it.title, context: it.uri}
		p.load = playlistLoader(c, it.id, p)
	case it.id == "liked":
		p = &page{kind: pageTracks, title: "Liked Songs"}
		p.load = likedLoader(c, p)
	case it.id == "albums":
		p = &page{kind: pageAlbums, title: "Saved Albums"}
		p.load = savedAlbumsLoader(c, p)
	case it.id == "artists":
		p = &page{kind: pageArtists, title: "Followed Artists"}
		p.load = followedArtistsLoader(c, p)
	case it.id == "queue":
		p = &page{kind: pageQueue, title: "Queue", loading: true}
		m.stack = []*page{p}
		return fetchQueue(c, p)
	case it.id == "devices":
		p = &page{kind: pageDevices, title: "Devices", loading: true}
		m.stack = []*page{p}
		return fetchDevices(c, p)
	default:
		return nil
	}
	m.stack = []*page{p}
	return m.initialLoad(p)
}

// activate is Enter on a content row.
func (m *Model) activate(it *item) tea.Cmd {
	if it == nil {
		return nil
	}
	c := m.client
	top := m.top()
	switch it.kind {
	case kindTrack:
		return m.playFrom(top)
	case kindAlbum:
		album := spotify.SimpleAlbum{ID: it.id, Name: it.title, URI: it.uri}
		p := &page{kind: pageTracks, title: it.title + "  " + dimStyle.Render(it.sub), context: it.uri}
		p.load = albumLoader(c, album, p)
		return m.push(p)
	case kindArtist:
		return m.openArtist(it)
	case kindPlaylist:
		p := &page{kind: pageTracks, title: it.title + "  " + dimStyle.Render(it.sub), context: it.uri}
		p.load = playlistLoader(c, it.id, p)
		return m.push(p)
	case kindDevice:
		m.setStatus("Transferring playback to "+it.title, false)
		return transfer(c, it.id)
	}
	return nil
}

// playFrom starts playback at the cursor of a tracks page. Playlists and
// albums are played as a context so Spotify continues past what is loaded;
// everything else is sent as an explicit list of URIs.
func (m *Model) playFrom(p *page) tea.Cmd {
	it := p.selected()
	if it == nil || it.kind != kindTrack {
		return nil
	}
	if p.context != "" {
		return playContext(m.client, p.context, it.uri)
	}
	const window = 100
	end := len(p.items)
	if end-p.cursor > window {
		end = p.cursor + window
	}
	uris := make([]spotify.URI, 0, end-p.cursor)
	for _, x := range p.items[p.cursor:end] {
		uris = append(uris, x.uri)
	}
	return playURIs(m.client, uris, 0)
}

// openArtist pushes the discography of the selected row's artist. Spotify
// blocks the top-tracks endpoint for development-mode apps, so albums and
// singles are the artist view.
func (m *Model) openArtist(it *item) tea.Cmd {
	if it == nil || it.artistID == "" {
		return nil
	}
	name := it.title
	if it.kind != kindArtist {
		name = strings.SplitN(it.sub, ",", 2)[0]
	}
	p := &page{kind: pageAlbums, title: name + "  " + dimStyle.Render("albums & singles")}
	p.load = artistAlbumsLoader(m.client, it.artistID, p)
	return m.push(p)
}

func (m *Model) openAlbum(it *item) tea.Cmd {
	if it == nil || it.albumID == "" {
		return nil
	}
	album := spotify.SimpleAlbum{ID: it.albumID, Name: it.extra, URI: spotify.URI("spotify:album:" + string(it.albumID))}
	p := &page{kind: pageTracks, title: it.extra, context: album.URI}
	p.load = albumLoader(m.client, album, p)
	return m.push(p)
}

func (m *Model) startSearch() tea.Cmd {
	m.prevFocus = m.focus
	m.focus = focusSearch
	m.search.SetValue("")
	return m.search.Focus()
}

func (m *Model) submitSearch() tea.Cmd {
	q := strings.TrimSpace(m.search.Value())
	m.search.Blur()
	if q == "" {
		m.focus = m.prevFocus
		return nil
	}
	p := &page{kind: pageSearch, title: "Search", query: q, loading: true}
	// A new search replaces an existing search page instead of stacking.
	if top := m.top(); top != nil && top.kind == pageSearch {
		m.stack[len(m.stack)-1] = p
	} else {
		m.stack = append(m.stack, p)
	}
	m.focus = focusContent
	return runSearch(m.client, q, p)
}

func (m *Model) setSearchTab(p *page, tab int) {
	if p.kind != pageSearch || tab < 0 || tab > 3 {
		return
	}
	p.searchTab = tab
	p.items = p.searchResults[tab]
	p.cursor, p.offset = 0, 0
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = m.width - sidebarWidth - 8
		for _, p := range m.stack {
			p.clamp(m.listHeight())
		}
		m.library.clamp(m.libraryHeight())
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tickMsg:
		if m.state != nil && m.state.Playing && m.state.Item != nil {
			m.progress = time.Duration(m.state.Progress)*time.Millisecond + time.Since(m.stateAt)
			if max := m.state.Item.TimeDuration(); m.progress > max {
				m.progress = max
			}
		}
		return m, tick()

	case pollMsg:
		return m, tea.Batch(fetchPlayerState(m.client), poll(pollEvery))

	case refreshMsg:
		return m, fetchPlayerState(m.client)

	case playerStateMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.state = msg.state
		m.stateAt = time.Now()
		if msg.state != nil {
			m.progress = time.Duration(msg.state.Progress) * time.Millisecond
		}
		return m, nil

	case userMsg:
		if msg.err == nil && msg.user != nil {
			m.country = msg.user.Country
		}
		return m, nil

	case playlistsMsg:
		if msg.err != nil {
			m.setStatus("playlists: "+msg.err.Error(), true)
		}
		m.library.items = libraryEntries(msg.items)
		m.library.clamp(m.libraryHeight())
		return m, nil

	case pageLoadedMsg:
		p := msg.target
		p.loading = false
		if msg.err != nil {
			p.loadErr = msg.err
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		if msg.offset == 0 && p.load != nil && p.after == "" {
			p.items = msg.items
		} else if p.load == nil {
			p.items = msg.items
		} else {
			p.items = append(p.items, msg.items...)
		}
		p.total = msg.total
		p.after = msg.after
		if msg.after == "" && p.kind == pageArtists && len(msg.items) == 0 {
			p.total = len(p.items)
		}
		p.clamp(m.listHeight())
		return m, m.maybeLoadMore(p)

	case searchDoneMsg:
		p := msg.target
		p.loading = false
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		p.searchResults = msg.results
		// Land on the first tab with results.
		tab := 0
		for i, r := range msg.results {
			if len(r) > 0 {
				tab = i
				break
			}
		}
		m.setSearchTab(p, tab)
		return m, nil

	case actionDoneMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		switch msg.what {
		case "queue":
			m.setStatus("Added to queue", false)
		case "save":
			m.setStatus("Saved to Liked Songs", false)
		case "unsave":
			m.setStatus("Removed from Liked Songs", false)
		}
		// Give Spotify a moment to apply the change before reading it back.
		return m, tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return refreshMsg{} })
	}

	if m.focus == focusSearch {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

type refreshMsg struct{}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	if m.focus == focusSearch {
		switch key {
		case "esc", "ctrl+c":
			m.search.Blur()
			m.focus = m.prevFocus
			return m, nil
		case "enter":
			return m, m.submitSearch()
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}

	// Global keys.
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "/":
		return m, m.startSearch()
	case " ":
		return m, m.togglePlay()
	case "n":
		return m, next(m.client)
	case "p":
		return m, previous(m.client)
	case ">", ".":
		return m, seek(m.client, m.progress+10*time.Second)
	case "<", ",":
		return m, seek(m.client, m.progress-10*time.Second)
	case "s":
		return m, m.toggleShuffle()
	case "r":
		return m, m.cycleRepeat()
	case "+", "=":
		return m, m.nudgeVolume(5)
	case "-", "_":
		return m, m.nudgeVolume(-5)
	case "d":
		m.library.cursor = indexOf(m.library.items, "devices")
		return m, m.openLibrary(m.library.items[m.library.cursor])
	case "u":
		m.library.cursor = indexOf(m.library.items, "queue")
		return m, m.openLibrary(m.library.items[m.library.cursor])
	case "R":
		return m, m.reload()
	case "tab", "h", "l", "left", "right":
		if m.focus == focusLibrary {
			m.focus = focusContent
		} else {
			m.focus = focusLibrary
		}
		return m, nil
	case "esc", "backspace":
		if m.focus == focusContent && len(m.stack) > 1 {
			m.pop()
		} else {
			m.focus = focusLibrary
		}
		return m, nil
	}

	if m.focus == focusLibrary {
		return m.handleLibraryKey(key)
	}
	return m.handleContentKey(key)
}

func indexOf(items []item, id spotify.ID) int {
	for i, it := range items {
		if it.id == id && it.kind == kindLibrary {
			return i
		}
	}
	return 0
}

func (m Model) handleLibraryKey(key string) (tea.Model, tea.Cmd) {
	p := &m.library
	h := m.libraryHeight()
	switch key {
	case "j", "down":
		p.move(1, h)
	case "k", "up":
		p.move(-1, h)
	case "ctrl+d", "pgdown":
		p.move(h/2, h)
	case "ctrl+u", "pgup":
		p.move(-h/2, h)
	case "g", "home":
		p.cursor = 0
		p.clamp(h)
	case "G", "end":
		p.cursor = len(p.items) - 1
		p.clamp(h)
	case "enter":
		if it := p.selected(); it != nil && !it.header {
			cmd := m.openLibrary(*it)
			m.focus = focusContent
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) handleContentKey(key string) (tea.Model, tea.Cmd) {
	p := m.top()
	if p == nil {
		return m, nil
	}
	h := m.listHeight()
	switch key {
	case "j", "down":
		p.move(1, h)
		return m, m.maybeLoadMore(p)
	case "k", "up":
		p.move(-1, h)
	case "ctrl+d", "pgdown":
		p.move(h/2, h)
		return m, m.maybeLoadMore(p)
	case "ctrl+u", "pgup":
		p.move(-h/2, h)
	case "g", "home":
		p.cursor = 0
		p.clamp(h)
	case "G", "end":
		p.cursor = len(p.items) - 1
		p.clamp(h)
		return m, m.maybeLoadMore(p)
	case "enter":
		if len(p.items) == 0 && p.loadErr != nil && p.context != "" {
			// Listing is blocked but playing the context usually still works.
			return m, playContext(m.client, p.context, "")
		}
		return m, m.activate(p.selected())
	case "a":
		if it := p.selected(); it != nil && it.kind == kindTrack {
			return m, addToQueue(m.client, it.id)
		}
	case "e":
		return m, m.openArtist(p.selected())
	case "b":
		return m, m.openAlbum(p.selected())
	case "f":
		if it := p.selected(); it != nil && it.kind == kindTrack {
			return m, saveTrack(m.client, it.id)
		}
	case "F":
		if it := p.selected(); it != nil && it.kind == kindTrack {
			return m, unsaveTrack(m.client, it.id)
		}
	case "1", "2", "3", "4":
		m.setSearchTab(p, int(key[0]-'1'))
	case "[":
		if p.kind == pageSearch {
			m.setSearchTab(p, (p.searchTab+3)%4)
		}
	case "]":
		if p.kind == pageSearch {
			m.setSearchTab(p, (p.searchTab+1)%4)
		}
	}
	return m, nil
}

// Optimistic player controls: update local state immediately so the bar
// reacts on the keypress, then let the poll correct it.

func (m *Model) togglePlay() tea.Cmd {
	if m.state == nil {
		return resume(m.client)
	}
	if m.state.Playing {
		m.state.Playing = false
		m.state.Progress = spotify.Numeric(m.progress.Milliseconds())
		return pause(m.client)
	}
	m.state.Playing = true
	m.stateAt = time.Now()
	return resume(m.client)
}

func (m *Model) toggleShuffle() tea.Cmd {
	on := true
	if m.state != nil {
		on = !m.state.ShuffleState
		m.state.ShuffleState = on
	}
	return setShuffle(m.client, on)
}

func (m *Model) cycleRepeat() tea.Cmd {
	next := "context"
	if m.state != nil {
		switch m.state.RepeatState {
		case "context":
			next = "track"
		case "track":
			next = "off"
		}
		m.state.RepeatState = next
	}
	return setRepeat(m.client, next)
}

func (m *Model) nudgeVolume(delta int) tea.Cmd {
	if m.state == nil {
		return nil
	}
	v := int(m.state.Device.Volume) + delta
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	m.state.Device.Volume = spotify.Numeric(v)
	return setVolume(m.client, v)
}

// reload refetches the current page and the sidebar.
func (m *Model) reload() tea.Cmd {
	cmds := []tea.Cmd{fetchPlaylists(m.client), fetchPlayerState(m.client)}
	p := m.top()
	if p == nil {
		return tea.Batch(cmds...)
	}
	p.items = nil
	p.cursor, p.offset, p.total, p.after = 0, 0, 0, ""
	switch p.kind {
	case pageQueue:
		p.loading = true
		cmds = append(cmds, fetchQueue(m.client, p))
	case pageDevices:
		p.loading = true
		cmds = append(cmds, fetchDevices(m.client, p))
	case pageSearch:
		p.loading = true
		cmds = append(cmds, runSearch(m.client, p.query, p))
	default:
		cmds = append(cmds, m.initialLoad(p))
	}
	return tea.Batch(cmds...)
}

func (m Model) statusLine() string {
	if m.status != "" && time.Now().Before(m.statusUntil) {
		if m.statusErr {
			return errorStyle.Render(pad(" "+m.status, m.width))
		}
		return infoStyle.Render(pad(" "+m.status, m.width))
	}
	hint := " ? help  / search  ⏎ open/play  space play/pause  n/p next/prev  d devices  q quit"
	return dimStyle.Render(pad(hint, m.width))
}
