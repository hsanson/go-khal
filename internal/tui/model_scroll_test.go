package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func TestEventDetailsShowReadOnlyFields(t *testing.T) {
	m := NewModel(&config.Config{}, calendar.Dataset{}, nil)
	ev := calendar.Event{
		Summary:      "Planning",
		Description:  "private notes",
		DisplayName:  "Personal",
		Organizer:    "organizer@example.test",
		Availability: "free",
		Visibility:   "private",
		Timezone:     "Europe/Berlin",
		URL:          "https://meet.google.com/abc-defg-hij",
	}

	got := m.renderEventDetailsFor(ev, 100, 100)
	for _, value := range []string{
		"Calendar", "Personal", "Organizer", "organizer@example.test",
		"Availability", "Visibility", "Timezone", "Description", "private notes",
		"Google Meet", ev.URL,
	} {
		if !strings.Contains(got, value) {
			t.Fatalf("event details do not contain %q:\n%s", value, got)
		}
	}
	if count := strings.Count(got, ev.URL); count != 1 {
		t.Fatalf("video URL rendered %d times, want once:\n%s", count, got)
	}
}

func TestTodoDetailsShowReadOnlyFields(t *testing.T) {
	start := time.Date(2026, time.October, 23, 9, 0, 0, 0, time.UTC)
	due := start.Add(2 * time.Hour)
	m := NewModel(&config.Config{}, calendar.Dataset{}, nil)
	todo := calendar.Todo{
		Summary: "Prepare notes", Description: "Draft agenda", DisplayName: "Work",
		Status: "IN-PROCESS", Percent: 50, Priority: 1, Start: &start, Due: &due,
	}

	got := m.renderTodoDetailsFor(todo, "todo-range", 100, 100)
	for _, value := range []string{
		"Calendar", "Work", "Start", "Due", "Status", "Completed",
		"Progress", "50%", "Priority", "Description", "Draft agenda",
	} {
		if !strings.Contains(got, value) {
			t.Fatalf("task details do not contain %q:\n%s", value, got)
		}
	}
}

func TestVJoinsSelectedVideoCall(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "local", Name: "personal"}
	ev := calendar.Event{
		UID: "event", Summary: "Event", Source: cal.Source, Calendar: cal.Name,
		Start: now, End: now.Add(time.Hour), URL: "https://meet.google.com/abc-defg-hij",
	}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{ev}}, nil)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	next := updated.(Model)
	if cmd == nil {
		t.Fatal("v in Agenda did not return a video-call command")
	}
	if next.eventForm != nil {
		t.Fatal("v opened the obsolete read-only event form")
	}
	if !strings.Contains(strings.Join(next.helpLines(), "\n"), "Join video call") {
		t.Fatal("Agenda help does not show v for an event with a video call")
	}

	next.focusedPane = paneDetails
	_, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if cmd == nil {
		t.Fatal("v in Details did not return a video-call command")
	}
}

func TestVDoesNothingWithoutVideoCall(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "local", Name: "personal"}
	ev := calendar.Event{
		UID: "event", Summary: "Event", Source: cal.Source, Calendar: cal.Name,
		Start: now, End: now.Add(time.Hour),
	}
	m := NewModel(&config.Config{}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{ev}}, nil)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	next := updated.(Model)
	if cmd != nil || next.eventForm != nil {
		t.Fatal("v should do nothing when the selected event has no video call")
	}
	if strings.Contains(strings.Join(next.helpLines(), "\n"), "Join video call") {
		t.Fatal("Agenda help shows v for an event without a video call")
	}
}

