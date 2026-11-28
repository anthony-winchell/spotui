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
	l.moveTo(l.cursor + delta)
}

// moveTo puts the cursor on item i, or as close as the list allows
func (l *listData[T]) moveTo(i int) {
	l.cursor = max(0, min(len(l.items)-1, i))
}

// cursorList is the part of a listData that the movement keys need. Lists
// hold different item types, so they can't share one listData type, but they
// all have these methods.
type cursorList interface {
	move(delta int)
	moveTo(i int)
}

// renderList draws the visible rows of l, at most width cells wide and height rows tall
func renderList[T any](l listData[T], width, height int, label func(T) string) string {
	switch {
	case len(l.items) == 0 && l.loading:
		return dimStyle.Render("Loading...")
	case len(l.items) == 0 && l.err != nil:
		return errorStyle.Render(errorText(l.err))
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

// next starts fetching l's next page if it needs one; otherwise it returns nil.
// source names the list, so that the page finds its way back to it: several
// lists hold tracks, and their pages all arrive as listMsg[spotify.Track].
func (l *listData[T]) next(source string, fetch func(context.Context, int) (spotify.Page[T], error)) tea.Cmd {
	if !l.needsFetch() {
		return nil
	}

	l.loading = true
	return fetchPage(source, fetch, len(l.items))
}

// apply adds a fetched page to l. A page that arrives when l isn't waiting
// for one is dropped: it's a duplicate, from before the list was reset.
func (l *listData[T]) apply(msg listMsg[T]) {
	if !l.loading {
		return
	}
	l.loading = false
	l.err = msg.err
	if msg.err != nil {
		return
	}
	l.items = append(l.items, msg.items...)
	l.hasMore = msg.hasMore
	l.loaded = true
}

// selected returns the highlighted item, or false if the list is empty
func (l listData[T]) selected() (T, bool) {
	if l.cursor >= len(l.items) {
		var zero T
		return zero, false
	}
	return l.items[l.cursor], true
}
