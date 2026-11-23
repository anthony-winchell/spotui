package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// listData is a list loaded from Spotify, plus its cursor
type listData[T any] struct {
	items   []T
	loading bool  // a fetch is in flight
	loaded  bool  // a fetch succeeded; don't fetch again
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
	case l.loading:
		return dimStyle.Render("Loading...")
	case l.err != nil:
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
