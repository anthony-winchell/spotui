package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anthony-winchell/spotui/internal/spotify"
)

// loadAhead is how close the cursor gets to the end of a list before the next page is fetched
const loadAhead = 10

// listData is a list loaded from Spotify one page at a time, plus its cursor
type listData[T any] struct {
	items   []T
	loading bool  // a fetch is in flight
	loaded  bool  // a fetch succeeded; don't fetch again
	hasMore bool  // Spotify has more pages
	err     error // last fetch error, if any
	cursor  int   // index of the highlighted item
}

// move shifts the cursor by delta, staying within the items
func (l *listData[T]) move(delta int) {
	l.cursor = max(0, min(len(l.items)-1, l.cursor+delta))
}

// renderList draws the visible rows of l, at most width cells wide and height rows tall
func renderList[T any](l listData[T], width, height int, label func(T) string) string {
	switch {
	case len(l.items) == 0 && l.loading:
		return dimStyle.Render("Loading...")
	case len(l.items) == 0 && l.err != nil:
		return errorStyle.Render("Error: " + l.err.Error())
	case len(l.items) == 0:
		return dimStyle.Render("Nothing here.")
	case height <= 0:
		return ""
	}

	// Keep the cursor in the middle once there's enough above it
	top := max(0, min(l.cursor-height/2, len(l.items)-height))
	end := min(len(l.items), top+height)

	clip := lipgloss.NewStyle().MaxWidth(width)
	rows := make([]string, 0, end-top)
	for i := top; i < end; i++ {
		if i == l.cursor {
			rows = append(rows, clip.Render(selectedStyle.Render("> "+label(l.items[i]))))
		} else {
			rows = append(rows, clip.Render(" "+label(l.items[i])))
		}
	}

	return strings.Join(rows, "\n")
}

// needsFetch reports whether l should request its next page now
func (l *listData[T]) needsFetch() bool {
	switch {
	case l.loading:
		return false
	case !l.loaded:
		return true // first page, or retrying after an error
	default:
		return l.hasMore && l.cursor >= len(l.items)-loadAhead
	}
}

// next starts fetching l's next page if it needs one; otherwise it returns nil
func (l *listData[T]) next(fetch func(context.Context, int) (spotify.Page[T], error)) tea.Cmd {
	if !l.needsFetch() {
		return nil
	}

	l.loading = true
	return fetchPage(fetch, len(l.items))
}

// apply adds a fetched page to l
func (l *listData[T]) apply(msg listMsg[T]) {
	l.loading = false
	l.err = msg.err
	if msg.err != nil {
		return
	}
	l.items = append(l.items, msg.items...)
	l.hasMore = msg.hasMore
	l.loading = true
}

// selected returns the highlighted item, or false if the list is empty
func (l listData[T]) selected() (T, bool) {
	if l.cursor >= len(l.items) {
		var zero T
		return zero, false
	}
	return l.items[l.cursor], true
}
