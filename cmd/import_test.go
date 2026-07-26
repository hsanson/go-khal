package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

func TestImportEventsBatchAddsAndUpdatesWithoutInput(t *testing.T) {
	calDir := filepath.Join(t.TempDir(), "personal")
	if err := os.MkdirAll(calDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := calendar.NewStore(&config.Config{Sources: []config.Source{{Path: calDir, Type: "calendar"}}})
	ds, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)
	if err := store.CreateEvent(ds.Calendars[0].Source, ds.Calendars[0].Name, calendar.Event{
		UID: "existing@example.test", Summary: "Old", Start: start, End: start.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	ds, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}

	created, updated, err := importEventsBatch(store, ds, []calendar.Event{
		{UID: "existing@example.test", Summary: "Updated", Start: start, End: start.Add(2 * time.Hour)},
		{UID: "new@example.test", Summary: "New", Start: start.AddDate(0, 0, 1), End: start.AddDate(0, 0, 1).Add(time.Hour)},
	}, "")
	if err != nil {
		t.Fatalf("importEventsBatch: %v", err)
	}
	if created != 1 || updated != 1 {
		t.Fatalf("created=%d updated=%d, want 1 and 1", created, updated)
	}
	existing, err := store.FindEvent("existing@example.test")
	if err != nil || existing.Summary != "Updated" {
		t.Fatalf("existing event was not updated: %+v, %v", existing, err)
	}
	added, err := store.FindEvent("new@example.test")
	if err != nil || added.Summary != "New" {
		t.Fatalf("new event was not added: %+v, %v", added, err)
	}
}

func TestImportEventsBatchUsesSelectedCalendar(t *testing.T) {
	root := t.TempDir()
	personalDir := filepath.Join(root, "personal")
	workDir := filepath.Join(root, "work")
	for _, dir := range []string{personalDir, workDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := calendar.NewStore(&config.Config{Sources: []config.Source{
		{Path: personalDir, Type: "calendar", DisplayName: "Personal"},
		{Path: workDir, Type: "calendar", DisplayName: "Work Calendar"},
	}})
	ds, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.July, 26, 9, 0, 0, 0, time.UTC)

	if _, _, err := importEventsBatch(store, ds, []calendar.Event{{
		UID: "work@example.test", Summary: "Work", Start: start, End: start.Add(time.Hour),
	}}, "Work Calendar"); err != nil {
		t.Fatalf("importEventsBatch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "work@example.test.ics")); err != nil {
		t.Fatalf("event was not imported into selected calendar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(personalDir, "work@example.test.ics")); !os.IsNotExist(err) {
		t.Fatalf("event unexpectedly imported into default calendar: %v", err)
	}
}
