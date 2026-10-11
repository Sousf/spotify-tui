// Package demo answers the Spotify Web API from a made-up library, so the
// interface can be shown and recorded without an account, a token, the
// network or a player. Playback state lives in memory: play, pause, skip,
// seek, shuffle, repeat and volume all work, and the progress bar moves.
package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zmb3/spotify/v2"
)

// Client returns an HTTP client whose requests never leave the process.
func Client() *http.Client {
	return &http.Client{Transport: newServer(time.Now)}
}

type album struct {
	id, name, artist, year string
	tracks                 []string
}

// The catalogue. Every name here is invented.
var albums = []album{
	{"al01", "Harbour Lights", "The Quiet Harbour", "2023", []string{
		"Lantern Swing", "Salt on the Window", "Fog Signal", "Tidewater", "Last Ferry Home", "Breakwater"}},
	{"al02", "Paper Moons", "Mira Solano", "2025", []string{
		"Paper Moons", "Slow Orbit", "Kitchen Radio", "Glass Garden", "Under the Overpass"}},
	{"al03", "Static Bloom", "Velvet Static", "2024", []string{
		"Static Bloom", "Neon Arcade", "Second Signal", "Midnight Bus", "Copper Wire", "Afterglow"}},
	{"al04", "Field Notes", "Juniper & Ash", "2022", []string{
		"Field Notes", "Wild Thyme", "River Stones", "The Long Field", "Barn Light"}},
	{"al05", "夜の海", "静かな海", "2026", []string{
		"波の音", "Blue Hour", "月明かり", "Sea Glass"}},
	{"al06", "Low Tide", "Low Tide Radio", "2021", []string{
		"Low Tide", "Postcards", "Cassette Summer", "Parking Lot Sunset", "Driftwood"}},
	{"al07", "Clockwork Sun", "Ondine", "2026", []string{
		"Clockwork Sun", "Marigold", "Halfway Up the Stairs", "Weather Vane", "Tin Roof Rain"}},
	{"al08", "Northbound", "The Quiet Harbour", "2026", []string{
		"Northbound", "Ice Road", "Lighthouse Keeper", "Long Exposure"}},
}

var playlistNames = []string{
	"Morning Coffee", "Deep Focus", "Late Night Drive", "Sunday Cleaning",
	"Running 5k", "Rainy Day Jazz", "Road Trip 2026", "Lo-fi for Coding",
	"Dinner Party", "Harbour Sessions", "Throwbacks", "Release Radar",
	"Discover Weekly",
}

var devices = []spotify.PlayerDevice{
	{ID: "dev1", Name: "This computer", Type: "Computer", Active: true, Volume: 64},
	{ID: "dev2", Name: "Living room speaker", Type: "Speaker", Volume: 40},
	{ID: "dev3", Name: "Phone", Type: "Smartphone", Volume: 80},
}

type catalogue struct {
	tracks    []spotify.FullTrack
	artists   []spotify.FullArtist
	albums    []spotify.SimpleAlbum
	playlists []spotify.SimplePlaylist
	lists     map[spotify.URI][]int // playlist or album URI -> track indexes
}

func build() *catalogue {
	c := &catalogue{lists: map[spotify.URI][]int{}}
	artistIdx := map[string]int{}
	for ai, a := range albums {
		i, ok := artistIdx[a.artist]
		if !ok {
			i = len(c.artists)
			artistIdx[a.artist] = i
			id := spotify.ID(fmt.Sprintf("ar%02d", i+1))
			c.artists = append(c.artists, spotify.FullArtist{
				SimpleArtist: spotify.SimpleArtist{ID: id, Name: a.artist, URI: spotify.URI("spotify:artist:" + id)},
				Followers:    spotify.Followers{Count: spotify.Numeric(18000 + 23117*(i+1)%400000)},
			})
		}
		artist := c.artists[i].SimpleArtist
		sa := spotify.SimpleAlbum{
			ID: spotify.ID(a.id), Name: a.name, URI: spotify.URI("spotify:album:" + a.id),
			Artists: []spotify.SimpleArtist{artist}, ReleaseDate: a.year + "-03-01", AlbumType: "album",
		}
		c.albums = append(c.albums, sa)
		for ti, name := range a.tracks {
			id := spotify.ID(fmt.Sprintf("tr%02d%02d", ai+1, ti+1))
			t := spotify.FullTrack{Album: sa}
			t.ID, t.Name, t.URI = id, name, spotify.URI("spotify:track:"+id)
			t.Artists = []spotify.SimpleArtist{artist}
			// 2:41 to 5:12, varied but fixed
			t.Duration = spotify.Numeric(161000 + (len(name)*7919+ti*3301)%151000)
			c.lists[sa.URI] = append(c.lists[sa.URI], len(c.tracks))
			c.tracks = append(c.tracks, t)
		}
	}
	for i, name := range playlistNames {
		id := spotify.ID(fmt.Sprintf("pl%02d", i+1))
		uri := spotify.URI("spotify:playlist:" + id)
		owner := "demo"
		if strings.HasPrefix(name, "Release") || strings.HasPrefix(name, "Discover") {
			owner = "Spotify"
		}
		// A stable shuffle of the catalogue, a different length per list.
		n := 12 + (i*7)%20
		for j := 0; j < n; j++ {
			c.lists[uri] = append(c.lists[uri], (i*11+j*7)%len(c.tracks))
		}
		p := spotify.SimplePlaylist{ID: id, Name: name, URI: uri}
		p.Owner.DisplayName = owner
		c.playlists = append(c.playlists, p)
	}
	return c
}

