package spotify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrNoActiveDevice means no Spotify device is currently active.
var ErrNoActiveDevice = errors.New("no active Spotify device")

// ErrPremiumRequired means the operation needs a Spotify Premium account
var ErrPremiumRequired = errors.New("spotify premium required")

// Device is a Spotify Connect playback device.
type Device struct {
	ID             string
	Name           string
	Type           string // "Computer", "Smartphone", ...
	Volume         int    // 0-100; only meaningful if SupportsVolume
	SupportsVolume bool
	Restricted     bool // true if the device rejects the Web API commands
}

// RepeatMode is Spotify's repeat setting
type RepeatMode string

const (
	RepeatOff     RepeatMode = "off"
	RepeatContext RepeatMode = "context" // repeat the playlist or album
	RepeatTrack   RepeatMode = "track"   // repeat the current track
)

// PlaybackState is a snapshot of the user's playback
// nil if nothing playable is loaded (e.g. an ad or podcast)
type PlaybackState struct {
	Playing   bool
	Shuffle   bool
	Repeat    RepeatMode
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
	IsPlaying    bool         `json:"is_playing"`
	ShuffleState bool         `json:"shuffle_state"`
	RepeatState  string       `json:"repeat_state"`
	ProgressMS   int          `json:"progress_ms"`
	Item         *trackObject `json:"item"`
}

// Playback returns the current playback state, or ErrNoActiveDevice
func (c *Client) Playback(ctx context.Context) (PlaybackState, error) {
	// Without additional_types, Spotify reports no item at all while a
	// podcast episode plays
	q := url.Values{"additional_types": {"episode"}}

	var pr *playbackResponse
	if err := c.do(ctx, http.MethodGet, "/me/player", q, nil, &pr); err != nil {
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

// playBody is the JSON body for PUT /my/player/play
type playBody struct {
	ContextURI string      `json:"context_uri,omitempty"`
	URIs       []string    `json:"uris,omitempty"`
	Offset     *playOffset `json:"offset,omitempty"`
}

// playOffset says which track in the context or URI list to start from:
// by its position, or by its URI. Set one of them.
type playOffset struct {
	Position *int   `json:"position,omitempty"` // a pointer, so position 0 is still sent
	URI      string `json:"uri,omitempty"`
}

// playerCommand sends a player-control request and maps well-known
// player failures to sentinel errors
func (c *Client) playerCommand(ctx context.Context, method, path string, query url.Values, body any) error {
	return playerError(c.do(ctx, method, path, query, body, nil))
}

// playerError maps well-known player failures to sentinel errors
func playerError(err error) error {
	var ae *APIError
	if errors.As(err, &ae) {
		switch ae.Reason {
		case "NO_ACTIVE_DEVICE":
			return fmt.Errorf("%w: %w", ErrNoActiveDevice, err)
		case "PREMIUM_REQUIRED":
			return fmt.Errorf("%w: %w", ErrPremiumRequired, err)
		}
	}

	return err
}

func (c *Client) Play(ctx context.Context) error {
	return c.playerCommand(ctx, http.MethodPut, "/me/player/play", nil, nil)
}

// PlayContext starts playing a playlist, album or artist from the track position
func (c *Client) PlayContext(ctx context.Context, contextURI string, position int) error {
	body := playBody{ContextURI: contextURI}

	// Spotify only accepts an offset for albums and playlists; leave it out
	// when starting from the top so artist contexts work too
	if position > 0 {
		body.Offset = &playOffset{Position: &position}
	}

	return c.playerCommand(ctx, http.MethodPut, "/me/player/play", nil, body)
}

// PlayTrackIn plays trackURI within contextURI, so playback carries on through
// the rest of that album, playlist or Liked Songs. Spotify only accepts a
// starting track for those contexts, so with any other context, or none, the
// track plays on its own. It also plays on its own if Spotify refuses the
// context, e.g. because the track has since been taken out of the playlist.
func (c *Client) PlayTrackIn(ctx context.Context, contextURI, trackURI string) error {
	if !acceptsOffset(contextURI) {
		return c.PlayTracks(ctx, []string{trackURI}, 0)
	}

	body := playBody{ContextURI: contextURI, Offset: &playOffset{URI: trackURI}}
	err := c.playerCommand(ctx, http.MethodPut, "/me/player/play", nil, body)
	if refusedContext(err) {
		return c.PlayTracks(ctx, []string{trackURI}, 0)
	}
	return err
}

// acceptsOffset reports whether Spotify can start contextURI at a given track
func acceptsOffset(contextURI string) bool {
	return strings.HasPrefix(contextURI, "spotify:album:") ||
		strings.HasPrefix(contextURI, "spotify:playlist:") ||
		strings.HasSuffix(contextURI, ":collection") // Liked Songs
}

// refusedContext reports whether err is Spotify rejecting the request itself,
// rather than something that playing the track alone wouldn't fix either
func refusedContext(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) || errors.Is(err, ErrNoActiveDevice) || errors.Is(err, ErrPremiumRequired) {
		return false
	}
	switch ae.StatusCode {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound:
		return true
	}
	return false
}

// PlayTracks plays a list of track URIs, starting at position.
// Spotify plays them with no context, so playback stops at the end of the list
func (c *Client) PlayTracks(ctx context.Context, uris []string, position int) error {
	if len(uris) == 0 {
		return errors.New("no tracks to play")
	}

	body := playBody{
		URIs:   uris,
		Offset: &playOffset{Position: &position},
	}

	return c.playerCommand(ctx, http.MethodPut, "/me/player/play", nil, body)
}

func (c *Client) Pause(ctx context.Context) error {
	return c.playerCommand(ctx, http.MethodPut, "/me/player/pause", nil, nil)
}

func (c *Client) Next(ctx context.Context) error {
	return c.playerCommand(ctx, http.MethodPost, "/me/player/next", nil, nil)
}

func (c *Client) Previous(ctx context.Context) error {
	return c.playerCommand(ctx, http.MethodPost, "/me/player/previous", nil, nil)
}

func (c *Client) Seek(ctx context.Context, pos time.Duration) error {
	if pos < 0 {
		pos = 0
	}

	q := url.Values{"position_ms": {strconv.FormatInt(pos.Milliseconds(), 10)}}
	return c.playerCommand(ctx, http.MethodPut, "/me/player/seek", q, nil)
}

func (c *Client) SetVolume(ctx context.Context, percent int) error {
	percent = max(0, min(100, percent))
	q := url.Values{"volume_percent": {strconv.Itoa(percent)}}
	return c.playerCommand(ctx, http.MethodPut, "/me/player/volume", q, nil)
}

// SetShuffle turns shuffle on or off
func (c *Client) SetShuffle(ctx context.Context, on bool) error {
	q := url.Values{"state": {strconv.FormatBool(on)}}
	return c.playerCommand(ctx, http.MethodPut, "/me/player/shuffle", q, nil)
}

// SetRepeat sets the repeat mode
func (c *Client) SetRepeat(ctx context.Context, mode RepeatMode) error {
	q := url.Values{"state": {string(mode)}}
	return c.playerCommand(ctx, http.MethodPut, "/me/player/repeat", q, nil)
}

func (pr *playbackResponse) toState(fetchedAt time.Time) PlaybackState {
	s := PlaybackState{
		Playing:   pr.IsPlaying,
		Shuffle:   pr.ShuffleState,
		Repeat:    RepeatMode(pr.RepeatState),
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
