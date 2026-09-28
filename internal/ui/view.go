package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/zmb3/spotify/v2"
)

func (m Model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	paneH := m.height - bottomHeight
	if paneH < 5 {
		return "terminal too small"
	}
	contentW := m.width - sidebarWidth
	if contentW < 20 {
		return "terminal too narrow"
	}

	var top string
	if m.showHelp {
		top = m.renderHelp(m.width, paneH)
	} else {
		left := m.renderLibrary(sidebarWidth, paneH)
		var right string
		if m.showViz {
			right = m.renderViz(contentW, paneH)
		} else {
			right = m.renderContent(contentW, paneH)
		}
		top = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, m.renderNowPlaying(), m.statusLine())
}

func (m Model) paneStyleFor(f focus) lipgloss.Style {
	if m.focus == f && !m.showHelp {
		return focusedPaneStyle
	}
	return paneStyle
}

func (m Model) renderLibrary(w, h int) string {
	inner := w - 2
	rows := []string{titleStyle.Render(pad("Library", inner))}
	p := m.library
	listH := h - 2 - 1
	end := p.offset + listH
	if end > len(p.items) {
		end = len(p.items)
	}
	for i := p.offset; i < end; i++ {
		it := p.items[i]
		var line string
		switch {
		case it.header:
			line = sectionStyle.Render(pad("── "+it.title+" ", inner))
		case i == p.cursor && m.focus == focusLibrary:
			line = cursorStyle.Render(pad("▸ "+it.title, inner))
		case i == p.cursor:
			line = pad("▸ "+it.title, inner)
		default:
			line = pad("  "+it.title, inner)
		}
		rows = append(rows, line)
	}
	for len(rows) < h-2 {
		rows = append(rows, strings.Repeat(" ", inner))
	}
	return m.paneStyleFor(focusLibrary).Width(inner).Height(h - 2).Render(strings.Join(rows, "\n"))
}

func (m Model) renderContent(w, h int) string {
	inner := w - 2
	p := m.top()
	var rows []string
	if p == nil {
		return m.paneStyleFor(focusContent).Width(inner).Height(h - 2).Render("")
	}

	// Title row: name on the left, count on the right.
	count := ""
	switch {
	case p.loading && len(p.items) == 0:
		count = "loading…"
	case p.total > 0 && p.kind != pageSearch:
		count = fmt.Sprintf("%d / %d", len(p.items), p.total)
	case len(p.items) > 0:
		count = fmt.Sprintf("%d", len(p.items))
	}
	if p.loading && len(p.items) > 0 {
		count += " …"
	}
	title := p.title
	if len(m.stack) > 1 {
		title = dimStyle.Render("‹ ") + title
	}
	countW := runewidth.StringWidth(count)
	rows = append(rows, titleStyle.Render(pad(title, inner-countW-1))+" "+dimStyle.Render(count))

	if p.kind == pageSearch {
		if m.focus == focusSearch {
			rows = append(rows, pad(m.search.View(), inner))
		} else {
			rows = append(rows, pad(dimStyle.Render("/ ")+p.query, inner))
		}
		names := []string{"Tracks", "Artists", "Albums", "Playlists"}
		var tabs []string
		for i, n := range names {
			label := fmt.Sprintf("%d %s (%d)", i+1, n, len(p.searchResults[i]))
			if i == p.searchTab {
				tabs = append(tabs, activeTab.Render(label))
			} else {
				tabs = append(tabs, tabStyle.Render(label))
			}
		}
		rows = append(rows, pad(lipgloss.JoinHorizontal(lipgloss.Top, tabs...), inner))
	} else if m.focus == focusSearch {
		rows = append(rows, pad(m.search.View(), inner))
	}

	cols := m.columns(p, inner)
	if hdr := cols.header(); hdr != "" {
		rows = append(rows, headerStyle.Render(pad(hdr, inner)))
	}

	listH := h - 2 - len(rows)
	if listH < 1 {
		listH = 1
	}
	var nowID spotify.ID
	if m.state != nil && m.state.Item != nil {
		nowID = m.state.Item.ID
	}
	end := p.offset + listH
	if end > len(p.items) {
		end = len(p.items)
	}
	if len(p.items) == 0 {
		msg := "nothing here"
		if p.loading {
			msg = "loading…"
		} else if p.loadErr != nil {
			msg = p.loadErr.Error()
			if p.context != "" {
				msg += ". Press enter to play it anyway"
			}
		} else if p.kind == pageDevices {
			msg = "no devices found. Open Spotify on a phone or computer and press R"
		} else if p.kind == pageQueue {
			msg = "queue is empty"
		}
		rows = append(rows, dimStyle.Render(pad("  "+msg, inner)))
	}
	for i := p.offset; i < end; i++ {
		it := p.items[i]
		line := cols.row(i, it, nowID)
		switch {
		case i == p.cursor && m.focus == focusContent:
			line = cursorStyle.Render(pad(line, inner))
		case it.id == nowID && it.kind == kindTrack, it.active:
			line = playingStyle.Render(pad(line, inner))
		default:
			line = pad(line, inner)
		}
		rows = append(rows, line)
	}
	for len(rows) < h-2 {
		rows = append(rows, strings.Repeat(" ", inner))
	}
	return m.paneStyleFor(focusContent).Width(inner).Height(h - 2).Render(strings.Join(rows, "\n"))
}

