package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

func TestMouseClickSelectsRenderedCalendarDay(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.width = 100
	m.height = 40
	m.selected = time.Date(2026, time.October, 5, 0, 0, 0, 0, time.Local)
	m.agendaStart = dayStart(m.selected)
	m.weekViewportStart = m.selected
	_ = m.View()

	updated, _ := m.Update(tea.MouseMsg{
		X:      6,
		Y:      5,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	m = *modelValue(t, updated)

	if got := m.selected.Format("2006-01-02"); got != "2026-10-06" {
		t.Fatalf("clicked date = %s, want 2026-10-06", got)
	}
}

func TestMouseCalendarHeaderNavigatesMonthsAndYears(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.width = 100
	m.height = 40
	m.selected = time.Date(2024, time.February, 29, 0, 0, 0, 0, time.Local)
	m.agendaStart = m.selected
	if view := m.View(); !strings.Contains(view, "Calendar  « ‹ › »") {
		t.Fatalf("calendar header navigation controls missing:\n%s", view)
	}

	for _, step := range []struct {
		kind mouseTarget
		want string
	}{
		{mouseCalendarNextYear, "2025-02-28"},
		{mouseCalendarPreviousMonth, "2025-01-28"},
		{mouseCalendarNextMonth, "2025-02-28"},
		{mouseCalendarPreviousYear, "2024-02-28"},
	} {
		m = clickRenderedHit(t, m, step.kind, -1, "")
		if got := m.selected.Format("2006-01-02"); got != step.want {
			t.Fatalf("mouse target %d date = %s, want %s", step.kind, got, step.want)
		}
	}
}

func TestMouseWheelAndUnsupportedActions(t *testing.T) {
	data := calendar.Dataset{Calendars: []calendar.Calendar{{Source: "src", Name: "a"}, {Source: "src", Name: "b"}}}
	m := NewModel(&config.Config{SidebarWidth: 30}, data, nil)
	m.focusCalendarPane = true

	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m = *modelValue(t, updated)
	if m.calendarCursor != 1 {
		t.Fatalf("wheel down selected calendar %d, want 1", m.calendarCursor)
	}

	updated, _ = m.Update(tea.MouseMsg{X: 3, Y: 3, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	m = *modelValue(t, updated)
	if m.calendarCursor != 1 {
		t.Fatalf("mouse motion changed calendar cursor to %d", m.calendarCursor)
	}
}

func TestMouseClicksCalendarAndAgendaRows(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	event := calendar.Event{
		UID: "event", Summary: "Event", Source: "src", Calendar: "cal",
		Start: now.Add(time.Hour), End: now.Add(2 * time.Hour),
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{cal},
		Events:    []calendar.Event{event},
	}, nil)
	m.width = 120
	m.height = 40
	key := calendarKey(cal.Source, cal.Name)

	m = clickRenderedHit(t, m, mouseCalendarRow, 0, "")
	if m.calendarVisibility[key] {
		t.Fatal("calendar row click did not hide the calendar")
	}
	m = clickRenderedHit(t, m, mouseCalendarRow, 0, "")
	if !m.calendarVisibility[key] {
		t.Fatal("second calendar row click did not show the calendar")
	}

	m = clickRenderedHit(t, m, mouseAgendaItem, -1, "")
	if m.eventForm == nil || m.eventForm.mode != "edit" {
		t.Fatal("agenda row click did not open the event editor")
	}
}

func TestMouseStagesChoiceUntilApply(t *testing.T) {
	calendars := []calendar.Calendar{{Source: "src", Name: "a"}, {Source: "src", Name: "b"}}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: calendars}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	rows := m.eventEditorRows()
	calendarRow := editorRowIndex(t, rows, "calendar")
	before := m.eventForm.calendarKey

	m = clickRenderedHit(t, m, mouseEventEditorRow, calendarRow, "")
	if m.eventForm.choicePicker == nil {
		t.Fatal("calendar row click did not open the choice picker")
	}
	choice := 0
	if m.eventForm.choicePicker.choices[choice].value == before {
		choice = 1
	}
	want := m.eventForm.choicePicker.choices[choice].value
	m = clickRenderedHit(t, m, mouseChoiceOption, choice, "")
	if m.eventForm.calendarKey != before {
		t.Fatal("single-choice click committed before Apply")
	}
	m = clickRenderedHit(t, m, mouseDialogApply, -1, "")
	if m.eventForm.calendarKey != want || m.eventForm.choicePicker != nil {
		t.Fatalf("Apply set calendar %q with picker open=%v, want %q", m.eventForm.calendarKey, m.eventForm.choicePicker != nil, want)
	}
}

func TestMouseCyclesAttendeesAndNotificationsBeforeApply(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{{Source: "src", Name: "cal"}},
	}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	m.eventForm.attendees = "person@example.com"
	m.eventForm.alarms = "10m before"

	m.eventForm.cursor = editorRowIndex(t, m.eventEditorRows(), "attendees")
	m.openEventEditorForm()
	m = clickRenderedHit(t, m, mouseAttendeeRow, 0, "")
	if !attendeeIsOptional(m.eventForm.attendeeManager.attendees[0].attendee) {
		t.Fatal("first attendee click did not make the attendee optional")
	}
	m = clickRenderedHit(t, m, mouseAttendeeRow, 0, "")
	if !m.eventForm.attendeeManager.attendees[0].remove {
		t.Fatal("second attendee click did not mark the attendee deleted")
	}
	m = clickRenderedHit(t, m, mouseAttendeeRow, 0, "")
	item := m.eventForm.attendeeManager.attendees[0]
	if item.remove || attendeeIsOptional(item.attendee) {
		t.Fatal("third attendee click did not restore the required attendee")
	}
	m = clickRenderedHit(t, m, mouseDialogApply, -1, "")

	m.eventForm.cursor = editorRowIndex(t, m.eventEditorRows(), "alarms")
	m.openEventEditorForm()
	m = clickRenderedHit(t, m, mouseNotificationRow, 0, "")
	if !m.eventForm.notificationManager.items[0].remove || m.eventForm.alarms != "10m before" {
		t.Fatal("notification click did not stage deletion")
	}
	m = clickRenderedHit(t, m, mouseDialogApply, -1, "")
	if m.eventForm.alarms != "" || m.eventForm.notificationManager != nil {
		t.Fatal("notification Apply did not commit deletion")
	}
}
func TestAttendeeDialogVisibleButtonsWorkWithPopulatedList(t *testing.T) {
	newDialog := func() Model {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
			Calendars: []calendar.Calendar{{Source: "src", Name: "cal"}},
		}, nil)
		m.width = 90
		m.height = 40
		m.openEventFormNew()
		m.eventForm.attendees = "Ada Lovelace <ada@example.test>"
		attendeesRow := editorRowIndex(t, m.eventEditorRows(), "attendees")
		m.eventForm.cursor = attendeesRow
		m = clickRenderedHit(t, m, mouseEventEditorRow, attendeesRow, "")
		return m
	}

	applied := clickVisibleDialogButton(t, newDialog(), "Apply")
	if applied.eventForm.attendeeManager != nil {
		t.Fatal("visible Apply button did not close the populated attendee dialog")
	}
	if !strings.Contains(applied.eventForm.attendees, "Ada Lovelace") {
		t.Fatalf("visible Apply button lost attendee: %q", applied.eventForm.attendees)
	}

	cancelled := clickVisibleDialogButton(t, newDialog(), "Cancel")
	if cancelled.eventForm.attendeeManager != nil {
		t.Fatal("visible Cancel button did not close the populated attendee dialog")
	}
}

