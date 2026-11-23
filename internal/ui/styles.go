package ui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Dracula palette
var (
	colorFg      = lipgloss.Color("#F8F8F2")
	colorPurple  = lipgloss.Color("#BD93F9")
	colorComment = lipgloss.Color("#6272A4")
	colorPink    = lipgloss.Color("#FF79C6")
	colorCyan    = lipgloss.Color("#8BE9FD")
	colorGreen   = lipgloss.Color("#50FA7B")
)

var (
	panelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorComment).Padding(0, 1)

	titleStyle  = lipgloss.NewStyle().Foreground(colorFg).Bold(true)
	artistStyle = lipgloss.NewStyle().Foreground(colorCyan)
	dimStyle    = lipgloss.NewStyle().Foreground(colorComment)
	errorStyle  = lipgloss.NewStyle().Foreground(colorPink)
	barStyle    = lipgloss.NewStyle().Foreground(colorPurple)

	selectedStyle = lipgloss.NewStyle().Foreground(colorPurple).Bold(true)
)

// progressBar draws a bar `width` cells wide,
// filled in proportion to pos/total
func progressBar(pos, total time.Duration, width int) string {
	if width <= 0 {
		return ""
	}

	filled := 0
	if total > 0 {
		filled = int(float64(width) * float64(pos) / float64(total))
	}

	filled = max(0, min(width, filled))
	return barStyle.Render(strings.Repeat("━", filled)) + dimStyle.Render(strings.Repeat("─", width-filled))
}
