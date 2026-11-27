package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// sidebarWidth is the sidebar's total width, including border and padding
const sidebarWidth = 22

const (
	minWidth  = 50 // sidebar (22) plus usable main panel
	minHeight = 16 // bottom panel (6) + hints (1) + sidebar (7), plus a little room
)

// searchWidth is how many cells of the query fit in the main panel
func (m Model) searchWidth() int {
	inner := m.width - sidebarWidth - panelStyle.GetHorizontalFrameSize()
	return max(1, inner-lipgloss.Width(m.search.Prompt)-1)
}

// View renders the model
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true

	switch {
	case m.width == 0:
		return v // no WindowSizeMsg yet
	case m.width < minWidth || m.height < minHeight:
		v.Content = dimStyle.Render(fmt.Sprintf("Terminal too small: %dx%d (need %dx%d)", m.width, m.height, minWidth, minHeight))
		return v
	}

	bottom := panelStyle.Width(m.width).Render(m.nowPlaying())

	hints := dimStyle.Render(m.hints())
	if m.status.visible(time.Now()) {
		hints = errorStyle.MaxWidth(m.width).Render(m.status.text)
	}

	// Whatever height is left over after the bottom rows goes to the top row
	topHeight := max(0, m.height-lipgloss.Height(bottom)-lipgloss.Height(hints))
	mainWidth := max(0, m.width-sidebarWidth)

	mainStyle := m.panel(focusMain).Width(mainWidth).Height(topHeight)
	main := mainStyle.Render(m.mainPanel(
		mainWidth-mainStyle.GetHorizontalFrameSize(),
		topHeight-mainStyle.GetVerticalFrameSize(),
	))

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.panel(focusSidebar).Width(sidebarWidth).Height(topHeight).MaxHeight(topHeight).Render(m.sidebar()),
		main)

	v.Content = lipgloss.JoinVertical(lipgloss.Left, top, bottom, hints)
	return v
}

// mainPanel renders the content for the active section
func (m Model) mainPanel(width, height int) string {
	title := titleStyle.Render(m.active.String())
	bodyHeight := height - 2 // title + blank line

	var body string
	switch m.active {
	case sectionNowPlaying:
		body = m.nowPlayingView(width, bodyHeight)
	case sectionPlaylists:
		body = renderList(m.playlists, width, bodyHeight, func(p spotify.Playlist) string {
			return p.Name
		})
	case sectionLiked:
		body = renderList(m.liked, width, bodyHeight, trackLabel)
	case sectionRecent:
		body = renderList(m.recent, width, bodyHeight, func(p spotify.Play) string {
			return p.PlayedAt.Local().Format("Mon 15:04") + " " + trackLabel(p.Track)
		})
	case sectionSearch:
		body = m.search.View() + "\n\n"
		if m.searchQuery == "" {
			body += dimStyle.Render("Press / to input. Enter to search")
		} else {
			body += renderList(m.results, width, bodyHeight-2, func(r searchResult) string {
				return fmt.Sprintf("%-6s %s", r.kind, r.label)
			})
		}

	default:
		body = dimStyle.Render("Nothing yet")
	}

	return title + "\n\n" + body
}

// trackLabel formats a track as "Name — Artist, Artist"
func trackLabel(t spotify.Track) string {
	return t.Name + " — " + strings.Join(t.Artists, ", ")
}

// panel returns the panel style, highlighted if f has focus
func (m Model) panel(f focus) lipgloss.Style {
	if m.focus == f {
		return panelStyle.BorderForeground(colorPurple)
	}
	return panelStyle
}

// nowPlaying renders the content of the bottom panel
func (m Model) nowPlaying() string {
	if n := m.playbackNotice(); n != "" {
		return n
	}

	t := m.playback.Track
	icon := "⏸"
	if m.playback.Playing {
		icon = lipgloss.NewStyle().Foreground(colorGreen).Render("▶")
	}
	pos := m.playback.ProgressAt(time.Now())

	times := fmt.Sprintf(" %s / %s", mmss(pos), mmss(t.Duration))
	inner := m.width - panelStyle.GetHorizontalFrameSize() // minus border + padding
	bar := progressBar(pos, t.Duration, inner-lipgloss.Width(times))

	lines := []string{
		icon + " " + titleStyle.Render(t.Name),
		artistStyle.Render(strings.Join(t.Artists, ", ")),
		bar + times,
	}

	d := m.playback.Device
	parts := []string{dimStyle.Render(d.Name)}
	if d.SupportsVolume {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("vol %d%%", d.Volume)))
	}

	parts = append(parts, modeLabel("shuffle", m.playback.Shuffle), repeatLabel(m.playback.Repeat))
	lines = append(lines, strings.Join(parts, dimStyle.Render(" • ")))

	return lipgloss.NewStyle().MaxWidth(inner).Render(strings.Join(lines, "\n"))
}

