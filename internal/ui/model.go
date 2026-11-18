// Package ui is the Bubble Tea terminal interface
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// requestTimeout bounds each Spotify call made from the UI
const requestTimeout = 10 * time.Second

// syncInterval is how often playback state is re-fetched from Spotify
const syncInterval = 5 * time.Second

// settleDelay is how long to wait after a command before re-fetching,
// giving Spotify time to reflect the change
const settleDelay = time.Second

// keyDebounce is how long repeatable keys (volume, seek) must be idle before we send a request
const keyDebounce = 300 * time.Millisecond

// confirmTimeout is how long a local change wins over fetched state while waiting for Spotify
const confirmTimeout = 5 * time.Second

// volumeStep is how much +/- changes the volume, in percent
const volumeStep = 5

// seekStep is how far h/l jumps
const seekStep = 5 * time.Second

// seekTolerance is how close a fetched position must be to ours to count as confirmed
const seekTolerance = 1500 * time.Millisecond

// Model is the root Bubble Tea Model
type Model struct {
	client *spotify.Client

	playback spotify.PlaybackState
	loading  bool
	err      error

	volPending  pending // local volume change awaiting confirmation
	seekPending pending // local seek awaiting confirmation
	playPending pending // local play/pause awaiting confirmation

	fetching bool      // a playback fetch is in flight
	nextSync time.Time // earliest time for the next periodic fetch
}

// tickMsg is sent once per second to redraw progress and drive sync
type tickMsg time.Time

// tick schedules the next tickMsg
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// New returns the initial Model
func New(client *spotify.Client) Model {
	return Model{client: client, loading: true, fetching: true} // Init starts a fetch
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

// --- Commands ------------------------------------------------

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
func runCommand(c *spotify.Client, fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return commandDoneMsg{err: fn(ctx)}
	}
}

// --- Bubble Tea Interface ------------------------------------------------

// Init runs once at startup; its command loads the initial state
func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchPlayback(m.client), tick())
}

// Update applies one message to the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "space":
			return m, togglePlayback(m.client, m.playback.Playing)
		case "n":
			return m, runCommand(m.client, m.client.Next)
		case "p":
			return m, runCommand(m.client, m.client.Previous)
		case "-":
			return m.changeVolume(-volumeStep)
		case "+", "=":
			return m.changeVolume(volumeStep)
		case "h":
			return m.seek(-seekStep)
		case "l":
			return m.seek(seekStep)
		}
	case playbackMsg:
		now := time.Now()
		m.loading = false
		m.fetching = false
		prev := m.playback
		m.playback = msg.state
		m.reconcile(prev, now)
		m.err = msg.err
		m.nextSync = now.Add(syncDelay(msg.err))

	case toggledMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Optimistic update: apply the change now instead of waiting for Spotify
		now := time.Now()
		m.playback.Progress = m.playback.ProgressAt(now)
		m.playback.FetchedAt = now
		m.playback.Playing = msg.playing
		m.playPending.start(confirmTimeout)
		m.err = nil
		m.fetching = true
		return m, fetchPlaybackAfter(m.client, settleDelay)

	case commandDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// We can't predict the new track, so ask Spotify once it has settled
		m.fetching = true
		return m, fetchPlaybackAfter(m.client, settleDelay)

	case tickMsg:
		if !m.fetching && time.Time(msg).After(m.nextSync) {
			m.fetching = true
			return m, tea.Batch(tick(), fetchPlayback(m.client))
		}
		return m, tick()

	case volumeFlushMsg:
		if msg.seq != m.volPending.seq {
			return m, nil // a newer key press superseded this one
		}
		vol := m.playback.Device.Volume
		return m, runCommand(m.client, func(ctx context.Context) error {
			return m.client.SetVolume(ctx, vol)
		})
	case seekFlushMsg:
		if msg.seq != m.seekPending.seq {
			return m, nil // a newer key press superseded this one
		}
		pos := m.playback.ProgressAt(time.Now())
		return m, runCommand(m.client, func(ctx context.Context) error {
			return m.client.Seek(ctx, pos)
		})
	}
	return m, nil
}

