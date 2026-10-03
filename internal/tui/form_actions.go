package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dialogFocus uint8

const (
	dialogFocusControl dialogFocus = iota
	dialogFocusApply
	dialogFocusCancel
)

func dialogWithActions(content string, focus dialogFocus, styles Styles) (string, []mouseHit) {
	button := func(label string, selected bool) string {
		style := styles.SecondaryButton
		if label == "Apply" {
			style = styles.PrimaryButton
		}
		if selected {
			style = styles.FocusedButton
		}
		return style.Render(label)
	}
	apply := button("Apply", focus == dialogFocusApply)
	cancel := button("Cancel", focus == dialogFocusCancel)
	y := lipgloss.Height(content) + 1
	applyWidth := lipgloss.Width(apply)
	return lipgloss.JoinVertical(lipgloss.Left, content, "", apply+"  "+cancel), []mouseHit{
		{rect: mouseRect{x: 0, y: y, width: applyWidth, height: 1}, kind: mouseDialogApply},
		{rect: mouseRect{x: applyWidth + 2, y: y, width: lipgloss.Width(cancel), height: 1}, kind: mouseDialogCancel},
	}
}

func addDialogActions(content string, hits []mouseHit, focus dialogFocus, styles Styles) (string, []mouseHit) {
	view, actionHits := dialogWithActions(content, focus, styles)
	return view, append(hits, actionHits...)
}

func updateDialogFocus(focus *dialogFocus, key string) (action string, handled bool) {
	if focus == nil {
		return "", false
	}
	switch key {
	case "tab":
		*focus = (*focus + 1) % 3
		return "", true
	case "shift+tab":
		*focus = (*focus + 2) % 3
		return "", true
	case "left", "h":
		if *focus == dialogFocusApply || *focus == dialogFocusCancel {
			*focus = dialogFocusApply
			return "", true
		}
	case "right", "l":
		if *focus == dialogFocusApply || *focus == dialogFocusCancel {
			*focus = dialogFocusCancel
			return "", true
		}
	case "enter", " ", "space":
		if *focus == dialogFocusApply {
			return "apply", true
		}
		if *focus == dialogFocusCancel {
			return "cancel", true
		}
	}
	if *focus != dialogFocusControl && key != "esc" && key != "ctrl+c" && key != "q" && key != "ctrl+s" {
		return "", true
	}
	return "", false
}

func (m *Model) updateEventDialogFocus(msg tea.KeyMsg) (bool, tea.Cmd) {
	s := m.eventForm
	if s == nil || !s.hasActiveFieldDialog() {
		return false, nil
	}
	action, handled := updateDialogFocus(&s.dialogFocus, msg.String())
	if !handled {
		return false, nil
	}
	switch action {
	case "apply":
		return true, m.applyEventDialog()
	case "cancel":
		s.cancelActive()
	}
	return true, nil
}

func (m *Model) updateTodoDialogFocus(msg tea.KeyMsg) (bool, tea.Cmd) {
	s := m.todoForm
	if s == nil || !s.hasActiveFieldDialog() {
		return false, nil
	}
	action, handled := updateDialogFocus(&s.dialogFocus, msg.String())
	if !handled {
		return false, nil
	}
	switch action {
	case "apply":
		return true, m.applyTodoDialog()
	case "cancel":
		s.cancelActive()
	}
	return true, nil
}

func (m *Model) applyEventDialog() tea.Cmd {
	s := m.eventForm
	if s == nil {
		return nil
	}
	switch {
	case s.attendeeManager != nil:
		s.attendees = attendeesInput(attendeeManagerValues(s.attendeeManager))
		s.attendeeManager = nil
	case s.notificationManager != nil:
		s.alarms = strings.Join(s.notificationManager.values(), "; ")
		s.notificationManager = nil
	case s.choicePicker != nil:
		m.applyEventChoicePicker()
		return nil
	case s.datePicker != nil:
		s.datePicker.done = true
		m.updateEventDatePicker(tea.KeyMsg{})
		return nil
	case s.timeEditor != nil:
		m.updateEventTimeEditor(tea.KeyMsg{Type: tea.KeyEnter})
		return nil
	case s.searchPicker != nil:
		if len(s.searchPicker.filtered) > 0 && s.searchPicker.selected == "" {
			s.searchPicker.selected = s.searchPicker.options[s.searchPicker.filtered[s.searchPicker.cursor]].value
		}
		s.searchPicker.done = true
		m.updateEventSearchPicker(tea.KeyMsg{})
		return nil
	case s.activeForm != nil:
		return m.updateActiveEventEditorForm(tea.KeyMsg{Type: tea.KeyEnter})
	}
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
	s.dialogFocus = dialogFocusControl
	return nil
}

func (m *Model) applyTodoDialog() tea.Cmd {
	s := m.todoForm
	if s == nil {
		return nil
	}
	switch {
	case s.choicePicker != nil:
		m.applyTodoChoicePicker()
		return nil
	case s.datePicker != nil:
		s.datePicker.done = true
		m.updateTodoDatePicker(tea.KeyMsg{})
		return nil
	case s.timeEditor != nil:
		m.updateTodoTimeEditor(tea.KeyMsg{Type: tea.KeyEnter})
		return nil
	case s.activeForm != nil:
		return m.updateActiveTodoEditorForm(tea.KeyMsg{Type: tea.KeyEnter})
	}
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
	s.dialogFocus = dialogFocusControl
	return nil
}

func (s *eventFormState) hasActiveFieldDialog() bool {
	return s != nil && s.activeKey != "edit-scope" && (s.activeForm != nil || s.attendeeManager != nil || s.notificationManager != nil || s.choicePicker != nil || s.datePicker != nil || s.timeEditor != nil || s.searchPicker != nil)
}

func (s *todoFormState) hasActiveFieldDialog() bool {
	return s != nil && (s.activeForm != nil || s.choicePicker != nil || s.datePicker != nil || s.timeEditor != nil)
}
