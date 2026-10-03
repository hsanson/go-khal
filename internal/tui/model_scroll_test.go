package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

func TestAttendeesInputPreservesOptionalRole(t *testing.T) {
	attendees := []calendar.Attendee{
		{Name: "Ada Lovelace", Email: "ada@example.test"},
		{Name: "Grace Hopper", Email: "grace@example.test", Role: "optional"},
	}
	raw := attendeesInput(attendees)
	if raw != "Ada Lovelace <ada@example.test>; Grace Hopper <grace@example.test> [optional]" {
		t.Fatalf("unexpected attendees input: %q", raw)
	}
	parsed := parseAttendeesInput(raw)
	if !reflect.DeepEqual(parsed, attendees) {
		t.Fatalf("parsed attendees mismatch:\n got: %#v\nwant: %#v", parsed, attendees)
	}
}

func TestEventResponseDisplaysAvoidDefaultLabel(t *testing.T) {
	if got := eventRSVPDisplayValue("needs-action"); got != "no response" {
		t.Fatalf("needs-action RSVP display = %q", got)
	}
	if got := eventRSVPDisplayValue(""); got != "-" {
		t.Fatalf("empty RSVP display = %q", got)
	}
	if got := eventAvailabilityDisplay(""); got != "calendar default" {
		t.Fatalf("empty availability display = %q", got)
	}
	if got := eventVisibilityDisplay(""); got != "calendar default" {
		t.Fatalf("empty visibility display = %q", got)
	}
}

func TestEventDetailsShowRSVP(t *testing.T) {
	m := NewModel(&config.Config{}, calendar.Dataset{}, nil)
	ev := calendar.Event{Summary: "Test2", UserRSVP: "yes"}

	got := m.renderEventDetailsFor(ev, 80, 20)
	if !strings.Contains(got, "RSVP") || !strings.Contains(got, "yes") {
		t.Fatalf("event details do not show RSVP:\n%s", got)
	}
}

func TestEventDetailsDoNotShowDescription(t *testing.T) {
	m := NewModel(&config.Config{}, calendar.Dataset{}, nil)
	ev := calendar.Event{Summary: "Test", Description: "private notes"}

	got := m.renderEventDetailsFor(ev, 80, 20)
	if strings.Contains(got, "Description") || strings.Contains(got, "private notes") {
		t.Fatalf("event details unexpectedly show description:\n%s", got)
	}
}

