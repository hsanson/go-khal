package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/hsanson/go-khal/internal/calendar"
)

func TestInteractiveAgendaTruncatesByDisplayWidth(t *testing.T) {
	day := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.Local)
	event := calendar.Event{
		Summary: "Roadmap 你好",
		Start:   day.Add(11*time.Hour + 30*time.Minute),
		End:     day.Add(12*time.Hour + 30*time.Minute),
	}
	items := []AgendaListItem{{Day: day, Start: event.Start, End: event.End, Event: &event}}
	fullLine := "  " + iconForEvent(event) + " 11:30-12:30  " + event.Summary
	fullWidth := lipgloss.Width(fullLine)

	rendered := renderAgendaFromItems(items, fullWidth, 6, "15:04", DefaultStyles(), 0, 0, false)
	line := agendaEventLine(rendered.Text)
	if line != fullLine {
		t.Fatalf("fitting agenda line = %q, want %q", line, fullLine)
	}

	rendered = renderAgendaFromItems(items, fullWidth-4, 6, "15:04", DefaultStyles(), 0, 0, false)
	line = agendaEventLine(rendered.Text)
	if !utf8.ValidString(line) || !strings.HasSuffix(line, "…") {
		t.Fatalf("truncated agenda line = %q, want valid Unicode ending in ellipsis", line)
	}
	if got := lipgloss.Width(line); got != fullWidth-4 {
		t.Fatalf("truncated agenda width = %d, want %d", got, fullWidth-4)
	}
}

func agendaEventLine(rendered string) string {
	lines := strings.Split(ansi.Strip(rendered), "\n")
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimRight(lines[1], " ")
}
