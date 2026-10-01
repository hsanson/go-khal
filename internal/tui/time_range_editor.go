package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type timeRangeEditor struct {
	slots     [8]rune
	cursor    int
	done      bool
	cancelled bool
	err       string
}

func newTimeRangeEditor(start, end string) *timeRangeEditor {
	editor := &timeRangeEditor{}
	digits := asciiDigits(start + end)
	for i, digit := range digits {
		if i == len(editor.slots) {
			break
		}
		editor.slots[i] = digit
	}
	return editor
}

func (e *timeRangeEditor) Update(msg tea.KeyMsg) {
	if e == nil || e.done || e.cancelled {
		return
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		e.cancelled = true
	case "left", "shift+tab":
		if e.cursor > 0 {
			e.cursor--
		}
	case "right", "tab":
		if e.cursor < len(e.slots)-1 {
			e.cursor++
		}
	case "up":
		if e.cursor >= 4 {
			e.cursor -= 4
		}
	case "down":
		if e.cursor < 4 {
			e.cursor += 4
		}
	case "home":
		e.cursor = 0
	case "end":
		e.cursor = len(e.slots) - 1
	case "backspace":
		if e.cursor > 0 {
			e.cursor--
		}
		e.slots[e.cursor] = 0
		e.err = ""
	case "delete":
		e.slots[e.cursor] = 0
		e.err = ""
	case "enter", "ctrl+s":
		if _, _, err := e.values(); err != nil {
			e.err = err.Error()
		} else {
			e.done = true
		}
	default:
		if msg.Type != tea.KeyRunes {
			return
		}
		for _, digit := range asciiDigits(string(msg.Runes)) {
			e.slots[e.cursor] = digit
			if e.cursor < len(e.slots)-1 {
				e.cursor++
			}
		}
		e.err = ""
	}
}

func (e *timeRangeEditor) values() (string, string, error) {
	if e == nil {
		return "", "", fmt.Errorf("time editor is unavailable")
	}
	for _, slot := range e.slots {
		if slot == 0 {
			return "", "", fmt.Errorf("enter both start and end times")
		}
	}
	start := string(e.slots[0:2]) + ":" + string(e.slots[2:4])
	end := string(e.slots[4:6]) + ":" + string(e.slots[6:8])
	if _, err := time.Parse("15:04", start); err != nil {
		return "", "", fmt.Errorf("invalid start time")
	}
	if _, err := time.Parse("15:04", end); err != nil {
		return "", "", fmt.Errorf("invalid end time")
	}
	return start, end, nil
}

func (e *timeRangeEditor) View(styles Styles) string {
	if e == nil {
		return ""
	}
	parts := make([]string, 0, 13)
	for i := range e.slots {
		if i == 2 || i == 6 {
			parts = append(parts, styles.Subtle.Render(":"))
		}
		if i == 4 {
			parts = append(parts, styles.Subtle.Render(" -> "))
		}
		value := e.slots[i]
		if value == 0 {
			value = timeSlotPlaceholder(i)
		}
		style := lipgloss.NewStyle()
		if e.slots[i] == 0 {
			style = style.Foreground(lipgloss.Color("245"))
		}
		if i == e.cursor {
			style = style.Reverse(true).Bold(true)
		}
		parts = append(parts, style.Render(string(value)))
	}
	lines := []string{
		styles.PanelTitle.Render("Time"),
		"",
		strings.Join(parts, ""),
	}
	if e.err != "" {
		lines = append(lines, "", errorText(e.err))
	}
	lines = append(lines, "", styles.Subtle.Render("[←/→ ↑/↓] Move  [backspace] Clear  [enter] Apply  [esc] Cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func asciiDigits(value string) []rune {
	digits := make([]rune, 0, len(value))
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	return digits
}

func timeSlotPlaceholder(index int) rune {
	if index%4 < 2 {
		return 'H'
	}
	return 'm'
}