func TestVOpensReadOnlyEventView(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "local", Name: "personal"}
	ev := calendar.Event{
		UID: "event", Summary: "Event", Source: cal.Source, Calendar: cal.Name,
		Start: now, End: now.Add(time.Hour),
	}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{ev}}, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(Model)
	if m.eventForm == nil || m.eventForm.mode != "view" {
		t.Fatalf("v did not open the read-only event view: %#v", m.eventForm)
	}
	for _, row := range m.eventEditorRows() {
		if row.key == "attendees-add" || row.key == "alarms-add" {
			t.Fatalf("read-only view contains add button %q", row.key)
		}
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = *updated.(*Model)
	if m.eventForm.activeForm != nil {
		t.Fatal("Enter made a field editable in read-only view")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = *updated.(*Model)
	if m.eventForm == nil || m.eventForm.mode != "edit" {
		t.Fatal("e did not open the Edit Event form")
	}
}

func TestReadOnlyTaskViewReturnsToListWithQ(t *testing.T) {
	now := time.Now()
	due := now.Add(time.Hour)
	cal := calendar.Calendar{Source: "local", Name: "personal"}
	todo := calendar.Todo{
		UID: "task", Summary: "Task", Source: cal.Source, Calendar: cal.Name,
		Start: &now, Due: &due,
	}
	m := NewTaskModeModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(Model)
	if m.todoForm == nil || m.todoForm.mode != "view" {
		t.Fatal("v did not open the read-only task view")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = *updated.(*Model)
	if m.todoForm != nil || !m.focusMain {
		t.Fatal("q did not return from task view to the task list")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = *updated.(*Model)
	if m.todoForm == nil || m.todoForm.mode != "edit" {
		t.Fatal("e did not open the Edit Task form")
	}
}

func TestRecurringDeleteUsesScopeThenConfirmation(t *testing.T) {
	m := NewModel(&config.Config{}, calendar.Dataset{}, nil)
	state := &deleteConfirmState{kind: "event", recurring: true, stage: "scope", scope: string(calendar.DeleteRecurringOccurrence)}
	state.form = m.buildDeleteConfirmForm(state).WithKeyMap(deleteConfirmFormKeyMap(state))
	m.deleteConfirm = state
	_ = state.form.Init()

	if got := state.form.GetFocusedField().GetKey(); got != "scope" {
		t.Fatalf("first delete field = %q, want scope", got)
	}

	var model tea.Model = &m
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
	m = *modelValue(t, model)
	state = m.deleteConfirm
	if state.stage != "confirm" {
		t.Fatalf("delete stage = %q, want confirm", state.stage)
	}
	if got := state.form.GetFocusedField().GetKey(); got != "confirm" {
		t.Fatalf("second delete field = %q, want confirm", got)
	}
}

func TestEventRSVPUsesCalendarOwnerInsteadOfCombiningAttendees(t *testing.T) {
	ev := calendar.Event{
		UserRSVP: "no",
		Attendees: []calendar.Attendee{
			{Email: "user@example.test", Status: "no"},
			{Email: "other@example.test", Status: "yes"},
		},
	}

	if got := eventRSVPValue(ev); got != "no" {
		t.Fatalf("event RSVP = %q, want no", got)
	}
	if !eventRSVPIsNo(ev) {
		t.Fatal("event should be recognized as declined")
	}
}

func TestPreserveAttendeeRSVPKeepsExistingResponse(t *testing.T) {
	attendees := []calendar.Attendee{
		{Name: "Guest", Email: "guest@example.test"},
		{Name: "New", Email: "new@example.test"},
	}
	existing := []calendar.Attendee{
		{Name: "Guest", Email: "guest@example.test", Status: "yes", RSVP: true},
	}

	preserveAttendeeRSVP(attendees, existing)

	if attendees[0].Status != "yes" || !attendees[0].RSVP {
		t.Fatalf("expected existing attendee RSVP to be preserved, got %+v", attendees[0])
	}
	if attendees[1].Status != "" || attendees[1].RSVP {
		t.Fatalf("expected new attendee RSVP to remain unset, got %+v", attendees[1])
	}
}

func TestCalendarKeyRoundTripsAbsoluteSourcePath(t *testing.T) {
	key := calendarKey("/home/user/.calendars/personal", "personal")
	parts := splitCalendarKey(key)

	if parts.source != "/home/user/.calendars/personal" {
		t.Fatalf("source = %q", parts.source)
	}
	if parts.name != "personal" {
		t.Fatalf("name = %q", parts.name)
	}
}

func TestEventFormKeepsAbsolutePathCalendarKey(t *testing.T) {
	source := "/home/user/.calendars/personal"
	cal := calendar.Calendar{Source: source, Name: "personal"}
	ev := calendar.Event{
		UID:      "event",
		Summary:  "Event",
		Source:   source,
		Calendar: "personal",
		Start:    time.Date(2026, time.July, 7, 12, 0, 0, 0, time.Local),
		End:      time.Date(2026, time.July, 7, 13, 0, 0, 0, time.Local),
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{ev}}, nil)

	form := m.newEventFormState("edit", ev.UID, ev)
	parts := splitCalendarKey(form.calendarKey)

	if parts.source != source {
		t.Fatalf("source = %q", parts.source)
	}
	if parts.name != "personal" {
		t.Fatalf("name = %q", parts.name)
	}
}

func TestWeekViewportScrollsDown(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.height = 30
	initial := m.weekViewportStart
	initialSelected := m.selected
	model := tea.Model(m)
	for i := 0; i < 80; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}

	updated := model.(Model)
	if !updated.selected.After(initialSelected) {
		t.Fatalf("expected selected day to move, initial=%v updated=%v", initialSelected, updated.selected)
	}
	if !updated.weekViewportStart.After(initial) {
		t.Fatalf("expected viewport start to move down, initial=%v updated=%v", initial, updated.weekViewportStart)
	}
}

func TestWeekViewportScrollsUp(t *testing.T) {
	now := time.Now()
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.height = 30
	m.selected = now.AddDate(0, 0, 140)
	m.weekViewportStart = now.AddDate(0, 0, 84)

	model := tea.Model(m)
	for i := 0; i < 20; i++ {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	}

	updated := model.(Model)
	if !updated.weekViewportStart.Before(m.weekViewportStart) {
		t.Fatalf("expected viewport start to move up, initial=%v updated=%v", m.weekViewportStart, updated.weekViewportStart)
	}
}

func TestAgendaCursorMovesBackwardPastWindowStart(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.selected = start
	m.agendaStart = start
	m.eventCursor = 0
	m.eventListOffset = 0

	m.moveEventCursor(-1)

	if !m.agendaStart.Equal(start.AddDate(0, 0, -1)) {
		t.Fatalf("expected agenda start to move back one day, got %v", m.agendaStart)
	}
	if !m.selected.Equal(start.AddDate(0, 0, -1)) {
		t.Fatalf("expected selected day to move back one day, got %v", m.selected)
	}
	if m.eventCursor != 0 {
		t.Fatalf("expected cursor to land on prepended first item, got %d", m.eventCursor)
	}
}

func TestAgendaCursorPageBackExtendsWindow(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.width = 120
	m.height = 40
	m.selected = start
	m.agendaStart = start
	m.eventCursor = 0
	m.eventListOffset = 0
	step := m.eventPageStep()

	m.moveEventCursor(-step)

	want := start.AddDate(0, 0, -step)
	if !m.agendaStart.Equal(want) {
		t.Fatalf("expected agenda start to move back %d days, got %v", step, m.agendaStart)
	}
	if !m.selected.Equal(want) {
		t.Fatalf("expected selected day to move back %d days, got %v", step, m.selected)
	}
	if m.eventCursor != 0 {
		t.Fatalf("expected cursor to land on prepended first page item, got %d", m.eventCursor)
	}
}

func TestMoveEventCursorResetsDetailScroll(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	events := []calendar.Event{
		{UID: "one", Summary: "One", Start: start.Add(9 * time.Hour), End: start.Add(10 * time.Hour)},
		{UID: "two", Summary: "Two", Start: start.Add(11 * time.Hour), End: start.Add(12 * time.Hour)},
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Events: events}, nil)
	m.selected = start
	m.agendaStart = start
	m.eventCursor = 0
	m.detailScroll = 4

	m.moveEventCursor(1)

	if m.detailScroll != 0 {
		t.Fatalf("expected detail scroll reset after changing event, got %d", m.detailScroll)
	}
}

