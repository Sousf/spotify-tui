package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/zmb3/spotify/v2"
)

// api bundles the typed client with the raw HTTP client. Spotify replaced
// the playlist tracks endpoints in 2026 and the typed client still targets
// the old ones, so those calls are made by hand here.
type api struct {
	*spotify.Client
	http *http.Client
	base string
}

const apiBase = "https://api.spotify.com/v1/"

func newAPI(hc *http.Client) *api {
	return &api{Client: spotify.New(hc, spotify.WithRetry(true)), http: hc, base: apiBase}
}

func (a *api) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	u := a.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error spotify.Error `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
			e.Error.Status = resp.StatusCode
			return e.Error
		}
		return fmt.Errorf("spotify: HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}

// playlistSummary is the subset of a playlist object the sidebar needs, read
// with the new items.total field.
type playlistSummary struct {
	ID    spotify.ID  `json:"id"`
	URI   spotify.URI `json:"uri"`
	Name  string      `json:"name"`
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Items struct {
		Total int `json:"total"`
	} `json:"items"`
}

type playlistSummaryPage struct {
	Items []playlistSummary `json:"items"`
	Total int               `json:"total"`
	Next  string            `json:"next"`
}

func (a *api) myPlaylists(ctx context.Context, limit, offset int) (*playlistSummaryPage, error) {
	q := url.Values{
		"limit":  {fmt.Sprint(limit)},
		"offset": {fmt.Sprint(offset)},
	}
	var pg playlistSummaryPage
	if err := a.getJSON(ctx, "me/playlists", q, &pg); err != nil {
		return nil, err
	}
	return &pg, nil
}

// playlistItemsPage is the /playlists/{id}/items response. Only tracks are
// decoded; episodes and unavailable items come back with an empty ID.
type playlistItemsPage struct {
	Items []struct {
		IsLocal bool `json:"is_local"`
		Item    struct {
			Type string `json:"type"`
			spotify.FullTrack
		} `json:"item"`
	} `json:"items"`
	Total int    `json:"total"`
	Next  string `json:"next"`
}

func (a *api) playlistItems(ctx context.Context, id spotify.ID, limit, offset int) ([]item, int, error) {
	q := url.Values{
		"limit":  {fmt.Sprint(limit)},
		"offset": {fmt.Sprint(offset)},
		"fields": {"total,next,items(is_local,item(type,id,uri,name,duration_ms,artists(id,name),album(id,name)))"},
	}
	var pg playlistItemsPage
	if err := a.getJSON(ctx, "playlists/"+string(id)+"/items", q, &pg); err != nil {
		return nil, 0, err
	}
	items := make([]item, 0, len(pg.Items))
	for _, e := range pg.Items {
		if e.Item.Type != "track" || e.Item.ID == "" {
			continue
		}
		items = append(items, trackItem(e.Item.FullTrack))
	}
	return items, pg.Total, nil
}