// columns describes how a page's rows are laid out.
type columns struct {
	kind  pageKind
	width int
	w     [5]int // per-column widths
}

func (m Model) columns(p *page, width int) columns {
	c := columns{kind: p.kind, width: width}
	switch p.kind {
	case pageTracks, pageQueue, pageSearch:
		isTracks := p.kind != pageSearch || p.searchTab == 0
		if isTracks {
			c.kind = pageTracks
			num, dur := 5, 6
			rest := width - num - dur - 4 // four single-space separators
			c.w = [5]int{num, rest * 45 / 100, rest * 30 / 100, 0, dur}
			c.w[3] = rest - c.w[1] - c.w[2]
			return c
		}
		switch p.searchTab {
		case 1:
			c.kind = pageArtists
		case 2:
			c.kind = pageAlbums
		case 3:
			c.kind = pagePlaylists
		}
	}
	switch c.kind {
	case pageAlbums:
		year := 6
		rest := width - 2 - year - 3
		c.w = [5]int{2, rest * 55 / 100, 0, year}
		c.w[2] = rest - c.w[1]
	case pageArtists:
		c.w = [5]int{2, (width - 4) * 60 / 100, 0}
		c.w[2] = width - 4 - c.w[1]
	case pagePlaylists:
		cnt := 12
		rest := width - 2 - cnt - 3
		c.w = [5]int{2, rest * 60 / 100, 0, cnt}
		c.w[2] = rest - c.w[1]
	case pageDevices:
		rest := width - 4 - 10 - 3
		c.w = [5]int{4, rest / 2, rest - rest/2, 10}
	}
	return c
}

func (c columns) header() string {
	switch c.kind {
	case pageTracks:
		return join(pad("#", c.w[0]), pad("Title", c.w[1]), pad("Artist", c.w[2]), pad("Album", c.w[3]), pad("Time", c.w[4]))
	case pageAlbums:
		return join(pad("", c.w[0]), pad("Album", c.w[1]), pad("Artist", c.w[2]), pad("Year", c.w[3]))
	case pagePlaylists:
		return join(pad("", c.w[0]), pad("Playlist", c.w[1]), pad("Owner", c.w[2]), pad("Tracks", c.w[3]))
	case pageDevices:
		return join(pad("", c.w[0]), pad("Device", c.w[1]), pad("Type", c.w[2]), pad("Volume", c.w[3]))
	}
	return ""
}