// server is the in-memory API. All fields after mu are guarded by it.
type server struct {
	now func() time.Time
	cat *catalogue

	mu       sync.Mutex
	context  spotify.URI // what is playing from
	list     []int       // its tracks
	cur      int         // index into list
	playing  bool
	posAt    time.Duration // progress at mark
	mark     time.Time
	shuffle  bool
	repeat   string
	device   int
	volume   int
	queued   []int
	liked    map[spotify.ID]bool
	likedIdx []int
}

func newServer(now func() time.Time) *server {
	s := &server{now: now, cat: build(), repeat: "off", liked: map[spotify.ID]bool{}}
	for i := range s.cat.tracks {
		if i%3 != 2 {
			s.likedIdx = append(s.likedIdx, i)
			s.liked[s.cat.tracks[i].ID] = true
		}
	}
	first := s.cat.playlists[2].URI // Late Night Drive
	s.context, s.list = first, s.cat.lists[first]
	s.cur, s.playing, s.posAt, s.mark = 3, true, 72*time.Second, now()
	s.volume = int(devices[0].Volume)
	return s
}

// progress is where the current track is, moving to the next one when the
// clock runs past its end. The caller holds mu.
func (s *server) progress() time.Duration {
	if !s.playing {
		return s.posAt
	}
	for {
		p := s.posAt + s.now().Sub(s.mark)
		d := time.Duration(s.track().Duration) * time.Millisecond
		if p < d {
			return p
		}
		s.mark = s.mark.Add(d - s.posAt)
		s.posAt = 0
		s.advance(1)
	}
}

func (s *server) track() spotify.FullTrack {
	return s.cat.tracks[s.list[s.cur]]
}

func (s *server) advance(by int) {
	if len(s.queued) > 0 && by > 0 {
		// A queued track plays next, then the list carries on where it was.
		next := s.queued[0]
		s.queued = s.queued[1:]
		s.list = append(append(append([]int{}, s.list[:s.cur+1]...), next), s.list[s.cur+1:]...)
	}
	s.cur = (s.cur + by + len(s.list)) % len(s.list)
}

// seekTo sets the progress, keeping play state. The caller holds mu.
func (s *server) seekTo(p time.Duration) {
	s.posAt, s.mark = p, s.now()
}

func (s *server) RoundTrip(req *http.Request) (*http.Response, error) {
	path := strings.TrimPrefix(req.URL.Path, "/v1/")
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}
	s.mu.Lock()
	out, status := s.handle(req.Method, path, req.URL.Query(), body)
	s.mu.Unlock()
	resp := &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    req,
	}
	if out != nil {
		b, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(b))
		resp.ContentLength = int64(len(b))
	}
	return resp, nil
}

type obj = map[string]any

// page wraps items the way Spotify's offset-paginated endpoints do.
func page(items []any, q map[string][]string) obj {
	limit, offset := atoi(q, "limit", 20), atoi(q, "offset", 0)
	total := len(items)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	next := ""
	if end < total {
		next = fmt.Sprintf("https://api.spotify.com/v1/next?offset=%d", end)
	}
	return obj{"items": items[offset:end], "total": total, "limit": limit, "offset": offset, "next": next}
}

func atoi(q map[string][]string, key string, def int) int {
	if v, ok := q[key]; ok && len(v) > 0 {
		if n, err := strconv.Atoi(v[0]); err == nil {
			return n
		}
	}
	return def
}

