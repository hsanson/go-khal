package tui

import (
	"strings"
	"testing"
	"time"

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

func TestMinimapExcludesFreeHiddenAndDeclinedEvents(t *testing.T) {
	start := time.Date(2026, time.October, 5, 9, 0, 0, 0, time.Local)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(config.Default(), calendar.Dataset{
		Calendars: []calendar.Calendar{cal},
		Events: []calendar.Event{
			{UID: "busy", Source: cal.Source, Calendar: cal.Name, Start: start, End: start.Add(time.Hour)},
			{UID: "free", Source: cal.Source, Calendar: cal.Name, Availability: "free", Start: start, End: start.Add(time.Hour)},
			{UID: "declined", Source: cal.Source, Calendar: cal.Name, UserRSVP: "no", Start: start, End: start.Add(time.Hour)},
		},
	}, nil)

	if got := m.minimapEventIndexes(); len(got) != 1 || m.data.Events[got[0]].UID != "busy" {
		t.Fatalf("default minimap events = %v, want only busy event", got)
	}
	m.showAllMode = true
	if got := m.minimapEventIndexes(); len(got) != 2 {
		t.Fatalf("show-all minimap event count = %d, want 2", len(got))
	}
	m.calendarVisibility[calendarKey(cal.Source, cal.Name)] = false
	if got := m.minimapEventIndexes(); len(got) != 0 {
		t.Fatalf("hidden-calendar minimap event count = %d, want 0", len(got))
	}
}

func TestMinimapMouseOpensAndCreatesEvents(t *testing.T) {
	day := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.Local)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	event := calendar.Event{
		UID: "event", Source: cal.Source, Calendar: cal.Name,
		Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour),
	}
	newModel := func(events []calendar.Event) Model {
		m := NewModel(config.Default(), calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: events}, nil)
		m.width = 120
		m.height = 40
		m.selected = day
		m.agendaStart = day
		return m
	}

	m := clickRenderedHit(t, newModel([]calendar.Event{event}), mouseMinimapEvent, 0, "")
	if m.eventForm == nil || m.eventForm.mode != "edit" || m.eventForm.targetUID != event.UID {
		t.Fatalf("occupied minimap cell did not open event editor: %#v", m.eventForm)
	}

	m = clickRenderedHit(t, newModel(nil), mouseMinimapTime, -1, "")
	if m.eventForm == nil || m.eventForm.allDay || m.eventForm.fromTime != "08:00" || m.eventForm.toTime != "08:30" {
		t.Fatalf("empty timed cell created wrong event: %#v", m.eventForm)
	}

	m = clickRenderedHit(t, newModel(nil), mouseMinimapAllDay, -1, "")
	if m.eventForm == nil || !m.eventForm.allDay || m.eventForm.fromDate != m.eventForm.toDate {
		t.Fatalf("empty all-day cell created wrong event: %#v", m.eventForm)
	}
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