func (c columns) row(i int, it item, nowID spotify.ID) string {
	switch c.kind {
	case pageTracks:
		num := fmt.Sprintf("%d", i+1)
		if it.id == nowID {
			num = "▶"
		}
		return join(pad(" "+num, c.w[0]), pad(it.title, c.w[1]), pad(it.sub, c.w[2]), pad(it.extra, c.w[3]), pad(fmtDur(it.dur), c.w[4]))
	case pageAlbums:
		return join(pad("", c.w[0]), pad(it.title, c.w[1]), pad(it.sub, c.w[2]), pad(it.extra, c.w[3]))
	case pageArtists:
		return join(pad("", c.w[0]), pad(it.title, c.w[1]), pad(it.sub, c.w[2]))
	case pagePlaylists:
		return join(pad("", c.w[0]), pad(it.title, c.w[1]), pad(it.sub, c.w[2]), pad(it.extra, c.w[3]))
	case pageDevices:
		mark := "  "
		if it.active {
			mark = " ●"
		}
		return join(pad(mark, c.w[0]), pad(it.title, c.w[1]), pad(it.sub, c.w[2]), pad(it.extra, c.w[3]))
	}
	return pad(it.title, c.width)
}

func join(cols ...string) string {
	return strings.Join(cols, " ")
}

func (m Model) renderNowPlaying() string {
	w := m.width
	st := m.state
	if st == nil || st.Item == nil {
		line1 := dimStyle.Render(pad("  nothing playing", w))
		line2 := progressTrack.Render(strings.Repeat("─", w))
		return line1 + "\n" + line2
	}
	icon := "▶"
	if !st.Playing {
		icon = "⏸"
	}
	left := fmt.Sprintf(" %s %s  %s", icon, st.Item.Name, dimStyle.Render(artistNames(st.Item.Artists)+" · "+st.Item.Album.Name))

	var flags []string
	if st.ShuffleState {
		flags = append(flags, accentStyle.Render("⇄ shuffle"))
	} else {
		flags = append(flags, dimStyle.Render("⇄ shuffle"))
	}
	switch st.RepeatState {
	case "context":
		flags = append(flags, accentStyle.Render("↻ repeat"))
	case "track":
		flags = append(flags, accentStyle.Render("↻ track"))
	default:
		flags = append(flags, dimStyle.Render("↻ repeat"))
	}
	flags = append(flags, fmt.Sprintf("♪ %d%%", st.Device.Volume))
	if st.Device.Name != "" {
		flags = append(flags, dimStyle.Render(st.Device.Name))
	}
	right := strings.Join(flags, "  ") + " "
	rightW := lipgloss.Width(right)
	line1 := pad(left, w-rightW-1) + " " + right

	total := st.Item.TimeDuration()
	elapsed := fmtDur(m.progress)
	length := fmtDur(total)
	barW := w - len(elapsed) - len(length) - 4
	if barW < 1 {
		barW = 1
	}
	filled := 0
	if total > 0 {
		filled = int(float64(barW) * float64(m.progress) / float64(total))
	}
	if filled > barW {
		filled = barW
	}
	bar := progressFill.Render(strings.Repeat("━", filled)) + progressTrack.Render(strings.Repeat("─", barW-filled))
	line2 := fmt.Sprintf(" %s %s %s ", elapsed, bar, length)
	return line1 + "\n" + line2
}

func (m Model) renderHelp(w, h int) string {
	type row struct{ k, d string }
	sections := []struct {
		name string
		rows []row
	}{
		{"Navigation", []row{
			{"j / k", "move down / up"},
			{"g / G", "top / bottom"},
			{"ctrl+d / ctrl+u", "half page down / up"},
			{"tab, h / l", "switch library / content"},
			{"enter", "open item, or play track"},
			{"esc, backspace", "go back"},
			{"/", "search"},
			{"1-4, [ ]", "switch search result tab"},
			{"R", "reload current page"},
		}},
		{"Playback", []row{
			{"space", "play / pause"},
			{"n / p", "next / previous track"},
			{"> / <", "seek forward / back 10s"},
			{"+ / -", "volume up / down"},
			{"s", "toggle shuffle"},
			{"r", "cycle repeat"},
			{"d", "devices"},
			{"u", "queue"},
			{"v", "toggle visualiser"},
		}},
		{"Selected track", []row{
			{"a", "add to queue"},
			{"f / F", "save / unsave (Liked Songs)"},
			{"e", "go to artist"},
			{"b", "go to album"},
		}},
	}
	inner := w - 2
	var blocks []string
	for _, s := range sections {
		var b strings.Builder
		b.WriteString(sectionStyle.Render(s.name) + "\n")
		for _, r := range s.rows {
			b.WriteString("  " + helpKeyStyle.Render(pad(r.k, 17)) + r.d + "\n")
		}
		blocks = append(blocks, strings.TrimRight(b.String(), "\n"))
	}
	// Three columns, two columns, or stacked, depending on the pane width.
	var body string
	sep := strings.Repeat(" ", 3)
	widthOf := func(bs ...string) int {
		n := len(sep) * (len(bs) - 1)
		for _, b := range bs {
			n += lipgloss.Width(b)
		}
		return n
	}
	right := blocks[1] + "\n\n" + blocks[2]
	switch {
	case widthOf(blocks...) <= inner:
		body = lipgloss.JoinHorizontal(lipgloss.Top, blocks[0], sep, blocks[1], sep, blocks[2])
	case widthOf(blocks[0], right) <= inner:
		body = lipgloss.JoinHorizontal(lipgloss.Top, blocks[0], sep, right)
	default:
		body = strings.Join(blocks, "\n\n")
	}
	rows := []string{titleStyle.Render(pad("Keys", inner-22)) + dimStyle.Render(pad("any key closes", 22))}
	for _, l := range strings.Split(body, "\n") {
		rows = append(rows, pad(l, inner))
	}
	if len(rows) > h-2 {
		rows = rows[:h-2]
	}
	for len(rows) < h-2 {
		rows = append(rows, strings.Repeat(" ", inner))
	}
	return paneStyle.Width(inner).Height(h - 2).Render(strings.Join(rows, "\n"))
}

