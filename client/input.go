package client

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/client/theme"
)

// lineInput wraps the Bubbles text input with the game's look. The cursor
// does not blink, so an idle text field never wakes the UI.
type lineInput struct {
	ti   textinput.Model
	hint string
}

func newLineInput(prompt string, limit int) lineInput {
	ti := textinput.New()
	ti.Prompt = prompt
	ti.CharLimit = limit
	st := textinput.DefaultDarkStyles()
	st.Cursor.Blink = false
	st.Cursor.Color = theme.Cyan
	st.Focused.Prompt = lipgloss.NewStyle().Foreground(theme.Cyan)
	st.Blurred.Prompt = lipgloss.NewStyle().Foreground(theme.Cyan)
	st.Focused.Text = lipgloss.NewStyle().Foreground(theme.Fg)
	st.Blurred.Text = lipgloss.NewStyle().Foreground(theme.Dim)
	st.Focused.Placeholder = lipgloss.NewStyle().Foreground(theme.Faint)
	st.Blurred.Placeholder = lipgloss.NewStyle().Foreground(theme.Faint)
	ti.SetStyles(st)
	ti.SetVirtualCursor(true)
	ti.Focus()
	return lineInput{ti: ti}
}

func (l *lineInput) Value() string { return l.ti.Value() }

func (l *lineInput) Set(s string) {
	l.ti.SetValue(s)
	l.ti.CursorEnd()
}

// handle edits the value. It returns "enter" or "esc" when those are pressed.
func (l *lineInput) handle(msg tea.KeyPressMsg) string {
	switch msg.String() {
	case "enter":
		return "enter"
	case "esc":
		return "esc"
	}
	l.ti.Focus()
	l.ti, _ = l.ti.Update(msg)
	return ""
}

func (l *lineInput) view(t *theme.Theme, width int, focused bool) string {
	l.ti.Placeholder = l.hint
	l.ti.SetWidth(max(1, width-lipgloss.Width(l.ti.Prompt)-1))
	if focused {
		l.ti.Focus()
	} else {
		l.ti.Blur()
	}
	return l.ti.View()
}
