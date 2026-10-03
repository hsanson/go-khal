package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type mouseTarget uint8

const (
	mouseCalendarDay mouseTarget = iota + 1
	mouseCalendarPreviousYear
	mouseCalendarPreviousMonth
	mouseCalendarNextMonth
	mouseCalendarNextYear
	mouseCalendarRow
	mouseAgendaItem
	mouseMinimapEvent
	mouseMinimapTime
	mouseMinimapAllDay
	mouseEventEditorRow
	mouseTodoEditorRow
	mouseEditScope
	mouseDeleteScope
	mouseChoiceOption
	mouseAttendeeRow
	mouseNotificationRow
	mouseSearchOption
	mouseTimeDigit
	mouseDateDay
	mouseDatePreviousYear
	mouseDatePreviousMonth
	mouseDateNextMonth
	mouseDateNextYear
	mouseDateToday
	mouseDateMultiDay
	mouseDateClear
	mouseDialogApply
	mouseDialogCancel
	mouseDeleteConfirm
)

type mouseRect struct {
	x      int
	y      int
	width  int
	height int
}

type mouseHit struct {
	rect  mouseRect
	kind  mouseTarget
	day   time.Time
	index int
	value string
}

type mouseState struct {
	hits []mouseHit
}

func (s *mouseState) reset() {
	s.hits = s.hits[:0]
}

func (s *mouseState) add(hit mouseHit) {
	if hit.rect.width > 0 && hit.rect.height > 0 {
		s.hits = append(s.hits, hit)
	}
}

func (s *mouseState) at(x, y int) (mouseHit, bool) {
	for i := len(s.hits) - 1; i >= 0; i-- {
		hit := s.hits[i]
		if x >= hit.rect.x && x < hit.rect.x+hit.rect.width && y >= hit.rect.y && y < hit.rect.y+hit.rect.height {
			return hit, true
		}
	}
	return mouseHit{}, false
}

func (m Model) addMouseHit(hit mouseHit) {
	if m.mouse != nil {
		m.mouse.add(hit)
	}
}

