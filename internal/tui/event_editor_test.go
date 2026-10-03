package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestTimeRangeEditorAdjustsFocusedComponents(t *testing.T) {
	editor := newTimeRangeEditor("23:50", "00:10")

	editor.Update(tea.KeyMsg{Type: tea.KeyUp})
	start, _, err := editor.values()
	if err != nil || start != "00:50" || editor.cursor != 0 {
		t.Fatalf("hour increment = %q, cursor=%d, err=%v", start, editor.cursor, err)
	}
	editor.Update(tea.KeyMsg{Type: tea.KeyDown})
	start, _, err = editor.values()
	if err != nil || start != "23:50" {
		t.Fatalf("hour decrement = %q, err=%v", start, err)
	}

	editor.cursor = 2
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	start, _, err = editor.values()
	if err != nil || start != "23:05" || editor.cursor != 2 {
		t.Fatalf("minute increment = %q, cursor=%d, err=%v", start, editor.cursor, err)
	}
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	start, _, err = editor.values()
	if err != nil || start != "23:50" {
		t.Fatalf("minute decrement = %q, err=%v", start, err)
	}

	editor.cursor = 3
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if editor.cursor != 4 {
		t.Fatalf("l did not skip the range separator: cursor=%d", editor.cursor)
	}
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if editor.cursor != 3 {
		t.Fatalf("h did not move to the previous digit: cursor=%d", editor.cursor)
	}
	editor.cursor = 0
	editor.Update(tea.KeyMsg{Type: tea.KeyLeft})
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if editor.cursor != 0 {
		t.Fatalf("left edge wrapped to cursor %d", editor.cursor)
	}
	editor.cursor = 7
	editor.Update(tea.KeyMsg{Type: tea.KeyRight})
	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if editor.cursor != 7 {
		t.Fatalf("right edge wrapped to cursor %d", editor.cursor)
	}

	editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !editor.cancelled {
		t.Fatal("q did not cancel the time editor")
	}
}

func TestSingleTimeEditorUsesFourDigitSlots(t *testing.T) {
	editor := newSingleTimeEditor("00:00")
	editor.cursor = 2
	editor.Update(tea.KeyMsg{Type: tea.KeyUp})
	value, err := editor.value()
	if err != nil || value != "00:15" {
		t.Fatalf("single time increment = %q, err=%v", value, err)
	}
	editor.cursor = 3
	editor.Update(tea.KeyMsg{Type: tea.KeyRight})
	if editor.cursor != 3 {
		t.Fatalf("single time right edge wrapped to cursor %d", editor.cursor)
	}
}

