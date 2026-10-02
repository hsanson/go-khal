package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openCustomTodoEditor(key string) bool {
	s := m.todoForm
	if s == nil {
		return true
	}
	switch key {
	case "due-date":
		value := s.dueDate
		if strings.TrimSpace(value) == "" {
			value = s.startDate
		}
		if strings.TrimSpace(value) == "" {
			value = m.selected.Format("2006-01-02")
		}
		s.datePicker = newTaskDatePicker(value)
	case "start-date":
		value := s.startDate
		if strings.TrimSpace(value) == "" {
			value = m.selected.Format("2006-01-02")
		}
		s.datePicker = newTaskDatePicker(value)
	case "due-time":
		s.timeEditor = newSingleTimeEditor(s.dueTime)
	case "start-time":
		s.timeEditor = newSingleTimeEditor(s.startTime)
	default:
		return false
	}
	return true
}

func (m *Model) updateTodoDatePicker(msg tea.KeyMsg) {
	s := m.todoForm
	picker := s.datePicker
	picker.Update(msg)
	if picker.cancelled {
		s.cancelActive()
		return
	}
	if !picker.done {
		return
	}
	value, _ := picker.dates()
	switch s.activeKey {
	case "due-date":
		if picker.cleared {
			s.dueDate = ""
			s.dueTime = ""
			break
		}
		s.dueDate = value.Format("2006-01-02")
		if strings.TrimSpace(s.dueTime) == "" {
			s.dueTime = "00:00"
		}
	case "start-date":
		if picker.cleared {
			s.startDate = ""
			s.startTime = ""
			break
		}
		s.startDate = value.Format("2006-01-02")
		if strings.TrimSpace(s.startTime) == "" {
			s.startTime = "00:00"
		}
	}
	s.datePicker = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (m *Model) updateTodoTimeEditor(msg tea.KeyMsg) {
	s := m.todoForm
	editor := s.timeEditor
	editor.Update(msg)
	if editor.cancelled {
		s.cancelActive()
		return
	}
	if !editor.done {
		return
	}
	value, err := editor.value()
	if err != nil {
		s.errMsg = err.Error()
		return
	}
	switch s.activeKey {
	case "due-time":
		s.dueTime = value
	case "start-time":
		s.startTime = value
	}
	s.timeEditor = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (m *Model) clearTodoSchedule() {
	s := m.todoForm
	rows := m.todoEditorRows()
	if s == nil || len(rows) == 0 {
		return
	}
	key := editorRowKey(rows[nearestSelectableEditorCursor(rows, s.cursor)])
	dateKey := ""
	switch key {
	case "due-date", "due-time":
		s.dueDate = ""
		s.dueTime = ""
		dateKey = "due-date"
	case "start-date", "start-time":
		s.startDate = ""
		s.startTime = ""
		dateKey = "start-date"
	default:
		return
	}
	for i, row := range m.todoEditorRows() {
		if editorRowKey(row) == dateKey {
			s.cursor = i
			break
		}
	}
	s.errMsg = ""
}
