// Package ui is the Bubble Tea terminal interface
package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

const (
	// requestTimeout bounds each Spotify call made from the UI
	requestTimeout = 10 * time.Second

	// syncInterval is how often playback state is re-fetched from Spotify
	syncInterval = 5 * time.Second

	// settleDelay is how long to wait after a command before re-fetching,
	// giving Spotify time to reflect the change
	settleDelay = time.Second

	// keyDebounce is how long repeatable keys (volume, seek) must be idle before we send a request
	keyDebounce = 300 * time.Millisecond

	// confirmTimeout is how long a local change wins over fetched state while waiting for Spotify
	confirmTimeout = 5 * time.Second

	// volumeStep is how much +/- changes the volume, in percent
	volumeStep = 5

	// seekStep is how far h/l jumps
	seekStep = 5 * time.Second

	// seekTolerance is how close a fetched position must be to ours to count as confirmed
	seekTolerance = 1500 * time.Millisecond
)

type focus int

const (
	focusSidebar focus = iota
	focusMain
)

// Model is the root Bubble Tea Model
type Model struct {
	client *spotify.Client

	active section // highlighted sidebar entry
	focus  focus   // panel that receives j/k

	playback spotify.PlaybackState
	loading  bool
	err      error

	volPending  pending // local volume change awaiting confirmation
	seekPending pending // local seek awaiting confirmation
	playPending pending // local play/pause awaiting confirmation

	fetching bool      // a playback fetch is in flight
	nextSync time.Time // earliest time for the next periodic fetch

	width, height int // terminal size, from tea.WindowSizeMsg
}

// New returns the initial Model
func New(client *spotify.Client) Model {
	return Model{client: client, loading: true, fetching: true} // Init starts a fetch
}

// Init runs once at startup; its command loads the initial state
func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchPlayback(m.client), tick())
}

// --- Messages ------------------------------------------------

// playbackMsg carries the result of fetching the playback state
type playbackMsg struct {
	state spotify.PlaybackState
	err   error
}

// toggledMsg reports the result of a play/pause command
type toggledMsg struct {
	playing bool
	err     error
}

// commandDoneMsg reports that a playback command finished
type commandDoneMsg struct {
	err error
}

// volumeFlushMsg fires after a volume key press; only the latest one is acted on
type volumeFlushMsg struct {
	seq int
}

// seekFlushMsg fires after a seek key press; only the latest one is acted on
type seekFlushMsg struct {
	seq int
}

// tickMsg is sent once per second to redraw progress and drive sync
type tickMsg time.Time

// --- Commands ------------------------------------------------

// tick schedules the next tickMsg
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetchPlayback asks Spotify for the current playback state
func fetchPlayback(c *spotify.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		st, err := c.Playback(ctx)
		return playbackMsg{state: st, err: err}
	}
}

// fetchPlaybackAfter waits d, then fetches playback state
// The delay gives Spotify time to reflect a command we just sent
func fetchPlaybackAfter(c *spotify.Client, d time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return fetchPlayback(c)()
	}
}

// togglePlayback pauses if playing, otherwise resumes
func togglePlayback(c *spotify.Client, playing bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		var err error
		if playing {
			err = c.Pause(ctx)
		} else {
			err = c.Play(ctx)
		}

		return toggledMsg{playing: !playing, err: err}
	}
}

// runCommand runs a playback command with a timeout and reports the result
func runCommand(fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return commandDoneMsg{err: fn(ctx)}
	}
}