func TestTaskScheduleUsesSeparateOptionalPickers(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewTaskModeModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.selected = time.Date(2026, time.October, 2, 0, 0, 0, 0, time.Local)
	m.openTodoFormNew()

	if m.todoForm.dueDate != "" || m.todoForm.dueTime != "" || m.todoForm.startDate != "" || m.todoForm.startTime != "" {
		t.Fatalf("new task schedule is not empty: %+v", m.todoForm)
	}
	rows := m.todoEditorRows()
	keys := make([]string, 0, 4)
	for _, row := range rows {
		switch editorRowKey(row) {
		case "due-date", "due-time", "start-date", "start-time":
			keys = append(keys, editorRowKey(row))
		}
	}
	if strings.Join(keys, ",") != "due-date,due-time,start-date,start-time" {
		t.Fatalf("task schedule row order = %v", keys)
	}
	if !isEditorDisabled(eventEditorRow(t, rows, "due-time")) || !isEditorDisabled(eventEditorRow(t, rows, "start-time")) {
		t.Fatal("time rows without dates are selectable")
	}

	setTodoEditorCursor(t, &m, "due-date")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = *modelValue(t, updated)
	if editorRowKey(m.todoEditorRows()[m.todoForm.cursor]) != "start-date" {
		t.Fatalf("navigation did not skip disabled Due time: cursor=%d", m.todoForm.cursor)
	}

	setTodoEditorCursor(t, &m, "start-date")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.datePicker == nil || !m.todoForm.datePicker.singleDate ||
		m.todoForm.datePicker.cursor.Format("2006-01-02") != "2026-10-02" {
		t.Fatalf("Start date picker = %+v", m.todoForm.datePicker)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.startDate != "2026-10-03" || m.todoForm.startTime != "00:00" {
		t.Fatalf("selected Start schedule = %q %q", m.todoForm.startDate, m.todoForm.startTime)
	}

	setTodoEditorCursor(t, &m, "start-time")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.timeEditor == nil {
		t.Fatal("Start time did not open the single-time editor")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.startTime != "01:00" {
		t.Fatalf("adjusted Start time = %q", m.todoForm.startTime)
	}

	setTodoEditorCursor(t, &m, "due-date")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.datePicker == nil || m.todoForm.datePicker.cursor.Format("2006-01-02") != "2026-10-03" {
		t.Fatalf("Due date did not initialize from Start date: %+v", m.todoForm.datePicker)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.dueDate != "2026-10-03" || m.todoForm.dueTime != "00:00" {
		t.Fatalf("selected Due schedule = %q %q", m.todoForm.dueDate, m.todoForm.dueTime)
	}

	m.todoForm.dueTime = "12:30"
	setTodoEditorCursor(t, &m, "due-date")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.dueTime != "12:30" {
		t.Fatalf("changing Due date reset existing time to %q", m.todoForm.dueTime)
	}

	setTodoEditorCursor(t, &m, "due-time")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	m = *modelValue(t, updated)
	if m.todoForm.dueDate != "" || m.todoForm.dueTime != "" {
		t.Fatalf("Delete did not clear the Due pair: %q %q", m.todoForm.dueDate, m.todoForm.dueTime)
	}
}

func TestTaskDatePickerStagesClearAndNavigationRestoresSelection(t *testing.T) {
	picker := newTaskDatePicker("2026-10-03")
	if view := picker.View(Styles{}); !strings.Contains(view, "[ Clear ]") || !strings.Contains(view, "Date: Oct 3, 2026") {
		t.Fatalf("task date picker is missing clear controls:\n%s", view)
	}

	picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !picker.cleared || !strings.Contains(picker.View(Styles{}), "Date: —") {
		t.Fatalf("Space did not stage a clear:\n%s", picker.View(Styles{}))
	}
	picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !picker.cleared {
		t.Fatal("Space toggled a staged clear back on")
	}

	picker.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if picker.cleared || picker.start.Format("2006-01-02") != "2026-10-02" {
		t.Fatalf("navigation did not restore the moved-to date: cleared=%v start=%s", picker.cleared, picker.start)
	}

	repeatUntil := newSingleDatePicker("2026-10-03")
	repeatUntil.Update(tea.KeyMsg{Type: tea.KeySpace})
	if repeatUntil.cleared || strings.Contains(repeatUntil.View(Styles{}), "spc  clear") {
		t.Fatal("Repeat Until gained task-only clearing")
	}
	eventDate := newDateRangePicker("2026-10-03", "2026-10-03")
	eventDate.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !eventDate.rangeMode || strings.Contains(eventDate.View(Styles{}), "spc  clear") {
		t.Fatal("Event Date lost its Multi-day Space behavior")
	}
}

func TestTaskDatePickerAppliesOrCancelsStagedPairClear(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewTaskModeModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.todoForm = m.newTodoFormState("edit", "task", calendar.Todo{
		UID:      "task",
		Source:   "src",
		Calendar: "cal",
	})
	m.todoForm.dueDate = "2026-10-03"
	m.todoForm.dueTime = "12:30"
	setTodoEditorCursor(t, &m, "due-date")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = *modelValue(t, updated)
	if m.todoForm.dueDate != "2026-10-03" || m.todoForm.dueTime != "12:30" {
		t.Fatalf("staging clear mutated the task early: %q %q", m.todoForm.dueDate, m.todoForm.dueTime)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.todoForm.datePicker != nil || m.todoForm.dueDate != "" || m.todoForm.dueTime != "" {
		t.Fatalf("applying clear kept the Due pair: %q %q", m.todoForm.dueDate, m.todoForm.dueTime)
	}

	m.todoForm.startDate = "2026-10-04"
	m.todoForm.startTime = "09:15"
	setTodoEditorCursor(t, &m, "start-date")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = *modelValue(t, updated)
	if m.todoForm.datePicker != nil || m.todoForm.startDate != "2026-10-04" || m.todoForm.startTime != "09:15" {
		t.Fatalf("cancelling clear changed the Start pair: %q %q", m.todoForm.startDate, m.todoForm.startTime)
	}
}

func TestTaskSchedulePreservesTimestampValidation(t *testing.T) {
	start, due, err := parseTodoFormTimesOptional(todoFormState{
		startDate: "2026-10-03",
		startTime: "09:15",
		dueDate:   "2026-10-03",
		dueTime:   "10:30",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2026, time.October, 3, 9, 15, 0, 0, time.Local)
	wantDue := time.Date(2026, time.October, 3, 10, 30, 0, 0, time.Local)
	if start == nil || due == nil || !start.Equal(wantStart) || !due.Equal(wantDue) {
		t.Fatalf("parsed task schedule = %v -> %v", start, due)
	}

	_, _, err = parseTodoFormTimesOptional(todoFormState{
		startDate: "2026-10-03",
		startTime: "10:30",
		dueDate:   "2026-10-03",
		dueTime:   "09:15",
	})
	if err == nil || !strings.Contains(err.Error(), "due must be after start") {
		t.Fatalf("invalid task range error = %v", err)
	}

	_, _, err = parseTodoFormTimesOptional(todoFormState{dueDate: "2026-10-03"})
	if err == nil || !strings.Contains(err.Error(), "date and time must both be set") {
		t.Fatalf("incomplete task pair error = %v", err)
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

func TestDateRangePickerKeepsWidthWhenEndDateChanges(t *testing.T) {
	picker := newDateRangePicker("2026-09-30", "2026-09-30")
	picker.toggleRange()
	withoutEnd := lipgloss.Width(picker.View(Styles{}))

	picker.selectByMouse(time.Date(2026, time.October, 2, 0, 0, 0, 0, time.UTC))
	withEnd := lipgloss.Width(picker.View(Styles{}))

	if withoutEnd != withEnd {
		t.Fatalf("date picker width changed from %d to %d after selecting end date", withoutEnd, withEnd)
	}
	if withEnd < lipgloss.Width("Start: Sep 30, 2026    End: Oct 2, 2026") {
		t.Fatalf("date picker width %d cannot contain full start and end dates", withEnd)
	}
}
func TestDateRangePickerMultiDayUsesSelectedStartThenSelectsEnd(t *testing.T) {
	picker := newDateRangePicker("2026-10-01", "2026-10-01")
	picker.selectByMouse(time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC))
	picker.toggleRange()
	picker.selectByMouse(time.Date(2026, time.October, 7, 0, 0, 0, 0, time.UTC))

	start, end := picker.dates()
	if got := start.Format("2006-01-02"); got != "2026-10-05" {
		t.Fatalf("multi-day start = %s, want selected start 2026-10-05", got)
	}
	if got := end.Format("2006-01-02"); got != "2026-10-07" {
		t.Fatalf("multi-day end = %s, want next click 2026-10-07", got)
	}
}

func TestDateRangePickerCrossesMonthsAndNormalizesReverseRange(t *testing.T) {
	picker := newDateRangePicker("2026-10-01", "2026-10-01")
	picker.Update(tea.KeyMsg{Type: tea.KeySpace})
	picker.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if picker.cursor.Format("2006-01-02") != "2026-09-30" || picker.month.Month() != time.September {
		t.Fatalf("left from month start = %s, visible month %s", picker.cursor, picker.month.Month())
	}
	start, end := picker.dates()
	if start.Format("2006-01-02") != "2026-09-30" || end.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("normalized range = %s -> %s", start, end)
	}
	picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !picker.done {
		t.Fatal("enter did not apply the date range")
	}
}

func TestDateRangePickerNavigatesMonthsAndYears(t *testing.T) {
	month := newSingleDatePicker("2025-01-31")
	month.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if got := month.start.Format("2006-01-02"); got != "2025-02-28" {
		t.Fatalf("Ctrl-J date = %s", got)
	}
	month.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if got := month.start.Format("2006-01-02"); got != "2025-01-28" {
		t.Fatalf("Ctrl-K date = %s", got)
	}

	year := newSingleDatePicker("2024-02-29")
	year.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	if got := year.start.Format("2006-01-02"); got != "2023-02-28" {
		t.Fatalf("Ctrl-H date = %s", got)
	}
	year.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if got := year.start.Format("2006-01-02"); got != "2024-02-28" {
		t.Fatalf("Ctrl-L date = %s", got)
	}
}

func TestDatePickerKeepsKeyboardShortcutsAndRendersMouseControls(t *testing.T) {
	picker := newDateRangePicker("2026-10-01", "2026-10-01")
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyTab},
		{Type: tea.KeyShiftTab},
		{Type: tea.KeyRunes, Runes: []rune{'r'}},
		{Type: tea.KeyCtrlS},
	} {
		picker.Update(key)
	}
	if picker.rangeMode || picker.done {
		t.Fatalf("disabled keys changed picker: range=%v done=%v", picker.rangeMode, picker.done)
	}

	view := picker.View(Styles{})
	for _, removed := range []string{"Ok", "[ctrl-s]", "[tab]", "r    range"} {
		if strings.Contains(view, removed) {
			t.Fatalf("date picker still renders %q:\n%s", removed, view)
		}
	}
	if !strings.Contains(view, "« ‹ › »") || !strings.Contains(view, "[ Today ]") || !strings.Contains(view, "[ ] Multi-day") {
		t.Fatalf("date picker is missing mouse controls:\n%s", view)
	}

	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !picker.cancelled || picker.done {
		t.Fatalf("q cancellation: cancelled=%v done=%v", picker.cancelled, picker.done)
	}
}

func TestRepeatUntilUsesSingleDatePicker(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	m.eventForm.fromDate = "2026-10-02"
	m.eventForm.recur = true
	m.eventForm.recurEnd = "until"
	m.eventForm.recurUntil = ""
	setEventEditorCursor(t, &m, "recur-until")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = *modelValue(t, updated)
	if m.eventForm.recurUntil != "" || m.eventForm.datePicker != nil {
		t.Fatal("direct h changed or opened Repeat Until")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	picker := m.eventForm.datePicker
	if picker == nil || !picker.singleDate || picker.cursor.Format("2006-01-02") != "2026-10-02" {
		t.Fatalf("Repeat Until picker = %+v", picker)
	}
	view := picker.View(Styles{})
	if !strings.Contains(view, "Date: Oct 2, 2026") || strings.Contains(view, "Start:") || strings.Contains(view, "Multi-day") {
		t.Fatalf("unexpected Repeat Until picker:\n%s", view)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = *modelValue(t, updated)
	if m.eventForm.datePicker.rangeMode {
		t.Fatal("Space enabled a range in Repeat Until")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = *modelValue(t, updated)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.eventForm.datePicker != nil || m.eventForm.recurUntil != "2026-10-01" {
		t.Fatalf("Repeat Until applied %q with picker=%v", m.eventForm.recurUntil, m.eventForm.datePicker != nil)
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

func TestEventEditorCyclesCalendarWithoutOpeningPopup(t *testing.T) {
	calendars := []calendar.Calendar{
		{Source: "src", Name: "one"},
		{Source: "src", Name: "two"},
	}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: calendars}, nil)
	m.openEventFormNew()
	setEventEditorCursor(t, &m, "calendar")

	for _, step := range []struct {
		key  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}, calendarKey("src", "two")},
		{tea.KeyMsg{Type: tea.KeyRight}, calendarKey("src", "one")},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, calendarKey("src", "two")},
		{tea.KeyMsg{Type: tea.KeyLeft}, calendarKey("src", "one")},
	} {
		updated, _ := m.Update(step.key)
		m = *modelValue(t, updated)
		if m.eventForm.calendarKey != step.want {
			t.Fatalf("%s selected %q, want %q", step.key.String(), m.eventForm.calendarKey, step.want)
		}
		if m.eventForm.activeForm != nil {
			t.Fatalf("%s opened a popup", step.key.String())
		}
	}
}

func TestEventEditorCyclesListedChoices(t *testing.T) {
	tests := []struct {
		name    string
		row     string
		key     tea.KeyMsg
		prepare func(*eventFormState)
		value   func(*eventFormState) string
		want    string
	}{
		{
			name: "RSVP wraps backward", row: "rsvp", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}},
			value: func(s *eventFormState) string { return s.rsvp }, want: "needs-action",
		},
		{
			name: "availability wraps with left", row: "availability", key: tea.KeyMsg{Type: tea.KeyLeft},
			value: func(s *eventFormState) string { return s.availability }, want: "free",
		},
		{
			name: "visibility advances", row: "visibility", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}},
			prepare: func(s *eventFormState) { s.visibility = "default" },
			value:   func(s *eventFormState) string { return s.visibility }, want: "public",
		},
		{
			name: "repeat wraps with right", row: "recur", key: tea.KeyMsg{Type: tea.KeyRight},
			prepare: func(s *eventFormState) { s.recurFreq = "YEARLY" },
			value:   repeatValue, want: "none",
		},
		{
			name: "until advances", row: "recur-end", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}},
			value: func(s *eventFormState) string { return s.recurEnd }, want: "until",
		},
		{
			name: "monthly by advances", row: "recur-monthly-by", key: tea.KeyMsg{Type: tea.KeyRight},
			value: func(s *eventFormState) string { return s.recurMonthlyBy }, want: "weekday ordinal",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cal := calendar.Calendar{Source: "src", Name: "cal"}
			m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
			m.openEventFormNew()
			m.eventForm.recur = true
			m.eventForm.recurFreq = "MONTHLY"
			m.eventForm.recurEnd = "forever"
			m.eventForm.recurMonthlyBy = "month day"
			if test.prepare != nil {
				test.prepare(m.eventForm)
			}
			setEventEditorCursor(t, &m, test.row)

			updated, _ := m.Update(test.key)
			result := modelValue(t, updated)
			if got := test.value(result.eventForm); got != test.want {
				t.Fatalf("%s = %q, want %q", test.row, got, test.want)
			}
			if result.eventForm.activeForm != nil {
				t.Fatalf("%s opened a popup", test.row)
			}
		})
	}
}

