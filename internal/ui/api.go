package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zmb3/spotify/v2"
)

const pageSize = 50

// Messages produced by API commands.
type (
	playerStateMsg struct {
		state *spotify.PlayerState
		err   error
	}
	pageLoadedMsg struct {
		target *page
		items  []item
		total  int
		offset int
		after  string // cursor for artist pagination
		err    error
	}
	searchDoneMsg struct {
		target  *page
		results [4][]item
		err     error
	}
	playlistsMsg struct {
		items []item
		err   error
	}
	userMsg struct {
		user *spotify.PrivateUser
		err  error
	}
	actionDoneMsg struct {
		what string
		err  error
	}
	tickMsg time.Time
	pollMsg time.Time
)

func apiCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

// friendlyErr rewrites the errors people actually hit into something that
// tells them what to do.
func friendlyErr(err error) error {
	if err == nil {
		return nil
	}
	var se spotify.Error
	if errors.As(err, &se) {
		switch {
		case se.Status == 404 && strings.Contains(strings.ToLower(se.Message), "device"):
			return errors.New("no active device. Open Spotify somewhere, then press d to pick it")
		case se.Status == 403 && strings.Contains(strings.ToLower(se.Message), "premium"):
			return errors.New("Spotify Premium is required to control playback")
		case se.Status == 429:
			return errors.New("rate limited by Spotify, slow down a little")
		case se.Status == 401:
			return errors.New("token rejected, run `spotify-tui logout` and log in again")
		}
		return errors.New(se.Message)
	}
	return err
}

func fetchPlayerState(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		st, err := c.PlayerState(ctx)
		return playerStateMsg{state: st, err: friendlyErr(err)}
	}
}

func fetchUser(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		u, err := c.CurrentUser(ctx)
		return userMsg{user: u, err: friendlyErr(err)}
	}
}

// fetchPlaylists walks every page of the user's playlists for the sidebar.
func fetchPlaylists(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		pg, err := c.CurrentUsersPlaylists(ctx, spotify.Limit(pageSize))
		if err != nil {
			return playlistsMsg{err: friendlyErr(err)}
		}
		var items []item
		for {
			for _, p := range pg.Playlists {
				items = append(items, playlistItem(p))
			}
			if err := c.NextPage(ctx, pg); err != nil {
				if errors.Is(err, spotify.ErrNoMorePages) {
					break
				}
				return playlistsMsg{items: items, err: friendlyErr(err)}
			}
		}
		return playlistsMsg{items: items}
	}
}

// Loaders: each returns a loader closure bound to a page so the page can
// pull more rows as the cursor approaches the end.

func likedLoader(c *spotify.Client, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			pg, err := c.CurrentUsersTracks(ctx, spotify.Limit(pageSize), spotify.Offset(offset))
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			items := make([]item, 0, len(pg.Tracks))
			for _, t := range pg.Tracks {
				items = append(items, trackItem(t.FullTrack))
			}
			return pageLoadedMsg{target: target, items: items, total: int(pg.Total), offset: offset}
		}
	}
}

func playlistLoader(c *spotify.Client, id spotify.ID, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			pg, err := c.GetPlaylistItems(ctx, id, spotify.Limit(pageSize), spotify.Offset(offset))
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			items := make([]item, 0, len(pg.Items))
			for _, pi := range pg.Items {
				if pi.Track.Track == nil {
					continue // episode or unavailable track
				}
				items = append(items, trackItem(*pi.Track.Track))
			}
			return pageLoadedMsg{target: target, items: items, total: int(pg.Total), offset: offset}
		}
	}
}

func albumLoader(c *spotify.Client, album spotify.SimpleAlbum, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			pg, err := c.GetAlbumTracks(ctx, album.ID, spotify.Limit(pageSize), spotify.Offset(offset))
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			items := make([]item, 0, len(pg.Tracks))
			for _, t := range pg.Tracks {
				items = append(items, simpleTrackItem(t, album))
			}
			return pageLoadedMsg{target: target, items: items, total: int(pg.Total), offset: offset}
		}
	}
}

func savedAlbumsLoader(c *spotify.Client, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			pg, err := c.CurrentUsersAlbums(ctx, spotify.Limit(pageSize), spotify.Offset(offset))
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			items := make([]item, 0, len(pg.Albums))
			for _, a := range pg.Albums {
				items = append(items, albumItem(a.SimpleAlbum))
			}
			return pageLoadedMsg{target: target, items: items, total: int(pg.Total), offset: offset}
		}
	}
}

func artistAlbumsLoader(c *spotify.Client, id spotify.ID, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			types := []spotify.AlbumType{spotify.AlbumTypeAlbum, spotify.AlbumTypeSingle}
			pg, err := c.GetArtistAlbums(ctx, id, types, spotify.Limit(pageSize), spotify.Offset(offset))
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			items := make([]item, 0, len(pg.Albums))
			for _, a := range pg.Albums {
				items = append(items, albumItem(a))
			}
			return pageLoadedMsg{target: target, items: items, total: int(pg.Total), offset: offset}
		}
	}
}

// followedArtistsLoader paginates with a cursor rather than an offset. The
// page stores the cursor in its "after" field between calls.
func followedArtistsLoader(c *spotify.Client, target *page) loader {
	return func(offset int) tea.Cmd {
		after := target.after
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			opts := []spotify.RequestOption{spotify.Limit(pageSize)}
			if after != "" {
				opts = append(opts, spotify.After(after))
			}
			pg, err := c.CurrentUsersFollowedArtists(ctx, opts...)
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			items := make([]item, 0, len(pg.Artists))
			for _, a := range pg.Artists {
				items = append(items, artistItem(a))
			}
			return pageLoadedMsg{target: target, items: items, total: int(pg.Total), offset: offset, after: pg.Cursor.After}
		}
	}
}

