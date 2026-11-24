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
}

// searchMsg carries the result for one query
type searchMsg struct {
	query   string // the query these results are for
	results []searchResult
	err     error
}

// searchCmd runs a search and reports the flattened results
func searchCmd(c *spotify.Client, query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		res, err := c.Search(ctx, query, 0)
		if err != nil {
			return searchMsg{query: query, err: err}
		}

		return searchMsg{query: query, results: flattenResults(res)}
	}
}

// flattenResults puts tracks, then artists, then albums into one list
func flattenResults(res spotify.SearchResults) []searchResult {
	out := make([]searchResult, 0, len(res.Tracks.Items)+len(res.Artists.Items)+len(res.Albums.Items))
	for _, t := range res.Tracks.Items {
		out = append(out, searchResult{kind: "track", label: trackLabel(t), uri: t.URI})
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

// play returns a command that starts playing r
func (r searchResult) play(c *spotify.Client) tea.Cmd {
	return runCommand(func(ctx context.Context) error {
		if r.kind == "track" {
			return c.PlayTracks(ctx, []string{r.uri}, 0)
		}

		return c.PlayContext(ctx, r.uri, 0)
	})
}
