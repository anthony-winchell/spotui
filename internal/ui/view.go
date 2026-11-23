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

// View renders the model
func (m Model) View() tea.View {
	body := m.nowPlaying()

	hints := dimStyle.Render("space play/pause • n next • N prev • h/l seek • +/- vol • q quit")

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, panelStyle.Width(m.width).Render(body), hints))
	v.AltScreen = true
	return v
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
