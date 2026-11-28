package ui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// trackView is a playlist or album opened to show its tracks. It covers the
// section it was opened from until it's closed with back.
type trackView struct {
	section    section // the section it was opened from; it shows only there
	title      string
	contextURI string // the playlist or album; tracks play within it
	fetch      func(context.Context, int) (spotify.Page[spotify.Track], error)
	tracks     listData[spotify.Track]
}

// next fetches the view's next page of tracks, if it needs one. Pages are
// tagged with the context URI, so a page for a view that has since been
// replaced is recognised and dropped.
func (v *trackView) next() tea.Cmd {
	return v.tracks.next(v.contextURI, v.fetch)
}

// playlistView opens a playlist
func playlistView(c *spotify.Client, p spotify.Playlist) *trackView {
	return &trackView{
		title:      p.Name,
		contextURI: p.URI,
		fetch: func(ctx context.Context, offset int) (spotify.Page[spotify.Track], error) {
			return c.PlaylistTracks(ctx, p.ID, offset)
		},
	}
}

// albumView opens an album
func albumView(c *spotify.Client, name, uri string) *trackView {
	id := spotify.IDFromURI(uri)
	return &trackView{
		title:      name,
		contextURI: uri,
		fetch: func(ctx context.Context, offset int) (spotify.Page[spotify.Track], error) {
			return c.AlbumTracks(ctx, id, offset)
		},
	}
}

// openView returns the open playlist or album if it belongs to the active
// section, or nil
func (m Model) openView() *trackView {
	if m.view != nil && m.view.section == m.active {
		return m.view
	}
	return nil
}

// open opens the highlighted playlist or album. On a track in the search
// results it opens the track's album.
func (m Model) open() (Model, tea.Cmd) {
	if m.openView() != nil {
		return m, nil // already in one; there's nothing deeper
	}

	var v *trackView
	switch m.active {
	case sectionPlaylists:
		if p, ok := m.playlists.selected(); ok {
			v = playlistView(m.client, p)
		}
	case sectionSearch:
		r, ok := m.results.selected()
		switch {
		case !ok:
		case r.kind == "album":
			v = albumView(m.client, r.label, r.uri)
		case r.kind == "track" && r.track.AlbumURI != "":
			v = albumView(m.client, r.track.Album, r.track.AlbumURI)
		default:
			m.status.set("Only playlists and albums can be opened")
		}
	}
	if v == nil {
		return m, nil
	}

	v.section = m.active
	m.view = v
	return m, v.next()
}

// playInView plays the open view from its highlighted track. A playlist
// Spotify won't list plays from the start.
func (m Model) playInView(v *trackView) tea.Cmd {
	uri, pos := v.contextURI, v.tracks.cursor
	if _, ok := v.tracks.selected(); !ok {
		if !errors.Is(v.tracks.err, spotify.ErrPlaylistHidden) {
			return nil
		}
		pos = 0
	}
	return runCommand(func(ctx context.Context) error {
		return m.client.PlayContext(ctx, uri, pos)
	})
}
