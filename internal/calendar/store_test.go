package calendar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
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

func TestCreateTodoWritesUTCDateTimes(t *testing.T) {
	store, _, calDir := testStore(t)
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	start := time.Date(2026, 7, 10, 9, 30, 0, 0, loc)
	due := time.Date(2026, 7, 10, 10, 45, 0, 0, loc)
	completed := time.Date(2026, 7, 10, 11, 0, 0, 0, loc)

	if err := store.CreateTodo("src", "cal", Todo{
		UID:       "todo-utc@example.test",
		Summary:   "UTC task",
		Start:     &start,
		Due:       &due,
		Completed: &completed,
	}); err != nil {
		t.Fatalf("CreateTodo: %v", err)
	}

	raw := readEventFile(t, filepath.Join(calDir, "todo-utc@example.test.ics"))
	if strings.Contains(raw, "TZID") {
		t.Fatalf("todo file should not contain TZID without VTIMEZONE:\n%s", raw)
	}
	for _, want := range []string{
		"DTSTART:20260710T003000Z",
		"DUE:20260710T014500Z",
		"COMPLETED:20260710T020000Z",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in todo file:\n%s", want, raw)
		}
	}
}

func TestDeleteTodoRemovesFileWithOnlyTimezoneRemaining(t *testing.T) {
	store, _, calDir := testStore(t)
	path := filepath.Join(calDir, "todo-delete-timezone@example.test.ics")
	writeICSFile(t, path, `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//go-khal test//EN
BEGIN:VTIMEZONE
TZID:Asia/Tokyo
END:VTIMEZONE
BEGIN:VTODO
UID:todo-delete-timezone@example.test
SUMMARY:Remove me
END:VTODO
END:VCALENDAR
`)

	if err := store.DeleteTodo("todo-delete-timezone@example.test"); err != nil {
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

func TestMoveEventRemovesSourceFileWithOnlyTimezoneRemaining(t *testing.T) {
	store, root, calDir := testStore(t)
	targetDir := filepath.Join(root, "work")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target calendar: %v", err)
	}
	store.config.Sources = append(store.config.Sources, config.Source{Path: targetDir, Type: "calendar"})

	start := time.Now().Truncate(time.Minute)
	path := filepath.Join(calDir, "move-event-timezone@example.test.ics")
	writeICSFile(t, path, `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//go-khal test//EN
BEGIN:VTIMEZONE
TZID:Asia/Tokyo
END:VTIMEZONE
BEGIN:VEVENT
UID:move-event-timezone@example.test
SUMMARY:Move me
DTSTAMP:`+start.UTC().Format("20060102T150405Z")+`
DTSTART:`+start.UTC().Format("20060102T150405Z")+`
DTEND:`+start.Add(time.Hour).UTC().Format("20060102T150405Z")+`
END:VEVENT
END:VCALENDAR
`)

	ev, err := store.FindEvent("move-event-timezone@example.test")
	if err != nil {
		t.Fatalf("FindEvent: %v", err)
	}
	if err := store.MoveEvent(ev, "src", "work"); err != nil {
		t.Fatalf("MoveEvent: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected source event file removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "move-event-timezone@example.test.ics")); err != nil {
		t.Fatalf("expected target event file: %v", err)
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

func TestLocalRSVPOccurrencePersistsWithoutAttendees(t *testing.T) {
	store, _, _ := testStore(t)
	start := time.Date(2026, time.July, 20, 13, 0, 0, 0, time.Local)
	if err := store.CreateEvent("src", "cal", Event{
		UID: "local-rsvp@example.test", Summary: "Test2", Start: start, End: start.Add(time.Hour),
		Recurrence: &Recurrence{Frequency: "DAILY", Interval: 1, Count: 3},
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	ds, err := store.Load()
	if err != nil || len(ds.Events) < 2 {
		t.Fatalf("Load recurrence: events=%d err=%v", len(ds.Events), err)
	}
	target := ds.Events[1]
	rsvp := "yes"
	if err := store.UpdateEventScoped(target, EventUpdate{UserRSVP: &rsvp}, EditRecurringOccurrence); err != nil {
		t.Fatalf("UpdateEventScoped occurrence: %v", err)
	}
	raw := readEventFile(t, target.FilePath)
	if !strings.Contains(raw, "RECURRENCE-ID") || !strings.Contains(raw, "X-GO-KHAL-RSVP") || !strings.Contains(raw, ":yes") {
		t.Fatalf("local occurrence RSVP was not persisted\n%s", raw)
	}
	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load after update: %v", err)
	}
	for _, ev := range reloaded.Events {
		if ev.UID != target.UID {
			continue
		}
		if ev.Start.Equal(target.Start) && ev.UserRSVP != "yes" {
			t.Fatalf("updated occurrence RSVP = %q, want yes", ev.UserRSVP)
		}
		if ev.Start.Equal(ds.Events[0].Start) && ev.UserRSVP != "" {
			t.Fatalf("other occurrence RSVP = %q, want empty", ev.UserRSVP)
		}
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
	if !strings.Contains(raw, "COUNT=2") || !strings.Contains(raw, "COUNT=3") {
		t.Fatalf("expected COUNT recurrence to be divided across the split series\n%s", raw)
	}
}

func TestAttendeeRSVPOccurrenceUpdatesOnlyExistingOverride(t *testing.T) {
	store, path := attendeeRecurringStore(t, false)
	target := findEventOnDate(t, store, "2026-07-20")
	attendees := attendeesWithUserStatus(target.Attendees, "user@example.test", "yes")

	if err := store.UpdateEventScoped(target, EventUpdate{Attendees: &attendees}, EditRecurringOccurrence); err != nil {
		t.Fatalf("UpdateEventScoped occurrence: %v", err)
	}
	raw := readEventFile(t, path)
	if strings.Count(raw, "RRULE:") != 1 || strings.Count(raw, "RECURRENCE-ID") != 1 {
		t.Fatalf("occurrence response should preserve master and override\n%s", raw)
	}
	if strings.Contains(raw, "PARTSTAT=DECLINED:mailto:user@example.test") {
		t.Fatalf("occurrence response was not updated\n%s", raw)
	}
	updated := findEventOnDate(t, store, "2026-07-20")
	if updated.UserRSVP != "yes" {
		t.Fatalf("updated occurrence RSVP = %q, want yes", updated.UserRSVP)
	}
	master := findEventOnDate(t, store, "2026-07-13")
	if master.UserRSVP != "yes" {
		t.Fatalf("master occurrence RSVP changed unexpectedly: %q", master.UserRSVP)
	}
}

func TestAttendeeRSVPAllUpdatesMasterAndOverridesWithoutChangingRecurrence(t *testing.T) {
	store, path := attendeeRecurringStore(t, true)
	target := findEventOnDate(t, store, "2026-07-20")
	attendees := attendeesWithUserStatus(target.Attendees, "user@example.test", "yes")

	if err := store.UpdateEventScoped(target, EventUpdate{Attendees: &attendees}, EditRecurringAll); err != nil {
		t.Fatalf("UpdateEventScoped all: %v", err)
	}
	raw := readEventFile(t, path)
	if strings.Count(raw, "RRULE:FREQ=WEEKLY;COUNT=4") != 1 {
		t.Fatalf("all response changed the recurrence rule\n%s", raw)
	}
	if !strings.Contains(raw, "DTSTART;TZID=Asia/Tokyo:20260713T130000") {
		t.Fatalf("all response changed the master start\n%s", raw)
	}
	if strings.Count(raw, "RECURRENCE-ID") != 2 {
		t.Fatalf("all response should preserve organizer exceptions\n%s", raw)
	}
	if strings.Contains(raw, "PARTSTAT=DECLINED:mailto:user@example.test") {
		t.Fatalf("all response left a declined owner exception\n%s", raw)
	}
	if !strings.Contains(raw, "PARTSTAT=TENTATIVE:mailto:other@example.test") {
		t.Fatalf("all response changed another attendee's exception status\n%s", raw)
	}
	for _, date := range []string{"2026-07-13", "2026-07-20", "2026-07-27", "2026-08-03"} {
		if got := findEventOnDate(t, store, date).UserRSVP; got != "yes" {
			t.Fatalf("%s RSVP = %q, want yes", date, got)
		}
	}
}

func TestAttendeeRSVPFutureUsesRangeOverrideAndReconcilesExceptions(t *testing.T) {
	store, path := attendeeRecurringStore(t, true)
	target := findEventOnDate(t, store, "2026-07-20")
	attendees := attendeesWithUserStatus(target.Attendees, "user@example.test", "yes")

	if err := store.UpdateEventScoped(target, EventUpdate{Attendees: &attendees}, EditRecurringFuture); err != nil {
		t.Fatalf("UpdateEventScoped future: %v", err)
	}
	raw := readEventFile(t, path)
	if strings.Count(raw, "RRULE:FREQ=WEEKLY;COUNT=4") != 1 {
		t.Fatalf("future RSVP should not split or truncate the organizer series\n%s", raw)
	}
	if !strings.Contains(raw, "RECURRENCE-ID;RANGE=THISANDFUTURE") {
		t.Fatalf("future RSVP did not create a range override\n%s", raw)
	}
	if strings.Count(raw, "UID:rsvp-series@example.test") != 4 {
		t.Fatalf("future RSVP should keep the organizer UID\n%s", raw)
	}
	if strings.Contains(raw, "PARTSTAT=DECLINED:mailto:user@example.test") {
		t.Fatalf("future RSVP left a conflicting declined exception\n%s", raw)
	}
	if !strings.Contains(raw, "PARTSTAT=TENTATIVE:mailto:other@example.test") {
		t.Fatalf("future RSVP changed another attendee's exception status\n%s", raw)
	}
	for _, date := range []string{"2026-07-20", "2026-07-27", "2026-08-03"} {
		if got := findEventOnDate(t, store, date).UserRSVP; got != "yes" {
			t.Fatalf("%s RSVP = %q, want yes", date, got)
		}
	}
}

func TestAttendeeUpdateDoesNotAddUserToExcludedOverride(t *testing.T) {
	comp := ical.NewComponent(ical.CompEvent)
	setEventAttendees(comp, []Attendee{{Email: "other@example.test", Status: "yes"}}, false)
	attendees := []Attendee{{Email: "user@example.test", Status: "yes"}}

	if applyAttendeeEventUpdate(comp, EventUpdate{Attendees: &attendees}, "user@example.test") {
		t.Fatal("excluded attendee should not be added to an override")
	}
	if got := propsToAttendees(comp.Props); len(got) != 1 || got[0].Email != "other@example.test" {
		t.Fatalf("override attendees changed unexpectedly: %+v", got)
	}
}

func TestRecurringAllFromOverrideUsesMasterTimingAndRule(t *testing.T) {
	store, path := attendeeRecurringStore(t, false)
	store.config.Sources[0].Email = "organizer@example.test"
	target := findEventOnDate(t, store, "2026-07-20")
	summary := "Updated series"

	if err := store.UpdateEventScoped(target, EventUpdate{Summary: &summary}, EditRecurringAll); err != nil {
		t.Fatalf("UpdateEventScoped all: %v", err)
	}
	raw := readEventFile(t, path)
	if !strings.Contains(raw, "RRULE:FREQ=WEEKLY;INTERVAL=1;COUNT=4") || !strings.Contains(raw, "DTSTART:20260713T040000Z") {
		t.Fatalf("all edit did not retain master recurrence identity\n%s", raw)
	}
	if strings.Count(raw, "SUMMARY:Updated series") != 1 {
		t.Fatalf("all edit should update the master component\n%s", raw)
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
	store.config.Sources[0].Email = "user@example.test"
	path := filepath.Join(calDir, "override.ics")
	ics := strings.ReplaceAll(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//go-khal test//EN
BEGIN:VEVENT
UID:override@example.test
DTSTAMP:20260701T000000Z
DTSTART;TZID=Asia/Tokyo:20260706T120000
DTEND;TZID=Asia/Tokyo:20260706T130000
SUMMARY:Override test
STATUS:CONFIRMED
END:VEVENT
BEGIN:VEVENT
UID:override@example.test
DTSTAMP:20260701T000000Z
DTSTART:20260706T030000Z
DTEND:20260706T040000Z
RECURRENCE-ID;TZID=Asia/Tokyo:20260803T123000
SUMMARY:Override test
STATUS:CONFIRMED
ATTENDEE;PARTSTAT=DECLINED:mailto:user@example.test
ATTENDEE;PARTSTAT=ACCEPTED:mailto:other@example.test
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
	var override Event
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
		if !ev.Recurring {
			t.Fatal("RECURRENCE-ID override should remain marked recurring")
		}
		if ev.UserRSVP != "no" {
			t.Fatalf("override user RSVP = %q, want no", ev.UserRSVP)
		}
		override = ev
	}
	if !found {
		t.Fatal("expected recovered override event")
	}
	updatedSummary := "Updated override"
	if err := store.UpdateEventScoped(override, EventUpdate{Summary: &updatedSummary}, EditRecurringOccurrence); err != nil {
		t.Fatalf("UpdateEventScoped moved override: %v", err)
	}
	raw := readEventFile(t, path)
	if strings.Count(raw, "RECURRENCE-ID") != 1 || !strings.Contains(raw, "RECURRENCE-ID:20260803T033000Z") {
		t.Fatalf("moved override lost its stable recurrence identity\n%s", raw)
	}
}

func TestCalendarUserEmailFallsBackToEmailShapedDirectoryName(t *testing.T) {
	root := t.TempDir()
	calDir := filepath.Join(root, "user@example.test")
	if err := os.MkdirAll(calDir, 0o755); err != nil {
		t.Fatalf("mkdir calendar: %v", err)
	}
	store := NewStore(&config.Config{Sources: []config.Source{{Path: calDir, Type: "calendar"}}})

	if got := store.CalendarUserEmail(calDir, "user@example.test"); got != "user@example.test" {
		t.Fatalf("calendar user email = %q, want user@example.test", got)
	}
	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(ds.Calendars) != 1 || ds.Calendars[0].Email != "user@example.test" {
		t.Fatalf("calendar fallback email not exposed: %+v", ds.Calendars)
	}
	role := store.EventUserRole(Event{
		Source:    calDir,
		Calendar:  "user@example.test",
		Organizer: "organizer@example.test",
		Attendees: []Attendee{{Email: "user@example.test", Status: "no"}},
	})
	if role != EventUserRoleAttendee {
		t.Fatalf("event user role = %q, want attendee", role)
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

func writeICSFile(t *testing.T, path, raw string) {
	t.Helper()
	raw = strings.ReplaceAll(raw, "\n", "\r\n")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write ics file: %v", err)
	}
}

func attendeeRecurringStore(t *testing.T, includeLaterOverride bool) (*Store, string) {
	t.Helper()
	calDir := filepath.Join(t.TempDir(), "user@example.test")
	if err := os.MkdirAll(calDir, 0o755); err != nil {
		t.Fatalf("mkdir calendar: %v", err)
	}
	later := ""
	if includeLaterOverride {
		later = `BEGIN:VEVENT
UID:rsvp-series@example.test
DTSTAMP:20260713T000000Z
DTSTART;TZID=Asia/Tokyo:20260803T130000
DTEND;TZID=Asia/Tokyo:20260803T133000
RECURRENCE-ID;TZID=Asia/Tokyo:20260803T130000
ORGANIZER:mailto:organizer@example.test
ATTENDEE;PARTSTAT=DECLINED:mailto:user@example.test
ATTENDEE;PARTSTAT=TENTATIVE:mailto:other@example.test
SUMMARY:Moved room
LOCATION:Room B
END:VEVENT
`
	}
	path := filepath.Join(calDir, "event.ics")
	writeICSFile(t, path, `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//go-khal test//EN
BEGIN:VEVENT
UID:rsvp-series@example.test
DTSTAMP:20260713T000000Z
DTSTART;TZID=Asia/Tokyo:20260713T130000
DTEND;TZID=Asia/Tokyo:20260713T133000
RRULE:FREQ=WEEKLY;COUNT=4
ORGANIZER:mailto:organizer@example.test
ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@example.test
ATTENDEE;PARTSTAT=ACCEPTED:mailto:other@example.test
SUMMARY:Series
END:VEVENT
BEGIN:VEVENT
UID:rsvp-series@example.test
DTSTAMP:20260713T000000Z
DTSTART;TZID=Asia/Tokyo:20260720T130000
DTEND;TZID=Asia/Tokyo:20260720T133000
RECURRENCE-ID;TZID=Asia/Tokyo:20260720T130000
ORGANIZER:mailto:organizer@example.test
ATTENDEE;PARTSTAT=DECLINED:mailto:user@example.test
ATTENDEE;PARTSTAT=ACCEPTED:mailto:other@example.test
SUMMARY:Series
END:VEVENT
`+later+`END:VCALENDAR
`)
	cfg := &config.Config{
		Sources:                   []config.Source{{Path: calDir, Type: "calendar"}},
		RecurrenceLookbackMonths:  1,
		RecurrenceLookaheadMonths: 1,
	}
	return NewStore(cfg), path
}

func findEventOnDate(t *testing.T, store *Store, date string) Event {
	t.Helper()
	ds, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, ev := range ds.Events {
		if ev.UID == "rsvp-series@example.test" && ev.Start.Format("2006-01-02") == date {
			return ev
		}
	}
	t.Fatalf("event on %s not found", date)
	return Event{}
}

func attendeesWithUserStatus(attendees []Attendee, email, status string) []Attendee {
	out := append([]Attendee{}, attendees...)
	for i := range out {
		if sameEmail(out[i].Email, email) {
			out[i].Status = status
		}
	}
	return out
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
