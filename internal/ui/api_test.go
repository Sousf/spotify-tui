package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zmb3/spotify/v2"
)

func TestPlaylistLoaderParsesTracks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playlists/abc/items" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("offset"); got != "50" {
			t.Errorf("offset = %q, want 50", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total": 120, "items": [
			{"item": {"type":"track","id":"t1","uri":"spotify:track:t1","name":"One","duration_ms":61000,
			  "artists":[{"id":"a1","name":"Ann"},{"id":"a2","name":"Bob"}],"album":{"id":"al1","name":"Alb"}}},
			{"item": null},
			{"item": {"type":"episode","id":"e1","name":"Pod"}}
		]}`))
	}))
	defer srv.Close()

	c := &api{Client: spotify.New(srv.Client(), spotify.WithBaseURL(srv.URL+"/")), http: srv.Client(), base: srv.URL + "/"}
	p := &page{kind: pageTracks}
	msg := playlistLoader(c, "abc", p)(50)().(pageLoadedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg.total != 120 || msg.offset != 50 {
		t.Errorf("total/offset = %d/%d", msg.total, msg.offset)
	}
	if len(msg.items) != 1 {
		t.Fatalf("got %d items, want 1 (null and episode entries skipped)", len(msg.items))
	}
	it := msg.items[0]
	if it.title != "One" || it.sub != "Ann, Bob" || it.extra != "Alb" || it.artistID != "a1" || it.albumID != "al1" {
		t.Errorf("unexpected item %+v", it)
	}
	if fmtDur(it.dur) != "1:01" {
		t.Errorf("duration = %s", fmtDur(it.dur))
	}
}

func TestFriendlyErrNoDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":{"status":404,"message":"Player command failed: No active device found"}}`))
	}))
	defer srv.Close()
	c := &api{Client: spotify.New(srv.Client(), spotify.WithBaseURL(srv.URL+"/")), http: srv.Client(), base: srv.URL + "/"}
	msg := next(c)().(actionDoneMsg)
	if msg.err == nil || msg.err.Error() != "no active device. Open Spotify somewhere, then press d to pick it" {
		t.Errorf("got %v", msg.err)
	}
}
