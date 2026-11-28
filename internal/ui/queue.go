package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// queuedMsg reports the result of adding a track to the queue
type queuedMsg struct {
	name string
	err  error
}

// enqueue adds the highlighted track to the end of the queue
func (m Model) enqueue() (Model, tea.Cmd) {
	t, ok := m.selectedTrack()
	if !ok {
		m.status.set("Highlight a track to add it to the queue")
		return m, nil
	}

	c := m.client
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return queuedMsg{name: t.Name, err: c.AddToQueue(ctx, t.URI)}
	}
}

// selectedTrack returns the highlighted track, or false if the main panel
// doesn't have focus or isn't showing a track (e.g. an artist search result)
func (m Model) selectedTrack() (spotify.Track, bool) {
	if m.focus != focusMain {
		return spotify.Track{}, false
	}

	var t spotify.Track
	var ok bool
	if v := m.openView(); v != nil {
		t, ok = v.tracks.selected()
	} else {
		switch m.active {
		case sectionQueue:
			t, ok = m.queue.selected()
		case sectionLiked:
			t, ok = m.liked.selected()
		case sectionRecent:
			var p spotify.Play
			p, ok = m.recent.selected()
			t = p.Track
		case sectionSearch:
			var r searchResult
			r, ok = m.results.selected()
			ok = ok && r.kind == "track"
			t = r.track
		}
	}

	return t, ok && t.URI != "" // unavailable playlist entries have no URI
}