func TestMouseDateRangeAndTimeDigitSelection(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{{Source: "src", Name: "cal"}},
	}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	m.eventForm.fromDate = "2026-10-01"
	m.eventForm.toDate = "2026-10-01"

	m.eventForm.cursor = editorRowIndex(t, m.eventEditorRows(), "date")
	m.openEventEditorForm()
	m = clickRenderedHit(t, m, mouseDateNextYear, -1, "")
	if got := m.eventForm.datePicker.cursor.Format("2006-01-02"); got != "2027-10-01" {
		t.Fatalf("next-year picker click date = %s", got)
	}
	m = clickRenderedHit(t, m, mouseDatePreviousYear, -1, "")
	m = clickRenderedHit(t, m, mouseDateNextMonth, -1, "")
	m = clickRenderedHit(t, m, mouseDatePreviousMonth, -1, "")
	if got := m.eventForm.datePicker.cursor.Format("2006-01-02"); got != "2026-10-01" {
		t.Fatalf("round-trip picker click date = %s", got)
	}
	m = clickRenderedHit(t, m, mouseDateMultiDay, -1, "")
	m = clickRenderedDate(t, m, 5)
	if gotStart, gotEnd := m.eventForm.datePicker.dates(); gotStart.Day() != 1 || gotEnd.Day() != 5 {
		t.Fatalf("first date click selected %s -> %s, want existing Oct 1 -> Oct 5", gotStart, gotEnd)
	}
	m = clickRenderedDate(t, m, 6)
	if m.eventForm.datePicker.start.Day() != 6 || m.eventForm.datePicker.end != nil {
		t.Fatal("second date click did not begin a new range")
	}
	m = clickRenderedDate(t, m, 7)
	if gotStart, gotEnd := m.eventForm.datePicker.dates(); gotStart.Day() != 6 || gotEnd.Day() != 7 {
		t.Fatalf("third date click selected %s -> %s, want Oct 6 -> Oct 7", gotStart, gotEnd)
	}
	m.eventForm.cancelActive()

	m.eventForm.cursor = editorRowIndex(t, m.eventEditorRows(), "time")
	m.openEventEditorForm()
	m = clickRenderedHit(t, m, mouseTimeDigit, 7, "")
	if m.eventForm.timeEditor.cursor != 7 {
		t.Fatalf("time digit click focused %d, want 7", m.eventForm.timeEditor.cursor)
	}
}

