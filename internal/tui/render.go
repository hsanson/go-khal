package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
)

func RenderAgenda(events []calendar.Event, now time.Time, timeFmt string, styles Styles) string {
	var b strings.Builder
	b.WriteString(styles.Title.Render("Agenda"))
	b.WriteString("\n")
	for _, ev := range events {
		if ev.End.Before(now) {
			continue
		}
		cal := ev.DisplayName
		if cal == "" {
			cal = ev.Calendar
		}
		if ev.Color != "" {
			cal = fmt.Sprintf("%s %s", cal, ev.Color)
		}
		line := fmt.Sprintf("%s  %s  [%s]", ev.Start.Format("2006-01-02 "+timeFmt), ev.Summary, cal)
		b.WriteString(styles.Event.Render(line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func RenderDayTimeline(events []calendar.Event, day time.Time, timeFmt string, styles Styles) string {
	hourly := make([][]calendar.Event, 24)
	for _, ev := range events {
		if ev.Start.Year() == day.Year() && ev.Start.Month() == day.Month() && ev.Start.Day() == day.Day() {
			hourly[ev.Start.Hour()] = append(hourly[ev.Start.Hour()], ev)
		}
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render(day.Format("Monday, 2006-01-02")))
	b.WriteString("\n")
	for h := 0; h < 24; h++ {
		hourTime := time.Date(day.Year(), day.Month(), day.Day(), h, 0, 0, 0, day.Location())
		b.WriteString(styles.Hour.Render(hourTime.Format(timeFmt)))
		if len(hourly[h]) == 0 {
			b.WriteString(" |\n")
			continue
		}
		for i, ev := range hourly[h] {
			if i == 0 {
				b.WriteString(" | ")
			} else {
				b.WriteString("\n      | ")
			}
			cal := ev.DisplayName
			if cal == "" {
				cal = ev.Calendar
			}
			if ev.Color != "" {
				cal = fmt.Sprintf("%s %s", cal, ev.Color)
			}
			b.WriteString(styles.Event.Render(fmt.Sprintf("%s-%s %s [%s]", ev.Start.Format(timeFmt), ev.End.Format(timeFmt), ev.Summary, cal)))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func RenderTodos(todos []calendar.Todo, styles Styles) string {
	var b strings.Builder
	b.WriteString(styles.Title.Render("VTODO"))
	b.WriteString("\n")
	for _, t := range todos {
		status := "[ ]"
		renderStyle := styles.TodoOpen
		if strings.EqualFold(t.Status, "COMPLETED") || t.Percent >= 100 {
			status = "[x]"
			renderStyle = styles.TodoDone
		}
		due := ""
		if t.Due != nil {
			due = " due:" + t.Due.Format("2006-01-02 15:04")
		}
		cal := t.DisplayName
		if cal == "" {
			cal = t.Calendar
		}
		if t.Color != "" {
			cal = fmt.Sprintf("%s %s", cal, t.Color)
		}
		b.WriteString(renderStyle.Render(fmt.Sprintf("%s %s (%s)%s [%s]", status, t.Summary, t.UID, due, cal)))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