func TestEventEditorTogglesAllDayWithoutPopup(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	setEventEditorCursor(t, &m, "all-day")

	for _, step := range []struct {
		key  tea.KeyMsg
		want bool
	}{
		{tea.KeyMsg{Type: tea.KeyRight}, true},
		{tea.KeyMsg{Type: tea.KeyLeft}, false},
		{tea.KeyMsg{Type: tea.KeyEnter}, true},
	} {
		updated, _ := m.Update(step.key)
		m = *modelValue(t, updated)
		if m.eventForm.allDay != step.want {
			t.Fatalf("%s set all-day=%v, want %v", step.key.String(), m.eventForm.allDay, step.want)
		}
		if m.eventForm.activeForm != nil {
			t.Fatalf("%s opened a popup", step.key.String())
		}
	}
	if !m.eventForm.timingDirty {
		t.Fatal("all-day changes did not mark timing dirty")
	}
}

func TestEventEditorCyclesFrequencyFromOneTo99(t *testing.T) {
	tests := []struct {
		value string
		key   rune
		want  string
	}{
		{"1", 'h', "99"},
		{"99", 'l', "1"},
		{"50", 'l', "51"},
		{"100", 'h', "99"},
		{"100", 'l', "1"},
	}
	for _, test := range tests {
		cal := calendar.Calendar{Source: "src", Name: "cal"}
		m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurEvery = test.value
		setEventEditorCursor(t, &m, "recur-every")

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{test.key}})
		result := modelValue(t, updated)
		if result.eventForm.recurEvery != test.want {
			t.Fatalf("%q %c = %q, want %q", test.value, test.key, result.eventForm.recurEvery, test.want)
		}
	}
}

