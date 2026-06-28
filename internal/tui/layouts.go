package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
)

func renderMiniMonth(current time.Time, events []calendar.Event, width int, styles Styles) string {
	first := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, current.Location())
	last := first.AddDate(0, 1, -1)
	hasEvent := map[int]bool{}
	dayColor := map[int]string{}
	for _, ev := range events {
		if ev.Start.Year() == current.Year() && ev.Start.Month() == current.Month() {
			hasEvent[ev.Start.Day()] = true
			if dayColor[ev.Start.Day()] == "" {
				dayColor[ev.Start.Day()] = ev.Color
			}
		}
	}
	var b strings.Builder
	b.WriteString(styles.Title.Render(first.Format("January 2006")))
	b.WriteString("\nMo Tu We Th Fr Sa Su\n")
	offset := (int(first.Weekday()) + 6) % 7
	for i := 0; i < offset; i++ {
		b.WriteString("   ")
	}
	for day := 1; day <= last.Day(); day++ {
		marker := " "
		if hasEvent[day] {
			marker = "*"
		}
		value := fmt.Sprintf("%2d%s", day, marker)
		if marker == "*" {
			value = fmt.Sprintf("%2d%s", day, styleForColor(styles.Accent, dayColor[day]).Render("*"))
		}
		if day == current.Day() {
			value = styles.Accent.Render(value)
		}
		b.WriteString(value)
		if (offset+day)%7 == 0 {
			b.WriteString("\n")
		} else {
			b.WriteString(" ")
		}
	}
	return lipgloss.NewStyle().Width(width).Render(strings.TrimRight(b.String(), "\n"))
}

func renderDayColumns(days []time.Time, events []calendar.Event, width int, timeFmt string, styles Styles) string {
	if timeFmt == "" {
		timeFmt = "15:04"
	}
	colWidth := max(20, width/max(1, len(days)))
	cols := make([]string, 0, len(days))
	for _, day := range days {
		dayEvents := calendar.EventsOnDay(events, day)
		lines := []string{styles.DayHeader.Render(day.Format("Mon 02"))}
		for hour := 0; hour < 24; hour++ {
			ht := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
			row := styles.Hour.Render(ht.Format(timeFmt)) + " "
			added := false
			for _, ev := range dayEvents {
				if ev.Start.Hour() != hour {
					continue
				}
				label := fmt.Sprintf("%s-%s %s", ev.Start.Format(timeFmt), ev.End.Format(timeFmt), truncate(ev.Summary, max(8, colWidth-16)))
				row += styleForColor(styles.Event, ev.Color).Render(label)
				added = true
				break
			}
			if !added {
				row += styles.Subtle.Render("|")
			}
			lines = append(lines, row)
		}
		cols = append(cols, lipgloss.NewStyle().Width(colWidth).Render(strings.Join(lines, "\n")))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func renderMonthGrid(current time.Time, events []calendar.Event, width int, styles Styles) string {
	first := time.Date(current.Year(), current.Month(), 1, 0, 0, 0, 0, current.Location())
	start := first.AddDate(0, 0, -((int(first.Weekday()) + 6) % 7))
	cellWidth := max(14, width/7)

	var rows []string
	headers := make([]string, 7)
	for i, d := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		headers[i] = lipgloss.NewStyle().Width(cellWidth).Render(styles.DayHeader.Render(d))
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, headers...))

	for week := 0; week < 6; week++ {
		cells := make([]string, 7)
		for dayIndex := 0; dayIndex < 7; dayIndex++ {
			day := start.AddDate(0, 0, week*7+dayIndex)
			dayEvents := calendar.EventsOnDay(events, day)
			lines := []string{fmt.Sprintf("%2d", day.Day())}
			for i := 0; i < min(3, len(dayEvents)); i++ {
				lines = append(lines, styleForColor(styles.Subtle, dayEvents[i].Color).Render(truncate(dayEvents[i].Summary, cellWidth-3)))
			}
			cellStyle := styles.GridCell
			if day.Year() == current.Year() && day.Month() == current.Month() && day.Day() == current.Day() {
				cellStyle = styles.SelectedCell
			}
			if day.Month() != current.Month() {
				lines[0] = styles.Subtle.Render(lines[0])
			}
			cells[dayIndex] = cellStyle.Width(cellWidth).Height(5).Render(strings.Join(lines, "\n"))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	return strings.Join(rows, "\n")
}

func renderYearGrid(year int, events []calendar.Event, width int, styles Styles) string {
	colWidth := max(24, width/3)
	monthViews := make([]string, 0, 12)
	for m := time.January; m <= time.December; m++ {
		monthDate := time.Date(year, m, 1, 0, 0, 0, 0, time.Local)
		monthEvents := make([]calendar.Event, 0)
		for _, ev := range events {
			if ev.Start.Year() == year && ev.Start.Month() == m {
				monthEvents = append(monthEvents, ev)
			}
		}
		monthViews = append(monthViews, lipgloss.NewStyle().Width(colWidth).Render(renderMiniMonth(monthDate, monthEvents, colWidth-1, styles)))
	}

	var rows []string
	for i := 0; i < len(monthViews); i += 3 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, monthViews[i], monthViews[i+1], monthViews[i+2]))
	}
	return strings.Join(rows, "\n\n")
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
