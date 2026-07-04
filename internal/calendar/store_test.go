package calendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hsanson/go-khal/internal/config"
)

func TestEventMetadataRoundTrip(t *testing.T) {
	store, _, _ := testStore(t)
	start := time.Now().Truncate(time.Minute)
	until := start.AddDate(0, 0, 7)

	err := store.CreateEvent("src", "cal", Event{
		UID:     "metadata@example.test",
		Summary: "Planning",
		Start:   start,
		End:     start.Add(time.Hour),
		AllDay:  false,
		Attendees: []Attendee{
			{Name: "Ada Lovelace", Email: "ada@example.test", Status: "yes"},
			{Name: "grace@example.test", Email: "grace@example.test", Status: "yes", Role: "optional"},
		},
		Availability: "free",
		Visibility:   "private",
		Recurrence:   &Recurrence{Frequency: "DAILY", Interval: 2, Until: &until},
		Alarms: []Alarm{
			{Offset: -10 * time.Minute, Action: "DISPLAY"},
			{Offset: time.Hour, Action: "DISPLAY"},
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ds.Events) == 0 {
		t.Fatal("expected recurrence instances")
	}
	ev := ds.Events[0]
	if len(ev.Attendees) != 2 {
		t.Fatalf("expected 2 attendees, got %d", len(ev.Attendees))
	}
	if ev.Attendees[0].Status != "yes" || ev.Attendees[1].Status != "yes" {
		t.Fatalf("expected attendee RSVP yes, got %+v", ev.Attendees)
	}
	if ev.Attendees[1].Role != "optional" {
		t.Fatalf("expected optional attendee role, got %+v", ev.Attendees[1])
	}
	if ev.Availability != "free" {
		t.Fatalf("expected free availability, got %q", ev.Availability)
	}
	if ev.Visibility != "private" {
		t.Fatalf("expected private visibility, got %q", ev.Visibility)
	}
	if ev.Recurrence == nil {
		t.Fatal("expected recurrence")
	}
	if ev.Recurrence.Frequency != "DAILY" || ev.Recurrence.Interval != 2 {
		t.Fatalf("unexpected recurrence: %+v", ev.Recurrence)
	}
	if len(ev.Alarms) != 2 {
		t.Fatalf("expected 2 alarms, got %d", len(ev.Alarms))
	}
}

func TestCreateEventWithAttendeesWritesOrganizerAndRSVP(t *testing.T) {
	store, _, calDir := testStore(t)
	store.config.Sources[0].Email = "organizer@example.test"
	start := time.Now().Truncate(time.Minute)
	if err := store.CreateEvent("src", "cal", Event{
		UID:     "invite@example.test",
		Summary: "Invite",
		Start:   start,
		End:     start.Add(time.Hour),
		Attendees: []Attendee{
			{Name: "Guest", Email: "guest@example.test"},
		},
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	raw := readEventFile(t, filepath.Join(calDir, "invite@example.test.ics"))
	for _, want := range []string{
		"ORGANIZER:mailto:organizer@example.test",
		"ATTENDEE;CN=Guest;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:guest@example.test",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in event file:\n%s", want, raw)
		}
	}
}

func TestAttendeeScopedUpdatePreservesOrganizerFields(t *testing.T) {
	store, _, _ := testStore(t)
	store.config.Sources[0].Email = "guest@example.test"
	start := time.Now().Truncate(time.Minute)
	if err := store.CreateEvent("src", "cal", Event{
		UID:       "attendee-update@example.test",
		Summary:   "Original",
		Organizer: "organizer@example.test",
		Start:     start,
		End:       start.Add(time.Hour),
		Attendees: []Attendee{
			{Name: "Guest", Email: "guest@example.test", Status: "needs-action", RSVP: true},
			{Name: "Other", Email: "other@example.test", Status: "needs-action", RSVP: true},
		},
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	ev, err := store.FindEvent("attendee-update@example.test")
	if err != nil {
		t.Fatalf("FindEvent: %v", err)
	}
	nextSummary := "Changed"
	nextLocation := "Somewhere else"
	nextStart := start.Add(24 * time.Hour)
	nextEnd := nextStart.Add(2 * time.Hour)
	alarms := []Alarm{{Offset: -10 * time.Minute, Action: "DISPLAY"}}
	availability := "free"
	attendees := []Attendee{
		{Name: "Guest", Email: "guest@example.test", Status: "yes", RSVP: true},
		{Name: "Other", Email: "other@example.test", Status: "no", RSVP: true},
	}
	if err := store.UpdateEventScoped(ev, EventUpdate{
		Summary:      &nextSummary,
		Location:     &nextLocation,
		Start:        &nextStart,
		End:          &nextEnd,
		Attendees:    &attendees,
		Availability: &availability,
		Alarms:       &alarms,
	}, EditRecurringAll); err != nil {
		t.Fatalf("UpdateEventScoped: %v", err)
	}
	updated, err := store.FindEvent("attendee-update@example.test")
	if err != nil {
		t.Fatalf("FindEvent updated: %v", err)
	}
	if updated.Summary != "Original" || updated.Location != "" || !updated.Start.Equal(start) || !updated.End.Equal(start.Add(time.Hour)) {
		t.Fatalf("organizer fields changed unexpectedly: %+v", updated)
	}
	if updated.Availability != "free" || len(updated.Alarms) != 1 {
		t.Fatalf("local attendee fields not updated: %+v", updated)
	}
	for _, attendee := range updated.Attendees {
		switch attendee.Email {
		case "guest@example.test":
			if attendee.Status != "yes" {
				t.Fatalf("expected guest RSVP yes, got %+v", attendee)
			}
		case "other@example.test":
			if attendee.Status != "needs-action" {
				t.Fatalf("expected other attendee unchanged, got %+v", attendee)
			}
		}
	}
}

func TestDeleteRecurringOccurrence(t *testing.T) {
	store, _, _ := testStore(t)
	start := time.Now().Truncate(time.Minute)
	err := store.CreateEvent("src", "cal", Event{
		UID:        "delete-one@example.test",
		Summary:    "Standup",
		Start:      start,
		End:        start.Add(30 * time.Minute),
		Recurrence: &Recurrence{Frequency: "DAILY", Interval: 1, Count: 3},
	})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load before delete: %v", err)
	}
	if len(ds.Events) != 3 {
		t.Fatalf("expected 3 instances before delete, got %d", len(ds.Events))
	}

	if err := store.DeleteEvent(ds.Events[1], DeleteRecurringOccurrence); err != nil {
		t.Fatalf("DeleteEvent occurrence: %v", err)
	}
	ds, err = store.Load()
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if len(ds.Events) != 2 {
		t.Fatalf("expected 2 instances after delete, got %d", len(ds.Events))
	}
	for _, ev := range ds.Events {
		if ev.Start.Equal(start.AddDate(0, 0, 1)) {
			t.Fatalf("deleted occurrence still present: %v", ev.Start)
		}
	}
}

func TestDeleteTodoRemovesFile(t *testing.T) {
	store, _, calDir := testStore(t)
	err := store.CreateTodo("src", "cal", Todo{
		UID:     "todo-delete@example.test",
		Summary: "Remove me",
	})
	if err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	path := filepath.Join(calDir, "todo-delete@example.test.ics")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected todo file: %v", err)
	}
	if err := store.DeleteTodo("todo-delete@example.test"); err != nil {
		t.Fatalf("DeleteTodo: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected todo file to be removed, stat err=%v", err)
	}
}

func TestMoveEventMovesFileToTargetCalendar(t *testing.T) {
	store, root, calDir := testStore(t)
	targetDir := filepath.Join(root, "work")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target calendar: %v", err)
	}
	store.config.Sources = append(store.config.Sources, config.Source{Path: targetDir, Type: "calendar"})

	start := time.Now().Truncate(time.Minute)
	if err := store.CreateEvent("src", "cal", Event{
		UID:     "move-event@example.test",
		Summary: "Move me",
		Start:   start,
		End:     start.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	oldPath := filepath.Join(calDir, "move-event@example.test.ics")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected source event file: %v", err)
	}
	ev, err := store.FindEvent("move-event@example.test")
	if err != nil {
		t.Fatalf("FindEvent: %v", err)
	}

	if err := store.MoveEvent(ev, "src", "work"); err != nil {
		t.Fatalf("MoveEvent: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected source event file removed, stat err=%v", err)
	}
	newPath := filepath.Join(targetDir, "move-event@example.test.ics")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected target event file: %v", err)
	}
	moved, err := store.FindEvent("move-event@example.test")
	if err != nil {
		t.Fatalf("FindEvent after move: %v", err)
	}
	if moved.Calendar != "work" || moved.CalendarDir != targetDir {
		t.Fatalf("expected moved event in work calendar, got %s %s", moved.Calendar, moved.CalendarDir)
	}
}

func TestMoveTodoMovesFileToTargetCalendar(t *testing.T) {
	store, root, calDir := testStore(t)
	targetDir := filepath.Join(root, "work")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target calendar: %v", err)
	}
	store.config.Sources = append(store.config.Sources, config.Source{Path: targetDir, Type: "calendar"})

	if err := store.CreateTodo("src", "cal", Todo{
		UID:     "move-todo@example.test",
		Summary: "Move task",
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}
	oldPath := filepath.Join(calDir, "move-todo@example.test.ics")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("expected source todo file: %v", err)
	}

	if err := store.MoveTodo("move-todo@example.test", "src", "work"); err != nil {
		t.Fatalf("MoveTodo: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected source todo file removed, stat err=%v", err)
	}
	newPath := filepath.Join(targetDir, "move-todo@example.test.ics")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected target todo file: %v", err)
	}
	moved, err := store.FindTodo("move-todo@example.test")
	if err != nil {
		t.Fatalf("FindTodo after move: %v", err)
	}
	if moved.Calendar != "work" || moved.CalendarDir != targetDir {
		t.Fatalf("expected moved todo in work calendar, got %s %s", moved.Calendar, moved.CalendarDir)
	}
}