func (s *server) handle(method, path string, q map[string][]string, body []byte) (any, int) {
	c := s.cat
	parts := strings.Split(path, "/")
	get := method == http.MethodGet
	switch {
	case get && path == "me":
		return obj{"id": "demo", "display_name": "demo", "country": "GB", "product": "premium"}, 200

	case get && path == "me/playlists":
		var items []any
		for _, p := range c.playlists {
			items = append(items, obj{
				"id": p.ID, "uri": p.URI, "name": p.Name,
				"owner": obj{"display_name": p.Owner.DisplayName},
				"items": obj{"total": len(c.lists[p.URI])},
			})
		}
		return page(items, q), 200

	case get && len(parts) == 3 && parts[0] == "playlists" && parts[2] == "items":
		var items []any
		for _, i := range c.lists[spotify.URI("spotify:playlist:"+parts[1])] {
			items = append(items, obj{"is_local": false, "item": withType(c.tracks[i])})
		}
		return page(items, q), 200

	case get && path == "me/tracks":
		var items []any
		for _, i := range s.likedIdx {
			items = append(items, obj{"added_at": "2026-09-01T12:00:00Z", "track": c.tracks[i]})
		}
		return page(items, q), 200

	case get && path == "me/albums":
		var items []any
		for _, a := range c.albums {
			items = append(items, obj{"added_at": "2026-09-01T12:00:00Z", "album": a})
		}
		return page(items, q), 200

	case get && path == "me/following":
		var items []any
		for _, a := range c.artists {
			items = append(items, a)
		}
		return obj{"artists": obj{"items": items, "total": len(items), "cursors": obj{"after": ""}}}, 200

	case get && len(parts) == 3 && parts[0] == "albums" && parts[2] == "tracks":
		var items []any
		for _, i := range c.lists[spotify.URI("spotify:album:"+parts[1])] {
			items = append(items, c.tracks[i].SimpleTrack)
		}
		return page(items, q), 200

	case get && len(parts) == 3 && parts[0] == "artists" && parts[2] == "albums":
		var items []any
		for _, a := range c.albums {
			if len(a.Artists) > 0 && string(a.Artists[0].ID) == parts[1] {
				items = append(items, a)
			}
		}
		return page(items, q), 200

	case get && path == "search":
		return s.search(firstOf(q, "q")), 200

	case get && path == "me/player/devices":
		ds := append([]spotify.PlayerDevice(nil), devices...)
		for i := range ds {
			ds[i].Active = i == s.device
			if ds[i].Active {
				ds[i].Volume = spotify.Numeric(s.volume)
			}
		}
		return obj{"devices": ds}, 200

	case get && path == "me/player/queue":
		s.progress() // let a finished track roll over first
		var queue []spotify.FullTrack
		for _, i := range s.queued {
			queue = append(queue, c.tracks[i])
		}
		for j := 1; len(queue) < 10 && j < len(s.list); j++ {
			queue = append(queue, c.tracks[s.list[(s.cur+j)%len(s.list)]])
		}
		return obj{"currently_playing": s.track(), "queue": queue}, 200

	case get && path == "me/player":
		p := s.progress()
		d := devices[s.device]
		d.Active, d.Volume = true, spotify.Numeric(s.volume)
		t := s.track()
		return obj{
			"device":        d,
			"shuffle_state": s.shuffle,
			"repeat_state":  s.repeat,
			"timestamp":     s.now().UnixMilli(),
			"context":       obj{"uri": s.context, "type": "playlist"},
			"progress_ms":   p.Milliseconds(),
			"is_playing":    s.playing,
			"item":          t,
		}, 200

	case method == http.MethodPut && path == "me/player/play":
		s.play(body)
		return nil, 204
	case method == http.MethodPut && path == "me/player/pause":
		s.posAt, s.playing = s.progress(), false
		return nil, 204
	case method == http.MethodPost && path == "me/player/next":
		s.progress()
		s.advance(1)
		s.seekTo(0)
		return nil, 204
	case method == http.MethodPost && path == "me/player/previous":
		if s.progress() > 3*time.Second {
			s.seekTo(0)
		} else {
			s.advance(-1)
			s.seekTo(0)
		}
		return nil, 204
	case method == http.MethodPut && path == "me/player/seek":
		s.seekTo(time.Duration(atoi(q, "position_ms", 0)) * time.Millisecond)
		return nil, 204
	case method == http.MethodPut && path == "me/player/shuffle":
		s.shuffle = firstOf(q, "state") == "true"
		return nil, 204
	case method == http.MethodPut && path == "me/player/repeat":
		s.repeat = firstOf(q, "state")
		return nil, 204
	case method == http.MethodPut && path == "me/player/volume":
		s.volume = atoi(q, "volume_percent", s.volume)
		return nil, 204
	case method == http.MethodPut && path == "me/player":
		var r struct {
			IDs []string `json:"device_ids"`
		}
		json.Unmarshal(body, &r)
		for i, d := range devices {
			if len(r.IDs) > 0 && string(d.ID) == r.IDs[0] {
				s.device, s.volume = i, int(d.Volume)
			}
		}
		return nil, 204
	case method == http.MethodPost && path == "me/player/queue":
		uri := spotify.URI(firstOf(q, "uri"))
		for i, t := range c.tracks {
			if t.URI == uri {
				s.queued = append(s.queued, i)
			}
		}
		return nil, 204
	case (method == http.MethodPut || method == http.MethodDelete) && path == "me/tracks":
		for _, id := range strings.Split(firstOf(q, "ids"), ",") {
			s.setLiked(spotify.ID(id), method == http.MethodPut)
		}
		return nil, 204
	}
	return obj{"error": obj{"status": 404, "message": "not in the demo: " + method + " " + path}}, 404
}

