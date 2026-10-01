package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
)

func (m *Model) openCustomEventEditor(key string) bool {
	s := m.eventForm
	if s == nil {
		return true
	}
	switch key {
	case "date":
		s.datePicker = newDateRangePicker(s.fromDate, s.toDate)
	case "time":
		s.timeEditor = newTimeRangeEditor(s.fromTime, s.toTime)
	case "timezone":
		at, _ := time.Parse("2006-01-02 15:04", s.fromDate+" "+s.fromTime)
		s.searchPicker = newSearchPicker("Timezone", timezoneOptions(s.timezone, localIANATimezone(), at), s.timezone)
	case "attendees-add":
		s.searchPicker = newSearchPicker("Add attendee", m.attendeeSearchOptions(), "")
	case "form-cancel":
		m.closeEventEditor()
	case "form-save":
		if err := m.saveEventEditor(); err != nil {
			s.errMsg = err.Error()
		}
	default:
		return false
	}
	return true
}

func (m *Model) updateEventDatePicker(msg tea.KeyMsg) {
	s := m.eventForm
	picker := s.datePicker
	picker.Update(msg)
	if picker.cancelled {
		s.cancelActive()
		return
	}
	if !picker.done {
		return
	}
	start, end := picker.dates()
	s.fromDate = start.Format("2006-01-02")
	s.toDate = end.Format("2006-01-02")
	s.timingDirty = true
	s.overnightAuto = false
	s.datePicker = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (m *Model) updateEventTimeEditor(msg tea.KeyMsg) {
	s := m.eventForm
	editor := s.timeEditor
	editor.Update(msg)
	if editor.cancelled {
		s.cancelActive()
		return
	}
	if !editor.done {
		return
	}
	startTime, endTime, err := editor.values()
	if err != nil {
		s.errMsg = err.Error()
		return
	}
	s.fromTime = startTime
	s.toTime = endTime
	s.adjustOvernightRange()
	s.timingDirty = true
	s.timeEditor = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (m *Model) updateEventSearchPicker(msg tea.KeyMsg) {
	s := m.eventForm
	picker := s.searchPicker
	picker.Update(msg)
	if picker.cancelled {
		s.cancelActive()
		return
	}
	if !picker.done {
		return
	}
	switch s.activeKey {
	case "attendees-add":
		s.attendees = mergeListInput(s.attendees, []string{picker.selected})
	case "timezone":
		s.timezone = picker.selected
		s.timezoneLocal = false
		s.timingDirty = true
	}
	s.searchPicker = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (s *eventFormState) adjustOvernightRange() {
	startDate, startDateErr := time.Parse("2006-01-02", s.fromDate)
	endDate, endDateErr := time.Parse("2006-01-02", s.toDate)
	startTime, startTimeErr := time.Parse("15:04", s.fromTime)
	endTime, endTimeErr := time.Parse("15:04", s.toTime)
	if startDateErr != nil || endDateErr != nil || startTimeErr != nil || endTimeErr != nil {
		return
	}
	endNotAfterStart := endTime.Hour() < startTime.Hour() || endTime.Hour() == startTime.Hour() && endTime.Minute() <= startTime.Minute()
	if sameDate(startDate, endDate) && endNotAfterStart {
		s.toDate = startDate.AddDate(0, 0, 1).Format("2006-01-02")
		s.overnightAuto = true
		return
	}
	if s.overnightAuto && sameDate(endDate, startDate.AddDate(0, 0, 1)) && !endNotAfterStart {
		s.toDate = s.fromDate
		s.overnightAuto = false
	}
}

func (m Model) attendeeSearchOptions() []searchOption {
	if m.store == nil || m.eventForm == nil {
		return nil
	}
	existing := make(map[string]bool)
	for _, attendee := range parseAttendeesInput(m.eventForm.attendees) {
		existing[strings.ToLower(strings.TrimSpace(attendee.Email))] = true
	}
	contacts, err := m.store.Contacts()
	if err != nil {
		return nil
	}
	options := make([]searchOption, 0, len(contacts))
	for _, contact := range contacts {
		email := strings.TrimSpace(contact.Email)
		if email == "" || existing[strings.ToLower(email)] {
			continue
		}
		label := email
		if name := strings.TrimSpace(contact.Name); name != "" && !strings.EqualFold(name, email) {
			label = fmt.Sprintf("%s <%s>", name, email)
		}
		options = append(options, searchOption{label: label, value: label})
	}
	return options
}

func (m *Model) saveEventEditor() error {
	if err := m.commitEventForm(); err != nil {
		return err
	}
	m.closeEventEditor()
	m.ensureEventSelectionValid()
	return nil
}

func (m *Model) closeEventEditor() {
	m.eventForm = nil
	m.focusDetails = false
	m.focusMain = true
}

func (m *Model) saveTodoEditor() error {
	if err := m.commitTodoForm(); err != nil {
		return err
	}
	m.closeTodoEditor()
	m.ensureEventSelectionValid()
	return nil
}

func (m *Model) closeTodoEditor() {
	m.todoForm = nil
	m.focusDetails = false
	m.focusMain = true
}

func eventFormTimezone(ev calendar.Event, mode string) (timezone string, local bool) {
	localTimezone := localIANATimezone()
	if mode == "create" && ev.Timezone != "" && ev.Timezone != calendar.EventTimezoneFloating {
		return ev.Timezone, false
	}
	if ev.Timezone == "" || ev.Timezone == calendar.EventTimezoneUTC || ev.Timezone == calendar.EventTimezoneFloating {
		return localTimezone, mode == "edit"
	}
	if _, err := time.LoadLocation(ev.Timezone); err != nil {
		return localTimezone, mode == "edit"
	}
	return ev.Timezone, false
}
