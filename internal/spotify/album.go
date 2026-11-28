package spotify

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// albumTracksPageSize is the most album tracks Spotify returns per request
const albumTracksPageSize = 50

// AlbumTracks returns one page of an album's tracks, in album order
func (c *Client) AlbumTracks(ctx context.Context, albumID string, offset int) (Page[Track], error) {
	q := url.Values{
		"limit":  {strconv.Itoa(albumTracksPageSize)},
		"offset": {strconv.Itoa(offset)},
	}

	var pr pagingResponse[trackObject]
	if err := c.do(ctx, http.MethodGet, "/albums/"+url.PathEscape(albumID)+"/tracks", q, nil, &pr); err != nil {
		return Page[Track]{}, err
	}

	// These are simplified track objects, which leave out the album
	albumURI := "spotify:album:" + albumID
	return mapPage(pr, func(t trackObject) Track {
		tr := t.toTrack()
		tr.AlbumURI = albumURI
		return tr
	}), nil
}

// IDFromURI returns the ID at the end of a Spotify URI, e.g. "4aawyAB9vmqN3uQ7FjRGTy"
// from "spotify:album:4aawyAB9vmqN3uQ7FjRGTy"
func IDFromURI(uri string) string {
	return uri[strings.LastIndex(uri, ":")+1:]
}
