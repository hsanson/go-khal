package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
)

type choicePicker struct {
	title    string
	choices  []editorChoice
	selected string
	checked  map[string]bool
	cursor   int
	multi    bool
}

func newSingleChoicePicker(title string, choices []editorChoice, selected string) *choicePicker {
	picker := &choicePicker{title: title, choices: append([]editorChoice(nil), choices...), selected: selected}
	for i, choice := range picker.choices {
		if choice.value == selected {
			picker.cursor = i
			break
		}
	}
	return picker
}

func newMultiChoicePicker(title string, choices []editorChoice, selected []string) *choicePicker {
	checked := make(map[string]bool, len(selected))
	for _, value := range selected {
		checked[value] = true
	}
	return &choicePicker{title: title, choices: append([]editorChoice(nil), choices...), checked: checked, multi: true}
}

func (p *choicePicker) move(delta int) {
	if p == nil || len(p.choices) == 0 {
		return
	}
	p.cursor = (p.cursor + delta + len(p.choices)) % len(p.choices)
}

func (p *choicePicker) choose(index int) {
	if p == nil || index < 0 || index >= len(p.choices) {
		return
	}
	p.cursor = index
	value := p.choices[index].value
	if p.multi {
		p.checked[value] = !p.checked[value]
		return
	}
	p.selected = value
}

func (p *choicePicker) values() []string {
	if p == nil || !p.multi {
		return nil
	}
	values := make([]string, 0, len(p.checked))
	for _, choice := range p.choices {
		if p.checked[choice.value] {
			values = append(values, choice.value)
		}
	}
	return values
}

func (p *choicePicker) render(styles Styles) (string, []mouseHit) {
	if p == nil {
		return "", nil
	}
	lines := []string{styles.PanelTitle.Render(p.title), ""}
	hits := make([]mouseHit, 0, len(p.choices))
	for i, choice := range p.choices {
		mark := "( )"
		selected := choice.value == p.selected
		if p.multi {
			mark = "[ ]"
			selected = p.checked[choice.value]
		}
		if selected {
			if p.multi {
				mark = "[x]"
			} else {
				mark = "(•)"
			}
		}
		prefix := "  "
		if i == p.cursor {
			prefix = " "
		}
		line := prefix + mark + " " + choice.label
		if i == p.cursor {
			line = styles.Accent.Render(line)
		}
		hits = append(hits, mouseHit{
			rect:  mouseRect{x: 0, y: len(lines), width: max(1, len(prefix+mark+" "+choice.label)), height: 1},
			kind:  mouseChoiceOption,
			index: i,
		})
		lines = append(lines, line)
	}
	legend := " Focused  (•) Selected  ( ) Not selected"
	if p.multi {
		legend = " Focused  [x] Selected  [ ] Not selected"
	}
	lines = append(lines, "", styles.Subtle.Render(legend))
	return strings.Join(lines, "\n"), hits
}

func (m Model) calendarEditorChoices() []editorChoice {
	labels := m.calendarOptionLabels()
	choices := make([]editorChoice, 0, len(m.calendarOrder))
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		choices = append(choices, editorChoice{labels[key], key})
	}
	if len(choices) == 0 {
		choices = append(choices, editorChoice{"No writable calendar", ""})
	}
	return choices
}

func (m Model) newEventChoicePicker(key string) *choicePicker {
	s := m.eventForm
	if s == nil {
		return nil
	}
	switch key {
	case "calendar":
		return newSingleChoicePicker("Calendar", m.calendarEditorChoices(), s.calendarKey)
	case "rsvp":
		return newSingleChoicePicker("RSVP", eventRSVPChoices[:], s.rsvp)
	case "availability":
		return newSingleChoicePicker("Availability", eventAvailabilityChoices[:], s.availability)
	case "visibility":
		return newSingleChoicePicker("Visibility", eventVisibilityChoices[:], s.visibility)
	case "recur":
		return newSingleChoicePicker("Repeat", eventRepeatChoices[:], repeatValue(s))
	case "recur-weekdays":
		return newMultiChoicePicker("Weekday", []editorChoice{{"Monday", "Mo"}, {"Tuesday", "Tu"}, {"Wednesday", "We"}, {"Thursday", "Th"}, {"Friday", "Fr"}, {"Saturday", "Sa"}, {"Sunday", "Su"}}, s.recurWeekdays)
	case "recur-monthly-by":
		return newSingleChoicePicker("By", eventMonthlyByChoices[:], s.recurMonthlyBy)
	case "recur-end":
		return newSingleChoicePicker("Until", eventRepeatEndChoices[:], s.recurEnd)
	}
	return nil
}

func (m Model) newTodoChoicePicker(key string) *choicePicker {
	s := m.todoForm
	if s == nil {
		return nil
	}
	switch key {
	case "calendar":
		return newSingleChoicePicker("Calendar", m.calendarEditorChoices(), s.calendarKey)
	case "priority":
		return newSingleChoicePicker("Priority", todoPriorityChoices[:], s.priorityLabel)
	}
	return nil
}

func (m *Model) applyEventChoicePicker() {
	s := m.eventForm
	if s == nil || s.choicePicker == nil {
		return
	}
	picker := s.choicePicker
	switch s.activeKey {
	case "calendar":
		s.calendarKey = picker.selected
	case "rsvp":
		s.rsvp = picker.selected
	case "availability":
		s.availability = picker.selected
	case "visibility":
		s.visibility = picker.selected
	case "recur":
		setEventRepeat(s, picker.selected)
	case "recur-weekdays":
		s.recurWeekdays = picker.values()
	case "recur-monthly-by":
		s.recurMonthlyBy = picker.selected
	case "recur-end":
		s.recurEnd = picker.selected
	}
	s.choicePicker = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
	s.dialogFocus = dialogFocusControl
}

func (m *Model) applyTodoChoicePicker() {
	s := m.todoForm
	if s == nil || s.choicePicker == nil {
		return
	}
	switch s.activeKey {
	case "calendar":
		s.calendarKey = s.choicePicker.selected
	case "priority":
		s.priorityLabel = s.choicePicker.selected
	}
	s.choicePicker = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
	s.dialogFocus = dialogFocusControl
}

func updateChoicePicker(picker *choicePicker, msg tea.KeyMsg) (apply, cancel bool) {
	if picker == nil {
		return false, false
	}
	switch msg.String() {
	case "esc", "ctrl+c", "q":
		return false, true
	case "j", "down":
		picker.move(1)
	case "k", "up":
		picker.move(-1)
	case " ", "space":
		picker.choose(picker.cursor)
	case "enter":
		if !picker.multi {
			picker.choose(picker.cursor)
		}
		return true, false
	}
	return false, false
}
