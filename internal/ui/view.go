package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// sidebarWidth is the sidebar's total width, including border and padding
const sidebarWidth = 22

const (
	minWidth  = 50 // sidebar (22) plus usable main panel
	minHeight = 16 // bottom panel (6) + hints (1) + sidebar (8), plus a little room
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

	hints := m.statusLine(time.Now())

	// Whatever height is left over after the bottom rows goes to the top row
	topHeight := max(0, m.height-lipgloss.Height(bottom)-lipgloss.Height(hints))
	mainWidth := max(0, m.width-sidebarWidth)

	mainStyle := m.panel(focusMain).Width(mainWidth).Height(topHeight)
	innerHeight := topHeight - mainStyle.GetVerticalFrameSize()
	content := m.mainPanel(mainWidth-mainStyle.GetHorizontalFrameSize(), innerHeight)
	// Clip what's inside, not the panel, so the bottom border stays
	main := mainStyle.Render(lipgloss.NewStyle().MaxHeight(innerHeight).Render(content))

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.panel(focusSidebar).Width(sidebarWidth).Height(topHeight).MaxHeight(topHeight).Render(m.sidebar()),
		main)

	v.Content = lipgloss.JoinVertical(lipgloss.Left, top, bottom, hints)
	return v
}

// mainPanel renders the content for the active section
func (m Model) mainPanel(width, height int) string {
	clip := lipgloss.NewStyle().MaxWidth(width)
	bodyHeight := height - 2 // title + blank line

	if m.showHelp {
		return clip.Render(titleStyle.Render("Keys")) + "\n\n" +
			m.fullHelp(width) + "\n\n" +
			dimStyle.Render("Press any key to close")
	}

	if v := m.openView(); v != nil {
		title := clip.Render(titleStyle.Render(m.active.String() + " › " + v.title))
		return title + "\n\n" + renderList(v.tracks, width, bodyHeight, trackLabel)
	}

	title := titleStyle.Render(m.active.String())

	var body string
	switch m.active {
	case sectionNowPlaying:
		body = m.nowPlayingView(width, bodyHeight)
	case sectionQueue:
		body = renderList(m.queue, width, bodyHeight, trackLabel)
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
			body += m.searchTabs() + "\n"
			body += renderList(m.results, width, bodyHeight-3, func(r searchResult) string {
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
		icon + " " + titleStyle.Render(t.Name) + m.heart(),
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

// shortHelp returns the keys for the hint line, most useful first. The
// help renderer drops whatever doesn't fit; ? shows the rest.
func (m Model) shortHelp() []key.Binding {
	k := m.keys
	if m.search.Focused() {
		return []key.Binding{k.submit, k.cancel, k.forceQuit}
	}

	keys := []key.Binding{k.Help}
	switch {
	case m.showHelp:
		return append(keys, k.Quit)
	case m.focus == focusSidebar:
		keys = append(keys, k.move, k.Open, k.Search)
	default:
		keys = append(keys, k.move)
		if m.active != sectionQueue {
			keys = append(keys, k.Play)
		}
		if m.canOpen() {
			keys = append(keys, k.Open)
		}
		keys = append(keys, k.Back)
		if _, ok := m.selectedTrack(); ok {
			keys = append(keys, k.Enqueue, k.LikeSelected)
		}
		if m.active == sectionSearch && m.searchQuery != "" && m.openView() == nil {
			keys = append(keys, k.SearchType)
		}
	}
	return append(keys, k.PlayPause, k.Next, k.Like, k.Quit)
}

// fullHelp renders every key binding in columns, in two rows of columns if
// they don't all fit side by side
func (m Model) fullHelp(width int) string {
	groups := m.keys.fullHelp()
	h := m.help
	h.SetWidth(0) // no limit, so the width can be measured
	if all := h.FullHelpView(groups); lipgloss.Width(all) <= width {
		return all
	}

	h.SetWidth(width)
	half := (len(groups) + 1) / 2
	return h.FullHelpView(groups[:half]) + "\n\n" + h.FullHelpView(groups[half:])
}

// canOpen reports whether the highlighted item can be opened
func (m Model) canOpen() bool {
	if m.openView() != nil {
		return false
	}
	switch m.active {
	case sectionPlaylists:
		return len(m.playlists.items) > 0
	case sectionSearch:
		r, ok := m.results.selected()
		return ok && (r.kind == "album" || r.kind == "track" && r.track.AlbumURI != "")
	}
	return false
}

// searchTabs renders the result kinds, highlighting the selected one
func (m Model) searchTabs() string {
	tabs := make([]string, kindCount)
	for k := range kindCount {
		if k == m.searchKind {
			tabs[k] = selectedStyle.Render(k.String())
		} else {
			tabs[k] = dimStyle.Render(k.String())
		}
	}
	return strings.Join(tabs, "  ")
}

// statusLine returns the bottom line: a recent command error, then an
// ongoing sync error, then the key hints
func (m Model) statusLine(now time.Time) string {
	line := errorStyle.MaxWidth(m.width)
	switch {
	case m.status.visible(now):
		return line.Render(m.status.text)
	case m.err != nil && !errors.Is(m.err, spotify.ErrNoActiveDevice):
		text := errorText(m.err)
		if !errors.Is(m.err, spotify.ErrLoginExpired) {
			text += " (retrying)"
		}
		return line.Render(text)
	}
	h := m.help
	h.SetWidth(m.width)
	return h.ShortHelpView(m.shortHelp())
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

	if p.Device.Restricted {
		device += dimStyle.Render(" • view only")
	}

	lines := []string{
		titleStyle.Render(t.Name) + m.heart(),
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
	case m.err != nil && m.playback.Track == nil:
		return errorStyle.Render("Couldn't reach Spotify")
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
