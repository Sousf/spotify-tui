package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zmb3/spotify/v2"
)

const (
	pageSize = 50
	// Spotify caps artist albums and search at 10 per page for new apps.
	smallPageSize = 10
)

// errForbidden is what development-mode apps get for content Spotify has
// walled off, most visibly playlists owned by other users.
var errForbidden = errors.New("Spotify blocks development-mode apps from reading this")

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
		case se.Status == 403:
			return errForbidden
		case se.Status == 429:
			return errors.New("rate limited by Spotify, slow down a little")
		case se.Status == 401:
			return errors.New("token rejected, run `spotify-tui logout` and log in again")
		}
		return errors.New(se.Message)
	}
	return err
}

func fetchPlayerState(c *api) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		st, err := c.PlayerState(ctx)
		return playerStateMsg{state: st, err: friendlyErr(err)}
	}
}

func fetchUser(c *api) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		u, err := c.CurrentUser(ctx)
		return userMsg{user: u, err: friendlyErr(err)}
	}
}

// fetchPlaylists walks every page of the user's playlists for the sidebar.
func fetchPlaylists(c *api) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var items []item
		for offset := 0; ; offset += pageSize {
			pg, err := c.myPlaylists(ctx, pageSize, offset)
			if err != nil {
				return playlistsMsg{items: items, err: friendlyErr(err)}
			}
			for _, p := range pg.Items {
				items = append(items, item{
					kind:  kindPlaylist,
					id:    p.ID,
					uri:   p.URI,
					title: p.Name,
					sub:   p.Owner.DisplayName,
					extra: fmt.Sprintf("%d tracks", p.Items.Total),
				})
			}
			if pg.Next == "" || len(pg.Items) == 0 {
				break
			}
		}
		return playlistsMsg{items: items}
	}
}

// Loaders: each returns a loader closure bound to a page so the page can
// pull more rows as the cursor approaches the end.

func likedLoader(c *api, target *page) loader {
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

func playlistLoader(c *api, id spotify.ID, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			items, total, err := c.playlistItems(ctx, id, pageSize, offset)
			if err != nil {
				return pageLoadedMsg{target: target, err: friendlyErr(err)}
			}
			return pageLoadedMsg{target: target, items: items, total: total, offset: offset}
		}
	}
}

func albumLoader(c *api, album spotify.SimpleAlbum, target *page) loader {
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

func savedAlbumsLoader(c *api, target *page) loader {
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

func artistAlbumsLoader(c *api, id spotify.ID, target *page) loader {
	return func(offset int) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := apiCtx()
			defer cancel()
			types := []spotify.AlbumType{spotify.AlbumTypeAlbum, spotify.AlbumTypeSingle}
			pg, err := c.GetArtistAlbums(ctx, id, types, spotify.Limit(smallPageSize), spotify.Offset(offset))
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
func followedArtistsLoader(c *api, target *page) loader {
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

func fetchDevices(c *api, target *page) tea.Cmd {
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

func fetchQueue(c *api, target *page) tea.Cmd {
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

func runSearch(c *api, query string, target *page) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := apiCtx()
		defer cancel()
		kinds := spotify.SearchTypeTrack | spotify.SearchTypeArtist | spotify.SearchTypeAlbum | spotify.SearchTypePlaylist
		res, err := c.Search(ctx, query, kinds, spotify.Limit(smallPageSize))
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

func playContext(c *api, contextURI spotify.URI, track spotify.URI) tea.Cmd {
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
func playURIs(c *api, uris []spotify.URI, index int) tea.Cmd {
	return action("play", func(ctx context.Context) error {
		return c.PlayOpt(ctx, &spotify.PlayOptions{
			URIs:           uris,
			PlaybackOffset: &spotify.PlaybackOffset{Position: &index},
		})
	})
}

func resume(c *api) tea.Cmd {
	return action("play", func(ctx context.Context) error { return c.Play(ctx) })
}

func pause(c *api) tea.Cmd {
	return action("pause", func(ctx context.Context) error { return c.Pause(ctx) })
}

func next(c *api) tea.Cmd {
	return action("next", func(ctx context.Context) error { return c.Next(ctx) })
}

func previous(c *api) tea.Cmd {
	return action("previous", func(ctx context.Context) error { return c.Previous(ctx) })
}

func seek(c *api, pos time.Duration) tea.Cmd {
	if pos < 0 {
		pos = 0
	}
	return action("seek", func(ctx context.Context) error { return c.Seek(ctx, int(pos.Milliseconds())) })
}

func setShuffle(c *api, on bool) tea.Cmd {
	return action("shuffle", func(ctx context.Context) error { return c.Shuffle(ctx, on) })
}

func setRepeat(c *api, state string) tea.Cmd {
	return action("repeat", func(ctx context.Context) error { return c.Repeat(ctx, state) })
}

func setVolume(c *api, pct int) tea.Cmd {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return action("volume", func(ctx context.Context) error { return c.Volume(ctx, pct) })
}

func transfer(c *api, id spotify.ID) tea.Cmd {
	return action("transfer", func(ctx context.Context) error { return c.TransferPlayback(ctx, id, true) })
}

func addToQueue(c *api, id spotify.ID) tea.Cmd {
	return action("queue", func(ctx context.Context) error { return c.QueueSong(ctx, id) })
}

func saveTrack(c *api, id spotify.ID) tea.Cmd {
	return action("save", func(ctx context.Context) error { return c.AddTracksToLibrary(ctx, id) })
}

func unsaveTrack(c *api, id spotify.ID) tea.Cmd {
	return action("unsave", func(ctx context.Context) error { return c.RemoveTracksFromLibrary(ctx, id) })
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func poll(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return pollMsg(t) })
}