func TestNewEventDefaultsToSelectedAgendaItemTime(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	events := []calendar.Event{
		{UID: "one", Summary: "One", Source: "src", Calendar: "cal", Start: start.Add(11 * time.Hour), End: start.Add(12 * time.Hour)},
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: events}, nil)
	m.selected = start
	m.agendaStart = start
	m.eventCursor = 0

	m.openEventFormNew()

	if m.eventForm.fromDate != "2026-07-03" || m.eventForm.fromTime != "11:00" {
		t.Fatalf("unexpected event start default: %s %s", m.eventForm.fromDate, m.eventForm.fromTime)
	}
	if m.eventForm.toDate != "2026-07-03" || m.eventForm.toTime != "12:00" {
		t.Fatalf("unexpected event end default: %s %s", m.eventForm.toDate, m.eventForm.toTime)
	}
}

func TestNewTaskScheduleDefaultsEmpty(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)

	m.openTodoFormNew()

	if m.todoForm.startDate != "" || m.todoForm.startTime != "" || m.todoForm.dueDate != "" || m.todoForm.dueTime != "" {
		t.Fatalf("new task schedule is not empty: %+v", m.todoForm)
	}
}

func TestAgendaShowsDeclinedEventsOnlyInShowAllMode(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	declined := calendar.Event{
		UID:      "declined",
		Summary:  "Declined",
		Source:   "src",
		Calendar: "cal",
		Start:    start.Add(11 * time.Hour),
		End:      start.Add(12 * time.Hour),
		Attendees: []calendar.Attendee{
			{Email: "user@example.test", Status: "no"},
		},
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{declined}}, nil)
	m.selected = start
	m.agendaStart = start

	for _, item := range m.agendaItems() {
		if item.Event != nil && item.Event.UID == "declined" {
			t.Fatal("declined event should be hidden by default")
		}
	}

	m.showAllMode = true
	found := false
	for _, item := range m.agendaItems() {
		if item.Event != nil && item.Event.UID == "declined" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("declined event should be visible in show-all mode")
	}
}

