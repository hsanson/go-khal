package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
)

func renderWeekViewport(startWeek, selected time.Time, events []calendar.Event, width, maxHeight int, weekStart time.Weekday, styles Styles) string {
	var lines []string
	used := 0
	lastHeaderMonth := time.Time{}

	for i := 0; i < 104; i++ {
		week := startWeek.AddDate(0, 0, i*7)
		headerMonth := time.Date(week.AddDate(0, 0, 3).Year(), week.AddDate(0, 0, 3).Month(), 1, 0, 0, 0, 0, week.Location())
		if i == 0 || headerMonth.Month() != lastHeaderMonth.Month() || headerMonth.Year() != lastHeaderMonth.Year() {
			header := styles.Title.Render(headerMonth.Format("January 2006"))
			headRow := styles.Subtle.Render("Mo Tu We Th Fr Sa Su")
			if used+2 > maxHeight {
				break
			}
			lines = append(lines, header, headRow)
			used += 2
			lastHeaderMonth = headerMonth
		}

		if used+1 > maxHeight {
			break
		}
		weekLine := renderWeekRow(week, selected, events, width, weekStart, styles)
		lines = append(lines, weekLine)
		used++
	}

	return strings.Join(lines, "\n")
}

func renderWeekRow(weekStartDate, selected time.Time, events []calendar.Event, width int, weekStart time.Weekday, styles Styles) string {
	_ = weekStart
	parts := make([]string, 0, 7)
	eventColorByDay := map[string]string{}
	for _, ev := range events {
		k := dayStart(ev.Start).Format("2006-01-02")
		if eventColorByDay[k] == "" && strings.TrimSpace(ev.Color) != "" {
			eventColorByDay[k] = ev.Color
		}
	}

	for i := 0; i < 7; i++ {
		day := weekStartDate.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		txt := fmt.Sprintf("%2d", day.Day())
		if color := eventColorByDay[key]; color != "" {
			txt = styleForColor(styles.Accent, color).Render(txt)
		}
		if day.Year() == selected.Year() && day.Month() == selected.Month() && day.Day() == selected.Day() {
			txt = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true).Render(txt)
		}
		parts = append(parts, txt)
	}

	line := strings.Join(parts, " ")
	return lipgloss.NewStyle().Width(width).Render(line)
}

type agendaRender struct {
	Text                  string
	EventCount            int
	LastVisibleEventIndex int
}

func renderAgendaFromDay(selected time.Time, events []calendar.Event, width, maxLines int, timeFmt string, styles Styles, eventCursor int, highlight bool) agendaRender {
	filtered := agendaEventsFromDay(dayStart(selected), events)
	return renderAgendaFromEvents(filtered, width, maxLines, timeFmt, styles, eventCursor, 0, highlight)
}

func renderAgendaFromEvents(filtered []calendar.Event, width, maxLines int, timeFmt string, styles Styles, eventCursor int, eventOffset int, highlight bool) agendaRender {
	if timeFmt == "" {
		timeFmt = "15:04"
	}
	if maxLines < 6 {
		maxLines = 6
	}
	if eventOffset < 0 {
		eventOffset = 0
	}
	if eventOffset > len(filtered) {
		eventOffset = len(filtered)
	}

	grouped := map[string][]calendar.Event{}
	orderedDays := make([]string, 0, 16)
	seen := map[string]bool{}
	for _, ev := range filtered {
		k := dayStart(ev.Start).Format("2006-01-02")
		if !seen[k] {
			seen[k] = true
			orderedDays = append(orderedDays, k)
		}
		grouped[k] = append(grouped[k], ev)
	}

	if len(orderedDays) == 0 {
		return agendaRender{Text: styles.Subtle.Width(width).Render("No upcoming events from selected day"), EventCount: 0, LastVisibleEventIndex: -1}
	}

	eventIndex := 0
	visibleEventIndex := -1
	lines := make([]string, 0, maxLines)
	loc := time.Local
	if len(filtered) > 0 {
		loc = filtered[0].Start.Location()
	}
	for _, k := range orderedDays {
		day, _ := time.ParseInLocation("2006-01-02", k, loc)
		if len(lines) >= maxLines {
			break
		}
		dayEvents := grouped[k]
		allDay := make([]calendar.Event, 0)
		timed := make([]calendar.Event, 0)
		for _, ev := range dayEvents {
			if ev.AllDay {
				allDay = append(allDay, ev)
			} else {
				timed = append(timed, ev)
			}
		}
		ordered := make([]calendar.Event, 0, len(dayEvents))
		ordered = append(ordered, allDay...)
		ordered = append(ordered, timed...)

		dayStartIndex := eventIndex
		dayEndIndex := dayStartIndex + len(ordered) - 1
		if dayEndIndex < eventOffset {
			eventIndex += len(ordered)
			continue
		}

		if len(lines)+2 > maxLines {
			break
		}
		lines = append(lines, styles.DayHeader.Render(day.Format("Mon 2006-01-02")))
		renderedAny := false

		for _, ev := range ordered {
			if eventIndex < eventOffset {
				eventIndex++
				continue
			}
			if len(lines) >= maxLines {
				break
			}
			line := ""
			if ev.AllDay {
				line = fmt.Sprintf("  all-day  %s", ev.Summary)
			} else {
				line = fmt.Sprintf("  %s-%s  %s", ev.Start.Format(timeFmt), ev.End.Format(timeFmt), ev.Summary)
			}
			styled := styleForColor(styles.Event, ev.Color).Render(truncate(line, max(10, width-1)))
			if highlight && eventIndex == eventCursor {
				styled = lipgloss.NewStyle().Background(lipgloss.Color("238")).Render(styled)
			}
			lines = append(lines, styled)
			renderedAny = true
			visibleEventIndex = eventIndex
			eventIndex++
		}

		if !renderedAny {
			lines = lines[:len(lines)-1]
			break
		}
		if len(lines) < maxLines {
			lines = append(lines, "")
		}
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return agendaRender{Text: lipgloss.NewStyle().Width(width).MaxHeight(maxLines).Render(strings.Join(lines, "\n")), EventCount: len(filtered), LastVisibleEventIndex: visibleEventIndex}
}

func agendaEventsFromDay(startDay time.Time, events []calendar.Event) []calendar.Event {
	filtered := make([]calendar.Event, 0, len(events))
	for _, ev := range events {
		if ev.Start.Before(startDay) {
			continue
		}
		filtered = append(filtered, ev)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Start.Equal(filtered[j].Start) {
			if filtered[i].AllDay != filtered[j].AllDay {
				return filtered[i].AllDay
			}
			return filtered[i].Summary < filtered[j].Summary
		}
		return filtered[i].Start.Before(filtered[j].Start)
	})
	return filtered
}


func styleForColor(base lipgloss.Style, color string) lipgloss.Style {
	if strings.TrimSpace(color) == "" {
		return base
	}
	return base.Foreground(lipgloss.Color(color))
}

func truncate(s string, n int) string {
	if n < 1 || len(s) <= n {
		return s
	}
	if n == 1 {
		return s[:1]
	}
	return s[:n-1] + "..."
}
