package cmd

import (
	"testing"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
)

func TestAgendaEventsExcludesBirthdaysByDefault(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{
		{Summary: "Past", Start: now.Add(-2 * time.Hour), End: now.Add(-time.Hour)},
		{Summary: "Birthday", Kind: calendar.EventKindBirthday, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		{Summary: "Anniversary", Kind: calendar.EventKindAnniversary, Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		{Summary: "Meeting", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)},
	}

	got := agendaEvents(events, now, false, 0)

	if len(got) != 1 || got[0].Summary != "Meeting" {
		t.Fatalf("agenda events = %#v, want only Meeting", got)
	}
}

func TestAgendaEventsCanShowOnlyBirthdaysAndAnniversaries(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{
		{Summary: "Birthday", Kind: calendar.EventKindBirthday, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		{Summary: "Anniversary", Kind: calendar.EventKindAnniversary, Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		{Summary: "Meeting", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)},
	}

	got := agendaEvents(events, now, true, 1)

	if len(got) != 1 || got[0].Summary != "Birthday" {
		t.Fatalf("birthday agenda events = %#v, want only first birthday item", got)
	}
}