func TestAgendaModeDoesNotShowTasks(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todos := []calendar.Todo{
		{UID: "task", Summary: "Task", Source: "src", Calendar: "cal", Start: &start, Priority: 1},
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: todos}, nil)
	m.selected = start
	m.agendaStart = start

	for _, item := range m.agendaItems() {
		if item.Todo != nil {
			t.Fatalf("agenda mode should not show task %q", item.Todo.UID)
		}
	}

	m.showAllMode = true
	for _, item := range m.agendaItems() {
		if item.Todo != nil {
			t.Fatalf("agenda show-all should not show task %q", item.Todo.UID)
		}
	}
}

func TestTaskModeShowsPastDuePendingTasksAndClampsNavigation(t *testing.T) {
	now := dayStart(time.Now())
	past := now.AddDate(0, 0, -14)
	pastStart := now.AddDate(0, 0, 1)
	future := now.AddDate(0, 0, 2)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todos := []calendar.Todo{
		{UID: "future", Summary: "Future", Source: "src", Calendar: "cal", Start: &future, Priority: 5},
		{UID: "past", Summary: "Past due", Source: "src", Calendar: "cal", Start: &pastStart, Due: &past, Priority: 1},
	}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: todos}, nil)

	items := m.agendaItems()
	if len(items) != 2 {
		t.Fatalf("expected both pending tasks in task mode, got %d", len(items))
	}
	if items[0].Todo == nil || items[0].Todo.UID != "past" {
		t.Fatalf("expected past due task first, got %#v", items[0].Todo)
	}
	if !items[0].Day.Equal(dayStart(past)) {
		t.Fatalf("expected past due task to be anchored on due date %v, got %v", dayStart(past), items[0].Day)
	}
	if m.eventCursor != 0 {
		t.Fatalf("expected task cursor at top, got %d", m.eventCursor)
	}

	m.moveEventCursor(-1)
	if m.eventCursor != 0 {
		t.Fatalf("expected task cursor to clamp at top, got %d", m.eventCursor)
	}

	m.moveEventCursor(20)
	if m.eventCursor != len(items)-1 {
		t.Fatalf("expected task cursor to clamp at bottom, got %d", m.eventCursor)
	}
}

func TestAgendaDateNavigationClampsAndResetsList(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.selected = time.Date(2024, time.February, 29, 0, 0, 0, 0, time.Local)
	m.agendaStart = m.selected
	m.eventCursor = 7
	m.eventListOffset = 4

	for _, step := range []struct {
		key  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyCtrlH}, "2023-02-28"},
		{tea.KeyMsg{Type: tea.KeyCtrlL}, "2024-02-28"},
		{tea.KeyMsg{Type: tea.KeyCtrlK}, "2024-01-28"},
		{tea.KeyMsg{Type: tea.KeyCtrlJ}, "2024-02-28"},
	} {
		updated, _ := m.Update(step.key)
		m = updated.(Model)
		if got := m.selected.Format("2006-01-02"); got != step.want {
			t.Fatalf("%s date = %s, want %s", step.key.String(), got, step.want)
		}
		if m.eventCursor != 0 || m.eventListOffset != 0 {
			t.Fatalf("%s list position = %d/%d, want 0/0", step.key.String(), m.eventCursor, m.eventListOffset)
		}
	}
}