func firstOf(q map[string][]string, key string) string {
	if v := q[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// withType adds the "type" field the playlist items endpoint carries.
func withType(t spotify.FullTrack) any {
	return struct {
		Type string `json:"type"`
		spotify.FullTrack
	}{"track", t}
}

func (s *server) play(body []byte) {
	var r struct {
		Context spotify.URI   `json:"context_uri"`
		URIs    []spotify.URI `json:"uris"`
		Offset  *struct {
			URI spotify.URI `json:"uri"`
		} `json:"offset"`
	}
	json.Unmarshal(body, &r)
	c := s.cat
	switch {
	case r.Context != "" && len(c.lists[r.Context]) > 0:
		s.context, s.list, s.cur = r.Context, c.lists[r.Context], 0
		if r.Offset != nil {
			for j, i := range s.list {
				if c.tracks[i].URI == r.Offset.URI {
					s.cur = j
				}
			}
		}
		s.seekTo(0)
	case len(r.URIs) > 0:
		var list []int
		for _, u := range r.URIs {
			for i, t := range c.tracks {
				if t.URI == u {
					list = append(list, i)
				}
			}
		}
		if len(list) > 0 {
			s.context, s.list, s.cur = "", list, 0
			s.seekTo(0)
		}
	default: // resume
		s.mark = s.now()
	}
	s.playing = true
}

func (s *server) setLiked(id spotify.ID, on bool) {
	if s.liked[id] == on {
		return
	}
	s.liked[id] = on
	s.likedIdx = s.likedIdx[:0]
	for i, t := range s.cat.tracks {
		if s.liked[t.ID] {
			s.likedIdx = append(s.likedIdx, i)
		}
	}
}

// search matches the query against names, falling back to everything so any
// query shows all four result tabs.
func (s *server) search(query string) obj {
	c := s.cat
	query = strings.ToLower(strings.TrimSpace(query))
	has := func(names ...string) bool {
		for _, n := range names {
			if query == "" || strings.Contains(strings.ToLower(n), query) {
				return true
			}
		}
		return false
	}
	var tracks, artists, albs, pls []any
	for _, t := range c.tracks {
		if has(t.Name, t.Album.Name, t.Artists[0].Name) && len(tracks) < 10 {
			tracks = append(tracks, t)
		}
	}
	for _, a := range c.artists {
		if has(a.Name) {
			artists = append(artists, a)
		}
	}
	for _, a := range c.albums {
		if has(a.Name, a.Artists[0].Name) {
			albs = append(albs, a)
		}
	}
	for _, p := range c.playlists {
		if has(p.Name) {
			pls = append(pls, p)
		}
	}
	if len(tracks)+len(artists)+len(albs)+len(pls) == 0 {
		return s.search("")
	}
	wrap := func(items []any) obj {
		if items == nil {
			items = []any{}
		}
		return obj{"items": items, "total": len(items)}
	}
	return obj{"tracks": wrap(tracks), "artists": wrap(artists), "albums": wrap(albs), "playlists": wrap(pls)}
}
