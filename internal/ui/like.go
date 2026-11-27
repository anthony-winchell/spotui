package ui

import (
	"context"
	"errors"

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
	uri   string
	liked bool // the state we asked for
	err   error
}

// checkLiked asks Spotify whether the current track is liked, unless we
// already know or are already checking
func (m *Model) checkLiked() tea.Cmd {
	t := m.playback.Track
	if t == nil || t.URI == "" {
		return nil
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

		return likeDoneMsg{uri: uri, liked: want, err: err}
	}
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
