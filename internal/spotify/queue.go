package spotify

import (
	"context"
	"net/http"
	"net/url"
)

// Queue returns what plays after the current item: tracks the user queued,
// then the rest of the album or playlist. Spotify only returns the next few.
func (c *Client) Queue(ctx context.Context) ([]Track, error) {
	var resp struct {
		Queue []trackObject `json:"queue"`
	}
	if err := playerError(c.do(ctx, http.MethodGet, "/me/player/queue", nil, nil, &resp)); err != nil {
		return nil, err
	}

	tracks := make([]Track, len(resp.Queue))
	for i, t := range resp.Queue {
		tracks[i] = t.toTrack()
	}
	return tracks, nil
}

// AddToQueue adds a track or episode to the end of the user's queue
func (c *Client) AddToQueue(ctx context.Context, uri string) error {
	q := url.Values{"uri": {uri}}
	return c.playerCommand(ctx, http.MethodPost, "/me/player/queue", q, nil)
}
