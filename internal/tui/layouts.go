package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
)

func renderMonthList(topMonth, selected time.Time, events []calendar.Event, width, maxHeight int, styles Styles) (string, int) {
	start := time.Date(topMonth.Year(), topMonth.Month(), 1, 0, 0, 0, 0, topMonth.Location())
	parts := make([]string, 0, 6)
	used := 0
	count := 0
	for i := 0; i < 24; i++ {
		month := start.AddDate(0, i, 0)
		monthEvents := make([]calendar.Event, 0)
		for _, ev := range events {
			if ev.Start.Year() == month.Year() && ev.Start.Month() == month.Month() {
				monthEvents = append(monthEvents, ev)
			}
		}
		height := monthBlockHeight(month, selected, width, styles)
		mini := renderMiniMonth(month, selected, monthEvents, width, styles)
		extra := 0
		if count > 0 {
			extra = 1
		}
		if count > 0 && used+extra+height > maxHeight {
			break
		}
		if count == 0 && height > maxHeight {
			parts = append(parts, mini)
			used = height
			count = 1
			break
		}
		parts = append(parts, mini)
		used += extra + height
		count++
	}
	if len(parts) == 0 {
		return "", 0
	}
	return strings.Join(parts, "\n\n"), count
}

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

func renderAgendaFromDay(selected time.Time, events []calendar.Event, width, maxLines int, timeFmt string, styles Styles) string {
	if timeFmt == "" {
		timeFmt = "15:04"
	}
	if maxLines < 6 {
		maxLines = 6
	}

	startDay := dayStart(selected)
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
		return styles.Subtle.Width(width).Render("No upcoming events from selected day")
	}

	lines := make([]string, 0, maxLines)
	for _, k := range orderedDays {
		day, _ := time.ParseInLocation("2006-01-02", k, selected.Location())
		if len(lines) >= maxLines {
			break
		}
		lines = append(lines, styles.DayHeader.Render(day.Format("Mon 2006-01-02")))

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

		for _, ev := range allDay {
			if len(lines) >= maxLines {
				break
			}
			line := fmt.Sprintf("  all-day  %s", ev.Summary)
			lines = append(lines, styleForColor(styles.Event, ev.Color).Render(truncate(line, max(10, width-1))))
		}
		for _, ev := range timed {
			if len(lines) >= maxLines {
				break
			}
			line := fmt.Sprintf("  %s-%s  %s", ev.Start.Format(timeFmt), ev.End.Format(timeFmt), ev.Summary)
			lines = append(lines, styleForColor(styles.Event, ev.Color).Render(truncate(line, max(10, width-1))))
		}
		if len(lines) < maxLines {
			lines = append(lines, "")
		}
	}

	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func monthBlockHeight(month, selected time.Time, width int, styles Styles) int {
	mini := renderMiniMonth(month, selected, nil, width, styles)
	return lipgloss.Height(mini)
}

func renderMiniMonth(month, selected time.Time, events []calendar.Event, width int, styles Styles) string {
	current := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
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
		if day == selected.Day() && selected.Month() == current.Month() && selected.Year() == current.Year() {
			value = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Render(value)
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
	if len(days) <= 1 {
		return renderSingleDayColumn(days, events, width, timeFmt, styles)
	}
	return renderMultiDayHourGrid(days, events, width, timeFmt, styles)
}

func renderSingleDayColumn(days []time.Time, events []calendar.Event, width int, timeFmt string, styles Styles) string {
	if len(days) == 0 {
		return ""
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

func renderMultiDayHourGrid(days []time.Time, events []calendar.Event, width int, timeFmt string, styles Styles) string {
	hourColWidth := 6
	dayColWidth := max(10, (width-hourColWidth-max(0, len(days)-1))/len(days))

	dayEvents := make([][]calendar.Event, len(days))
	for i, day := range days {
		dayEvents[i] = calendar.EventsOnDay(events, day)
	}

	var rows []string
	headerCells := make([]string, 0, len(days)+1)
	headerCells = append(headerCells, lipgloss.NewStyle().Width(hourColWidth).Render(""))
	for _, day := range days {
		headerCells = append(headerCells, styles.DayHeader.Width(dayColWidth).Render(day.Format("Mon 02")))
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))

	for hour := 0; hour < 24; hour++ {
		hourLists := make([][]calendar.Event, len(days))
		maxLines := 1
		for i := range days {
			for _, ev := range dayEvents[i] {
				if ev.Start.Hour() == hour {
					hourLists[i] = append(hourLists[i], ev)
				}
			}
			if len(hourLists[i]) > maxLines {
				maxLines = len(hourLists[i])
			}
		}

		for line := 0; line < maxLines; line++ {
			cells := make([]string, 0, len(days)+1)
			if line == 0 {
				hourLabel := time.Date(days[0].Year(), days[0].Month(), days[0].Day(), hour, 0, 0, 0, days[0].Location()).Format(timeFmt)
				cells = append(cells, styles.Hour.Width(hourColWidth).Render(hourLabel))
			} else {
				cells = append(cells, lipgloss.NewStyle().Width(hourColWidth).Render(""))
			}

			for i := range days {
				text := ""
				if line < len(hourLists[i]) {
					ev := hourLists[i][line]
					text = fmt.Sprintf("%s-%s %s", ev.Start.Format(timeFmt), ev.End.Format(timeFmt), truncate(ev.Summary, max(5, dayColWidth-12)))
					text = styleForColor(styles.Event, ev.Color).Render(text)
				} else if line == 0 {
					text = styles.Subtle.Render("|")
				}
				cells = append(cells, lipgloss.NewStyle().Width(dayColWidth).Render(text))
			}

			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		}
	}

	return strings.Join(rows, "\n")
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
		monthViews = append(monthViews, lipgloss.NewStyle().Width(colWidth).Render(renderMiniMonth(monthDate, monthDate, monthEvents, colWidth-1, styles)))
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