func TestVideoCallLaunchFailureSetsActionError(t *testing.T) {
	m := NewModel(&config.Config{}, calendar.Dataset{}, nil)
	updated, _ := m.Update(videoCallOpenedMsg{err: errors.New("launcher failed")})
	if got := updated.(Model).actionErr; got != "launcher failed" {
		t.Fatalf("action error = %q, want launcher failure", got)
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

func TestNewTaskDueDefaultsToSelectedTaskTime(t *testing.T) {
	due := time.Date(2026, time.July, 3, 11, 30, 0, 0, time.Local)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todo := calendar.Todo{UID: "selected", Source: cal.Source, Calendar: cal.Name, Due: &due}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)

	m.openTodoFormNew()

	if m.todoForm.dueDate != "2026-07-03" || m.todoForm.dueTime != "11:30" {
		t.Fatalf("new task due = %s %s, want selected task time", m.todoForm.dueDate, m.todoForm.dueTime)
	}
	if m.todoForm.startDate != "" || m.todoForm.startTime != "" {
		t.Fatalf("new task start = %s %s, want unset", m.todoForm.startDate, m.todoForm.startTime)
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
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}, "2025-02-01"},
		{tea.KeyMsg{Type: tea.KeyRight}, "2025-02-02"},
		{tea.KeyMsg{Type: tea.KeyCtrlJ}, "2025-03-02"},
		{tea.KeyMsg{Type: tea.KeyCtrlK}, "2025-02-02"},
		{tea.KeyMsg{Type: tea.KeyCtrlL}, "2026-02-02"},
		{tea.KeyMsg{Type: tea.KeyCtrlH}, "2025-02-02"},
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

func TestCalendarsPaneQAndEscReturnToAgenda(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	for _, key := range []string{"q", "esc"} {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
		m.focusedPane = paneCalendars

		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		if key == "esc" {
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		updated, cmd := m.Update(msg)
		next := updated.(Model)

		if cmd != nil {
			t.Fatalf("%s in Calendars should not quit or run a command", key)
		}
		if next.focusedPane != paneAgenda {
			t.Fatalf("%s focus = %v, want Agenda", key, next.focusedPane)
		}
	}
}
func TestEnterFocusesSelectedEventDetails(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	event := calendar.Event{UID: "event", Summary: "Event", Source: "src", Calendar: "cal", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Events: []calendar.Event{event}}, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(Model)
	if next.focusedPane != paneDetails || next.eventForm != nil {
		t.Fatal("enter should focus event details without opening the editor")
	}
}

func TestEnterFocusesSelectedTaskDetails(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	todo := calendar.Todo{UID: "task", Summary: "Task", Source: "src", Calendar: "cal", Due: &now}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(Model)
	if next.focusedPane != paneDetails || next.todoForm != nil {
		t.Fatal("enter should focus task details without opening the editor")
	}
}

func TestCalendarsShortcutQReturnsToAgenda(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)

	opened, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	closed, cmd := opened.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		t.Fatal("q in Calendars should not quit")
	}
	if closed.(Model).focusedPane != paneAgenda {
		t.Fatal("q should return focus to Agenda")
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
	for _, key := range []rune{' ', 'x', 'd'} {
		m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)
		doneModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		done := doneModel.(Model)
		if !isTodoDone(done.data.Todos[0]) {
			t.Fatalf("%q should mark selected task done", key)
		}
	}

	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}, Todos: []calendar.Todo{todo}}, nil)
	priorityModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if got := priorityModel.(Model).data.Todos[0].Priority; got != 9 {
		t.Fatalf("p should cycle mid priority to low, got %d", got)
	}
}

func TestWorkspacePaneNavigation(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	if m.focusedPane != paneAgenda {
		t.Fatalf("initial focus = %v, want Agenda", m.focusedPane)
	}

	for _, want := range []workspacePane{paneDetails, paneCalendars, paneCalendar, paneAgenda} {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = updated.(Model)
		if m.focusedPane != want {
			t.Fatalf("tab focus = %v, want %v", m.focusedPane, want)
		}
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := updated.(Model).focusedPane; got != paneCalendar {
		t.Fatalf("shift-tab focus = %v, want Calendar", got)
	}
}

func TestCalendarNavigationAndAgendaFocus(t *testing.T) {
	selected := time.Date(2026, time.October, 8, 0, 0, 0, 0, time.Local)
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.selected = selected
	m.agendaStart = selected
	m.focusedPane = paneCalendar

	for _, step := range []struct {
		key  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}}, "2026-10-07"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, "2026-10-14"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, "2026-10-07"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}}, "2026-10-08"},
	} {
		updated, _ := m.Update(step.key)
		m = updated.(Model)
		if got := m.selected.Format("2006-01-02"); got != step.want {
			t.Fatalf("%s selected %s, want %s", step.key.String(), got, step.want)
		}
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.focusedPane != paneAgenda {
		t.Fatalf("enter focus = %v, want Agenda", m.focusedPane)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if got := updated.(Model).focusedPane; got != paneCalendar {
		t.Fatalf("h focus = %v, want Calendar", got)
	}

}

func TestCalendarEnterSelectsFirstTaskOnOrAfterDate(t *testing.T) {
	before := time.Date(2026, time.October, 6, 9, 0, 0, 0, time.Local)
	after := time.Date(2026, time.October, 9, 9, 0, 0, 0, time.Local)
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewTaskModeModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{cal},
		Todos: []calendar.Todo{
			{UID: "before", Source: cal.Source, Calendar: cal.Name, Due: &before},
			{UID: "after", Source: cal.Source, Calendar: cal.Name, Due: &after},
		},
	}, nil)
	m.selected = time.Date(2026, time.October, 8, 0, 0, 0, 0, time.Local)
	m.focusedPane = paneCalendar

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(Model)
	item := next.agendaItems()[next.eventCursor]
	if next.focusedPane != paneAgenda || item.Todo == nil || item.Todo.UID != "after" {
		t.Fatalf("calendar enter focus/item = %v/%#v, want Agenda/after", next.focusedPane, item)
	}
}

