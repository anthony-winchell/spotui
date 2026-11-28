package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// searchResult is one row of a search result; track, artist or album
type searchResult struct {
	kind  string // "track", "artist", or "album"
	label string
	uri   string
	track spotify.Track // the track, if kind is "track"
}

// searchKind is which result the search shows
type searchKind int

const (
	kindAll searchKind = iota // the first page of every type, no paging
	kindTracks
	kindArtists
	kindAlbums
	kindCount
)

var kindNames = [kindCount]string{"All", "Tracks", "Artists", "Albums"}

func (k searchKind) String() string {
	return kindNames[k]
}

// next returns the kind after k, wrapping around
func (k searchKind) next() searchKind {
	return (k + 1) % kindCount
}

// types returns the Spotify types to search for; nil means all of them
func (k searchKind) types() []spotify.SearchType {
	switch k {
	case kindTracks:
		return []spotify.SearchType{spotify.SearchTracks}
	case kindArtists:
		return []spotify.SearchType{spotify.SearchArtists}
	case kindAlbums:
		return []spotify.SearchType{spotify.SearchAlbums}
	}
	return nil
}

// searchMsg carries one page of results for a query and kind
type searchMsg struct {
	query   string // the query these results are for
	kind    searchKind
	results []searchResult
	hasMore bool
	err     error
}

// searchCmd fetches one page of results, starting at offset
func searchCmd(c *spotify.Client, query string, kind searchKind, offset int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		res, err := c.Search(ctx, query, offset, kind.types()...)
		if err != nil {
			return searchMsg{query: query, kind: kind, err: err}
		}

		// Only a single kind pages. Types we didn't ask for come back empty,
		// so at most one kind of these can be true
		hasMore := kind != kindAll && (res.Tracks.HasNext || res.Artists.HasNext || res.Albums.HasNext)
		return searchMsg{query: query, kind: kind, results: flattenResults(res), hasMore: hasMore}
	}
}

// flattenResults puts tracks, then artists, then albums into one list
func flattenResults(res spotify.SearchResults) []searchResult {
	out := make([]searchResult, 0, len(res.Tracks.Items)+len(res.Artists.Items)+len(res.Albums.Items))
	for _, t := range res.Tracks.Items {
		out = append(out, searchResult{kind: "track", label: trackLabel(t), uri: t.URI, track: t})
	}

	for _, a := range res.Artists.Items {
		out = append(out, searchResult{kind: "artist", label: a.Name, uri: a.URI})
	}

	for _, a := range res.Albums.Items {
		label := a.Name + " — " + strings.Join(a.Artists, ", ")
		out = append(out, searchResult{kind: "album", label: label, uri: a.URI})
	}

	return out
}

// play returns a command that starts playing r. A track plays within its
// album, so playback carries on to the album's next track.
func (r searchResult) play(c *spotify.Client) tea.Cmd {
	return runCommand(func(ctx context.Context) error {
		if r.kind == "track" {
			return c.PlayTrackIn(ctx, r.track.AlbumURI, r.uri)
		}

		return c.PlayContext(ctx, r.uri, 0)
	})
}
