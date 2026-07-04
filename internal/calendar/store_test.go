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
			{Name: "Ada Lovelace", Email: "ada@example.test"},
			{Name: "grace@example.test", Email: "grace@example.test"},
		},
		Recurrence: &Recurrence{Frequency: "DAILY", Interval: 2, Until: &until},
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
			Name: "src",
			Path: root,
			Calendars: []config.CalendarConfig{{
				Name: "cal",
				Path: calDir,
			}},
		}},
		RecurrenceLookbackMonths:  1,
		RecurrenceLookaheadMonths: 1,
	}
	return NewStore(cfg), root, calDir
}