func TestFormReturnsToOriginatingPane(t *testing.T) {
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{Calendars: []calendar.Calendar{cal}}, nil)
	m.focusedPane = paneDetails

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = *modelValue(t, updated)
	if m.eventForm == nil {
		t.Fatal("n did not open the event form")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = *modelValue(t, updated)
	if m.eventForm != nil || m.focusedPane != paneDetails {
		t.Fatalf("cancel returned form/focus = %v/%v, want nil/Details", m.eventForm != nil, m.focusedPane)
	}
}

func TestCancellingRecurringEditScopeReturnsToOriginatingPane(t *testing.T) {
	now := time.Now()
	cal := calendar.Calendar{Source: "src", Name: "cal"}
	event := calendar.Event{
		UID: "recurring", Source: cal.Source, Calendar: cal.Name,
		Summary: "Recurring", Start: now, End: now.Add(time.Hour), Recurring: true,
	}
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{
		Calendars: []calendar.Calendar{cal},
		Events:    []calendar.Event{event},
	}, nil)
	m.focusedPane = paneDetails

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = *modelValue(t, updated)
	if m.eventForm == nil || m.eventForm.activeKey != "edit-scope" {
		t.Fatal("e did not open the recurring event edit-scope dialog")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = *modelValue(t, updated)
	if m.eventForm != nil || m.focusedPane != paneDetails {
		t.Fatalf("cancel returned form/focus = %v/%v, want nil/Details", m.eventForm != nil, m.focusedPane)
	}
}

func TestViewRendersFourPanesWithinTerminal(t *testing.T) {
	m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
	m.width = 100
	m.height = 40
	view := m.View()

	for _, title := range []string{"Calendar", "Calendars", "Agenda from", "Details"} {
		if !strings.Contains(view, title) {
			t.Fatalf("view is missing %q", title)
		}
	}
	if got := strings.Count(view, "╭"); got != 4 {
		t.Fatalf("top border count = %d, want 4", got)
	}
	if got := lipgloss.Width(view); got != m.width {
		t.Fatalf("view width = %d, want %d", got, m.width)
	}
	if got := lipgloss.Height(view); got != m.height {
		t.Fatalf("view height = %d, want %d", got, m.height)
	}
}

func TestCalendarsAndDetailsPanesStayAligned(t *testing.T) {
	for _, height := range []int{16, 25, 40} {
		m := NewModel(&config.Config{SidebarWidth: 30}, calendar.Dataset{}, nil)
		m.width = 100
		m.height = height

		topBorders := 0
		bottomBorders := 0
		for _, line := range strings.Split(m.View(), "\n") {
			if strings.Count(line, "╭") == 2 {
				topBorders++
			}
			if strings.Count(line, "╰") == 2 {
				bottomBorders++
			}
		}
		if topBorders != 2 || bottomBorders != 2 {
			t.Fatalf("height %d has %d aligned top and %d aligned bottom borders, want 2 each", height, topBorders, bottomBorders)
		}
	}
}
