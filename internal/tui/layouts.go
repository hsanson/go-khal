package tui

import (
	"fmt"
	"sort"
	"strconv"
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
	now := time.Now()
	for _, ev := range events {
		k := dayStart(ev.Start).Format("2006-01-02")
		if strings.TrimSpace(ev.Color) == "" {
			continue
		}
		if ev.Start.After(now) || (ev.Start.Before(now) && ev.End.After(now)) {
			if eventColorByDay[k] == "" {
				eventColorByDay[k] = ev.Color
			}
		}
	}
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

type AgendaListItem struct {
	Day    time.Time
	IsFree bool
	Start  time.Time
	End    time.Time
	Event  *calendar.Event
	Todo   *calendar.Todo
	Mode   string
}

func renderAgendaFromDay(selected time.Time, events []calendar.Event, width, maxLines int, timeFmt string, styles Styles, eventCursor int, highlight bool) agendaRender {
	filtered := agendaEventsFromDay(dayStart(selected), events)
	return renderAgendaFromEvents(filtered, width, maxLines, timeFmt, styles, eventCursor, 0, highlight)
}

func renderAgendaFromItems(items []AgendaListItem, width, maxLines int, timeFmt string, styles Styles, cursor int, offset int, highlight bool) agendaRender {
	if timeFmt == "" {
		timeFmt = "15:04"
	}
	if maxLines < 6 {
		maxLines = 6
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(items) {
		offset = len(items)
	}

	lines := make([]string, 0, maxLines)
	lastVisible := -1
	var currentDay string
	for i := offset; i < len(items); i++ {
		if len(lines) >= maxLines {
			break
		}
		item := items[i]
		dayKey := item.Day.Format("2006-01-02")
		if dayKey != currentDay {
			needed := 2
			if currentDay != "" {
				needed++
			}
			if len(lines)+needed > maxLines {
				break
			}
			if currentDay != "" {
				lines = append(lines, "")
			}
			currentDay = dayKey
			lines = append(lines, styles.DayHeader.Render(item.Day.Format("Mon 2006-01-02")))
		}

		line := ""
		if item.IsFree {
			line = fmt.Sprintf("  %s-%s  (no events)", formatTimeBoundary(item.Start, item.Day, timeFmt), formatTimeBoundary(item.End, item.Day, timeFmt))
			line = styles.Subtle.Render(truncate(line, max(10, width-1)))
		} else if item.Event != nil {
			ev := *item.Event
			glyph := iconForEvent(ev)
			icon := styleForColor(styles.Event, ev.Color).Render(glyph)
			if ev.AllDay && (ev.Kind == calendar.EventKindBirthday || ev.Kind == calendar.EventKindAnniversary) {
				line = fmt.Sprintf("  %s %s", icon, ev.Summary)
			} else if ev.AllDay {
				line = fmt.Sprintf("  %s all-day  %s", icon, ev.Summary)
			} else {
				line = fmt.Sprintf("  %s %s-%s  %s", icon, ev.Start.Format(timeFmt), ev.End.Format(timeFmt), ev.Summary)
			}
			line = styleForColor(styles.Event, ev.Color).Render(truncate(line, max(10, width-1)))
		} else if item.Todo != nil {
			todo := *item.Todo
			todoStyle := styleForColor(styles.Event, todo.Color)
			if isTodoDone(todo) {
				todoStyle = styles.Subtle
			}
			icon := todoStyle.Render("󰄱")
			summary := todo.Summary
			if strings.TrimSpace(summary) == "" {
				summary = "(untitled todo)"
			}
			switch item.Mode {
			case "todo-range":
				line = fmt.Sprintf("  %s %s-%s  %s", icon, item.Start.Format(timeFmt), item.End.Format(timeFmt), summary)
			case "todo-start":
				line = fmt.Sprintf("  %s %s  %s", icon, item.Start.Format(timeFmt), summary)
			case "todo-end":
				line = fmt.Sprintf("  %s -%s  %s", icon, item.End.Format(timeFmt), summary)
			default:
				line = fmt.Sprintf("  %s %s", icon, summary)
			}
			line = todoStyle.Render(truncate(line, max(10, width-1)))
		}

		if highlight && i == cursor {
			line = lipgloss.NewStyle().Background(lipgloss.Color("238")).Render(line)
		}
		lines = append(lines, line)
		lastVisible = i
	}

	if len(lines) == 0 {
		lines = append(lines, styles.Subtle.Render("No agenda items"))
	}

	return agendaRender{
		Text:                  lipgloss.NewStyle().Width(width).MaxHeight(maxLines).Render(strings.Join(lines, "\n")),
		EventCount:            len(items),
		LastVisibleEventIndex: lastVisible,
	}
}

func buildAgendaItems(startDay time.Time, events []calendar.Event, todos []calendar.Todo, days int, includeFree bool, includeDoneTodos bool) []AgendaListItem {
	if days < 1 {
		days = 1
	}
	out := make([]AgendaListItem, 0, days*8)
	endDay := startDay.AddDate(0, 0, days)

	todosByDay := map[string][]AgendaListItem{}
	today := dayStart(time.Now().In(startDay.Location()))
	for i := range todos {
		t := todos[i]
		if !includeDoneTodos && isTodoDone(t) {
			continue
		}
		item, ok := agendaItemFromTodo(t, today)
		if !ok {
			continue
		}
		if item.Day.Before(startDay) || !item.Day.Before(endDay) {
			continue
		}
		k := item.Day.Format("2006-01-02")
		todosByDay[k] = append(todosByDay[k], item)
	}

	for d := 0; d < days; d++ {
		day := startDay.AddDate(0, 0, d)
		dayEnd := day.Add(24 * time.Hour)
		dayEvents := calendar.EventsOnDay(events, day)
		dayTodos := todosByDay[day.Format("2006-01-02")]

		allDay := make([]AgendaListItem, 0)
		timed := make([]AgendaListItem, 0)
		for _, ev := range dayEvents {
			item := AgendaListItem{Day: day, IsFree: false, Start: ev.Start, End: ev.End, Event: &ev, Mode: "event"}
			if ev.AllDay {
				allDay = append(allDay, item)
			} else {
				timed = append(timed, item)
			}
		}
		for _, td := range dayTodos {
			if td.Mode == "todo-all-day" {
				allDay = append(allDay, td)
			} else {
				timed = append(timed, td)
			}
		}
		sort.Slice(timed, func(i, j int) bool {
			if timed[i].Start.Equal(timed[j].Start) {
				return agendaItemSummary(timed[i]) < agendaItemSummary(timed[j])
			}
			return timed[i].Start.Before(timed[j].Start)
		})

		out = append(out, allDay...)

		if !includeFree {
			out = append(out, timed...)
			if len(allDay) == 0 && len(timed) == 0 {
				out = append(out, AgendaListItem{Day: day, IsFree: true, Start: day, End: dayEnd})
			}
			continue
		}

		cursor := day
		if len(timed) == 0 {
			if len(allDay) == 0 {
				out = append(out, AgendaListItem{Day: day, IsFree: true, Start: day, End: dayEnd})
			}
			continue
		}

		for i := range timed {
			it := timed[i]
			if it.Start.After(cursor) {
				out = append(out, AgendaListItem{Day: day, IsFree: true, Start: cursor, End: it.Start})
			}
			out = append(out, it)
			if it.End.After(cursor) {
				cursor = it.End
			}
		}
		if cursor.Before(dayEnd) {
			out = append(out, AgendaListItem{Day: day, IsFree: true, Start: cursor, End: dayEnd})
		}
	}
	return out
}

func agendaItemSummary(it AgendaListItem) string {
	if it.Event != nil {
		return it.Event.Summary
	}
	if it.Todo != nil {
		return it.Todo.Summary
	}
	return ""
}

func agendaItemFromTodo(todo calendar.Todo, today time.Time) (AgendaListItem, bool) {
	it := AgendaListItem{Todo: &todo, Mode: "todo-all-day"}
	if todo.Start != nil && todo.Due != nil {
		it.Day = dayStart(*todo.Start)
		it.Start = *todo.Start
		it.End = *todo.Due
		it.Mode = "todo-range"
		return it, true
	}
	if todo.Start != nil {
		it.Day = dayStart(*todo.Start)
		it.Start = *todo.Start
		it.End = todo.Start.Add(time.Hour)
		it.Mode = "todo-start"
		return it, true
	}
	if todo.Due != nil {
		it.Day = dayStart(*todo.Due)
		it.End = *todo.Due
		it.Start = todo.Due.Add(-time.Hour)
		it.Mode = "todo-end"
		return it, true
	}
	it.Day = dayStart(today)
	it.Start = it.Day
	it.End = it.Day.Add(24 * time.Hour)
	it.Mode = "todo-all-day"
	return it, true
}

func isTodoDone(todo calendar.Todo) bool {
	if strings.EqualFold(todo.Status, "COMPLETED") {
		return true
	}
	if todo.Completed != nil {
		return true
	}
	if todo.Percent >= 100 {
		return true
	}
	return false
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
			glyph := iconForEvent(ev)
			icon := styleForColor(styles.Event, ev.Color).Render(glyph)
			if ev.AllDay {
				if ev.Kind == calendar.EventKindBirthday || ev.Kind == calendar.EventKindAnniversary {
					line = fmt.Sprintf("  %s %s", icon, ev.Summary)
				} else {
					line = fmt.Sprintf("  %s all-day  %s", icon, ev.Summary)
				}
			} else {
				line = fmt.Sprintf("  %s %s-%s  %s", icon, ev.Start.Format(timeFmt), ev.End.Format(timeFmt), ev.Summary)
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
	normalized := normalizeColor(color)
	if normalized == "" {
		return base
	}
	return base.Foreground(lipgloss.Color(normalized))
}

func normalizeColor(color string) string {
	c := strings.TrimSpace(color)
	c = strings.Trim(c, "\"'")
	if c == "" {
		return ""
	}

	if strings.HasPrefix(c, "#") {
		if len(c) == 9 {
			return c[:7]
		}
		return c
	}

	if strings.HasPrefix(strings.ToLower(c), "0x") {
		h := c[2:]
		if len(h) >= 6 {
			return "#" + h[:6]
		}
	}

	lc := strings.ToLower(c)
	if strings.HasPrefix(lc, "rgb(") && strings.HasSuffix(c, ")") {
		inner := c[4 : len(c)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 3 {
			vals := [3]int{}
			for i := 0; i < 3; i++ {
				n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
				if err != nil || n < 0 || n > 255 {
					return ""
				}
				vals[i] = n
			}
			return fmt.Sprintf("#%02x%02x%02x", vals[0], vals[1], vals[2])
		}
	}

	return c
}

func formatTimeBoundary(t time.Time, day time.Time, timeFmt string) string {
	if t.Equal(day.Add(24 * time.Hour)) {
		if timeFmt == "3:04PM" || strings.Contains(strings.ToLower(timeFmt), "pm") {
			return "12:00AM"
		}
		return "24:00"
	}
	return t.Format(timeFmt)
}

func iconForEvent(ev calendar.Event) string {
	if ev.Kind == calendar.EventKindBirthday {
		return ""
	}
	if ev.Kind == calendar.EventKindAnniversary {
		return ""
	}
	if ev.Recurring {
		return "󰑖"
	}
	if ev.AllDay {
		return ""
	}
	if ev.HasAlarm {
		return "󰀠"
	}
	return ""
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
