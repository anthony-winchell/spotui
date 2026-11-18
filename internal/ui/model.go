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

// Model is the root Bubble Tea Model
type Model struct {
	client *spotify.Client

	playback spotify.PlaybackState
	loading  bool
	err      error

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

// toggleMsg reports the result of a play/pause command
type toggledMsg struct {
	playing bool
	err     error
}

// commandDoneMsg reports that a playback command finished
type commandDoneMsg struct {
	err error
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
// The delay gives Spotify time to reflext a command we just sent
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
		}
	case playbackMsg:
		m.loading = false
		m.fetching = false
		m.playback = msg.state
		m.err = msg.err
		m.nextSync = time.Now().Add(syncDelay(msg.err))

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
		m.err = nil
		m.fetching = true
		return m, fetchPlaybackAfter(m.client, time.Second)

	case tickMsg:
		if !m.fetching && time.Time(msg).After(m.nextSync) {
			m.fetching = true
			return m, tea.Batch(tick(), fetchPlayback(m.client))
		}
		return m, tick()
	}
	return m, nil
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

	}
	b.WriteString("\n\nspace play/pause • q quit")
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
