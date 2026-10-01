package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

func TestTimeRangeEditorUsesFixedDigitSlots(t *testing.T) {
	editor := newTimeRangeEditor("09:00", "10:00")
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("23:45 -> 01:15")})
	start, end, err := editor.values()
	if err != nil {
		t.Fatal(err)
	}
	if start != "23:45" || end != "01:15" {
		t.Fatalf("time range = %s -> %s", start, end)
	}

	editor.cursor = 3
	editor.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if editor.cursor != 2 || editor.slots[2] != 0 {
		t.Fatalf("backspace did not clear the previous digit slot: cursor=%d slots=%q", editor.cursor, editor.slots)
	}
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	if editor.slots[2] != 0 {
		t.Fatal("separator input changed a digit slot")
	}

	invalid := newTimeRangeEditor("29:00", "10:00")
	invalid.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if invalid.done || invalid.err == "" {
		t.Fatal("invalid hour was accepted")
	}
}

func TestTimeRangeEditorMakesOvernightEndDateVisible(t *testing.T) {
	state := eventFormState{
		fromDate: "2026-10-01",
		toDate:   "2026-10-01",
		fromTime: "22:00",
		toTime:   "01:00",
	}
	state.adjustOvernightRange()
	if state.toDate != "2026-10-02" || !state.overnightAuto {
		t.Fatalf("overnight range = %s, auto=%v", state.toDate, state.overnightAuto)
	}

	state.toTime = "23:00"
	state.adjustOvernightRange()
	if state.toDate != state.fromDate || state.overnightAuto {
		t.Fatalf("daytime range did not undo automatic overnight date: %+v", state)
	}
}

func TestDateRangePickerCrossesMonthsAndNormalizesReverseRange(t *testing.T) {
	picker := newDateRangePicker("2026-10-01", "2026-10-01")
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	picker.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if picker.cursor.Format("2006-01-02") != "2026-09-30" || picker.month.Month() != time.September {
		t.Fatalf("left from month start = %s, visible month %s", picker.cursor, picker.month.Month())
	}
	start, end := picker.dates()
	if start.Format("2006-01-02") != "2026-09-30" || end.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("normalized range = %s -> %s", start, end)
	}
	picker.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !picker.done {
		t.Fatal("ctrl-s did not apply the date range")
	}
}

func TestAllDayRowsKeepDateAndDisableTimeAndTimezone(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.Local)
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.eventForm = m.newEventFormState("edit", "event", calendar.Event{
		UID: "event", Source: "src", Calendar: "cal", AllDay: true, Start: start, End: start.AddDate(0, 0, 2),
	})

	rows := m.eventEditorRows()
	date := eventEditorRow(t, rows, "date")
	if date.value != "2026-10-01 → 2026-10-02" {
		t.Fatalf("inclusive date row = %q", date.value)
	}
	if !isEditorDisabled(eventEditorRow(t, rows, "time")) || !isEditorDisabled(eventEditorRow(t, rows, "timezone")) {
		t.Fatal("all-day Time and Timezone rows are focusable")
	}
	view := m.renderEventEditorList(80, 40)
	if !strings.Contains(view, "Cancel") || !strings.Contains(view, "Save") {
		t.Fatalf("event actions missing:\n%s", view)
	}

	m.todoForm = m.newTodoFormState("edit", "todo", calendar.Todo{UID: "todo", Source: "src", Calendar: "cal"})
	todoView := m.renderTodoEditorList(80, 40)
	if !strings.Contains(todoView, "Cancel") || !strings.Contains(todoView, "Save") {
		t.Fatalf("todo actions missing:\n%s", todoView)
	}
}

func TestSearchPickerKeepsJKAsQueryAndUsesCtrlJKForNavigation(t *testing.T) {
	picker := newSearchPicker("Contacts", []searchOption{
		{label: "Alice", value: "alice"},
		{label: "Bob", value: "bob"},
		{label: "J K Contact", value: "jk"},
	}, "")
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if picker.query != "j" || len(picker.filtered) != 1 {
		t.Fatalf("j did not filter as text: query=%q matches=%d", picker.query, len(picker.filtered))
	}
	picker.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	picker.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !picker.done || picker.selected != "bob" {
		t.Fatalf("ctrl-j selection = %q, done=%v", picker.selected, picker.done)
	}
}