func TestTaskDateNavigationPreservesListPosition(t *testing.T) {
	first := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.Local)
	second := first.AddDate(0, 0, 1)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{cal},
		Todos: []calendar.Todo{
			{UID: "one", Summary: "One", Source: cal.Source, Calendar: cal.Name, Due: &first},
			{UID: "two", Summary: "Two", Source: cal.Source, Calendar: cal.Name, Due: &second},
		},
	}, nil)
	m.selected = time.Date(2025, time.January, 31, 0, 0, 0, 0, time.Local)
	m.agendaStart = m.selected
	m.eventCursor = 1

	for _, step := range []struct {
		key  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, "2025-01-30"},
		{tea.KeyMsg{Type: tea.KeyRight}, "2025-01-31"},
		{tea.KeyMsg{Type: tea.KeyCtrlJ}, "2025-02-28"},
		{tea.KeyMsg{Type: tea.KeyCtrlK}, "2025-01-28"},
		{tea.KeyMsg{Type: tea.KeyCtrlL}, "2026-01-28"},
		{tea.KeyMsg{Type: tea.KeyCtrlH}, "2025-01-28"},
	} {
		updated, _ := m.Update(step.key)
		m = updated.(Model)
		if got := m.selected.Format("2006-01-02"); got != step.want {
			t.Fatalf("%s date = %s, want %s", step.key.String(), got, step.want)
		}
		if m.eventCursor != 1 {
			t.Fatalf("%s task cursor = %d, want 1", step.key.String(), m.eventCursor)
		}
	}
}

func TestTaskModeShowAllControlsCompletedTasks(t *testing.T) {
	now := dayStart(time.Now())
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todos := []calendar.Todo{
		{UID: "pending", Summary: "Pending", Source: "src", Calendar: "cal", Start: &now, Status: "NEEDS-ACTION"},
		{UID: "done", Summary: "Done", Source: "src", Calendar: "cal", Start: &now, Status: "COMPLETED"},
	}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: todos}, nil)

	if got := len(m.agendaItems()); got != 1 {
		t.Fatalf("expected only pending tasks by default, got %d", got)
	}
	m.showAllMode = true
	if got := len(m.agendaItems()); got != 2 {
		t.Fatalf("expected completed tasks in task show-all mode, got %d", got)
	}
}

func TestCalendarPaneQAndEscReturnToMainList(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	for _, key := range []string{"q", "esc"} {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.focusCalendarPane = true

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		if key == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		updated, cmd := m.Update(msg)
		next := updated.(Model)

		if cmd != nil {
			t.Fatalf("%s in calendar pane should not quit or run a command", key)
		}
		if next.focusCalendarPane {
			t.Fatalf("%s should close calendar pane focus", key)
		}
		if !next.focusMain {
			t.Fatalf("%s should return focus to main list", key)
		}
	}
}
func TestEnterOpensSelectedEventEditor(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	event := calendar.Event{UID: "event", Summary: "Event", Source: "src", Calendar: "cal", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{event}}, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).eventForm == nil || updated.(Model).eventForm.mode != "edit" {
		t.Fatal("enter should open selected event editor")
	}
}

func TestEnterOpensSelectedTaskEditor(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todo := calendar.Todo{UID: "task", Summary: "Task", Source: "src", Calendar: "cal", Due: &now}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).todoForm == nil || updated.(Model).todoForm.mode != "edit" {
		t.Fatal("enter should open selected task editor")
	}
}

func TestCalendarPaneQReturnsToItemList(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)

	opened, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	closed, cmd := opened.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("q in calendar pane should not quit")
	}
	if closed.(Model).focusCalendarPane {
		t.Fatal("q should return focus to item list")
	}
}

func TestLegendRendersBelowMainView(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.width = 120
	m.height = 30
	view := m.View()
	legendAt := strings.Index(view, "[esc/q] Exit")
	agendaAt := strings.Index(view, "Agenda from")
	if legendAt < 0 || agendaAt < 0 || legendAt < agendaAt {
		t.Fatalf("legend should render below main view: agenda=%d legend=%d", agendaAt, legendAt)
	}
}