func TestEventEditorCyclesRepeatCountFromOneTo99(t *testing.T) {
	tests := []struct {
		value string
		key   rune
		want  string
	}{
		{"1", 'h', "99"},
		{"99", 'l', "1"},
		{"100", 'h', "99"},
		{"100", 'l', "1"},
	}
	for _, test := range tests {
		cal := calendar.Calendar{Source: "src", Name: "cal"}
		m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurEnd = "count"
		m.eventForm.recurCount = test.value
		setEventEditorCursor(t, &m, "recur-count")

		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{test.key}})
		result := modelValue(t, updated)
		if result.eventForm.recurCount != test.want {
			t.Fatalf("%q %c = %q, want %q", test.value, test.key, result.eventForm.recurCount, test.want)
		}
	}
}

func TestTodoEditorCyclesCalendarAndPriority(t *testing.T) {
	calendars := []calendar.Calendar{
		{Source: "src", Name: "one"},
		{Source: "src", Name: "two"},
	}
	m := NewTaskModeModel(&config.Config{}, calendar.Dataset{Calendars: calendars}, nil)
	m.openTodoFormNew()
	setTodoEditorCursor(t, &m, "calendar")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = *modelValue(t, updated)
	if m.todoForm.calendarKey != calendarKey("src", "two") {
		t.Fatalf("calendar = %q, want second calendar", m.todoForm.calendarKey)
	}

	setTodoEditorCursor(t, &m, "priority")
	m.todoForm.priorityLabel = "high"
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = *modelValue(t, updated)
	if m.todoForm.priorityLabel != "low" {
		t.Fatalf("priority = %q, want low", m.todoForm.priorityLabel)
	}
	if m.todoForm.activeForm != nil {
		t.Fatal("direct todo cycling opened a popup")
	}
}