func TestSearchPickerDoesNotMatchAcrossDuplicatedValues(t *testing.T) {
	picker := newSearchPicker("Timezone", []searchOption{
		{label: "Australia/Brisbane", value: "Australia/Brisbane"},
		{label: "Europe/Berlin", value: "Europe/Berlin"},
	}, "")
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("berlin")})
	picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if picker.selected != "Europe/Berlin" {
		t.Fatalf("berlin selected %q", picker.selected)
	}
}

func TestAttendeeSearchOmitsExistingContacts(t *testing.T) {
	dir := t.TempDir()
	vcf := "BEGIN:VCARD\nVERSION:3.0\nFN:Ada Lovelace\nEMAIL:ada@example.test\nEND:VCARD\n" +
		"BEGIN:VCARD\nVERSION:3.0\nFN:Grace Hopper\nEMAIL:grace@example.test\nEND:VCARD\n"
	if err := os.WriteFile(filepath.Join(dir, "contacts.vcf"), []byte(vcf), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: []config.Source{{Path: dir, Type: "addressbook"}}}
	m := NewModel(cfg, calendar.Dataset{}, calendar.NewStore(cfg))
	m.eventForm = &eventFormState{attendees: "Ada Lovelace <ada@example.test>"}

	options := m.attendeeSearchOptions()
	if len(options) != 1 || options[0].value != "Grace Hopper <grace@example.test>" {
		t.Fatalf("attendee options = %+v", options)
	}
	m.eventForm.activeKey = "attendees-add"
	if !m.openCustomEventEditor("attendees-add") || m.eventForm.searchPicker == nil || m.eventForm.searchPicker.query != "" {
		t.Fatal("Add attendee did not open directly in search mode")
	}
	m.updateEventSearchPicker(tea.KeyMsg{Type: tea.KeyEnter})
	if m.eventForm.searchPicker != nil || !strings.Contains(m.eventForm.attendees, "Grace Hopper") {
		t.Fatalf("Enter did not add one attendee and close: %+v", m.eventForm)
	}
}

func TestEventFormTimesUseSelectedTimezoneAndInclusiveAllDayEnd(t *testing.T) {
	start, end, err := parseEventFormTimes(eventFormState{
		fromDate: "2026-01-15", fromTime: "09:00",
		toDate: "2026-01-15", toTime: "10:00",
		timezone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if start.Location().String() != "Europe/Berlin" || start.UTC().Hour() != 8 || end.Sub(start) != time.Hour {
		t.Fatalf("zoned range = %s -> %s", start, end)
	}

	start, end, err = parseEventFormTimes(eventFormState{
		fromDate: "2026-10-01", toDate: "2026-10-03", allDay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if end.Sub(start) != 72*time.Hour {
		t.Fatalf("all-day inclusive range duration = %s", end.Sub(start))
	}
}

func TestTimezoneLabelUsesEventDateOffset(t *testing.T) {
	winter := timezoneLabel("Europe/Berlin", time.Date(2026, time.January, 15, 9, 0, 0, 0, time.UTC))
	summer := timezoneLabel("Europe/Berlin", time.Date(2026, time.July, 15, 9, 0, 0, 0, time.UTC))
	if !strings.Contains(winter, "UTC+01:00") || !strings.Contains(summer, "UTC+02:00") {
		t.Fatalf("event-date offsets: winter=%q summer=%q", winter, summer)
	}
}

func eventEditorRow(t *testing.T, rows []editorRow, key string) editorRow {
	t.Helper()
	for _, row := range rows {
		if editorRowKey(row) == key {
			return row
		}
	}
	t.Fatalf("event editor row %q not found", key)
	return editorRow{}
}
