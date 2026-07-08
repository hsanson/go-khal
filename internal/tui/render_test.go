package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
)

func TestRenderAgendaDoesNotPrintTitle(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{{
		Summary:     "Meeting",
		Start:       now.Add(time.Hour),
		End:         now.Add(2 * time.Hour),
		Calendar:    "Work",
		DisplayName: "Work Calendar",
		Color:       "#00aa00",
	}}

	got := RenderAgenda(events, now, time.Monday, "15:04", DefaultStyles(), 0)

	if strings.HasPrefix(got, "Agenda") || strings.Contains(got, "\nAgenda\n") {
		t.Fatalf("RenderAgenda() printed title: %q", got)
	}
	if !strings.Contains(got, "Meeting") {
		t.Fatalf("RenderAgenda() = %q, want event summary", got)
	}
	if strings.Contains(got, "Work") || strings.Contains(got, "#00aa00") {
		t.Fatalf("RenderAgenda() printed calendar metadata: %q", got)
	}
}

func TestRenderAgendaTruncatesLinesToMaxLength(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{{
		Summary: "Long meeting summary",
		Start:   now.Add(time.Hour),
		End:     now.Add(2 * time.Hour),
	}}

	got := RenderAgenda(events, now, time.Monday, "15:04", DefaultStyles(), 12)
	runes := []rune(got)

	if len(runes) != 12 {
		t.Fatalf("truncated agenda line length = %d, want 12: %q", len(runes), got)
	}
	if runes[len(runes)-1] != []rune(agendaEllipsisGlyph)[0] {
		t.Fatalf("truncated agenda line = %q, want Nerd Font ellipsis glyph suffix", got)
	}
}

func TestRenderAgendaFormatsTodayWithTimeOnly(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{{
		Summary: "Daily standup",
		Start:   time.Date(2026, 7, 8, 13, 30, 0, 0, time.UTC),
		End:     time.Date(2026, 7, 8, 14, 0, 0, 0, time.UTC),
	}}

	got := RenderAgenda(events, now, time.Monday, "15:04", DefaultStyles(), 0)

	if got != "13:30 Daily standup" {
		t.Fatalf("RenderAgenda() = %q, want today time-only prefix", got)
	}
}

func TestRenderAgendaFormatsCurrentWeekWithWeekday(t *testing.T) {
	now := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{{
		Summary: "Planning",
		Start:   time.Date(2026, 7, 7, 14, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 7, 7, 15, 0, 0, 0, time.UTC),
	}}

	got := RenderAgenda(events, now, time.Monday, "15:04", DefaultStyles(), 0)

	if got != "(Tu) 14:00 Planning" {
		t.Fatalf("RenderAgenda() = %q, want current-week weekday prefix", got)
	}
}

func TestRenderAgendaFormatsOutsideCurrentWeekWithMonthAndDay(t *testing.T) {
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	events := []calendar.Event{{
		Summary: "Conference",
		Start:   time.Date(2026, 6, 13, 14, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 6, 13, 15, 0, 0, 0, time.UTC),
	}}

	got := RenderAgenda(events, now, time.Monday, "15:04", DefaultStyles(), 0)

	if got != "Jun 13 Conference" {
		t.Fatalf("RenderAgenda() = %q, want month/day prefix outside current week", got)
	}
}
