package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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

func TestNewTaskDefaultsToSelectedFreeSlotTime(t *testing.T) {
	start := dayStart(time.Date(2026, time.July, 3, 10, 0, 0, 0, time.Local))
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	events := []calendar.Event{
		{UID: "one", Summary: "One", Source: "src", Calendar: "cal", Start: start.Add(10 * time.Hour), End: start.Add(11 * time.Hour)},
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: events}, nil)
	m.selected = start
	m.agendaStart = start
	m.showAllMode = true
	m.eventCursor = 2

	m.openTodoFormNew()

	if m.todoForm.startDate != "2026-07-03" || m.todoForm.startTime != "11:00" {
		t.Fatalf("unexpected task start default: %s %s", m.todoForm.startDate, m.todoForm.startTime)
	}
	if m.todoForm.dueDate != "2026-07-03" || m.todoForm.dueTime != "12:00" {
		t.Fatalf("unexpected task due default: %s %s", m.todoForm.dueDate, m.todoForm.dueTime)
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

func TestContextualLegendsAndHelp(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	if got := m.shortcutsLegend(); got != "[esc/q] Exit  [j/k] Next / Previous  [t] Today  [enter] Open  [n] New  [m] Tasks  [ctrl-d] Delete  [c] Calendars  [?] Help" {
		t.Fatalf("unexpected event legend: %q", got)
	}

	m.focusCalendarPane = true
	calendarHelp := strings.Join(m.helpLines(), "\n")
	if strings.Contains(calendarHelp, "New event") || !strings.Contains(calendarHelp, "Hide/show calendar") {
		t.Fatalf("calendar help contains unrelated shortcuts: %q", calendarHelp)
	}

	m.focusCalendarPane = false
	m.openEventFormNew()
	editorHelp := strings.Join(m.helpLines(), "\n")
	if strings.Contains(editorHelp, "Open tasks") || !strings.Contains(editorHelp, "Save") {
		t.Fatalf("editor help contains unrelated shortcuts: %q", editorHelp)
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

func TestEditDialogDoesNotOpenApplicationHelp(t *testing.T) {
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
	if got := updatedModel.shortcutsLegend(); got != "" {
		t.Fatalf("application legend should be hidden behind edit dialog: %q", got)
	}
}

func TestEmptyNotificationsOpenMessageInsteadOfChoice(t *testing.T) {
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
	if !m.eventForm.noNotifications || m.eventForm.activeForm != nil {
		t.Fatal("empty notifications should open message dialog")
	}
	view := m.renderEventFormMainPanel(100, 34)
	if !strings.Contains(view, "No notifications") || strings.Contains(view, "None") {
		t.Fatalf("unexpected empty notification dialog: %q", view)
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

func TestEventWhenDialogEnterAdvancesThenSubmits(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	setEventEditorCursor(t, &m, "when")
	m.openEventEditorForm()
	legend := activeFormModalView(m.eventForm.activeForm, 70, 20, "")
	if !strings.Contains(legend, "ctrl+j") || strings.Contains(legend, "shift+tab") {
		t.Fatalf("multi-field dialog should prefer ctrl+j/ctrl+k legend: %q", legend)
	}

	var model tea.Model = &m
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
	if got := modelValue(t, model).eventForm.activeForm.GetFocusedField().GetKey(); got != "from-time" {
		t.Fatalf("first enter focused %q, want from-time", got)
	}
	for range 2 {
		model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
	}
	if got := modelValue(t, model).eventForm.activeForm.GetFocusedField().GetKey(); got != "to-time" {
		t.Fatalf("third enter focused %q, want to-time", got)
	}
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
	if modelValue(t, model).eventForm.activeForm != nil {
		t.Fatal("enter on final event time field should submit dialog")
	}
}

func TestTaskDateDialogEnterAdvancesThenSubmits(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openTodoFormNew()
	setTodoEditorCursor(t, &m, "start")
	m.openTodoEditorForm()

	var model tea.Model = &m
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
	if got := modelValue(t, model).todoForm.activeForm.GetFocusedField().GetKey(); got != "start-time" {
		t.Fatalf("first enter focused %q, want start-time", got)
	}
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyEnter})
	if modelValue(t, model).todoForm.activeForm != nil {
		t.Fatal("enter on final task start field should submit dialog")
	}
}

func TestMultiFieldDialogCtrlJAndCtrlKNavigate(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	setEventEditorCursor(t, &m, "when")
	m.openEventEditorForm()
	fromTime := m.eventForm.fromTime

	var model tea.Model = &m
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyCtrlJ})
	if got := modelValue(t, model).eventForm.activeForm.GetFocusedField().GetKey(); got != "from-time" {
		t.Fatalf("ctrl+j focused %q, want from-time", got)
	}
	model = updateModelAndRunHuhNavigation(model, tea.KeyMsg{Type: tea.KeyCtrlK})
	if got := modelValue(t, model).eventForm.activeForm.GetFocusedField().GetKey(); got != "from-date" {
		t.Fatalf("ctrl+k focused %q, want from-date", got)
	}
	if got := modelValue(t, model).eventForm.fromTime; got != fromTime {
		t.Fatalf("ctrl+k changed field value from %q to %q", fromTime, got)
	}
}

func TestDialogErrorCanBeCorrectedAndDoesNotPersist(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.openEventFormNew()
	setEventEditorCursor(t, &m, "when")
	m.openEventEditorForm()
	m.eventForm.fromDate = "invalid"
	m.eventForm.activeForm.State = huh.StateCompleted

	m.updateActiveEventEditorForm(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.eventForm.activeForm == nil {
		t.Fatal("dialog should remain open after apply error")
	}
	if m.eventForm.errMsg == "" {
		t.Fatal("dialog should show apply error")
	}
	if got := m.eventForm.activeForm.GetFocusedField().GetKey(); got != "from-date" {
		t.Fatalf("rebuilt dialog focused %q, want from-date", got)
	}

	m.updateEventEditor(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.eventForm.activeForm != nil || m.eventForm.errMsg != "" {
		t.Fatalf("cancel should clear dialog and error: active=%v error=%q", m.eventForm.activeForm != nil, m.eventForm.errMsg)
	}
	setEventEditorCursor(t, &m, "title")
	m.openEventEditorForm()
	if m.eventForm.errMsg != "" {
		t.Fatalf("error persisted into next dialog: %q", m.eventForm.errMsg)
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

func setEventEditorCursor(t *testing.T, m *Model, key string) {
	t.Helper()
	for i, row := range m.eventEditorRows() {
		if row.key == key {
			m.eventForm.cursor = i
			return
		}
	}
	t.Fatalf("event editor row %q not found", key)
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
