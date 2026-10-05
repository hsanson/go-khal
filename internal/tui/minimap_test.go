package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

func TestMinimapGlyphUsesHalfHourOverlap(t *testing.T) {
	hour := time.Date(2026, time.October, 5, 9, 0, 0, 0, time.Local)
	for name, test := range map[string]struct {
		start time.Time
		end   time.Time
		want  string
	}{
		"upper": {start: hour, end: hour.Add(30 * time.Minute), want: "⠛"},
		"lower": {start: hour.Add(30 * time.Minute), end: hour.Add(time.Hour), want: "⣤"},
		"both":  {start: hour.Add(15 * time.Minute), end: hour.Add(45 * time.Minute), want: "⣿"},
	} {
		t.Run(name, func(t *testing.T) {
			event := calendar.Event{Start: test.start, End: test.end}
			if got := minimapGlyph(&event, hour); got != test.want {
				t.Fatalf("minimapGlyph() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMinimapLayoutFallsBackFromWeekToWorkweekToDay(t *testing.T) {
	m := NewModel(config.Default(), calendar.Dataset{}, nil)
	m.selected = time.Date(2026, time.October, 4, 0, 0, 0, 0, time.Local) // Sunday
	height := m.minimapHeight()
	labelWidth := m.minimapLabelWidth()

	for _, test := range []struct {
		name  string
		width int
		days  int
		slots int
		ok    bool
	}{
		{name: "week", width: minimapWidth(labelWidth, 7, 4) + minimapGap + minimapAgendaMinWidth, days: 7, slots: 4, ok: true},
		{name: "workweek", width: minimapWidth(labelWidth, 7, 4) + minimapGap + minimapAgendaMinWidth - 1, days: 5, slots: 4, ok: true},
		{name: "day", width: minimapWidth(labelWidth, 5, 4) + minimapGap + minimapAgendaMinWidth - 1, days: 1, slots: 8, ok: true},
		{name: "hidden", width: minimapWidth(labelWidth, 1, 8) + minimapGap + minimapAgendaMinWidth - 1, ok: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			layout, ok := m.minimapLayout(test.width, height)
			if ok != test.ok {
				t.Fatalf("minimapLayout() visible = %v, want %v", ok, test.ok)
			}
			if !ok {
				return
			}
			if len(layout.days) != test.days || layout.slots != test.slots {
				t.Fatalf("minimapLayout() = %d days/%d slots, want %d/%d", len(layout.days), layout.slots, test.days, test.slots)
			}
			if test.days == 5 && layout.days[0].Weekday() != time.Monday {
				t.Fatalf("workweek starts on %s, want Monday", layout.days[0].Weekday())
			}
		})
	}

	if _, ok := m.minimapLayout(200, height-1); ok {
		t.Fatal("minimap rendered without enough vertical space")
	}
}

func TestMinimapIncludesDimmedFreeEventsAndFiltersHiddenAndDeclined(t *testing.T) {
	start := time.Date(2026, time.October, 5, 9, 0, 0, 0, time.Local)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(config.Default(), calendar.Dataset{
		Calendars: []calendar.Calendar{cal},
		Events: []calendar.Event{
			{UID: "busy", Source: cal.Source, Calendar: cal.Name, Start: start, End: start.Add(time.Hour)},
			{UID: "free", Source: cal.Source, Calendar: cal.Name, Availability: "free", Color: "#123456", Start: start, End: start.Add(time.Hour)},
			{UID: "declined", Source: cal.Source, Calendar: cal.Name, UserRSVP: "no", Start: start, End: start.Add(time.Hour)},
		},
	}, nil)

	if got := m.minimapEventIndexes(); len(got) != 2 {
		t.Fatalf("default minimap event count = %d, want busy and free events", len(got))
	}
	if minimapEventStyle(m.styles.Event, &m.data.Events[0]).GetFaint() {
		t.Fatal("busy minimap event is faint")
	}
	if !minimapEventStyle(m.styles.Event, &m.data.Events[1]).GetFaint() {
		t.Fatal("free minimap event is not faint")
	}
	m.showAllMode = true
	if got := m.minimapEventIndexes(); len(got) != 3 {
		t.Fatalf("show-all minimap event count = %d, want 3", len(got))
	}
	m.calendarVisibility[calendarKey(cal.Source, cal.Name)] = false
	if got := m.minimapEventIndexes(); len(got) != 0 {
		t.Fatalf("hidden-calendar minimap event count = %d, want 0", len(got))
	}
}

func TestMinimapMouseNavigatesAndStagesEventCreation(t *testing.T) {
	day := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.Local)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	newModel := func(events []calendar.Event) Model {
		m := NewModel(config.Default(), calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: events}, nil)
		m.width = 120
		m.height = 40
		m.selected = day
		m.agendaStart = day
		return m
	}

	t.Run("event cell selects the clicked day occurrence", func(t *testing.T) {
		event := calendar.Event{
			UID: "event", Source: cal.Source, Calendar: cal.Name,
			Start: day.Add(9 * time.Hour), End: day.AddDate(0, 0, 1).Add(10 * time.Hour),
		}
		clickedDay := day.AddDate(0, 0, 1)
		m := newModel([]calendar.Event{event})
		m.focusCalendarPane = true
		m.focusMain = false
		m = clickRenderedMinimapHit(t, m, mouseMinimapEvent, 0, clickedDay)

		if m.eventForm != nil {
			t.Fatal("event minimap click opened the event editor")
		}
		if !dayStart(m.selected).Equal(clickedDay) || !m.focusMain || m.focusCalendarPane || m.focusDetails {
			t.Fatalf("event minimap click selected date/focus = %v/%v/%v/%v", m.selected, m.focusMain, m.focusCalendarPane, m.focusDetails)
		}
		item := m.agendaItems()[m.eventCursor]
		if item.Event == nil || item.Event.UID != event.UID || !item.Day.Equal(clickedDay) {
			t.Fatalf("selected agenda item = %#v, want clicked event on %v", item, clickedDay)
		}
	})

	t.Run("empty hour selects a temporary row used by new event", func(t *testing.T) {
		event := calendar.Event{
			UID: "later", Source: cal.Source, Calendar: cal.Name,
			Start: day.Add(10 * time.Hour), End: day.Add(11 * time.Hour),
		}
		hour := day.Add(8 * time.Hour)
		m := clickRenderedMinimapHit(t, newModel([]calendar.Event{event}), mouseMinimapTime, -1, hour)

		if m.eventForm != nil {
			t.Fatal("empty minimap click opened the event editor")
		}
		item := m.agendaItems()[m.eventCursor]
		if item.Mode != temporaryFreeAgendaMode || !item.Start.Equal(hour) || !item.End.Equal(hour.Add(time.Hour)) {
			t.Fatalf("selected temporary row = %#v, want %v-%v", item, hour, hour.Add(time.Hour))
		}

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		m = *modelValue(t, updated)
		if m.eventForm == nil || m.eventForm.allDay || m.eventForm.fromTime != "08:00" || m.eventForm.toTime != "09:00" {
			t.Fatalf("new event defaults = %#v, want clicked hour", m.eventForm)
		}
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m = *modelValue(t, updated)
		if m.temporaryFreeStart.IsZero() {
			t.Fatal("cancel removed the temporary row")
		}
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = *modelValue(t, updated)
		if !m.temporaryFreeStart.IsZero() {
			t.Fatal("moving the agenda cursor retained the temporary row")
		}
		item = m.agendaItems()[m.eventCursor]
		if item.Event == nil || item.Event.UID != event.UID {
			t.Fatalf("cursor moved to %#v, want next event", item)
		}
	})

	t.Run("temporary row replaces a covering free interval", func(t *testing.T) {
		event := calendar.Event{
			UID: "later", Source: cal.Source, Calendar: cal.Name,
			Start: day.Add(10 * time.Hour), End: day.Add(11 * time.Hour),
		}
		hour := day.Add(8 * time.Hour)
		m := newModel([]calendar.Event{event})
		m.showAllMode = true
		m = clickRenderedMinimapHit(t, m, mouseMinimapTime, -1, hour)

		covering := 0
		for _, item := range m.agendaItems() {
			if item.IsFree && !hour.Before(item.Start) && hour.Before(item.End) {
				covering++
				if item.Mode != temporaryFreeAgendaMode || !item.Start.Equal(hour) || !item.End.Equal(hour.Add(time.Hour)) {
					t.Fatalf("covering free row = %#v, want only the temporary hour", item)
				}
			}
		}
		if covering != 1 {
			t.Fatalf("free rows covering clicked hour = %d, want 1", covering)
		}
	})

	t.Run("empty all-day cell uses configured first hour", func(t *testing.T) {
		m := newModel(nil)
		m.cfg.MinimapStartTime = "06:00"
		m = clickRenderedMinimapHit(t, m, mouseMinimapAllDay, -1, day)
		want := day.Add(6 * time.Hour)
		item := m.agendaItems()[m.eventCursor]
		if item.Mode != temporaryFreeAgendaMode || !item.Start.Equal(want) {
			t.Fatalf("all-day temporary row = %#v, want start %v", item, want)
		}
	})
}

func TestMinimapFillsSlotsProportionallyAndUsesUnderscoresWhenEmpty(t *testing.T) {
	day := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.Local)
	hour := day.Add(13 * time.Hour)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	events := []calendar.Event{
		{UID: "top", Source: cal.Source, Calendar: cal.Name, Start: hour, End: hour.Add(30 * time.Minute)},
		{UID: "full", Source: cal.Source, Calendar: cal.Name, Start: hour, End: hour.Add(time.Hour)},
		{UID: "lower", Source: cal.Source, Calendar: cal.Name, Start: hour.Add(30 * time.Minute), End: hour.Add(time.Hour)},
	}
	m := NewModel(config.Default(), calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: events}, nil)
	m.selected = day

	for _, test := range []struct {
		slots       int
		wantGlyphs  string
		wantIndexes []int
	}{
		{slots: 4, wantGlyphs: "⠛⠛⣿⣤", wantIndexes: []int{0, 0, 1, 2}},
		{slots: 8, wantGlyphs: "⠛⠛⠛⣿⣿⣿⣤⣤", wantIndexes: []int{0, 0, 0, 1, 1, 1, 2, 2}},
	} {
		m.mouse.reset()
		layout := minimapLayout{days: []time.Time{day}, slots: test.slots, labelWidth: m.minimapLabelWidth()}
		lines := strings.Split(ansi.Strip(m.renderMinimap(layout, 0, 0)), "\n")
		if got := strings.TrimPrefix(lines[7], "13:00 "); got != test.wantGlyphs {
			t.Fatalf("%d-slot row = %q, want %q", test.slots, got, test.wantGlyphs)
		}
		gotIndexes := make([]int, 0, test.slots)
		for _, hit := range m.mouse.hits {
			if hit.kind == mouseMinimapEvent && hit.rect.y == 7 {
				gotIndexes = append(gotIndexes, hit.index)
			}
		}
		if len(gotIndexes) != len(test.wantIndexes) {
			t.Fatalf("%d-slot event hits = %v, want %v", test.slots, gotIndexes, test.wantIndexes)
		}
		for i := range gotIndexes {
			if gotIndexes[i] != test.wantIndexes[i] {
				t.Fatalf("%d-slot event hits = %v, want %v", test.slots, gotIndexes, test.wantIndexes)
			}
		}
	}

	empty := NewModel(config.Default(), calendar.Dataset{}, nil)
	empty.selected = day
	layout := minimapLayout{days: []time.Time{day}, slots: 4, labelWidth: empty.minimapLabelWidth()}
	lines := strings.Split(ansi.Strip(empty.renderMinimap(layout, 0, 0)), "\n")
	if got, want := lines[1], strings.Repeat(" ", layout.labelWidth+1)+"____"; got != want {
		t.Fatalf("empty all-day row = %q, want %q", got, want)
	}
	if got := strings.TrimPrefix(lines[2], "08:00 "); got != "____" {
		t.Fatalf("empty timed row = %q, want underscores", got)
	}
}

func clickRenderedMinimapHit(t *testing.T, m Model, kind mouseTarget, index int, when time.Time) Model {
	t.Helper()
	_ = m.View()
	for _, hit := range m.mouse.hits {
		if hit.kind == kind && (index < 0 || hit.index == index) && hit.day.Equal(when) {
			updated, _ := m.Update(tea.MouseMsg{
				X:      hit.rect.x,
				Y:      hit.rect.y,
				Action: tea.MouseActionPress,
				Button: tea.MouseButtonLeft,
			})
			return *modelValue(t, updated)
		}
	}
	t.Fatalf("rendered minimap target kind=%d index=%d time=%v not found", kind, index, when)
	return m
}
