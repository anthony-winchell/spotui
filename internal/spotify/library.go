package spotify

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// likedSongsPageSize is the most saved tracks Spotify returns per request
const likedSongsPageSize = 50

// savedTrackObject is one entry in the user's Liked Songs
type savedTrackObject struct {
	Track trackObject `json:"track"`
}

// LikedSongs returns one page of the user's Liked Songs, most recently liked first.
func (c *Client) LikedSongs(ctx context.Context, offset int) (Page[Track], error) {
	q := url.Values{
		"limit":  {strconv.Itoa(likedSongsPageSize)},
		"offset": {strconv.Itoa(offset)},
	}

	var pr pagingResponse[savedTrackObject]
	if err := c.do(ctx, http.MethodGet, "/me/tracks", q, nil, &pr); err != nil {
		return Page[Track]{}, err
	}

	return mapPage(pr, func(s savedTrackObject) Track {
		return s.Track.toTrack()
	}), nil
}
