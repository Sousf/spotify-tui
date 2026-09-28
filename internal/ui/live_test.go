package ui

import (
	"context"
	"io"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zmb3/spotify/v2"

	"github.com/Sousf/spotify-tui/internal/auth"
)

// TestLive exercises the loaders against the real API using the cached
// login. Run with SPOTIFY_TUI_LIVE=1; it is skipped otherwise.
func TestLive(t *testing.T) {
	if os.Getenv("SPOTIFY_TUI_LIVE") == "" {
		t.Skip("set SPOTIFY_TUI_LIVE=1 to hit the real API")
	}
	cfg, err := auth.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	hc, err := auth.Login(context.Background(), cfg, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	c := newAPI(hc)

	load := func(name string, cmd func() tea.Msg) pageLoadedMsg {
		msg := cmd().(pageLoadedMsg)
		if msg.err != nil {
			t.Fatalf("%s: %v", name, msg.err)
		}
		t.Logf("%-16s %3d items, total %d", name, len(msg.items), msg.total)
		return msg
	}

	p := &page{}
	load("liked", likedLoader(c, p)(0))
	load("saved albums", savedAlbumsLoader(c, p)(0))
	artists := load("followed", followedArtistsLoader(c, p)(0))
	load("devices", fetchDevices(c, p))
	load("queue", fetchQueue(c, p))

	pl := fetchPlaylists(c)().(playlistsMsg)
	if pl.err != nil {
		t.Fatal(pl.err)
	}
	t.Logf("%-16s %3d items", "playlists", len(pl.items))
	// Development-mode apps get 403 for playlists owned by other users, so
	// find one that is readable.
	var tracks pageLoadedMsg
	for _, it := range pl.items {
		tracks = playlistLoader(c, it.id, p)(0)().(pageLoadedMsg)
		if tracks.err == nil && len(tracks.items) > 0 {
			t.Logf("%-16s %3d items, total %d (%s)", "playlist tracks", len(tracks.items), tracks.total, it.title)
			break
		}
	}
	{
		if len(tracks.items) > 0 {
			it := tracks.items[0]
			load("album tracks", albumLoader(c, spotify.SimpleAlbum{ID: it.albumID, Name: it.extra}, p)(0))
			load("artist albums", artistAlbumsLoader(c, it.artistID, p)(0))
		}
	}
	if len(artists.items) > 0 {
		load("artist albums 2", artistAlbumsLoader(c, artists.items[0].id, p)(0))
	}

	s := runSearch(c, "daft punk", p)().(searchDoneMsg)
	if s.err != nil {
		t.Fatal(s.err)
	}
	t.Logf("%-16s tracks %d artists %d albums %d playlists %d", "search", len(s.results[0]), len(s.results[1]), len(s.results[2]), len(s.results[3]))
}