var vizPartial = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// vizColor grades a bar from green at the bottom to red at the top.
func vizColor(frac float64) lipgloss.Style {
	switch {
	case frac > 0.85:
		return lipgloss.NewStyle().Foreground(red)
	case frac > 0.6:
		return lipgloss.NewStyle().Foreground(yellow)
	default:
		return lipgloss.NewStyle().Foreground(green)
	}
}

func (m Model) renderViz(w, h int) string {
	inner := w - 2
	rows := []string{titleStyle.Render(pad("Visualiser", inner-14)) + dimStyle.Render(pad("v to close", 14))}
	height := h - 2 - len(rows)
	if height < 1 {
		height = 1
	}
	if m.vizErr != nil {
		rows = append(rows, errorStyle.Render(pad("  "+m.vizErr.Error(), inner)))
	} else if m.viz == nil || len(m.vizLevels) == 0 {
		rows = append(rows, dimStyle.Render(pad("  listening…", inner)))
	} else {
		n := len(m.vizLevels)
		// Each bar is one cell wide with a one-cell gap, centred in the pane.
		used := n*2 - 1
		lead := (inner - used) / 2
		if lead < 0 {
			lead = 0
		}
		// Pre-render each bar as a column of runes, top row first.
		cols := make([][]string, n)
		for b := 0; b < n; b++ {
			level := m.vizLevels[b]
			peak := m.vizPeaks[b]
			eighths := int(level*float64(height)*8 + 0.5)
			peakRow := height - 1 - int(peak*float64(height-1)+0.5)
			col := make([]string, height)
			for r := 0; r < height; r++ {
				fromBottom := height - 1 - r // rows counted from the floor
				full := eighths / 8
				frac := float64(fromBottom+1) / float64(height)
				var ch rune
				switch {
				case fromBottom < full:
					ch = '█'
				case fromBottom == full:
					ch = vizPartial[eighths%8]
				default:
					ch = ' '
				}
				cell := string(ch)
				if ch != ' ' {
					cell = vizColor(frac).Render(cell)
				} else if r == peakRow && peak > 0.02 {
					cell = dimStyle.Render("▔")
				}
				col[r] = cell
			}
			cols[b] = col
		}
		for r := 0; r < height; r++ {
			var line strings.Builder
			line.WriteString(strings.Repeat(" ", lead))
			for b := 0; b < n; b++ {
				if b > 0 {
					line.WriteByte(' ')
				}
				line.WriteString(cols[b][r])
			}
			rows = append(rows, pad(line.String(), inner))
		}
	}
	for len(rows) < h-2 {
		rows = append(rows, strings.Repeat(" ", inner))
	}
	return m.paneStyleFor(focusContent).Width(inner).Height(h - 2).Render(strings.Join(rows, "\n"))
}