func fetchArtistTop(c *spotify.Client, id spotify.ID, country string, target *page) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		if country == "" {
			country = "from_token"
		}
		tracks, err := c.GetArtistsTopTracks(ctx, id, country)
		if err != nil {
			return pageLoadedMsg{target: target, err: friendlyErr(err)}
		}
		items := make([]item, 0, len(tracks))
		for _, t := range tracks {
			items = append(items, trackItem(t))
		}
		return pageLoadedMsg{target: target, items: items, total: len(items)}
	}
}

func fetchDevices(c *spotify.Client, target *page) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		devs, err := c.PlayerDevices(ctx)
		if err != nil {
			return pageLoadedMsg{target: target, err: friendlyErr(err)}
		}
		items := make([]item, 0, len(devs))
		for _, d := range devs {
			items = append(items, deviceItem(d))
		}
		return pageLoadedMsg{target: target, items: items, total: len(items)}
	}
}

func fetchQueue(c *spotify.Client, target *page) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		q, err := c.GetQueue(ctx)
		if err != nil {
			return pageLoadedMsg{target: target, err: friendlyErr(err)}
		}
		items := make([]item, 0, len(q.Items))
		for _, t := range q.Items {
			items = append(items, trackItem(t))
		}
		return pageLoadedMsg{target: target, items: items, total: len(items)}
	}
}

func runSearch(c *spotify.Client, query string, target *page) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		kinds := spotify.SearchTypeTrack | spotify.SearchTypeArtist | spotify.SearchTypeAlbum | spotify.SearchTypePlaylist
		res, err := c.Search(ctx, query, kinds, spotify.Limit(20))
		if err != nil {
			return searchDoneMsg{target: target, err: friendlyErr(err)}
		}
		var out [4][]item
		if res.Tracks != nil {
			for _, t := range res.Tracks.Tracks {
				out[0] = append(out[0], trackItem(t))
			}
		}
		if res.Artists != nil {
			for _, a := range res.Artists.Artists {
				out[1] = append(out[1], artistItem(a))
			}
		}
		if res.Albums != nil {
			for _, a := range res.Albums.Albums {
				out[2] = append(out[2], albumItem(a))
			}
		}
		if res.Playlists != nil {
			for _, p := range res.Playlists.Playlists {
				if p.ID == "" {
					continue // Spotify returns null entries in playlist search
				}
				out[3] = append(out[3], playlistItem(p))
			}
		}
		return searchDoneMsg{target: target, results: out}
	}
}

// Player controls. Each one reports back through actionDoneMsg so the model
// can refresh state and surface errors.

func action(what string, f func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		return actionDoneMsg{what: what, err: friendlyErr(f(ctx))}
	}
}

func playContext(c *spotify.Client, contextURI spotify.URI, track spotify.URI) tea.Cmd {
	return action("play", func(ctx context.Context) error {
		opts := &spotify.PlayOptions{PlaybackContext: &contextURI}
		if track != "" {
			opts.PlaybackOffset = &spotify.PlaybackOffset{URI: track}
		}
		return c.PlayOpt(ctx, opts)
	})
}

// playURIs plays a flat list of tracks starting at index. Used for search
// results and other lists that are not a Spotify context.
func playURIs(c *spotify.Client, uris []spotify.URI, index int) tea.Cmd {
	return action("play", func(ctx context.Context) error {
		return c.PlayOpt(ctx, &spotify.PlayOptions{
			URIs:           uris,
			PlaybackOffset: &spotify.PlaybackOffset{Position: &index},
		})
	})
}

func resume(c *spotify.Client) tea.Cmd {
	return action("play", func(ctx context.Context) error { return c.Play(ctx) })
}

func pause(c *spotify.Client) tea.Cmd {
	return action("pause", func(ctx context.Context) error { return c.Pause(ctx) })
}

func next(c *spotify.Client) tea.Cmd {
	return action("next", func(ctx context.Context) error { return c.Next(ctx) })
}

func previous(c *spotify.Client) tea.Cmd {
	return action("previous", func(ctx context.Context) error { return c.Previous(ctx) })
}

func seek(c *spotify.Client, pos time.Duration) tea.Cmd {
	if pos < 0 {
		pos = 0
	}
	return action("seek", func(ctx context.Context) error { return c.Seek(ctx, int(pos.Milliseconds())) })
}

func setShuffle(c *spotify.Client, on bool) tea.Cmd {
	return action("shuffle", func(ctx context.Context) error { return c.Shuffle(ctx, on) })
}

func setRepeat(c *spotify.Client, state string) tea.Cmd {
	return action("repeat", func(ctx context.Context) error { return c.Repeat(ctx, state) })
}

func setVolume(c *spotify.Client, pct int) tea.Cmd {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return action("volume", func(ctx context.Context) error { return c.Volume(ctx, pct) })
}

func transfer(c *spotify.Client, id spotify.ID) tea.Cmd {
	return action("transfer", func(ctx context.Context) error { return c.TransferPlayback(ctx, id, true) })
}

func addToQueue(c *spotify.Client, id spotify.ID) tea.Cmd {
	return action("queue", func(ctx context.Context) error { return c.QueueSong(ctx, id) })
}

func saveTrack(c *spotify.Client, id spotify.ID) tea.Cmd {
	return action("save", func(ctx context.Context) error { return c.AddTracksToLibrary(ctx, id) })
}

func unsaveTrack(c *spotify.Client, id spotify.ID) tea.Cmd {
	return action("unsave", func(ctx context.Context) error { return c.RemoveTracksFromLibrary(ctx, id) })
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func poll(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return pollMsg(t) })
}
