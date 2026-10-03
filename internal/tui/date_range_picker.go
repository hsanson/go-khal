package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	mousePhase int
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
		picker.mousePhase = 2
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
	case "[", "pgup", "ctrl+k":
		p.moveMonth(-1)
	case "]", "pgdown", "ctrl+j":
		p.moveMonth(1)
	case "ctrl+h":
		p.moveMonth(-12)
	case "ctrl+l":
		p.moveMonth(12)
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
	p.cursor = addMonthsClamped(p.cursor, months)
	p.month = firstOfMonth(p.cursor)
	p.updateSelection()
}

func (p *dateRangePicker) toggleRange() {
	p.rangeMode = !p.rangeMode
	if p.rangeMode {
		p.start = p.cursor
		p.end = nil
		p.mousePhase = 1
		return
	}
	p.cursor = p.start
	p.month = firstOfMonth(p.cursor)
	p.end = nil
	p.mousePhase = 0
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

func (p *dateRangePicker) selectByMouse(date time.Time) {
	if p == nil {
		return
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	p.cursor = date
	p.month = firstOfMonth(date)
	p.cleared = false
	if p.singleDate || !p.rangeMode {
		p.start = date
		p.end = nil
		return
	}
	switch p.mousePhase {
	case 1:
		p.end = &date
		p.mousePhase = 2
	default:
		p.start = date
		p.end = nil
		p.mousePhase = 1
	}
}

func (p *dateRangePicker) selectToday() {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	p.cursor = today
	p.month = firstOfMonth(today)
	p.updateSelection()
}

func (p *dateRangePicker) render(styles Styles) (string, []mouseHit) {
	if p == nil {
		return "", nil
	}
	header := fmt.Sprintf("%-18s « ‹ › »", p.month.Format("January 2006"))
	lines := []string{
		styles.PanelTitle.Render(header),
		styles.Subtle.Render("Mo Tu We Th Fr Sa Su"),
	}
	hits := []mouseHit{
		{rect: mouseRect{x: 19, y: 0, width: 1, height: 1}, kind: mouseDatePreviousYear},
		{rect: mouseRect{x: 21, y: 0, width: 1, height: 1}, kind: mouseDatePreviousMonth},
		{rect: mouseRect{x: 23, y: 0, width: 1, height: 1}, kind: mouseDateNextMonth},
		{rect: mouseRect{x: 25, y: 0, width: 1, height: 1}, kind: mouseDateNextYear},
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
				cell = styles.Focus.Render(cell)
			case sameDate(date, p.start) || p.end != nil && sameDate(date, *p.end):
				cell = styles.RangeEndpoint.Render(cell)
			case p.rangeContains(date):
				cell = styles.Range.Render(cell)
			}
			hits = append(hits, mouseHit{
				rect: mouseRect{x: weekday * 3, y: len(lines), width: 2, height: 1},
				kind: mouseDateDay,
				day:  date,
			})
			cells = append(cells, cell)
		}
		lines = append(lines, strings.Join(cells, " "))
	}
	lines = append(lines, "")
	controlsY := len(lines)
	today := "[ Today ]"
	controls := today
	hits = append(hits, mouseHit{rect: mouseRect{x: 0, y: controlsY, width: len(today), height: 1}, kind: mouseDateToday})
	if p.clearable {
		clear := "[ Clear ]"
		clearX := len(today) + 2
		controls += "  " + clear
		hits = append(hits, mouseHit{rect: mouseRect{x: clearX, y: controlsY, width: len(clear), height: 1}, kind: mouseDateClear})
	} else if !p.singleDate {
		check := "[ ]"
		if p.rangeMode {
			check = "[x]"
		}
		multi := check + " Multi-day"
		multiX := len(today) + 2
		controls += "  " + multi
		hits = append(hits, mouseHit{rect: mouseRect{x: multiX, y: controlsY, width: len(multi), height: 1}, kind: mouseDateMultiDay})
	}
	lines = append(lines, controls)
	if p.singleDate {
		endLabel := p.start.Format("Jan 2, 2006")
		if p.cleared {
			endLabel = "—"
		}
		lines = append(lines, "", fmt.Sprintf("Date: %-12s", endLabel))
		return strings.Join(lines, "\n"), hits
	}
	endLabel := "—"
	if p.rangeMode && p.end != nil {
		endLabel = p.end.Format("Jan 2, 2006")
	}
	lines = append(lines, "", fmt.Sprintf("Start: %-12s    End: %-12s", p.start.Format("Jan 2, 2006"), endLabel))
	return strings.Join(lines, "\n"), hits
}

func (p *dateRangePicker) View(styles Styles) string {
	view, _ := p.render(styles)
	return view
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

func addMonthsClamped(value time.Time, months int) time.Time {
	target := time.Date(value.Year(), value.Month(), 1, value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location()).AddDate(0, months, 0)
	lastDay := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, target.Location()).Day()
	return time.Date(target.Year(), target.Month(), min(value.Day(), lastDay), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), value.Location())
}