const (
	maxArtHeight    = 12 // tallest the artwork square gets, in rows
	minArtHeight    = 6  // below this, skip the artwork
	minDetailsWidth = 30 // narrowest the details column can be beside the artwork
)

// nowPlayingView renders the Now Playing section: artwork beside track details
func (m Model) nowPlayingView(width, height int) string {
	if n := m.playbackNotice(); n != "" {
		return n
	}

	artHeight := min(height, maxArtHeight)
	artWidth := artHeight * 2 // terminal cells are about twice as tall as wide
	gap := 2

	if artHeight < minArtHeight || width-artWidth-gap < minDetailsWidth {
		return lipgloss.NewStyle().MaxHeight(height).Render(m.trackDetails(width)) // no room: text only
	}

	view := lipgloss.JoinHorizontal(lipgloss.Top, m.artworkView(artWidth, artHeight), strings.Repeat(" ", gap), m.trackDetails(width-artWidth-gap))
	return lipgloss.NewStyle().MaxHeight(height).Render(view)
}

// artworkView renders the current track's artwork, or a placeholder
func (m Model) artworkView(width, height int) string {
	if m.art.img == nil || m.art.url != m.playback.Track.ArtworkURL {
		return artPlaceholder(width, height)
	}
	return renderArtwork(m.art.img, width, height)
}

// artPlaceholder draws an empty frame where album artwork will go
func artPlaceholder(width, height int) string {
	return lipgloss.NewStyle().Width(width).Height(height).Border(lipgloss.RoundedBorder()).BorderForeground(colorComment).Foreground(colorComment).Align(lipgloss.Center, lipgloss.Center).Render("♪")
}

// hints returns the key help for the current context, as many as fit in the terminal width
func (m Model) hints() string {
	var keys []string
	switch {
	case m.search.Focused():
		keys = []string{"enter search", "esc cancel", "ctrl+c quit"}
	case m.focus == focusSidebar:
		keys = []string{"j/k sections", "l open", "/ search"}
	default:
		keys = []string{"j/k move", "enter play", "n/N next/prev", "/ search"}
	}

	if !m.search.Focused() {
		keys = append(keys, "space pause/play", "esc/h back", ",/. seek", "-/+ vol", "s shuffle", "r repeat", "q quit")
	}

	line := ""

	for _, k := range keys {
		next := k
		if line != "" {
			next = line + " • " + k
		}
		if lipgloss.Width(next) > m.width {
			break // keep whole hints only
		}
		line = next
	}
	return line
}

// trackDetails renders the current track's details, at most width cells wide
func (m Model) trackDetails(width int) string {
	p := m.playback
	t := p.Track

	pos := p.ProgressAt(time.Now())
	times := fmt.Sprintf(" %s / %s", mmss(pos), mmss(t.Duration))

	state := dimStyle.Render("⏸ Paused")
	if p.Playing {
		state = onStyle.Render("▶ Playing")
	}

	device := dimStyle.Render(fmt.Sprintf("on %s (%s)", p.Device.Name, p.Device.Type))
	if p.Device.SupportsVolume {
		device += dimStyle.Render(fmt.Sprintf(" • vol %d%%", p.Device.Volume))
	}

	lines := []string{
		titleStyle.Render(t.Name),
		artistStyle.Render(strings.Join(t.Artists, ", ")),
		dimStyle.Render(t.Album),
		"",
		progressBar(pos, t.Duration, width-lipgloss.Width(times)) + times,
		"",
		state,
		device,
		modeLabel("shuffle", p.Shuffle) + dimStyle.Render(" • ") + repeatLabel(p.Repeat),
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(lines, "\n"))
}

// playbackNotice returns a message to show instead of track details, or "" when there is a track to show
func (m Model) playbackNotice() string {
	switch {
	case m.loading:
		return dimStyle.Render("Loading...")
	case errors.Is(m.err, spotify.ErrNoActiveDevice):
		return dimStyle.Render("No active device. Open Spotify and start playing something.")
	case m.err != nil:
		return errorStyle.Render("Error: " + m.err.Error())
	case m.playback.Track == nil:
		return dimStyle.Render("Nothing playing")
	}

	return ""
}

// modeLabel renders a playback mode, highlighted when on
func modeLabel(name string, on bool) string {
	if on {
		return onStyle.Render(name)
	}
	return dimStyle.Render(name)
}

// repeatLabel renders the repeat mode
func repeatLabel(r spotify.RepeatMode) string {
	switch r {
	case spotify.RepeatTrack:
		return modeLabel("repeat one", true)
	case spotify.RepeatContext:
		return modeLabel("repeat", true)
	default:
		return modeLabel("repeat", false)
	}
}

// mmss formats d as m:ss
func mmss(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
