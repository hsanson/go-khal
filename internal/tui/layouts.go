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
			eventStyle := styleForColor(styles.Event, ev.Color)
			if eventRSVPIsNo(ev) {
				eventStyle = styles.Subtle
			}
			icon := eventStyle.Render(glyph)
			if ev.AllDay && (ev.Kind == calendar.EventKindBirthday || ev.Kind == calendar.EventKindAnniversary) {
				line = fmt.Sprintf("  %s %s", icon, ev.Summary)
			} else if ev.AllDay {
				line = fmt.Sprintf("  %s all-day  %s", icon, ev.Summary)
			} else {
				line = fmt.Sprintf("  %s %s-%s  %s", icon, ev.Start.Format(timeFmt), ev.End.Format(timeFmt), ev.Summary)
			}
			line = eventStyle.Render(truncate(line, max(10, width-1)))
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

func buildAgendaItems(startDay time.Time, events []calendar.Event, days int, includeFree bool) []AgendaListItem {
	if days < 1 {
		days = 1
	}
	out := make([]AgendaListItem, 0, days*8)

	for d := 0; d < days; d++ {
		day := startDay.AddDate(0, 0, d)
		dayEnd := day.Add(24 * time.Hour)
		dayEvents := calendar.EventsOnDay(events, day)

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

func buildTaskItems(todos []calendar.Todo, includeDoneTodos bool, loc *time.Location) []AgendaListItem {
	if loc == nil {
		loc = time.Local
	}
	today := dayStart(time.Now().In(loc))
	out := make([]AgendaListItem, 0, len(todos))
	for i := range todos {
		t := todos[i]
		if !includeDoneTodos && isTodoDone(t) {
			continue
		}
		item, ok := taskListItemFromTodo(t, today)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			pi := todoSortPriority(out[i].Todo)
			pj := todoSortPriority(out[j].Todo)
			if pi == pj {
				return agendaItemSummary(out[i]) < agendaItemSummary(out[j])
			}
			return pi < pj
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

func todoSortPriority(todo *calendar.Todo) int {
	if todo == nil || todo.Priority <= 0 {
		return 10
	}
	return todo.Priority
}

func taskListItemFromTodo(todo calendar.Todo, today time.Time) (AgendaListItem, bool) {
	if todo.Due != nil {
		return AgendaListItem{
			Day:   dayStart(*todo.Due),
			Start: *todo.Due,
			End:   *todo.Due,
			Todo:  &todo,
			Mode:  "todo-end",
		}, true
	}
	if todo.Start != nil {
		return AgendaListItem{
			Day:   dayStart(*todo.Start),
			Start: *todo.Start,
			End:   todo.Start.Add(time.Hour),
			Todo:  &todo,
			Mode:  "todo-start",
		}, true
	}
	day := dayStart(today)
	return AgendaListItem{
		Day:   day,
		Start: day,
		End:   day.Add(24 * time.Hour),
		Todo:  &todo,
		Mode:  "todo-all-day",
	}, true
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