func TestFieldDialogButtonsAndOutsideClicks(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{{Source: "src", Name: "cal"}},
	}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	m.eventForm.cursor = editorRowIndex(t, m.eventEditorRows(), "title")
	m.openEventEditorForm()
	view := m.View()
	for _, obsolete := range []string{"submit", "[enter] ok", "[enter] select"} {
		if strings.Contains(strings.ToLower(view), obsolete) {
			t.Fatalf("field form still renders obsolete action %q:\n%s", obsolete, view)
		}
	}

	updated, _ := m.Update(tea.MouseMsg{X: 3, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	m = *modelValue(t, updated)
	if m.eventForm.activeForm == nil {
		t.Fatal("click outside the active field form escaped to the calendar")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = *modelValue(t, updated)
	if m.eventForm.dialogFocus != dialogFocusApply {
		t.Fatalf("Tab focused %d, want Apply", m.eventForm.dialogFocus)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = *modelValue(t, updated)
	if m.eventForm.dialogFocus != dialogFocusCancel {
		t.Fatalf("second Tab focused %d, want Cancel", m.eventForm.dialogFocus)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = *modelValue(t, updated)
	if m.eventForm.dialogFocus != dialogFocusControl {
		t.Fatalf("third Tab focused %d, want control", m.eventForm.dialogFocus)
	}

	m = clickRenderedHit(t, m, mouseDialogCancel, -1, "")
	if m.eventForm.activeForm != nil {
		t.Fatal("Cancel button did not close the field form")
	}
}

func TestOuterActionsRenderHorizontallyAndNavigate(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{{Source: "src", Name: "cal"}},
	}, nil)
	m.width = 120
	m.height = 50
	m.openEventFormNew()
	rows := m.eventEditorRows()
	saveRow := editorRowIndex(t, rows, "form-save")
	cancelRow := editorRowIndex(t, rows, "form-cancel")
	_ = m.View()
	save := renderedHit(t, m, mouseEventEditorRow, saveRow, "")
	cancel := renderedHit(t, m, mouseEventEditorRow, cancelRow, "")
	if save.rect.y != cancel.rect.y || save.rect.x >= cancel.rect.x {
		t.Fatalf("outer actions not horizontal Save then Cancel: save=%+v cancel=%+v", save.rect, cancel.rect)
	}

	m.eventForm.cursor = saveRow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = *modelValue(t, updated)
	if m.eventForm.cursor != cancelRow {
		t.Fatal("Right did not move from Save to Cancel")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = *modelValue(t, updated)
	if m.eventForm.cursor != saveRow {
		t.Fatal("Left did not move from Cancel to Save")
	}
}

func TestMouseRecurrenceScopeIsImmediate(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{{Source: "src", Name: "cal"}},
	}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	m.eventForm.editScope = string(calendar.EditRecurringOccurrence)
	m.openEventEditScopeForm()

	m = clickRenderedHit(t, m, mouseEditScope, -1, string(calendar.EditRecurringFuture))
	if m.eventForm.editScope != string(calendar.EditRecurringFuture) || m.eventForm.activeForm != nil {
		t.Fatal("recurrence scope click did not apply immediately")
	}
}

func editorRowIndex(t *testing.T, rows []editorRow, key string) int {
	t.Helper()
	for i, row := range rows {
		if editorRowKey(row) == key {
			return i
		}
	}
	t.Fatalf("editor row %q not found", key)
	return -1
}

func clickRenderedHit(t *testing.T, m Model, kind mouseTarget, index int, value string) Model {
	t.Helper()
	_ = m.View()
	hit := renderedHit(t, m, kind, index, value)
	updated, _ := m.Update(tea.MouseMsg{
		X:      hit.rect.x,
		Y:      hit.rect.y,
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
	})
	return *modelValue(t, updated)
}

func clickVisibleDialogButton(t *testing.T, m Model, label string) Model {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for y, line := range lines {
		if strings.Contains(line, "[enter]") || !strings.Contains(line, "Apply") || !strings.Contains(line, "Cancel") {
			continue
		}
		at := strings.Index(line, label)
		if at < 0 {
			break
		}
		x := lipgloss.Width(line[:at]) + len(label)/2
		wantKind := mouseDialogApply
		if label == "Cancel" {
			wantKind = mouseDialogCancel
		}
		hit, ok := m.mouse.at(x, y)
		if !ok || hit.kind != wantKind {
			actions := make([]mouseHit, 0, 2)
			for _, candidate := range m.mouse.hits {
				if candidate.kind == mouseDialogApply || candidate.kind == mouseDialogCancel {
					actions = append(actions, candidate)
				}
			}
			t.Fatalf("visible %s at (%d,%d) resolves to %+v, action hits: %+v", label, x, y, hit, actions)
		}
		updated, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		return *modelValue(t, updated)
	}
	t.Fatalf("visible dialog button %q not found", label)
	return m
}

func clickRenderedDate(t *testing.T, m Model, day int) Model {
	t.Helper()
	_ = m.View()
	for _, hit := range m.mouse.hits {
		if hit.kind == mouseDateDay && hit.day.Day() == day {
			updated, _ := m.Update(tea.MouseMsg{X: hit.rect.x, Y: hit.rect.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			return *modelValue(t, updated)
		}
	}
	t.Fatalf("rendered date %d not found", day)
	return m
}

func renderedHit(t *testing.T, m Model, kind mouseTarget, index int, value string) mouseHit {
	t.Helper()
	for _, hit := range m.mouse.hits {
		if hit.kind == kind && (index < 0 || hit.index == index) && (value == "" || hit.value == value) {
			return hit
		}
	}
	t.Fatalf("rendered mouse target kind=%d index=%d value=%q not found", kind, index, value)
	return mouseHit{}
}
