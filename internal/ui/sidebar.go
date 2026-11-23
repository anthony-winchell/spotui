package ui

import "strings"

// section is one entry in the sidebar
type section int

const (
	sectionNowPlaying section = iota
	sectionPlaylists
	sectionLiked
	sectionRecent
	sectionSearch

	sectionCount // number of sections; must stay last
)

// String returns the label shown in the sidebar
func (s section) String() string {
	switch s {
	case sectionNowPlaying:
		return "Now Playing"
	case sectionPlaylists:
		return "Playlists"
	case sectionLiked:
		return "Liked Songs"
	case sectionRecent:
		return "Recently Played"
	case sectionSearch:
		return "Search"
	}
	return "?"
}

// sidebar renders the section list, highlighting the active one
func (m Model) sidebar() string {
	lines := make([]string, 0, sectionCount)
	for s := range sectionCount {
		if s == m.active {
			lines = append(lines, selectedStyle.Render("> "+s.String()))
		} else {
			lines = append(lines, " "+s.String())
		}
	}

	return strings.Join(lines, "\n")
}
