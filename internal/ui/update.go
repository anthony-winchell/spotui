package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// Update applies one message to the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.SetWidth(m.searchWidth())

	case tea.KeyPressMsg:
		if m.search.Focused() {
			return m.updateSearchInput(msg)
		}
		if m.playback.Device.Restricted && m.playbackKey(msg.String()) {
			m.status.set("This device can't be controlled from spotui")
			return m, nil
		}
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
		case "s":
			return m.toggleShuffle()
		case "r":
			return m.cycleRepeat()
		case "h", "esc":
			m.focus = focusSidebar
		case "/":
			return m.startSearch()
		case "l":
			return m.enterMain()
		case "enter":
			if m.focus == focusSidebar {
				return m.enterMain()
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
		m.err = msg.err // stays set until a sync succeeds
		m.nextSync = now.Add(syncDelay(msg.err))
		switch {
		case msg.err == nil:
			prev := m.playback
			m.playback = msg.state
			m.reconcile(prev, now)
			cmd := m.fetchArtwork(now)
			return m, cmd
		case errors.Is(msg.err, spotify.ErrNoActiveDevice):
			m.playback = spotify.PlaybackState{} // Spotify says nothing is playing
		}
		// other errors: keep showing the last known state

	case artworkMsg:
		if msg.url != m.art.url {
			return m, nil // stale: the track changed while this was downloading
		}
		m.art.loading = false
		if msg.err != nil {
			m.art.retryAt = time.Now().Add(artworkRetryDelay)
			return m, nil // the placeholder stays up
		}
		m.art.img = msg.img

	case toggledMsg:
		if msg.err != nil {
			m.status.set(errorText(msg.err))
			return m, nil
		}

		// Optimistic update: apply the change now instead of waiting for Spotify
		now := time.Now()
		m.playback.Progress = m.playback.ProgressAt(now)
		m.playback.FetchedAt = now
		m.playback.Playing = msg.playing
		m.playPending.start(confirmTimeout)
		m.fetching = true
		return m, fetchPlaybackAfter(m.client, settleDelay)

	case commandDoneMsg:
		if msg.err != nil {
			m.status.set(errorText(msg.err))
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

	case shuffleFlushMsg:
		if msg.seq != m.shufflePending.seq {
			return m, nil // a newer key press superseded this one
		}
		on := m.playback.Shuffle
		return m, runCommand(func(ctx context.Context) error {
			return m.client.SetShuffle(ctx, on)
		})
	case repeatFlushMsg:
		if msg.seq != m.repeatPending.seq {
			return m, nil // a newer key press superseded this one
		}
		mode := m.playback.Repeat
		return m, runCommand(func(ctx context.Context) error {
			return m.client.SetRepeat(ctx, mode)
		})

	case listMsg[spotify.Playlist]:
		m.playlists.apply(msg)

	case listMsg[spotify.Track]:
		m.liked.apply(msg) // the only track list until search arrives

	case listMsg[spotify.Play]:
		m.recent.apply(msg)

	case searchMsg:
		if msg.query != m.searchQuery || !m.results.loading {
			return m, nil // stale: an older query, or a duplicate of one already applied
		}
		m.results.apply(listMsg[searchResult]{items: msg.results, err: msg.err})
	default:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
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

// toggleShuffle flips shuffle locally and schedules a debounced send
func (m Model) toggleShuffle() (Model, tea.Cmd) {
	if m.playback.Device.ID == "" {
		return m, nil // no device to send to
	}

	m.playback.Shuffle = !m.playback.Shuffle
	seq := m.shufflePending.start(keyDebounce + confirmTimeout)
	return m, tea.Tick(keyDebounce, func(time.Time) tea.Msg {
		return shuffleFlushMsg{seq: seq}
	})
}

// cycleRepeat advances repeat locally and schedules a debounced send
func (m Model) cycleRepeat() (Model, tea.Cmd) {
	if m.playback.Device.ID == "" {
		return m, nil // no device to send to
	}

	m.playback.Repeat = nextRepeat(m.playback.Repeat)
	seq := m.repeatPending.start(keyDebounce + confirmTimeout)
	return m, tea.Tick(keyDebounce, func(time.Time) tea.Msg {
		return repeatFlushMsg{seq: seq}
	})
}

// nextRepeat cycles off -> context -> track -> off
func nextRepeat(mode spotify.RepeatMode) spotify.RepeatMode {
	switch mode {
	case spotify.RepeatContext:
		return spotify.RepeatTrack
	case spotify.RepeatTrack:
		return spotify.RepeatOff
	default:
		return spotify.RepeatContext
	}
}

// playbackKey reports whether a key sends a playback control
func (m Model) playbackKey(key string) bool {
	switch key {
	case "space", "n", "N", "-", "+", "=", ",", ".", "s", "r":
		return true
	case "enter":
		return m.focus == focusMain // in the sidebar, enter only moves focus
	}
	return false
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
	reconcileValue(&m.shufflePending, now, prev.Shuffle, &m.playback.Shuffle)
	reconcileValue(&m.repeatPending, now, prev.Repeat, &m.playback.Repeat)
}

// reconcileValue keeps our pending value until Spotify reports the same one
func reconcileValue[T comparable](p *pending, now time.Time, ours T, theirs *T) {
	if !p.active(now) {
		return
	}

	if *theirs == ours {
		p.confirm() // Spotify caught up
		return
	}

	*theirs = ours // not yet; keep ours
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
	if errors.Is(err, spotify.ErrLoginExpired) {
		return time.Hour // retrying can't fix this; the user has to sign in again
	}
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
	case sectionSearch:
		m.results.move(delta)
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
		uri := spotify.LikedSongsURI(m.userID)
		pos := m.liked.cursor
		return runCommand(func(ctx context.Context) error {
			return m.client.PlayContext(ctx, uri, pos)
		})
	case sectionRecent:
		p, ok := m.recent.selected()
		if !ok {
			return nil
		}
		return runCommand(func(ctx context.Context) error {
			return m.client.PlayTracks(ctx, []string{p.Track.URI}, 0)
		})
	case sectionSearch:
		r, ok := m.results.selected()
		if !ok {
			return nil
		}
		return r.play(m.client)
	}
	return nil
}

// startSearch jumps to the Search section and focuses the search box
func (m Model) startSearch() (Model, tea.Cmd) {
	m.active = sectionSearch
	m.focus = focusMain
	cmd := m.search.Focus()
	return m, cmd
}

// enterMain moves focus to the main panel; on Search it also opens the search box
func (m Model) enterMain() (Model, tea.Cmd) {
	if m.active == sectionSearch {
		return m.startSearch()
	}

	m.focus = focusMain
	return m, nil
}

// updateSearchInput handles keys while the search box has focus
func (m Model) updateSearchInput(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.search.Blur()
		return m, nil
	case "enter":
		m.search.Blur()
		q := strings.TrimSpace(m.search.Value())
		if q == "" {
			return m, nil
		}
		m.searchQuery = q
		m.results = listData[searchResult]{loading: true} // clear old results

		return m, searchCmd(m.client, q)
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}
