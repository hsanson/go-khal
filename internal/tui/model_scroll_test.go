package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

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