func TestTodoEditorTogglesCompletedWithoutPopup(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewTaskModeModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openTodoFormNew()
	setTodoEditorCursor(t, &m, "completed")

	for _, step := range []struct {
		key  tea.KeyMsg
		want bool
	}{
		{tea.KeyMsg{Type: tea.KeyRight}, true},
		{tea.KeyMsg{Type: tea.KeyLeft}, false},
		{tea.KeyMsg{Type: tea.KeyEnter}, true},
	} {
		updated, _ := m.Update(step.key)
		m = *modelValue(t, updated)
		if m.todoForm.completed != step.want {
			t.Fatalf("%s set completed=%v, want %v", step.key.String(), m.todoForm.completed, step.want)
		}
		if m.todoForm.activeForm != nil {
			t.Fatalf("%s opened a popup", step.key.String())
		}
	}
}

func TestEnterStillOpensNonBooleanChoicePopups(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	eventModel := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	eventModel.openEventFormNew()
	eventModel.eventForm.recur = true
	eventModel.eventForm.recurFreq = "MONTHLY"
	eventModel.eventForm.recurMonthlyBy = "month day"
	setEventEditorCursor(t, &eventModel, "recur-monthly-by")

	updated, _ := eventModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	eventResult := modelValue(t, updated)
	if eventResult.eventForm.activeKey != "recur-monthly-by" || eventResult.eventForm.choicePicker == nil {
		t.Fatal("Enter did not open the two-choice By selector")
	}

	todoModel := NewTaskModeModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	todoModel.openTodoFormNew()
	setTodoEditorCursor(t, &todoModel, "priority")
	updated, _ = todoModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	todoResult := modelValue(t, updated)
	if todoResult.todoForm.activeKey != "priority" || todoResult.todoForm.choicePicker == nil {
		t.Fatal("Enter did not open the Priority selector")
	}
}

