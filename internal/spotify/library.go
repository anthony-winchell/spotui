package spotify

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// likedSongsPageSize is the most saved tracks Spotify returns per request
const likedSongsPageSize = 50

// savedTrackObject is one entry in the user's Liked Songs
type savedTrackObject struct {
	Track trackObject `json:"track"`
}

// Play is one entry in the user's listening history
type Play struct {
	Track    Track
	PlayedAt time.Time
}

// playHistoryObject is Spotify's JSON play history object
type playHistoryObject struct {
	Track    trackObject `json:"track"`
	PlayedAt time.Time   `json:"played_at"`
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

// RecentlyPlayed returns the user's most recent plays, newest first.
// Spotify keeps only the last 50, so this is a single request
func (c *Client) RecentlyPlayed(ctx context.Context) ([]Play, error) {
	q := url.Values{"limit": {"50"}}

	var resp struct {
		Items []playHistoryObject `json:"items"`
	}

	if err := c.do(ctx, http.MethodGet, "/me/player/recently-played", q, nil, &resp); err != nil {
		return nil, err
	}

	plays := make([]Play, len(resp.Items))
	for i, it := range resp.Items {
		plays[i] = Play{Track: it.Track.toTrack(), PlayedAt: it.PlayedAt}
	}

	return plays, nil
}