// changeVolume applies a volume change locally and schedules a debounced send
func (m Model) changeVolume(delta int) (Model, tea.Cmd) {
	if !m.playback.Device.SupportsVolume {
		return m, nil
	}

	m.playback.Device.Volume = max(0, min(100, m.playback.Device.Volume+delta))
	seq := m.volPending.start(keyDebounce + confirmTimeout)
	return m, tea.Tick(keyDebounce, func(time.Time) tea.Msg {
		return volumeFlushMsg{seq: seq}
	})
}

// seek moves the position locally by delta and schedules a debounced send
func (m Model) seek(delta time.Duration) (Model, tea.Cmd) {
	t := m.playback.Track
	if t == nil {
		return m, nil
	}

	now := time.Now()
	m.playback.Progress = max(0, min(t.Duration, m.playback.ProgressAt(now)+delta))
	m.playback.FetchedAt = now // ProgressAt counts forward from here

	seq := m.seekPending.start(keyDebounce + confirmTimeout)
	return m, tea.Tick(keyDebounce, func(time.Time) tea.Msg {
		return seekFlushMsg{seq: seq}
	})
}

// reconcile keeps local changes that Spotify hasn't confirmed yet.
// prev is the state before this fetch; it still holds our pending values
func (m *Model) reconcile(prev spotify.PlaybackState, now time.Time) {
	m.reconcilePlay(prev, now)
	m.reconcileVolume(prev, now)
	m.reconcileSeek(prev, now)
}

func (m *Model) reconcileVolume(prev spotify.PlaybackState, now time.Time) {
	if !m.volPending.active(now) {
		return
	}

	if m.playback.Device.Volume == prev.Device.Volume {
		m.volPending.confirm() // Spotify caught up
		return
	}

	m.playback.Device.Volume = prev.Device.Volume // not yet; keep ours
}

func (m *Model) reconcileSeek(prev spotify.PlaybackState, now time.Time) {
	if !m.seekPending.active(now) {
		return
	}

	ours := prev.ProgressAt(now)
	theirs := m.playback.ProgressAt(now)
	switch {
	case !sameTrack(prev, m.playback):
		m.seekPending.confirm() // track changed; our seek no longer applies
	case (theirs - ours).Abs() < seekTolerance:
		m.seekPending.confirm() // Spotify caught up
	default:
		m.playback.Progress = ours // not yet; keep
		m.playback.FetchedAt = now
	}
}

func (m *Model) reconcilePlay(prev spotify.PlaybackState, now time.Time) {
	if !m.playPending.active(now) {
		return
	}

	if m.playback.Playing == prev.Playing {
		m.playPending.confirm() // Spotify caught up
		return
	}

	m.playback.Playing = prev.Playing
}

// View renders the model
func (m Model) View() tea.View {
	var b strings.Builder

	switch {
	case m.loading:
		b.WriteString("Loading...")
	case errors.Is(m.err, spotify.ErrNoActiveDevice):
		b.WriteString("No active device. Open Spotify and start playing something.")
	case m.err != nil:
		fmt.Fprintf(&b, "Error: %v", m.err)
	case m.playback.Track == nil:
		b.WriteString("Nothing playing.")
	default:
		t := m.playback.Track
		icon := "⏸"
		if m.playback.Playing {
			icon = "▶"
		}
		fmt.Fprintf(&b, "%s %s\n %s\n\n %s / %s", icon, t.Name, strings.Join(t.Artists, ", "), mmss(m.playback.ProgressAt(time.Now())), mmss(t.Duration))
		if d := m.playback.Device; d.SupportsVolume {
			fmt.Fprintf(&b, "\n %s • vol %d%%", d.Name, d.Volume)
		}

	}
	b.WriteString("\n\nspace play/pause • n next • p prev • h/l seek • +/- vol • q quit")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// mmss formats d as m:ss
func mmss(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// syncDelay returns how long to wait before the next periodic sync,
// honoring Spotify's Retry-After when rate-limited.
func syncDelay(err error) time.Duration {
	var ae *spotify.APIError
	if errors.As(err, &ae) && ae.RetryAfter > syncInterval {
		return ae.RetryAfter
	}

	return syncInterval
}

// sameTrack reports whether a and b are playing the same track
func sameTrack(a, b spotify.PlaybackState) bool {
	return a.Track != nil && b.Track != nil && a.Track.ID == b.Track.ID
}
