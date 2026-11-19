package spotify

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// playlistPageSize is the most playlists Spotify returns per request
const playlistPageSize = 50

// Playlist is a playlist the user owns or follows
type Playlist struct {
	ID            string
	URI           string // "spotify:playlist:..."; used to start playback
	Name          string
	Collaborative bool
	OwnerID       string
	OwnerName     string
	TrackCount    int // -1 if Spotify didn't report it
}

// playlistObject is Spotify's JSON simplified playlist object
type playlistObject struct {
	ID            string `json:"id"`
	URI           string `json:"uri"`
	Name          string `json:"name"`
	Collaborative bool   `json:"collaborative"`
	Owner         struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	// Not the page's items: this is the playlist's own item collection.
	// Spotify may leave it out for playlists the user doesn't own.
	Items *struct {
		Total int `json:"total"`
	} `json:"items"`
}

func (p playlistObject) toPlaylist() Playlist {
	count := -1
	if p.Items != nil {
		count = p.Items.Total
	}

	return Playlist{
		ID:            p.ID,
		URI:           p.URI,
		Name:          p.Name,
		Collaborative: p.Collaborative,
		OwnerID:       p.Owner.ID,
		OwnerName:     p.Owner.DisplayName,
		TrackCount:    count,
	}
}

// Playlists returns one page of the user's playlists, starting at offset
func (c *Client) Playlists(ctx context.Context, offset int) (Page[Playlist], error) {
	q := url.Values{
		"limit":  {strconv.Itoa(playlistPageSize)},
		"offset": {strconv.Itoa(offset)},
	}

	var pr pagingResponse[playlistObject]
	if err := c.do(ctx, http.MethodGet, "/me/playlists", q, nil, &pr); err != nil {
		return Page[Playlist]{}, err
	}

	return mapPage(pr, playlistObject.toPlaylist), nil
}