func TestMultiSelectRowsIgnoreDirectCyclingAndStillOpen(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	m.eventForm.recur = true
	m.eventForm.recurFreq = "WEEKLY"
	m.eventForm.recurWeekdays = []string{"Mo"}
	setEventEditorCursor(t, &m, "recur-weekdays")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = *modelValue(t, updated)
	if strings.Join(m.eventForm.recurWeekdays, ",") != "Mo" || m.eventForm.activeForm != nil {
		t.Fatal("direct cycling changed or opened the weekday multiselect")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, updated)
	if m.eventForm.activeKey != "recur-weekdays" || m.eventForm.choicePicker == nil {
		t.Fatal("Enter did not open the weekday multiselect")
	}
}

func TestFrequencyPopupAcceptsOnlyNormalizedValuesFromOneTo99(t *testing.T) {
	t.Run("filters non-digits and normalizes leading zeroes", func(t *testing.T) {
		cal := calendar.Calendar{Source: "src", Name: "cal"}
		m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurEvery = ""
		setEventEditorCursor(t, &m, "recur-every")
		m.openEventEditorForm()

		var model tea.Model = &m
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a0x1")})
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
		m = *modelValue(t, model)
		if m.eventForm.activeForm != nil || m.eventForm.recurEvery != "1" {
			t.Fatalf("frequency popup remained open=%v value=%q", m.eventForm.activeForm != nil, m.eventForm.recurEvery)
		}
	})

	t.Run("rejects values above 99", func(t *testing.T) {
		cal := calendar.Calendar{Source: "src", Name: "cal"}
		m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurEvery = "100"
		setEventEditorCursor(t, &m, "recur-every")
		m.openEventEditorForm()

		var model tea.Model = &m
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
		m = *modelValue(t, model)
		if m.eventForm.activeForm == nil {
			t.Fatal("frequency 100 was accepted")
		}
	})
}

