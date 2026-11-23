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

// View renders the model
func (m Model) View() tea.View {
	bottom := panelStyle.Width(m.width).Render(m.nowPlaying())
	hints := dimStyle.Render("h/l panels • j/k move • space play/pause • n next • N prev • ,/. seek • +/- vol • q quit")

	// Whatever height is left over after the bottom rows goes to the top row
	topHeight := max(0, m.height-lipgloss.Height(bottom)-lipgloss.Height(hints))
	mainWidth := max(0, m.width-sidebarWidth)

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		m.panel(focusSidebar).Width(sidebarWidth).Height(topHeight).Render(m.sidebar()),
		m.panel(focusMain).Width(mainWidth).Height(topHeight).Render(m.mainPanel()))

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, top, bottom, hints))
	v.AltScreen = true
	return v
}

// mainPanel renders the content for the active section
func (m Model) mainPanel() string {
	return titleStyle.Render(m.active.String()) + "\n\n" + dimStyle.Render("Nothing yet")
}

func (m Model) panel(f focus) lipgloss.Style {
	if m.focus == f {
		return panelStyle.BorderForeground(colorPurple)
	}
	return panelStyle
}

// nowPlaying renders the content of the bottom panel
func (m Model) nowPlaying() string {
	switch {
	case m.loading:
		return dimStyle.Render("Loading...")
	case errors.Is(m.err, spotify.ErrNoActiveDevice):
		return dimStyle.Render("No active device. Open Spotify and start playing something.")
	case m.err != nil:
		return errorStyle.Render("Error: " + m.err.Error())
	case m.playback.Track == nil:
		return dimStyle.Render("Nothing playing.")
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

	if d := m.playback.Device; d.SupportsVolume {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%s • vol %d%%", d.Name, d.Volume)))
	}

	return strings.Join(lines, "\n")
}

// mmss formats d as m:ss
func mmss(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
