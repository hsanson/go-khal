package calendar

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNamedEventTimezoneRoundTripsAndSurvivesUnrelatedEdit(t *testing.T) {
	store, _, calDir := testStore(t)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.January, 15, 9, 0, 0, 0, berlin)
	if err := store.CreateEvent("src", "cal", Event{
		UID: "timezone@example.test", Summary: "Planning", Start: start, End: start.Add(time.Hour), Timezone: "Europe/Berlin",
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(calDir, "timezone@example.test.ics")
	raw := readEventFile(t, path)
	if !strings.Contains(raw, "DTSTART;TZID=Europe/Berlin:20260115T090000") {
		t.Fatalf("named DTSTART missing:\n%s", raw)
	}

	event, err := store.FindEvent("timezone@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if event.Timezone != "Europe/Berlin" || !event.Start.Equal(start) {
		t.Fatalf("round-tripped event = %+v", event)
	}
	summary := "Updated"
	if err := store.UpdateEvent(event.UID, EventUpdate{Summary: &summary}); err != nil {
		t.Fatal(err)
	}
	raw = readEventFile(t, path)
	if !strings.Contains(raw, "DTSTART;TZID=Europe/Berlin:20260115T090000") {
		t.Fatalf("summary edit changed timezone representation:\n%s", raw)
	}
}

func TestRecurringNamedTimezoneKeepsWallClockAcrossDST(t *testing.T) {
	store, _, _ := testStore(t)
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.October, 25, 9, 0, 0, 0, newYork)
	if err := store.CreateEvent("src", "cal", Event{
		UID: "dst@example.test", Summary: "Weekly", Start: start, End: start.Add(time.Hour), Timezone: "America/New_York",
		Recurrence: &Recurrence{Frequency: "WEEKLY", Interval: 1, Count: 3},
	}); err != nil {
		t.Fatal(err)
	}

	dataset, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var occurrences []Event
	for _, event := range dataset.Events {
		if event.UID == "dst@example.test" {
			occurrences = append(occurrences, event)
		}
	}
	if len(occurrences) < 2 {
		t.Fatalf("occurrence count = %d: %+v", len(occurrences), occurrences)
	}
	for _, event := range occurrences {
		if event.Timezone != "America/New_York" || event.Start.In(newYork).Hour() != 9 {
			t.Fatalf("DST occurrence changed wall clock: %+v", event)
		}
	}
}

func TestParseEventsDistinguishesUTCAndFloatingTimes(t *testing.T) {
	events, err := ParseEvents(strings.NewReader(`BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:utc@example.test
DTSTART:20261001T090000Z
DTEND:20261001T100000Z
END:VEVENT
BEGIN:VEVENT
UID:floating@example.test
DTSTART:20261002T090000
DTEND:20261002T100000
END:VEVENT
END:VCALENDAR
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d", len(events))
	}
	if events[0].Timezone != EventTimezoneUTC || events[1].Timezone != EventTimezoneFloating {
		t.Fatalf("timezone kinds = %q, %q", events[0].Timezone, events[1].Timezone)
	}
}

func TestAllDayEventDoesNotWriteTimezone(t *testing.T) {
	store, _, calDir := testStore(t)
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.Local)
	if err := store.CreateEvent("src", "cal", Event{
		UID: "all-day-timezone@example.test", Summary: "Trip", Start: start, End: start.AddDate(0, 0, 3),
		AllDay: true, Timezone: "Europe/Berlin",
	}); err != nil {
		t.Fatal(err)
	}
	raw := readEventFile(t, filepath.Join(calDir, "all-day-timezone@example.test.ics"))
	if strings.Contains(raw, "TZID") || !strings.Contains(raw, "DTEND;VALUE=DATE:20261004") {
		t.Fatalf("all-day event has invalid timezone/end:\n%s", raw)
	}
}