func TestUpdateRecurringOccurrenceCreatesOverride(t *testing.T) {
	store, _, _ := testStore(t)
	start := time.Date(2026, time.July, 7, 12, 0, 0, 0, time.Local)
	if err := store.CreateEvent("src", "cal", Event{
		UID:        "edit-one@example.test",
		Summary:    "Series",
		Start:      start,
		End:        start.Add(time.Hour),
		Recurrence: &Recurrence{Frequency: "WEEKLY", Interval: 1, Count: 3},
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ds.Events) < 2 {
		t.Fatalf("expected recurrence instances, got %d", len(ds.Events))
	}
	updatedSummary := "Changed occurrence"
	occurrence := ds.Events[1]
	if err := store.UpdateEventScoped(occurrence, EventUpdate{Summary: &updatedSummary}, EditRecurringOccurrence); err != nil {
		t.Fatalf("UpdateEventScoped occurrence: %v", err)
	}

	raw := readEventFile(t, occurrence.FilePath)
	if got := strings.Count(raw, "BEGIN:VEVENT"); got != 2 {
		t.Fatalf("expected master and override components, got %d\n%s", got, raw)
	}
	if !strings.Contains(raw, "RECURRENCE-ID") {
		t.Fatalf("expected override RECURRENCE-ID\n%s", raw)
	}
	if !strings.Contains(raw, "SUMMARY:Changed occurrence") {
		t.Fatalf("expected edited override summary\n%s", raw)
	}
	if strings.Count(raw, "RRULE:") != 1 {
		t.Fatalf("expected only the master to keep RRULE\n%s", raw)
	}
}

func TestUpdateRecurringFutureSplitsSeries(t *testing.T) {
	store, _, _ := testStore(t)
	start := time.Date(2026, time.July, 7, 12, 0, 0, 0, time.Local)
	if err := store.CreateEvent("src", "cal", Event{
		UID:        "edit-future@example.test",
		Summary:    "Series",
		Start:      start,
		End:        start.Add(time.Hour),
		Recurrence: &Recurrence{Frequency: "WEEKLY", Interval: 1, Count: 5},
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ds.Events) < 3 {
		t.Fatalf("expected recurrence instances, got %d", len(ds.Events))
	}
	updatedSummary := "Changed future"
	future := ds.Events[2]
	if err := store.UpdateEventScoped(future, EventUpdate{Summary: &updatedSummary}, EditRecurringFuture); err != nil {
		t.Fatalf("UpdateEventScoped future: %v", err)
	}

	raw := readEventFile(t, future.FilePath)
	if got := strings.Count(raw, "BEGIN:VEVENT"); got != 2 {
		t.Fatalf("expected truncated master and new future series, got %d\n%s", got, raw)
	}
	if !strings.Contains(raw, "SUMMARY:Changed future") {
		t.Fatalf("expected new future summary\n%s", raw)
	}
	if strings.Count(raw, "UID:") != 2 {
		t.Fatalf("expected split series to use two UIDs\n%s", raw)
	}
	if strings.Count(raw, "RRULE:") != 2 {
		t.Fatalf("expected both split components to have RRULEs\n%s", raw)
	}
	if !strings.Contains(raw, "UNTIL=") {
		t.Fatalf("expected original series to be truncated with UNTIL\n%s", raw)
	}
}

func TestUpdateRecurringAllCanRemoveRecurrence(t *testing.T) {
	store, _, _ := testStore(t)
	start := time.Date(2026, time.July, 7, 12, 0, 0, 0, time.Local)
	if err := store.CreateEvent("src", "cal", Event{
		UID:        "edit-all@example.test",
		Summary:    "Series",
		Start:      start,
		End:        start.Add(time.Hour),
		Recurrence: &Recurrence{Frequency: "WEEKLY", Interval: 1, Count: 3},
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	updatedSummary := "One event"
	noRecurrence := (*Recurrence)(nil)
	if err := store.UpdateEventScoped(ds.Events[0], EventUpdate{Summary: &updatedSummary, Recurrence: &noRecurrence}, EditRecurringAll); err != nil {
		t.Fatalf("UpdateEventScoped all: %v", err)
	}

	raw := readEventFile(t, ds.Events[0].FilePath)
	if strings.Contains(raw, "RRULE:") || strings.Contains(raw, "EXDATE") || strings.Contains(raw, "RDATE") {
		t.Fatalf("expected recurrence properties removed\n%s", raw)
	}
	if !strings.Contains(raw, "SUMMARY:One event") {
		t.Fatalf("expected updated summary\n%s", raw)
	}
}

func TestGoogleOverrideUsesRecurrenceIDDateWhenStartWasFlattened(t *testing.T) {
	store, _, calDir := testStore(t)
	path := filepath.Join(calDir, "override.ics")
	ics := strings.ReplaceAll(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//go-khal test//EN
BEGIN:VEVENT
UID:override@example.test
DTSTART;TZID=Asia/Tokyo:20260706T120000
DTEND;TZID=Asia/Tokyo:20260706T130000
SUMMARY:Override test
STATUS:CONFIRMED
END:VEVENT
BEGIN:VEVENT
UID:override@example.test
DTSTART:20260706T030000Z
DTEND:20260706T040000Z
RECURRENCE-ID;TZID=Asia/Tokyo:20260803T123000
SUMMARY:Override test
STATUS:CONFIRMED
END:VEVENT
END:VCALENDAR
`, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(ics), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var found bool
	seen := map[string]bool{}
	for _, ev := range ds.Events {
		key := ev.UID + ev.Start.Format(time.RFC3339)
		if seen[key] {
			t.Fatalf("duplicate event occurrence: %s %s", ev.UID, ev.Start)
		}
		seen[key] = true
		if ev.UID != "override@example.test" || ev.Start.Month() != time.August {
			continue
		}
		found = true
		if got := ev.Start.Format("2006-01-02 15:04"); got != "2026-08-03 12:00" {
			t.Fatalf("unexpected recovered override start: %s", got)
		}
	}
	if !found {
		t.Fatal("expected recovered override event")
	}
}

func readEventFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read event file: %v", err)
	}
	return string(raw)
}

func TestFormatICalDurationGoogleCompatible(t *testing.T) {
	tests := map[time.Duration]string{
		-10 * time.Minute: "-P0DT0H10M0S",
		-2 * time.Hour:    "-P0DT2H0M0S",
		26 * time.Hour:    "P1DT2H0M0S",
	}
	for input, want := range tests {
		if got := formatICalDuration(input); got != want {
			t.Fatalf("formatICalDuration(%v) = %q, want %q", input, got, want)
		}
	}
}

func testStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	calDir := filepath.Join(root, "cal")
	if err := os.MkdirAll(calDir, 0o755); err != nil {
		t.Fatalf("mkdir calendar: %v", err)
	}
	cfg := &config.Config{
		Sources: []config.Source{{
			Path: calDir,
			Type: "calendar",
		}},
		RecurrenceLookbackMonths:  1,
		RecurrenceLookaheadMonths: 1,
	}
	return NewStore(cfg), root, calDir
}
