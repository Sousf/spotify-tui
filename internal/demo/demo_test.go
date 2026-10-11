package demo

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/zmb3/spotify/v2"
)

// clock is a time source the test moves by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestDemoAnswersTheClient(t *testing.T) {
	ck := &clock{t: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	c := spotify.New(&http.Client{Transport: newServer(ck.now)})
	ctx := context.Background()

	u, err := c.CurrentUser(ctx)
	if err != nil || u.ID != "demo" {
		t.Fatalf("CurrentUser = %v, %v", u, err)
	}
	liked, err := c.CurrentUsersTracks(ctx, spotify.Limit(50))
	if err != nil || len(liked.Tracks) == 0 || liked.Tracks[0].Name == "" {
		t.Fatalf("liked tracks: %v, %v", liked, err)
	}
	res, err := c.Search(ctx, "harbour", spotify.SearchTypeTrack|spotify.SearchTypeArtist|spotify.SearchTypeAlbum|spotify.SearchTypePlaylist)
	if err != nil || len(res.Artists.Artists) == 0 || len(res.Albums.Albums) == 0 {
		t.Fatalf("search: %+v, %v", res, err)
	}

	st, err := c.PlayerState(ctx)
	if err != nil || !st.Playing || st.Item == nil {
		t.Fatalf("player state: %+v, %v", st, err)
	}
	first, at := st.Item.ID, st.Progress
	ck.t = ck.t.Add(5 * time.Second)
	if st, _ = c.PlayerState(ctx); st.Progress != at+5000 {
		t.Errorf("progress %d after 5s, want %d", st.Progress, at+5000)
	}

	if err := c.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	ck.t = ck.t.Add(time.Minute)
	if st, _ = c.PlayerState(ctx); st.Playing || st.Progress != at+5000 {
		t.Errorf("paused: playing=%v progress=%d", st.Playing, st.Progress)
	}
	if err := c.Next(ctx); err != nil {
		t.Fatal(err)
	}
	if st, _ = c.PlayerState(ctx); st.Item.ID == first || st.Progress != 0 {
		t.Errorf("next: still on %s at %d", st.Item.ID, st.Progress)
	}

	// Left playing, a track rolls over to the next one at its end.
	if err := c.Play(ctx); err != nil {
		t.Fatal(err)
	}
	second := st.Item.ID
	ck.t = ck.t.Add(time.Duration(st.Item.Duration+1000) * time.Millisecond)
	if st, _ = c.PlayerState(ctx); st.Item.ID == second || st.Progress != 1000 {
		t.Errorf("rollover: on %s at %d", st.Item.ID, st.Progress)
	}
}
