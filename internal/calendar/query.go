package calendar

import "time"

func EventsInRange(events []Event, start, end time.Time) []Event {
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		if ev.End.Before(start) || ev.Start.After(end) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func EventsOnDay(events []Event, day time.Time) []Event {
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	dayEnd := dayStart.Add(24*time.Hour - time.Nanosecond)
	return EventsInRange(events, dayStart, dayEnd)
}

func StartOfWeek(t time.Time, weekStart time.Weekday) time.Time {
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	offset := (7 + int(dayStart.Weekday()) - int(weekStart)) % 7
	return dayStart.AddDate(0, 0, -offset)
}

func FilterVisibleEvents(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		if ev.Hidden {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func FilterVisibleTodos(todos []Todo) []Todo {
	out := make([]Todo, 0, len(todos))
	for _, todo := range todos {
		if todo.Hidden {
			continue
		}
		out = append(out, todo)
	}
	return out
}