func TestAttendeeDialogShowsStateIconsAndFooterKeybindings(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.width = 140
	m.height = 40
	m.openEventFormNew()
	m.eventForm.attendees = attendeesInput([]calendar.Attendee{
		{Name: "Ada Lovelace", Email: "ada@example.test"},
		{Name: "Grace Hopper", Email: "grace@example.test", Role: "optional"},
	})
	setEventEditorCursor(t, &m, "attendees")
	m.openEventEditorForm()

	view := m.View()
	for _, label := range []string{"", "Required", "", "Optional", "", "Removed"} {
		if !strings.Contains(view, label) {
			t.Fatalf("attendee dialog is missing %q:\n%s", label, view)
		}
	}
	want := "[j/k] Move  [spc] State  [o] Optional  [x] Remove  [tab] Actions  [enter] Apply  [esc/q] Cancel"
	if got := m.shortcutsLegend(); got != want {
		t.Fatalf("attendee footer = %q, want %q", got, want)
	}
	if strings.Index(view, want) < strings.Index(view, "Ada Lovelace") {
		t.Fatal("attendee keybindings should render below the dialog")
	}
}
func TestNotificationDialogExplainsStateIcons(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	m.eventForm.alarms = "10m before; 1h before"
	setEventEditorCursor(t, &m, "alarms")
	m.openEventEditorForm()
	m.eventForm.notificationManager.cycle(1)

	view := m.View()
	for _, label := range []string{"", "Focused", "󰀠", "Active", "", "Removed"} {
		if !strings.Contains(view, label) {
			t.Fatalf("notification dialog is missing %q:\n%s", label, view)
		}
	}
	want := "[j/k] Move  [spc] Remove  [tab] Actions  [enter] Apply  [esc/q] Cancel"
	if got := m.shortcutsLegend(); got != want {
		t.Fatalf("notification footer = %q, want %q", got, want)
	}
}
func TestChoiceAndSearchDialogsExplainListMarkers(t *testing.T) {
	t.Run("choice", func(t *testing.T) {
		calendars := []calendar.Calendar{{Source: "src", Name: "one"}, {Source: "src", Name: "two"}}
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: calendars}, nil)
		m.width = 120
		m.height = 40
		m.openEventFormNew()
		setEventEditorCursor(t, &m, "calendar")
		m.openEventEditorForm()

		view := m.View()
		for _, label := range []string{" Focused", "(•) Selected", "( ) Not selected"} {
			if !strings.Contains(view, label) {
				t.Fatalf("choice dialog is missing %q:\n%s", label, view)
			}
		}
		want := "[j/k] Move  [spc] Select  [tab] Actions  [enter] Apply  [esc/q] Cancel"
		if got := m.shortcutsLegend(); got != want {
			t.Fatalf("choice footer = %q, want %q", got, want)
		}
	})

	t.Run("multi-choice", func(t *testing.T) {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
		m.width = 120
		m.height = 40
		m.openEventFormNew()
		m.eventForm.recur = true
		m.eventForm.recurFreq = "WEEKLY"
		m.eventForm.recurWeekdays = []string{"Mo"}
		setEventEditorCursor(t, &m, "recur-weekdays")
		m.openEventEditorForm()

		view := m.View()
		for _, label := range []string{" Focused", "[x] Selected", "[ ] Not selected"} {
			if !strings.Contains(view, label) {
				t.Fatalf("multi-choice dialog is missing %q:\n%s", label, view)
			}
		}
	})

	t.Run("search", func(t *testing.T) {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
		m.width = 120
		m.height = 40
		m.openEventFormNew()
		m.eventForm.activeKey = "attendees-add"
		m.eventForm.searchPicker = newSearchPicker("Contacts", []searchOption{{label: "Ada", value: "ada"}}, "")

		if view := m.View(); !strings.Contains(view, " Focused") {
			t.Fatalf("search dialog is missing its focus icon legend:\n%s", view)
		}
		want := "[type] Search  [↑/↓ ctrl-j/ctrl-k] Move  [tab] Actions  [enter] Apply  [esc] Cancel"
		if got := m.shortcutsLegend(); got != want {
			t.Fatalf("search footer = %q, want %q", got, want)
		}
	})
}
func TestTimeDialogKeybindingsRenderOnlyInFooter(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.width = 120
	m.height = 40
	m.openEventFormNew()
	setEventEditorCursor(t, &m, "time")
	m.openEventEditorForm()

	want := "[←/→] Digit  [↑/↓] Adjust  [tab] Actions  [enter] Apply  [esc/q] Cancel"
	if got := m.shortcutsLegend(); got != want {
		t.Fatalf("time footer = %q, want %q", got, want)
	}
	view := m.View()
	if count := strings.Count(view, "[←/→] Digit"); count != 1 {
		t.Fatalf("time keybindings rendered %d times, want footer only:\n%s", count, view)
	}
}

