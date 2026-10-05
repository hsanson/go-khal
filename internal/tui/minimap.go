package tui

import (
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
)

const (
	minimapAgendaMinWidth = 24
	minimapGap            = 2
)

type minimapLayout struct {
	days       []time.Time
	slots      int
	width      int
	labelWidth int
}

func (m Model) minimapLayout(availableWidth, availableHeight int) (minimapLayout, bool) {
	if !m.showMinimap || m.showTasksMode || availableHeight < m.minimapHeight() {
		return minimapLayout{}, false
	}

	labelWidth := m.minimapLabelWidth()
	selected := dayStart(m.selected)
	weekStart := calendar.StartOfWeek(selected, m.weekStart())
	monday := weekStart
	if monday.Weekday() == time.Sunday {
		monday = monday.AddDate(0, 0, 1)
	}

	candidates := []struct {
		start time.Time
		days  int
		slots int
	}{
		{start: weekStart, days: 7, slots: 4},
		{start: monday, days: 5, slots: 4},
		{start: selected, days: 1, slots: 8},
	}
	for _, candidate := range candidates {
		width := minimapWidth(labelWidth, candidate.days, candidate.slots)
		if availableWidth-width-minimapGap < minimapAgendaMinWidth {
			continue
		}
		days := make([]time.Time, candidate.days)
		for i := range days {
			days[i] = candidate.start.AddDate(0, 0, i)
		}
		return minimapLayout{days: days, slots: candidate.slots, width: width, labelWidth: labelWidth}, true
	}
	return minimapLayout{}, false
}

func (m Model) minimapHeight() int {
	startHour, endHour := m.cfg.MinimapHours()
	return endHour - startHour + 3 // weekday header, all-day row, and inclusive hourly rows
}

func (m Model) minimapLabelWidth() int {
	startHour, endHour := m.cfg.MinimapHours()
	format := m.cfg.TimeFormat
	if format == "" {
		format = "15:04"
	}
	width := lipgloss.Width("All")
	location := m.selected.Location()
	for hour := startHour; hour <= endHour; hour++ {
		label := time.Date(2000, time.January, 1, hour, 0, 0, 0, location).Format(format)
		width = max(width, lipgloss.Width(label))
	}
	return width
}

func minimapWidth(labelWidth, days, slots int) int {
	return labelWidth + 1 + days*slots + max(0, days-1)
}

func (m Model) renderMinimap(layout minimapLayout, originX, originY int) string {
	eventIndexes := m.minimapEventIndexes()
	lines := make([]string, 0, m.minimapHeight())

	var header strings.Builder
	header.WriteString(strings.Repeat(" ", layout.labelWidth+1))
	for dayIndex, day := range layout.days {
		if dayIndex > 0 {
			header.WriteByte(' ')
		}
		header.WriteString(m.styles.DayHeader.Render(centerMinimapLabel(day.Format("Mon")[:2], layout.slots)))
	}
	lines = append(lines, header.String())

	startHour, endHour := m.cfg.MinimapHours()
	format := m.cfg.TimeFormat
	if format == "" {
		format = "15:04"
	}
	for row := range endHour - startHour + 2 {
		allDay := row == 0
		label := ""
		if !allDay {
			hour := startHour + row - 1
			label = time.Date(2000, time.January, 1, hour, 0, 0, 0, m.selected.Location()).Format(format)
		}

		var line strings.Builder
		line.WriteString(m.styles.Hour.Render(strings.Repeat(" ", layout.labelWidth-lipgloss.Width(label)) + label + " "))
		for dayIndex, day := range layout.days {
			if dayIndex > 0 {
				line.WriteByte(' ')
			}
			var matching [8]int
			count := 0
			var hourStart time.Time
			if !allDay {
				hourStart = time.Date(day.Year(), day.Month(), day.Day(), startHour+row-1, 0, 0, 0, day.Location())
			}
			for _, eventIndex := range eventIndexes {
				event := &m.data.Events[eventIndex]
				matches := event.AllDay && allDay && minimapOverlaps(event, day, day.AddDate(0, 0, 1))
				if !allDay {
					matches = !event.AllDay && minimapOverlaps(event, hourStart, hourStart.Add(time.Hour))
				}
				if matches {
					matching[count] = eventIndex
					count++
					if count == layout.slots {
						break
					}
				}
			}
			var slots [8]int
			assigned := 0
			if count > 0 {
				span := layout.slots / count
				remainder := layout.slots % count
				for eventPosition := range count {
					eventSpan := span
					if eventPosition < remainder {
						eventSpan++
					}
					for range eventSpan {
						slots[assigned] = matching[eventPosition]
						assigned++
					}
				}
			}

			for slot := range layout.slots {
				x := originX + layout.labelWidth + 1 + dayIndex*(layout.slots+1) + slot
				y := originY + row + 1
				if count > 0 {
					eventIndex := slots[slot]
					event := &m.data.Events[eventIndex]
					glyph := "⣤"
					if !allDay {
						glyph = minimapGlyph(event, hourStart)
					}
					line.WriteString(minimapEventStyle(m.styles.Event, event).Render(glyph))
					m.addMouseHit(mouseHit{rect: mouseRect{x: x, y: y, width: 1, height: 1}, kind: mouseMinimapEvent, day: day, index: eventIndex})
					continue
				}

				line.WriteString(m.styles.Subtle.Render("_"))
				kind := mouseMinimapTime
				when := hourStart
				if allDay {
					kind = mouseMinimapAllDay
					when = day
				}
				m.addMouseHit(mouseHit{rect: mouseRect{x: x, y: y, width: 1, height: 1}, kind: kind, day: when})
			}
		}
		lines = append(lines, line.String())
	}

	return strings.Join(lines, "\n")
}

func (m Model) minimapEventIndexes() []int {
	indexes := make([]int, 0, len(m.data.Events))
	for i := range m.data.Events {
		event := &m.data.Events[i]
		if !m.calendarVisibility[calendarKey(event.Source, event.Calendar)] ||
			(!m.showAllMode && eventRSVPIsNo(*event)) {
			continue
		}
		indexes = append(indexes, i)
	}
	sort.Slice(indexes, func(i, j int) bool {
		left := &m.data.Events[indexes[i]]
		right := &m.data.Events[indexes[j]]
		if !left.Start.Equal(right.Start) {
			return left.Start.Before(right.Start)
		}
		if !left.End.Equal(right.End) {
			return left.End.Before(right.End)
		}
		if left.UID != right.UID {
			return left.UID < right.UID
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Calendar != right.Calendar {
			return left.Calendar < right.Calendar
		}
		return left.FilePath < right.FilePath
	})
	return indexes
}

func minimapEventStyle(base lipgloss.Style, event *calendar.Event) lipgloss.Style {
	style := styleForColor(base, event.Color)
	if strings.EqualFold(strings.TrimSpace(event.Availability), "free") {
		style = style.Faint(true)
	}
	return style
}

func minimapOverlaps(event *calendar.Event, start, end time.Time) bool {
	return event.End.After(start) && event.Start.Before(end)
}

func minimapGlyph(event *calendar.Event, hour time.Time) string {
	middle := hour.Add(30 * time.Minute)
	top := minimapOverlaps(event, hour, middle)
	bottom := minimapOverlaps(event, middle, hour.Add(time.Hour))
	switch {
	case top && bottom:
		return "⣿"
	case top:
		return "⠛"
	default:
		return "⣤"
	}
}

func centerMinimapLabel(label string, width int) string {
	padding := max(0, width-lipgloss.Width(label))
	left := padding / 2
	return strings.Repeat(" ", left) + label + strings.Repeat(" ", padding-left)
}
