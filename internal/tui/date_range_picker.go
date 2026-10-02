package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dateRangePicker struct {
	cursor     time.Time
	start      time.Time
	end        *time.Time
	month      time.Time
	rangeMode  bool
	singleDate bool
	clearable  bool
	cleared    bool
	done       bool
	cancelled  bool
}

func newDateRangePicker(startValue, endValue string) *dateRangePicker {
	start, err := time.Parse("2006-01-02", startValue)
	if err != nil {
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	picker := &dateRangePicker{
		cursor: start,
		start:  start,
		month:  firstOfMonth(start),
	}
	end, err := time.Parse("2006-01-02", endValue)
	if err == nil && end.After(start) {
		picker.rangeMode = true
		picker.cursor = end
		picker.month = firstOfMonth(end)
		picker.end = &end
	}
	return picker
}

func newSingleDatePicker(value string) *dateRangePicker {
	picker := newDateRangePicker(value, value)
	picker.singleDate = true
	return picker
}

func newTaskDatePicker(value string) *dateRangePicker {
	picker := newSingleDatePicker(value)
	picker.clearable = true
	return picker
}

func (p *dateRangePicker) Update(msg tea.KeyMsg) {
	if p == nil || p.done || p.cancelled {
		return
	}
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		p.cancelled = true
	case "enter":
		p.done = true
	case "space", " ":
		if p.clearable {
			p.cleared = true
		} else if !p.singleDate {
			p.toggleRange()
		}
	case "[", "pgup":
		p.moveMonth(-1)
	case "]", "pgdown":
		p.moveMonth(1)
	case "left", "h":
		p.moveDays(-1)
	case "right", "l":
		p.moveDays(1)
	case "up", "k":
		p.moveDays(-7)
	case "down", "j":
		p.moveDays(7)
	case "t":
		now := time.Now()
		p.cursor = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		p.month = firstOfMonth(p.cursor)
		p.updateSelection()
	}
}

func (p *dateRangePicker) dates() (time.Time, time.Time) {
	start := p.start
	end := start
	if p.rangeMode && p.end != nil {
		end = *p.end
	}
	if end.Before(start) {
		start, end = end, start
	}
	return start, end
}

func (p *dateRangePicker) moveDays(days int) {
	p.cursor = p.cursor.AddDate(0, 0, days)
	p.month = firstOfMonth(p.cursor)
	p.updateSelection()
}

func (p *dateRangePicker) moveMonth(months int) {
	day := p.cursor.Day()
	nextMonth := firstOfMonth(p.cursor).AddDate(0, months, 0)
	lastDay := nextMonth.AddDate(0, 1, -1).Day()
	if day > lastDay {
		day = lastDay
	}
	p.cursor = time.Date(nextMonth.Year(), nextMonth.Month(), day, 0, 0, 0, 0, time.UTC)
	p.month = nextMonth
	p.updateSelection()
}

func (p *dateRangePicker) toggleRange() {
	p.rangeMode = !p.rangeMode
	if p.rangeMode {
		p.start = p.cursor
		p.end = nil
		return
	}
	p.cursor = p.start
	p.month = firstOfMonth(p.cursor)
	p.end = nil
}

func (p *dateRangePicker) updateSelection() {
	p.cleared = false
	if p.rangeMode {
		cursor := p.cursor
		p.end = &cursor
		return
	}
	p.start = p.cursor
	p.end = nil
}

func (p *dateRangePicker) View(styles Styles) string {
	if p == nil {
		return ""
	}
	header := fmt.Sprintf("%-18s ‹ ›", p.month.Format("January 2006"))
	lines := []string{
		styles.PanelTitle.Render(header),
		styles.Subtle.Render("Mo Tu We Th Fr Sa Su"),
	}
	firstWeekday := (int(p.month.Weekday()) + 6) % 7
	monthEnd := p.month.AddDate(0, 1, -1).Day()
	for week := range 6 {
		cells := make([]string, 0, 7)
		for weekday := range 7 {
			day := week*7 + weekday - firstWeekday + 1
			if day < 1 || day > monthEnd {
				cells = append(cells, "  ")
				continue
			}
			date := time.Date(p.month.Year(), p.month.Month(), day, 0, 0, 0, 0, time.UTC)
			cell := fmt.Sprintf("%2d", day)
			switch {
			case sameDate(date, p.cursor):
				cell = lipgloss.NewStyle().Background(lipgloss.Color("117")).Foreground(lipgloss.Color("232")).Bold(true).Render(cell)
			case sameDate(date, p.start) || p.end != nil && sameDate(date, *p.end):
				cell = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true).Render(cell)
			case p.rangeContains(date):
				cell = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")).Render(cell)
			}
			cells = append(cells, cell)
		}
		line := strings.Join(cells, " ")
		switch week {
		case 1:
			line += styles.Subtle.Render("   ←↓↑→ navigate")
		case 2:
			line += styles.Subtle.Render("   t    today")
		case 3:
			if p.clearable {
				line += styles.Subtle.Render("   spc  clear")
			} else if !p.singleDate {
				line += styles.Subtle.Render("   spc  multi-day")
			}
		}
		lines = append(lines, line)
	}
	if p.singleDate {
		endLabel := p.start.Format("Jan 2, 2006")
		if p.cleared {
			endLabel = "—"
		}
		lines = append(lines,
			"",
			fmt.Sprintf("End: %s", endLabel),
			"",
			styles.Subtle.Render("[enter] Apply  [esc/q] Cancel"),
		)
		return lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	check := "[ ]"
	if p.rangeMode {
		check = "[x]"
	}
	rangeLine := check + " Multi-day"
	start := p.start
	endLabel := "—"
	if p.rangeMode && p.end != nil {
		endLabel = p.end.Format("Jan 2, 2006")
	}
	lines = append(lines,
		"",
		rangeLine,
		fmt.Sprintf("Start: %s    End: %s", start.Format("Jan 2, 2006"), endLabel),
		"",
		styles.Subtle.Render("[enter] Apply  [esc/q] Cancel"),
	)
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *dateRangePicker) rangeContains(date time.Time) bool {
	start, end := p.dates()
	return !date.Before(start) && !date.After(end)
}

func firstOfMonth(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func sameDate(left, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}
