package ui

import (
	"context"
	"errors"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anthony-winchell/spotui/internal/spotify"
)

// likeStatus is whether the current track is in the user's Liked Songs
type likeStatus struct {
	uri      string // the track this is about
	known    bool   // false until Spotify has answered
	liked    bool
	checking bool // a Liked request is in flight
	saving   bool // a Like or Unlike request is in flight
}

// likedMsg carries whether the track at uri is liked
type likedMsg struct {
	uri   string
	liked bool
	err   error
}

// likeDoneMsg reports the result of liking or unliking the track at uri
type likeDoneMsg struct {
	uri        string
	name       string // the track's name, for the status message
	liked      bool   // the state we asked for
	optimistic bool   // the heart was flipped before asking, so undo it on failure
	err        error
}

// checkLiked asks Spotify whether the current track is liked, unless we
// already know or are already checking
func (m *Model) checkLiked() tea.Cmd {
	t := m.playback.Track
	if t == nil || t.URI == "" || t.Episode {
		return nil // episodes go in Your Episodes, not Liked Songs
	}

	if t.URI != m.like.uri {
		m.like = likeStatus{uri: t.URI}
	}

	if m.like.known || m.like.checking {
		return nil
	}

	m.like.checking = true
	c, uri := m.client, t.URI
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		liked, err := c.Liked(ctx, uri)
		if err == nil && len(liked) == 0 {
			err = errors.New("empty response")
		}
		if err != nil {
			return likedMsg{uri: uri, err: err}
		}
		return likedMsg{uri: uri, liked: liked[0]}
	}
}

// toggleLike likes or unlikes the current track, updating the heart right away
func (m Model) toggleLike() (Model, tea.Cmd) {
	if t := m.playback.Track; t != nil && t.Episode {
		m.status.set("Only songs can be liked")
		return m, nil
	}
	if !m.like.known || m.like.saving || m.like.uri != currentURI(m.playback) {
		return m, nil // nothing to toggle yet, or a change is already on its way
	}

	m.like.liked = !m.like.liked
	m.like.saving = true

	c, uri, want := m.client, m.like.uri, m.like.liked
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		var err error
		if want {
			err = c.Like(ctx, uri)
		} else {
			err = c.Unlike(ctx, uri)
		}

		return likeDoneMsg{uri: uri, liked: want, optimistic: true, err: err}
	}
}

// likeSelected likes the highlighted track, or unlikes it if it's already liked
func (m Model) likeSelected() (Model, tea.Cmd) {
	t, ok := m.selectedTrack()
	if !ok || !strings.HasPrefix(t.URI, "spotify:track:") {
		m.status.set("Highlight a song to like it")
		return m, nil
	}

	c := m.client
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		liked, err := c.Liked(ctx, t.URI)
		if err == nil && len(liked) == 0 {
			err = errors.New("empty response")
		}
		if err != nil {
			return likeDoneMsg{uri: t.URI, name: t.Name, err: err}
		}

		want := !liked[0]
		if want {
			err = c.Like(ctx, t.URI)
		} else {
			err = c.Unlike(ctx, t.URI)
		}
		return likeDoneMsg{uri: t.URI, name: t.Name, liked: want, err: err}
	}
}

// likeDone applies the result of liking or unliking a track
func (m Model) likeDone(msg likeDoneMsg) (Model, tea.Cmd) {
	if msg.uri == m.like.uri {
		if msg.optimistic {
			m.like.saving = false
			if msg.err != nil {
				m.like.liked = !msg.liked // undo the optimistic change
			}
		}
		if msg.err == nil {
			m.like.known, m.like.liked = true, msg.liked // F on the playing track
		}
	}

	if msg.err != nil {
		m.status.set(errorText(msg.err))
		return m, nil
	}

	if !msg.optimistic { // the heart doesn't show it, so say so
		if msg.liked {
			m.status.set("Liked " + msg.name)
		} else {
			m.status.set("Removed " + msg.name + " from Liked Songs")
		}
	}

	m.likedChanged(msg.uri, msg.liked)
	return m, m.fetchActive()
}

// likedChanged updates Liked Songs after the track at uri was liked or
// unliked. An unliked song is taken out where it is, so the cursor stays put;
// a newly liked song goes to the top, so the list is reloaded.
func (m *Model) likedChanged(uri string, liked bool) {
	if liked {
		m.liked = listData[spotify.Track]{}
		return
	}

	i := slices.IndexFunc(m.liked.items, func(t spotify.Track) bool { return t.URI == uri })
	if i < 0 {
		return
	}
	// Clone first: older copies of the Model share the backing array
	m.liked.items = slices.Delete(slices.Clone(m.liked.items), i, i+1)
	m.liked.move(0) // keep the cursor within the list
}

// currentURI returns the playing track's URI, or "" if nothing's playing
func currentURI(p spotify.PlaybackState) string {
	if p.Track == nil {
		return ""
	}
	return p.Track.URI
}

// heart renders the like status: filled if liked, outlined if not, nothing if unknown
func (m Model) heart() string {
	switch {
	case !m.like.known || m.like.uri != currentURI(m.playback):
		return ""
	case m.like.liked:
		return " " + lipgloss.NewStyle().Foreground(colorPink).Render("♥")
	default:
		return " " + dimStyle.Render("♥")
	}
}
