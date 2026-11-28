package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// keyMap holds every key binding. Each binding knows its keys and its help
// text, so the hint line and the ? screen are drawn from the same place the
// keys are matched against.
type keyMap struct {
	Up, Down, Top, Bottom, PageUp, PageDown key.Binding
	Open, Back, Play                        key.Binding
	Search, SearchType                      key.Binding

	PlayPause, Next, Prev             key.Binding
	SeekBack, SeekFwd, VolDown, VolUp key.Binding
	Shuffle, Repeat                   key.Binding

	Like, LikeSelected, Enqueue key.Binding

	Help, Quit key.Binding

	// Shown in the hint line only; never matched against
	move, submit, cancel, forceQuit key.Binding
}

func newKeyMap(volumeStep int, seekStep time.Duration) keyMap {
	seek := int(seekStep / time.Second)
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		PageUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "half page up")),
		PageDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "half page down")),

		Open:       key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "open")),
		Back:       key.NewBinding(key.WithKeys("h", "left", "esc"), key.WithHelp("h/esc", "back")),
		Play:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "play")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		SearchType: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "result type")),

		PlayPause: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "play/pause")),
		Next:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
		Prev:      key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "previous")),
		SeekBack:  key.NewBinding(key.WithKeys(","), key.WithHelp(",", fmt.Sprintf("back %ds", seek))),
		SeekFwd:   key.NewBinding(key.WithKeys("."), key.WithHelp(".", fmt.Sprintf("forward %ds", seek))),
		VolDown:   key.NewBinding(key.WithKeys("-"), key.WithHelp("-", fmt.Sprintf("volume -%d%%", volumeStep))),
		VolUp:     key.NewBinding(key.WithKeys("+", "="), key.WithHelp("+", fmt.Sprintf("volume +%d%%", volumeStep))),
		Shuffle:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "shuffle")),
		Repeat:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "repeat")),

		Like:         key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "like playing")),
		LikeSelected: key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "like highlighted")),
		Enqueue:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add to queue")),

		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "all keys")),
		Quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		// A binding with no keys counts as disabled, and help skips disabled
		// bindings, so these get their keys even though they're never matched
		move:      key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		submit:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search")),
		cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		forceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// playback returns the bindings that send a command to the playback device
func (k keyMap) playback() []key.Binding {
	return []key.Binding{
		k.PlayPause, k.Next, k.Prev, k.SeekBack, k.SeekFwd, k.VolDown, k.VolUp,
		k.Shuffle, k.Repeat, k.Enqueue,
	}
}

// fullHelp groups every binding into the columns of the ? screen
func (k keyMap) fullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom, k.PageUp, k.PageDown},
		{k.Open, k.Back, k.Play, k.Search, k.SearchType},
		{k.PlayPause, k.Next, k.Prev, k.SeekBack, k.SeekFwd, k.VolDown, k.VolUp},
		{k.Shuffle, k.Repeat, k.Like, k.LikeSelected, k.Enqueue, k.Help, k.Quit},
	}
}

// newHelp returns the help renderer, styled to match the rest of the UI
func newHelp() help.Model {
	h := help.New()
	keyStyle := lipgloss.NewStyle().Foreground(colorPurple)
	h.Styles.ShortKey, h.Styles.FullKey = keyStyle, keyStyle
	h.Styles.ShortDesc, h.Styles.FullDesc = dimStyle, dimStyle
	h.Styles.ShortSeparator, h.Styles.FullSeparator = dimStyle, dimStyle
	h.Styles.Ellipsis = dimStyle
	return h
}
