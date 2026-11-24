package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// Update applies one message to the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "space":
			return m, togglePlayback(m.client, m.playback.Playing)
		case "n":
			return m, runCommand(m.client.Next)
		case "N":
			return m, runCommand(m.client.Previous)
		case "-":
			return m.changeVolume(-volumeStep)
		case "+", "=":
			return m.changeVolume(volumeStep)
		case ",":
			return m.seek(-seekStep)
		case ".":
			return m.seek(seekStep)
		case "h", "esc":
			m.focus = focusSidebar
		case "l":
			m.focus = focusMain
		case "enter":
			if m.focus == focusSidebar {
				m.focus = focusMain
				return m, nil
			}
			return m, m.playSelected()
		case "j":
			if m.focus == focusSidebar {
				m.active = min(m.active+1, sectionCount-1)
			} else {
				m.moveCursor(1)
			}
			cmd := m.fetchActive()
			return m, cmd
		case "k":
			if m.focus == focusSidebar {
				m.active = max(m.active-1, 0)
			} else {
				m.moveCursor(-1)
			}
			cmd := m.fetchActive()
			return m, cmd
		}
	case playbackMsg:
		now := time.Now()
		m.loading = false
		m.fetching = false
		m.err = msg.err
		m.nextSync = now.Add(syncDelay(msg.err))
		switch {
		case msg.err == nil:
			prev := m.playback
			m.playback = msg.state
			m.reconcile(prev, now)
		case errors.Is(msg.err, spotify.ErrNoActiveDevice):
			m.playback = spotify.PlaybackState{} // Spotify says nothing is playing
		}

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
		return m, runCommand(func(ctx context.Context) error {
			return m.client.SetVolume(ctx, vol)
		})
	case seekFlushMsg:
		if msg.seq != m.seekPending.seq {
			return m, nil // a newer key press superseded this one
		}
		pos := m.playback.ProgressAt(time.Now())
		return m, runCommand(func(ctx context.Context) error {
			return m.client.Seek(ctx, pos)
		})

	case listMsg[spotify.Playlist]:
		m.playlists.apply(msg)

	case listMsg[spotify.Track]:
		m.liked.apply(msg) // the only track list until search arrives

	case listMsg[spotify.Play]:
		m.recent.apply(msg)
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

// sameTrack reports whether a and b are playing the same track
func sameTrack(a, b spotify.PlaybackState) bool {
	return a.Track != nil && b.Track != nil && a.Track.ID == b.Track.ID
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

// fetchActive starts fetching the active section's next page, if it needs one
func (m *Model) fetchActive() tea.Cmd {
	switch m.active {
	case sectionPlaylists:
		return m.playlists.next(m.client.Playlists)
	case sectionLiked:
		return m.liked.next(m.client.LikedSongs)
	case sectionRecent:
		return m.recent.next(recentPage(m.client))
	}
	return nil
}

// moveCursor moves the cursor of the active section's list
func (m *Model) moveCursor(delta int) {
	switch m.active {
	case sectionPlaylists:
		m.playlists.move(delta)
	case sectionLiked:
		m.liked.move(delta)
	case sectionRecent:
		m.recent.move(delta)
	}
}

// playSelected returns a command that plays the highlighted item, or nil if there is none
func (m Model) playSelected() tea.Cmd {
	switch m.active {
	case sectionPlaylists:
		p, ok := m.playlists.selected()
		if !ok {
			return nil
		}
		return runCommand(func(ctx context.Context) error {
			return m.client.PlayContext(ctx, p.URI, 0)
		})
	case sectionLiked:
		if len(m.liked.items) == 0 {
			return nil
		}
		uris := trackURIs(m.liked.items[m.liked.cursor:], playQueueSize)
		return runCommand(func(ctx context.Context) error {
			return m.client.PlayTracks(ctx, uris, 0)
		})
	case sectionRecent:
		p, ok := m.recent.selected()
		if !ok {
			return nil
		}
		return runCommand(func(ctx context.Context) error {
			return m.client.PlayTracks(ctx, []string{p.Track.URI}, 0)
		})
	}
	return nil
}

// trackURIs returns the URIs of the first n tracks
func trackURIs(tracks []spotify.Track, n int) []string {
	tracks = tracks[:min(n, len(tracks))]
	uris := make([]string, len(tracks))
	for i, t := range tracks {
		uris[i] = t.URI
	}
	return uris
}