func (m Model) updateMouse(event tea.MouseEvent) (tea.Model, tea.Cmd) {
	if event.Button == tea.MouseButtonWheelUp {
		return m.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if event.Button == tea.MouseButtonWheelDown {
		return m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if event.Action != tea.MouseActionPress || event.Button != tea.MouseButtonLeft || m.showHelpOverlay || m.mouse == nil {
		return m, nil
	}

	hit, ok := m.mouse.at(event.X, event.Y)
	if !ok {
		return m, nil
	}
	activeDialog := m.deleteConfirm != nil ||
		(m.eventForm != nil && m.eventForm.hasActiveDialog()) ||
		(m.todoForm != nil && (m.todoForm.activeForm != nil || m.todoForm.datePicker != nil || m.todoForm.timeEditor != nil))
	if activeDialog && !mouseDialogTarget(hit.kind) {
		return m, nil
	}

	switch hit.kind {
	case mouseCalendarDay:
		m.selected = dayStart(hit.day)
		m.agendaStart = m.selected
		m.eventCursor = 0
		m.eventListOffset = 0
		m.scrollForSelection()
		m.ensureEventSelectionValid()
	case mouseCalendarPreviousYear:
		m.navigateToDate(addMonthsClamped(m.selected, -12))
	case mouseCalendarPreviousMonth:
		m.navigateToDate(addMonthsClamped(m.selected, -1))
	case mouseCalendarNextMonth:
		m.navigateToDate(addMonthsClamped(m.selected, 1))
	case mouseCalendarNextYear:
		m.navigateToDate(addMonthsClamped(m.selected, 12))
	case mouseCalendarRow:
		if hit.index >= 0 && hit.index < len(m.calendarOrder) {
			m.calendarCursor = hit.index
			key := m.calendarOrder[hit.index]
			m.calendarVisibility[key] = !m.calendarVisibility[key]
			m.ensureEventSelectionValid()
		}
	case mouseAgendaItem:
		items := m.agendaItems()
		if hit.index < 0 || hit.index >= len(items) || (items[hit.index].Event == nil && items[hit.index].Todo == nil) {
			break
		}
		m.eventCursor = hit.index
		m.ensureEventSelectionValid()
		if m.openEditFormForSelected() {
			if m.eventForm != nil {
				return m, m.initCurrentEventForm()
			}
			if m.todoForm != nil {
				return m, m.todoForm.form.Init()
			}
		}
	case mouseMinimapEvent:
		if hit.index >= 0 && hit.index < len(m.data.Events) && m.openEventFormEdit(&m.data.Events[hit.index]) {
			return m, m.initCurrentEventForm()
		}
	case mouseMinimapTime:
		m.openEventFormNewAt(hit.day, hit.day.Add(30*time.Minute), false)
		return m, m.eventForm.form.Init()
	case mouseMinimapAllDay:
		start := dayStart(hit.day)
		m.openEventFormNewAt(start, start.AddDate(0, 0, 1), true)
		return m, m.eventForm.form.Init()
	case mouseEventEditorRow:
		if m.eventForm == nil || m.eventForm.mode == "view" {
			break
		}
		rows := m.eventEditorRows()
		if hit.index < 0 || hit.index >= len(rows) || !isEditorSelectable(rows[hit.index]) {
			break
		}
		m.eventForm.cursor = hit.index
		if m.toggleEventEditorBoolean() {
			break
		}
		return m, m.openEventEditorForm()
	case mouseTodoEditorRow:
		if m.todoForm == nil || m.todoForm.mode == "view" {
			break
		}
		rows := m.todoEditorRows()
		if hit.index < 0 || hit.index >= len(rows) || !isEditorSelectable(rows[hit.index]) {
			break
		}
		m.todoForm.cursor = hit.index
		if m.toggleTodoEditorBoolean() {
			break
		}
		return m, m.openTodoEditorForm()
	case mouseChoiceOption:
		if m.eventForm != nil && m.eventForm.choicePicker != nil {
			m.eventForm.choicePicker.choose(hit.index)
		} else if m.todoForm != nil && m.todoForm.choicePicker != nil {
			m.todoForm.choicePicker.choose(hit.index)
		}
	case mouseAttendeeRow:
		if m.eventForm != nil {
			cycleAttendeeState(m.eventForm.attendeeManager, hit.index)
		}
	case mouseNotificationRow:
		if m.eventForm != nil && m.eventForm.notificationManager != nil {
			m.eventForm.notificationManager.cycle(hit.index)
		}
	case mouseSearchOption:
		if m.eventForm != nil && m.eventForm.searchPicker != nil {
			picker := m.eventForm.searchPicker
			if hit.index >= 0 && hit.index < len(picker.filtered) {
				picker.cursor = hit.index
				picker.ensureVisible()
				picker.selected = picker.options[picker.filtered[hit.index]].value
			}
		}
	case mouseTimeDigit:
		if editor := m.activeTimeEditor(); editor != nil && hit.index >= 0 && hit.index < editor.slotCount {
			editor.cursor = hit.index
		}
	case mouseDateDay:
		if picker := m.activeDatePicker(); picker != nil {
			picker.selectByMouse(hit.day)
		}
	case mouseDatePreviousYear:
		if picker := m.activeDatePicker(); picker != nil {
			picker.moveMonth(-12)
		}
	case mouseDatePreviousMonth:
		if picker := m.activeDatePicker(); picker != nil {
			picker.moveMonth(-1)
		}
	case mouseDateNextMonth:
		if picker := m.activeDatePicker(); picker != nil {
			picker.moveMonth(1)
		}
	case mouseDateNextYear:
		if picker := m.activeDatePicker(); picker != nil {
			picker.moveMonth(12)
		}
	case mouseDateToday:
		if picker := m.activeDatePicker(); picker != nil {
			picker.selectToday()
		}
	case mouseDateMultiDay:
		if picker := m.activeDatePicker(); picker != nil && !picker.singleDate {
			picker.toggleRange()
		}
	case mouseDateClear:
		if picker := m.activeDatePicker(); picker != nil && picker.clearable {
			picker.cleared = true
		}
	case mouseDialogApply:
		if m.eventForm != nil && m.eventForm.hasActiveFieldDialog() {
			return m, m.applyEventDialog()
		}
		if m.todoForm != nil && m.todoForm.hasActiveFieldDialog() {
			return m, m.applyTodoDialog()
		}
	case mouseDialogCancel:
		if m.eventForm != nil && m.eventForm.hasActiveFieldDialog() {
			m.eventForm.cancelActive()
		} else if m.todoForm != nil && m.todoForm.hasActiveFieldDialog() {
			m.todoForm.cancelActive()
		}
	case mouseEditScope:
		if m.eventForm != nil && m.eventForm.activeKey == "edit-scope" {
			m.eventForm.editScope = hit.value
			m.eventForm.activeForm = nil
			m.eventForm.activeKey = ""
			m.eventForm.backup = nil
		}
	case mouseDeleteScope:
		if m.deleteConfirm != nil && m.deleteConfirm.stage == "scope" {
			m.deleteConfirm.scope = hit.value
			return m.finishDeleteConfirm()
		}
	case mouseDeleteConfirm:
		if m.deleteConfirm != nil && m.deleteConfirm.stage == "confirm" {
			m.deleteConfirm.confirm = hit.value == "true"
			return m.finishDeleteConfirm()
		}
	}
	return m, nil
}

func mouseDialogTarget(kind mouseTarget) bool {
	switch kind {
	case mouseEditScope, mouseDeleteScope, mouseDeleteConfirm,
		mouseChoiceOption, mouseAttendeeRow, mouseNotificationRow, mouseSearchOption,
		mouseTimeDigit, mouseDateDay, mouseDatePreviousYear, mouseDatePreviousMonth, mouseDateNextMonth, mouseDateNextYear,
		mouseDateToday, mouseDateMultiDay, mouseDateClear, mouseDialogApply, mouseDialogCancel:
		return true
	default:
		return false
	}
}

func (m Model) activeDatePicker() *dateRangePicker {
	if m.eventForm != nil && m.eventForm.datePicker != nil {
		return m.eventForm.datePicker
	}
	if m.todoForm != nil {
		return m.todoForm.datePicker
	}
	return nil
}

func (m Model) activeTimeEditor() *timeRangeEditor {
	if m.eventForm != nil && m.eventForm.timeEditor != nil {
		return m.eventForm.timeEditor
	}
	if m.todoForm != nil {
		return m.todoForm.timeEditor
	}
	return nil
}
