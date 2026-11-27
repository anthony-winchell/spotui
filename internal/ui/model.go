// Package ui is the Bubble Tea terminal interface
package ui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

	// seekStep is how far ,/. jumps
	seekStep = 5 * time.Second

	// seekTolerance is how close a fetched position must be to ours to count as confirmed
	seekTolerance = 1500 * time.Millisecond

	// artworkRetryDelay is how long to wait before retrying a failed artwork download
	artworkRetryDelay = 30 * time.Second
)

type focus int

const (
	focusSidebar focus = iota
	focusMain
)

// Model is the root Bubble Tea Model
type Model struct {
	client *spotify.Client
	userID string // the logged-in user, for the Liked Songs context

	active section // highlighted sidebar entry
	focus  focus   // panel that receives j/k
	status status  // short-lived message, e.g. a failed command

	playlists listData[spotify.Playlist]
	liked     listData[spotify.Track]
	recent    listData[spotify.Play]

	search      textinput.Model // search box; focused means it captures all keys
	searchQuery string          // last submitted query
	results     listData[searchResult]

	playback spotify.PlaybackState
	loading  bool
	err      error

	art  artwork    // artwork of the current track
	like likeStatus // whether the current track is liked

	volPending     pending // local volume change awaiting confirmation
	seekPending    pending // local seek awaiting confirmation
	playPending    pending // local play/pause awaiting confirmation
	shufflePending pending // local shuffle change awaiting confirmation
	repeatPending  pending // local repeat change awaiting confirmation

	fetching bool      // a playback fetch is in flight
	nextSync time.Time // earliest time for the next periodic fetch

	width, height int // terminal size, from tea.WindowSizeMsg
}

// New returns the initial Model
func New(client *spotify.Client, userID string) Model {
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "tracks, artists, albums"

	s := textinput.DefaultDarkStyles()
	s.Focused.Prompt = lipgloss.NewStyle().Foreground(colorPurple)
	s.Focused.Placeholder = dimStyle
	s.Blurred.Placeholder = dimStyle
	in.SetStyles(s)

	return Model{client: client, userID: userID, loading: true, fetching: true, search: in} // Init starts a fetch
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

// shuffleFlushMsg fires after a shuffle key press; only the latest one is acted on
type shuffleFlushMsg struct {
	seq int
}

// repeatFlushMsg fires after a repeat key press; only the latest one is acted on
type repeatFlushMsg struct {
	seq int
}

// tickMsg is sent once per second to redraw progress and drive sync
type tickMsg time.Time

// listMsg carries one page of items for a list
type listMsg[T any] struct {
	items   []T
	hasMore bool // Spotify has another page after this one
	err     error
}

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

// fetchPage requests one page of a list starting at offset
func fetchPage[T any](fetch func(context.Context, int) (spotify.Page[T], error), offset int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		page, err := fetch(ctx, offset)
		return listMsg[T]{items: page.Items, hasMore: page.HasNext, err: err}
	}
}

// recentPage adapts RecentlyPlayed to the shape fetchPage expects.
// Spotify keeps only the last 50 plays, so there is never a next page.
func recentPage(c *spotify.Client) func(context.Context, int) (spotify.Page[spotify.Play], error) {
	return func(ctx context.Context, _ int) (spotify.Page[spotify.Play], error) {
		plays, err := c.RecentlyPlayed(ctx)
		return spotify.Page[spotify.Play]{Items: plays}, err
	}
}
