package tui

import (
	"reflect"
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
