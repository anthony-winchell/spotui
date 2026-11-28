package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
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
		return m.handleKey(msg)

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
			if currentURI(prev) != currentURI(m.playback) {
				m.queue = listData[spotify.Track]{} // it moved on; reload when shown
			}
			art := m.fetchArtwork(now)
			like := m.checkLiked()
			return m, tea.Batch(art, like, m.fetchActive())
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

	case likedMsg:
		if msg.uri != m.like.uri {
			return m, nil
		}
		m.like.checking = false
		if msg.err == nil {
			m.like.known = true
			m.like.liked = msg.liked
		}
	// on error, known state stays false and the next sync asks again

	case likeDoneMsg:
		return m.likeDone(msg)

	case queuedMsg:
		if msg.err != nil {
			m.status.set(errorText(msg.err))
			return m, nil
		}
		m.status.set("Added " + msg.name + " to the queue")
		m.queue = listData[spotify.Track]{} // reload when shown
		return m, m.fetchActive()

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
		switch {
		case msg.source == sourceLiked:
			m.liked.apply(msg)
		case msg.source == sourceQueue:
			m.queue.apply(msg)
		case m.view != nil && msg.source == m.view.contextURI:
			m.view.tracks.apply(msg)
			if errors.Is(msg.err, spotify.ErrPlaylistHidden) {
				m.view.tracks.loaded = true // asking again won't change the answer
			}
		}
		// anything else is for a view that has since been closed

	case listMsg[spotify.Play]:
		m.recent.apply(msg)

	case searchMsg:
		if msg.query != m.searchQuery || msg.kind != m.searchKind || !m.results.loading {
			return m, nil // stale: an older query, or a duplicate of one already applied
		}
		m.results.apply(listMsg[searchResult]{items: msg.results, hasMore: msg.hasMore, err: msg.err})
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

