package spotify

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// ErrNoActiveDevice means no Spotify device is currently active.
var ErrNoActiveDevice = errors.New("no active Spotify device")

// Device is a Spotify Connect playback device.
type Device struct {
	ID             string
	Name           string
	Type           string // "Computer", "Smartphone", ...
	Volume         int    // 0-100; only meaningful if SupportsVolume
	SupportsVolume bool
	Restricted     bool // true if the device rejects the Web API commands
}

// PlaybackState is a snapshot of the user's playback
// nil if nothing playable is loaded (e.g. an ad or podcast)
type PlaybackState struct {
	Playing   bool
	Progress  time.Duration
	FetchedAt time.Time // when Progress was measured
	Track     *Track
	Device    Device
}

// playbackResponse is the JSON body of GET /me/player.
type playbackResponse struct {
	Device struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Type           string `json:"type"`
		VolumePercent  int    `json:"volume_percent"`
		SupportsVolume bool   `json:"supports_volume"`
		IsRestricted   bool   `json:"is_restricted"`
	} `json:"device"`
	IsPlaying  bool         `json:"is_playing"`
	ProgressMS int          `json:"progress_ms"`
	Item       *trackObject `json:"item"`
}

// Playback returns the current playback state, or ErrNoActiveDevice
func (c *Client) Playback(ctx context.Context) (PlaybackState, error) {
	var pr *playbackResponse
	if err := c.do(ctx, http.MethodGet, "/me/player", nil, nil, &pr); err != nil {
		return PlaybackState{}, err
	}

	if pr == nil {
		return PlaybackState{}, ErrNoActiveDevice
	}

	return pr.toState(time.Now()), nil
}

// ProgressAt estimates playback progress now, advancing locally from the last sync.
func (s PlaybackState) ProgressAt(now time.Time) time.Duration {
	if !s.Playing {
		return s.Progress
	}

	p := s.Progress + now.Sub(s.FetchedAt)
	if s.Track != nil && p > s.Track.Duration {
		p = s.Track.Duration
	}

	return p
}

func (pr *playbackResponse) toState(fetchedAt time.Time) PlaybackState {
	s := PlaybackState{
		Playing:   pr.IsPlaying,
		Progress:  time.Duration(pr.ProgressMS) * time.Millisecond,
		FetchedAt: fetchedAt,
		Device: Device{
			ID:             pr.Device.ID,
			Name:           pr.Device.Name,
			Type:           pr.Device.Type,
			Volume:         pr.Device.VolumePercent,
			SupportsVolume: pr.Device.SupportsVolume,
			Restricted:     pr.Device.IsRestricted,
		},
	}

	if pr.Item != nil {
		t := pr.Item.toTrack()
		s.Track = &t
	}

	return s
}
