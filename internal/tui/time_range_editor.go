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
	slotCount int
	cursor    int
	done      bool
	cancelled bool
	err       string
}

func newTimeRangeEditor(start, end string) *timeRangeEditor {
	return newTimeEditor(start+end, 8)
}

func newSingleTimeEditor(value string) *timeRangeEditor {
	return newTimeEditor(value, 4)
}

func newTimeEditor(value string, slotCount int) *timeRangeEditor {
	editor := &timeRangeEditor{slotCount: slotCount}
	digits := asciiDigits(value)
	for i, digit := range digits {
		if i == slotCount {
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
	case "esc", "ctrl+c", "q":
		e.cancelled = true
	case "left", "h", "shift+tab":
		if e.cursor > 0 {
			e.cursor--
		}
	case "right", "l", "tab":
		if e.cursor < e.slotCount-1 {
			e.cursor++
		}
	case "up", "k":
		e.adjust(1)
	case "down", "j":
		e.adjust(-1)
	case "home":
		e.cursor = 0
	case "end":
		e.cursor = e.slotCount - 1
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
		var err error
		if e.slotCount == 4 {
			_, err = e.value()
		} else {
			_, _, err = e.values()
		}
		if err != nil {
			e.err = err.Error()
		} else {
			e.done = true
		}
	default:
		if msg.Type != tea.KeyRunes {
			return
		}
		for _, digit := range msg.Runes {
			if digit < '0' || digit > '9' {
				continue
			}
			e.slots[e.cursor] = digit
			if e.cursor < e.slotCount-1 {
				e.cursor++
			}
		}
		e.err = ""
	}
}

func (e *timeRangeEditor) adjust(delta int) {
	base := e.cursor - e.cursor%4
	limit, step := 24, 1
	if e.cursor%4 >= 2 {
		base += 2
		limit, step = 60, 15
	}
	value := 0
	if e.slots[base] >= '0' && e.slots[base] <= '9' {
		value += int(e.slots[base]-'0') * 10
	}
	if e.slots[base+1] >= '0' && e.slots[base+1] <= '9' {
		value += int(e.slots[base+1] - '0')
	}
	value = (value + delta*step + limit) % limit
	e.slots[base] = rune('0' + value/10)
	e.slots[base+1] = rune('0' + value%10)
	e.err = ""
}

func (e *timeRangeEditor) value() (string, error) {
	if e == nil || e.slotCount < 4 {
		return "", fmt.Errorf("time editor is unavailable")
	}
	for _, slot := range e.slots[:4] {
		if slot == 0 {
			return "", fmt.Errorf("enter a time")
		}
	}
	value := string(e.slots[0:2]) + ":" + string(e.slots[2:4])
	if _, err := time.Parse("15:04", value); err != nil {
		return "", fmt.Errorf("invalid time")
	}
	return value, nil
}

func (e *timeRangeEditor) values() (string, string, error) {
	if e == nil || e.slotCount < 8 {
		return "", "", fmt.Errorf("time editor is unavailable")
	}
	for _, slot := range e.slots[:8] {
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

func (e *timeRangeEditor) render(styles Styles) (string, []mouseHit) {
	if e == nil {
		return "", nil
	}
	parts := make([]string, 0, e.slotCount+3)
	hits := make([]mouseHit, 0, e.slotCount)
	x := 0
	for i := 0; i < e.slotCount; i++ {
		if i == 2 || i == 6 {
			parts = append(parts, styles.Subtle.Render(":"))
			x++
		}
		if i == 4 {
			parts = append(parts, styles.Subtle.Render(" -> "))
			x += 4
		}
		value := e.slots[i]
		if value == 0 {
			value = timeSlotPlaceholder(i)
		}
		style := lipgloss.NewStyle()
		if e.slots[i] == 0 {
			style = styles.Subtle
		}
		if i == e.cursor {
			style = styles.TimeCursor
		}
		hits = append(hits, mouseHit{rect: mouseRect{x: x, y: 2, width: 1, height: 1}, kind: mouseTimeDigit, index: i})
		parts = append(parts, style.Render(string(value)))
		x++
	}
	lines := []string{
		styles.PanelTitle.Render("Time"),
		"",
		strings.Join(parts, ""),
	}
	if e.err != "" {
		lines = append(lines, "", styles.Error.Render(e.err))
	}
	return strings.Join(lines, "\n"), hits
}

func (e *timeRangeEditor) View(styles Styles) string {
	view, _ := e.render(styles)
	return view
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