func TestEditDialogUsesFooterWithoutOpeningApplicationHelp(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	m.eventForm.cursor = nearestSelectableEditorCursor(m.eventEditorRows(), 0)
	m.openEventEditorForm()
	if m.eventForm.activeForm == nil {
		t.Fatal("expected active edit dialog")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	updatedModel, ok := updated.(*Model)
	if !ok {
		t.Fatalf("updated model type = %T", updated)
	}
	if updatedModel.showHelpOverlay {
		t.Fatal("? should not open application help from edit dialog")
	}
	want := "[tab] Actions  [enter] Apply  [esc] Cancel"
	if got := updatedModel.shortcutsLegend(); got != want {
		t.Fatalf("edit dialog footer = %q, want %q", got, want)
	}
	view := updatedModel.View()
	if strings.Contains(strings.ToLower(view), "enter apply") {
		t.Fatalf("edit dialog still renders inline keybindings:\n%s", view)
	}
}
func TestSpecialFieldFormsUseContextualFooters(t *testing.T) {
	t.Run("event description", func(t *testing.T) {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
		m.openEventFormNew()
		setEventEditorCursor(t, &m, "description")
		m.openEventEditorForm()

		want := "[ctrl+enter] New line  [tab] Actions  [enter] Apply  [esc] Cancel"
		if got := m.shortcutsLegend(); got != want {
			t.Fatalf("event description footer = %q, want %q", got, want)
		}
	})

	t.Run("task description", func(t *testing.T) {
		m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
		m.openTodoFormNew()
		setTodoEditorCursor(t, &m, "description")
		m.openTodoEditorForm()

		want := "[ctrl+enter] New line  [tab] Actions  [enter] Apply  [esc] Cancel"
		if got := m.shortcutsLegend(); got != want {
			t.Fatalf("task description footer = %q, want %q", got, want)
		}
	})

	t.Run("recurring edit scope", func(t *testing.T) {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
		m.openEventFormNew()
		m.eventForm.activeKey = "edit-scope"
		m.eventForm.activeForm = m.buildEventEditorForm("edit-scope")

		want := "[j/k] Move  [enter] Apply  [esc/q] Cancel"
		if got := m.shortcutsLegend(); got != want {
			t.Fatalf("edit scope footer = %q, want %q", got, want)
		}
	})
}

func TestDeleteDialogKeybindingsRenderInFooter(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.deleteConfirm = &deleteConfirmState{kind: "event", stage: "scope", scope: string(calendar.DeleteRecurringOccurrence)}

	scopeWant := "[j/k] Move  [enter] Continue  [esc/q] Cancel"
	if got := m.shortcutsLegend(); got != scopeWant {
		t.Fatalf("delete scope footer = %q, want %q", got, scopeWant)
	}

	m.deleteConfirm.stage = "confirm"
	confirmWant := "[h/l] Choose  [enter] Delete  [esc/q] Cancel"
	if got := m.shortcutsLegend(); got != confirmWant {
		t.Fatalf("delete confirmation footer = %q, want %q", got, confirmWant)
	}
}

func TestEmptyNotificationsOpenManager(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	rows := m.eventEditorRows()
	for i, row := range rows {
		if row.key == "alarms" {
			m.eventForm.cursor = i
			break
		}
	}

	m.openEventEditorForm()
	if m.eventForm.notificationManager == nil || m.eventForm.activeForm != nil {
		t.Fatal("empty notifications should open the notification manager")
	}
	view := m.renderEventFormMainPanel(100, 34)
	if !strings.Contains(view, "No notifications") || !strings.Contains(view, "Apply") {
		t.Fatalf("unexpected empty notification manager: %q", view)
	}
}

func TestPreferredFormKeyMapUsesJKAndCtrlEnter(t *testing.T) {
	keymap := NewPreferredFormKeyMap()
	if got := keymap.Select.Up.Help().Key; got != "k" {
		t.Fatalf("select up help key = %q", got)
	}
	if got := keymap.Select.Down.Help().Key; got != "j" {
		t.Fatalf("select down help key = %q", got)
	}
	if got := keymap.Text.NewLine.Help().Key; got != "ctrl+enter / ctrl+j" {
		t.Fatalf("text newline help key = %q", got)
	}
	for _, key := range keymap.Text.NewLine.Keys() {
		if key == "alt+enter" {
			t.Fatal("alt+enter should not be bound to text newline")
		}
	}
}

func TestMultiFieldFormKeyMapPrefersCtrlJK(t *testing.T) {
	keymap := NewPreferredMultiFieldFormKeyMap()
	if got := keymap.Input.Next.Help().Key; got != "ctrl+j" {
		t.Fatalf("input next help key = %q", got)
	}
	if got := keymap.Input.Prev.Help().Key; got != "ctrl+k" {
		t.Fatalf("input previous help key = %q", got)
	}
	if got := keymap.Text.NewLine.Help().Key; got != "ctrl+enter" {
		t.Fatalf("multi-field text newline help key = %q", got)
	}
	if containsString(keymap.Select.Down.Keys(), "ctrl+j") || containsString(keymap.Select.Up.Keys(), "ctrl+k") {
		t.Fatal("ctrl+j/ctrl+k should navigate fields instead of select options")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func setTodoEditorCursor(t *testing.T, m *Model, key string) {
	t.Helper()
	for i, row := range m.todoEditorRows() {
		if row.key == key {
			m.todoForm.cursor = i
			return
		}
	}
	t.Fatalf("todo editor row %q not found", key)
}

func updateModelAndRunHuhNavigation(model tea.Model, msg tea.Msg) tea.Model {
	updated, cmd := model.Update(msg)
	return runHuhNavigationCommands(updated, cmd)
}

func runHuhNavigationCommands(model tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return model
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			model = runHuhNavigationCommands(model, child)
		}
		return model
	}
	typeOf := reflect.TypeOf(msg)
	if typeOf == nil || typeOf.PkgPath() != "github.com/charmbracelet/huh" {
		return model
	}
	switch typeOf.Name() {
	case "nextFieldMsg", "prevFieldMsg", "nextGroupMsg", "prevGroupMsg":
		updated, next := model.Update(msg)
		return runHuhNavigationCommands(updated, next)
	default:
		return model
	}
}

func modelValue(t *testing.T, model tea.Model) *Model {
	t.Helper()
	switch value := model.(type) {
	case Model:
		return &value
	case *Model:
		return value
	default:
		t.Fatalf("model type = %T", model)
		return nil
	}
}

func TestTaskShortcutsToggleDoneAndCyclePriority(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todo := calendar.Todo{UID: "task", Summary: "Task", Source: "src", Calendar: "cal", Due: &now, Status: "NEEDS-ACTION", Priority: 5}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)

	doneModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	done := doneModel.(Model)
	if !isTodoDone(done.data.Todos[0]) {
		t.Fatal("x should mark selected task done")
	}

	m = NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)
	priorityModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if got := priorityModel.(Model).data.Todos[0].Priority; got != 9 {
		t.Fatalf("p should cycle mid priority to low, got %d", got)
	}
}