func TestRepeatCountPopupAcceptsOnlyNormalizedValuesFromOneTo99(t *testing.T) {
	t.Run("filters non-digits and normalizes leading zeroes", func(t *testing.T) {
		cal := calendar.Calendar{Source: "src", Name: "cal"}
		m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurEnd = "count"
		m.eventForm.recurCount = ""
		setEventEditorCursor(t, &m, "recur-count")
		m.openEventEditorForm()

		var model tea.Model = &m
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a0x2")})
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
		m = *modelValue(t, model)
		if m.eventForm.activeForm != nil || m.eventForm.recurCount != "2" {
			t.Fatalf("count popup remained open=%v value=%q", m.eventForm.activeForm != nil, m.eventForm.recurCount)
		}
	})

	t.Run("rejects values above 99", func(t *testing.T) {
		cal := calendar.Calendar{Source: "src", Name: "cal"}
		m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurEnd = "count"
		m.eventForm.recurCount = "100"
		setEventEditorCursor(t, &m, "recur-count")
		m.openEventEditorForm()

		var model tea.Model = &m
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
		m = *modelValue(t, model)
		if m.eventForm.activeForm == nil {
			t.Fatal("repeat count 100 was accepted")
		}
	})
}

func setEventEditorCursor(t *testing.T, m *Model, key string) {
	t.Helper()
	for i, row := range m.eventEditorRows() {
		if editorRowKey(row) == key {
			m.eventForm.cursor = i
			return
		}
	}
	t.Fatalf("event editor row %q not found", key)
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