// playbackKey reports whether a key sends a command to the playback device
func (m Model) playbackKey(msg tea.KeyPressMsg) bool {
	if key.Matches(msg, m.keys.playback()...) {
		return true
	}
	return m.focus == focusMain && key.Matches(msg, m.keys.Play) // in the sidebar, enter only moves focus
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

// Sources of the track lists' pages; see listData.next. An open view uses its
// context URI, which can't clash with these.
const (
	sourceLiked = "liked"
	sourceQueue = "queue"
)

// fetchActive starts fetching the active section's next page, if it needs one
func (m *Model) fetchActive() tea.Cmd {
	if v := m.openView(); v != nil {
		return v.next()
	}

	switch m.active {
	case sectionQueue:
		return m.queue.next(sourceQueue, queuePage(m.client))
	case sectionPlaylists:
		return m.playlists.next("playlists", m.client.Playlists)
	case sectionLiked:
		return m.liked.next(sourceLiked, m.client.LikedSongs)
	case sectionRecent:
		return m.recent.next("recent", recentPage(m.client))
	case sectionSearch:
		if m.searchQuery == "" || !m.results.needsFetch() {
			return nil
		}
		m.results.loading = true
		return searchCmd(m.client, m.searchQuery, m.searchKind, len(m.results.items))
	}
	return nil
}

// far is more rows than any list has, for jumping to either end
const far = 1 << 30

// moveCursor moves the sidebar's cursor, or the showing list's, by delta
func (m *Model) moveCursor(delta int) {
	if m.focus == focusSidebar {
		m.active = section(max(0, min(int(sectionCount)-1, int(m.active)+delta)))
		return
	}
	if l := m.activeList(); l != nil {
		l.move(delta)
	}
}

// activeList returns the list showing in the main panel, or nil if there's none
func (m *Model) activeList() cursorList {
	if v := m.openView(); v != nil {
		return &v.tracks
	}

	switch m.active {
	case sectionQueue:
		return &m.queue
	case sectionPlaylists:
		return &m.playlists
	case sectionLiked:
		return &m.liked
	case sectionRecent:
		return &m.recent
	case sectionSearch:
		return &m.results
	}
	return nil
}

// pageSize is how far ctrl+d and ctrl+u move: half the rows a list shows.
// It's worked out from the layout in View: the bottom panel (6 rows), the
// hint line, the main panel's border and its title with a blank line.
func (m Model) pageSize() int {
	return max(1, (m.height-11)/2)
}

// playSelected returns a command that plays the highlighted item, or nil if there is none
func (m Model) playSelected() tea.Cmd {
	if v := m.openView(); v != nil {
		return m.playInView(v)
	}

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
		// Play it where it was played from, so playback carries on from there
		return runCommand(func(ctx context.Context) error {
			return m.client.PlayTrackIn(ctx, p.ContextURI, p.Track.URI)
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
	if m.openView() != nil {
		m.view = nil // close the album opened from the old results
	}
	cmd := m.search.Focus()
	return m, cmd
}

// setSearchKind switches the result type and searches again from the first page
func (m Model) setSearchKind(k searchKind) (Model, tea.Cmd) {
	m.searchKind = k
	m.results = listData[searchResult]{loading: true}
	return m, searchCmd(m.client, m.searchQuery, k, 0)
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

		return m, searchCmd(m.client, q, m.searchKind, 0)
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	return m, cmd
}

// handleKey handles a key press
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case m.search.Focused():
		return m.updateSearchInput(msg)
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case m.showHelp:
		m.showHelp = false // any other key closes it
		return m, nil
	case m.playback.Device.Restricted && m.playbackKey(msg):
		m.status.set("This device can't be controlled from spotui")
		return m, nil
	}

	switch {
	case key.Matches(msg, k.Help):
		m.showHelp = true

	// Playback
	case key.Matches(msg, k.PlayPause):
		return m, togglePlayback(m.client, m.playback.Playing)
	case key.Matches(msg, k.Next):
		return m, runCommand(m.client.Next)
	case key.Matches(msg, k.Prev):
		return m, runCommand(m.client.Previous)
	case key.Matches(msg, k.VolDown):
		return m.changeVolume(-m.volumeStep)
	case key.Matches(msg, k.VolUp):
		return m.changeVolume(m.volumeStep)
	case key.Matches(msg, k.SeekBack):
		return m.seek(-m.seekStep)
	case key.Matches(msg, k.SeekFwd):
		return m.seek(m.seekStep)
	case key.Matches(msg, k.Shuffle):
		return m.toggleShuffle()
	case key.Matches(msg, k.Repeat):
		return m.cycleRepeat()

	// Library
	case key.Matches(msg, k.Like):
		return m.toggleLike()
	case key.Matches(msg, k.LikeSelected):
		return m.likeSelected()
	case key.Matches(msg, k.Enqueue):
		return m.enqueue()

	// Moving around
	case key.Matches(msg, k.SearchType):
		if m.active == sectionSearch && m.searchQuery != "" && m.openView() == nil {
			return m.setSearchKind(m.searchKind.next())
		}
	case key.Matches(msg, k.Back):
		if m.focus == focusMain && m.openView() != nil {
			m.view = nil // back out of the playlist or album
		} else {
			m.focus = focusSidebar
		}
	case key.Matches(msg, k.Search):
		return m.startSearch()
	case key.Matches(msg, k.Open):
		if m.focus == focusSidebar {
			return m.enterMain()
		}
		return m.open()
	case key.Matches(msg, k.Play):
		switch {
		case m.focus == focusSidebar:
			return m.enterMain()
		case m.active == sectionQueue:
			m.status.set("Spotify can't jump ahead in the queue: press n to skip")
			return m, nil
		}
		return m, m.playSelected()
	case key.Matches(msg, k.Down):
		return m.moved(1)
	case key.Matches(msg, k.Up):
		return m.moved(-1)
	case key.Matches(msg, k.PageDown):
		return m.moved(m.pageSize())
	case key.Matches(msg, k.PageUp):
		return m.moved(-m.pageSize())
	case key.Matches(msg, k.Bottom):
		return m.moved(far)
	case key.Matches(msg, k.Top):
		return m.moved(-far)
	}
	return m, nil
}

// moved moves the cursor by delta, then fetches more of the list if it's
// near the end, or the new section's list if it's the sidebar that moved
func (m Model) moved(delta int) (Model, tea.Cmd) {
	m.moveCursor(delta)
	return m, m.fetchActive()
}
