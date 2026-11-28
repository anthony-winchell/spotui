package spotify

import (
	"context"
	"errors"
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

// playlistItemsPageSize is the most playlist entries Spotify returns per request
const playlistItemsPageSize = 50

// ErrPlaylistHidden means Spotify won't list a playlist's tracks. Since
// February 2026 it only lists them for playlists the user owns or collaborates on.
var ErrPlaylistHidden = errors.New("spotify only lists the tracks of playlists you own or collaborate on")

// playlistItemObject is one entry in a playlist. The track or episode is null
// if it's no longer available.
type playlistItemObject struct {
	Item *trackObject `json:"item"` // called "track" before February 2026
}

func (p playlistItemObject) toTrack() Track {
	if p.Item == nil {
		return Track{Name: "(unavailable)"}
	}
	return p.Item.toTrack()
}

// PlaylistTracks returns one page of a playlist's tracks, in playlist order.
// Unavailable entries are kept, as tracks with no URI, so that a track's index
// is still its position in the playlist.
func (c *Client) PlaylistTracks(ctx context.Context, playlistID string, offset int) (Page[Track], error) {
	q := url.Values{
		"limit":  {strconv.Itoa(playlistItemsPageSize)},
		"offset": {strconv.Itoa(offset)},
	}

	var pr pagingResponse[playlistItemObject]
	err := c.do(ctx, http.MethodGet, "/playlists/"+url.PathEscape(playlistID)+"/items", q, nil, &pr)

	var ae *APIError
	if errors.As(err, &ae) && ae.StatusCode == http.StatusForbidden {
		return Page[Track]{}, ErrPlaylistHidden
	}
	if err != nil {
		return Page[Track]{}, err
	}

	return mapPage(pr, playlistItemObject.toTrack), nil
}
