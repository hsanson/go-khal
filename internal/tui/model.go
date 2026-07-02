package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

type Model struct {
	store              *calendar.Store
	cfg                *config.Config
	data               calendar.Dataset
	styles             Styles
	selected           time.Time
	width              int
	height             int
	calendarVisibility map[string]bool
	calendarOrder      []string
	weekViewportStart  time.Time
	focusCalendarPane  bool
	calendarCursor     int
	calendarOffset     int
	focusMain          bool
	focusDetails       bool
	showFreeMode       bool
	showTasksMode      bool
	agendaStart        time.Time
	eventCursor        int
	eventListOffset    int
	detailScroll       int
	eventForm          *eventFormState
	todoForm           *todoFormState
	showHelpOverlay    bool
}

type eventFormState struct {
	mode        string
	targetUID   string
	form        *huh.Form
	summary     string
	calendarKey string
	location    string
	description string
	url         string
	allDay      bool
	fromDate    string
	fromTime    string
	toDate      string
	toTime      string
	errMsg      string
}

type todoFormState struct {
	mode          string
	targetUID     string
	form          *huh.Form
	summary       string
	description   string
	location      string
	calendarKey   string
	startDate     string
	startTime     string
	dueDate       string
	dueTime       string
	completed     bool
	priorityLabel string
	errMsg        string
}

var eventFormFieldOrder = []string{
	"title",
	"calendar",
	"location",
	"description",
	"url",
	"all-day",
	"from-date",
	"from-time",
	"to-date",
	"to-time",
}

var todoFormFieldOrder = []string{
	"summary",
	"description",
	"location",
	"calendar",
	"start-date",
	"start-time",
	"due-date",
	"due-time",
	"completed",
	"priority",
}

func NewModel(cfg *config.Config, data calendar.Dataset, store *calendar.Store) Model {
	vis := map[string]bool{}
	order := make([]string, 0, len(data.Calendars))
	for _, cal := range data.Calendars {
		k := calendarKey(cal.Source, cal.Name)
		vis[k] = !cal.Hidden
		order = append(order, k)
	}
	sort.Strings(order)
	birthdayKey := calendarKey(calendar.SpecialSourceBirthdays, calendar.SpecialCalendarBirthdays)
	for i, k := range order {
		if k != birthdayKey {
			continue
		}
		copy(order[1:i+1], order[0:i])
		order[0] = birthdayKey
		break
	}

	now := time.Now()
	start := calendar.StartOfWeek(now, cfg.WeekStart())
	m := Model{
		store:              store,
		cfg:                cfg,
		data:               data,
		styles:             DefaultStyles(),
		selected:           now,
		agendaStart:        dayStart(now),
		weekViewportStart:  start,
		calendarVisibility: vis,
		calendarOrder:      order,
		focusMain:          true,
	}
	m.ensureEventSelectionValid()
	m.ensureCalendarCursorVisible(m.calendarPaneHeight())
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollForSelection()
	case tea.KeyMsg:
		if msg.String() == "?" {
			m.showHelpOverlay = !m.showHelpOverlay
			return m, nil
		}
		if m.showHelpOverlay {
			if msg.String() == "q" || msg.String() == "esc" {
				m.showHelpOverlay = false
				return m, nil
			}
			return m, nil
		}
		if (msg.String() == "q" || msg.String() == "ctrl+c") && m.eventForm == nil && m.todoForm == nil {
			return m, tea.Quit
		}

		if m.eventForm != nil {
			switch msg.String() {
			case "ctrl+s":
				if err := m.commitEventForm(); err != nil {
					m.eventForm.errMsg = err.Error()
					return m, nil
				}
				m.eventForm = nil
				m.focusDetails = false
				m.focusMain = true
				m.ensureEventSelectionValid()
				return m, nil
			case "ctrl+c":
				m.eventForm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			case "tab":
				if m.isEventFormFocusedLastField() {
					return m, nil
				}
				m.eventForm.form.UpdateFieldPositions()
				return m, m.eventForm.form.NextField()
			case "shift+tab":
				if m.isEventFormFocusedFirstField() {
					return m, nil
				}
				m.eventForm.form.UpdateFieldPositions()
				return m, m.eventForm.form.PrevField()
			case "esc":
				m.eventForm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			}
			updated, cmd := m.eventForm.form.Update(msg)
			if fm, ok := updated.(*huh.Form); ok {
				m.eventForm.form = fm
			}
			m.eventForm.form.UpdateFieldPositions()
			if m.eventForm.form.GetFocusedField() == nil {
				cmd = tea.Batch(cmd, m.eventForm.form.Init())
			}
			if m.eventForm.form.State == huh.StateAborted {
				m.eventForm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			}
			if m.eventForm.form.State == huh.StateCompleted {
				m.eventForm.form.State = huh.StateNormal
			}
			return m, cmd
		}
		if m.todoForm != nil {
			switch msg.String() {
			case "ctrl+s":
				if err := m.commitTodoForm(); err != nil {
					m.todoForm.errMsg = err.Error()
					return m, nil
				}
				m.todoForm = nil
				m.focusDetails = false
				m.focusMain = true
				m.ensureEventSelectionValid()
				return m, nil
			case "ctrl+c", "esc":
				m.todoForm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			case "tab":
				if m.isTodoFormFocusedLastField() {
					return m, nil
				}
				m.todoForm.form.UpdateFieldPositions()
				return m, m.todoForm.form.NextField()
			case "shift+tab":
				if m.isTodoFormFocusedFirstField() {
					return m, nil
				}
				m.todoForm.form.UpdateFieldPositions()
				return m, m.todoForm.form.PrevField()
			}
			updated, cmd := m.todoForm.form.Update(msg)
			if fm, ok := updated.(*huh.Form); ok {
				m.todoForm.form = fm
			}
			m.todoForm.form.UpdateFieldPositions()
			if m.todoForm.form.GetFocusedField() == nil {
				cmd = tea.Batch(cmd, m.todoForm.form.Init())
			}
			if m.todoForm.form.State == huh.StateAborted {
				m.todoForm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			}
			if m.todoForm.form.State == huh.StateCompleted {
				m.todoForm.form.State = huh.StateNormal
			}
			return m, cmd
		}

		if msg.String() == "c" {
			m.focusCalendarPane = true
			m.ensureCalendarCursorVisible(m.calendarPaneHeight())
			m.ensureEventSelectionValid()
			return m, nil
		}
		if m.focusDetails {
			switch msg.String() {
			case "esc", "enter", " ":
				m.focusDetails = false
				m.focusMain = true
				m.detailScroll = 0
			case "e":
				if m.openEventFormEditSelected() {
					return m, m.eventForm.form.Init()
				}
			case "j", "down":
				m.detailScroll++
			case "k", "up":
				if m.detailScroll > 0 {
					m.detailScroll--
				}
			}
			return m, nil
		}
		if m.focusCalendarPane {
			switch msg.String() {
			case "j", "down":
				m.moveCalendarCursor(1)
			case "k", "up":
				m.moveCalendarCursor(-1)
			case " ", "enter":
				if len(m.calendarOrder) > 0 {
					key := m.calendarOrder[m.calendarCursor]
					m.calendarVisibility[key] = !m.calendarVisibility[key]
					m.ensureEventSelectionValid()
				}
			case "esc", "h":
				m.focusCalendarPane = false
			}
			m.ensureCalendarCursorVisible(m.calendarPaneHeight())
			return m, nil
		}
		switch msg.String() {
		case "j", "down":
			m.moveEventCursor(1)
		case "k", "up":
			m.moveEventCursor(-1)
		case "ctrl+f":
			m.moveEventCursor(m.eventPageStep())
		case "ctrl+b":
			m.moveEventCursor(-m.eventPageStep())
		case "ctrl+j":
			m.detailScroll++
		case "ctrl+k":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
		case "h", "left":
			m.selected = m.selected.AddDate(0, 0, -1)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "l", "right":
			m.selected = m.selected.AddDate(0, 0, 1)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "ctrl+h":
			m.selected = m.selected.AddDate(0, 0, -7)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "ctrl+l":
			m.selected = m.selected.AddDate(0, 0, 7)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "n":
			if m.showTasksMode {
				m.openTodoFormNew()
				return m, m.todoForm.form.Init()
			}
			m.openEventFormNew()
			return m, m.eventForm.form.Init()
		case "e":
			if m.openEditFormForSelected() {
				if m.eventForm != nil {
					return m, m.eventForm.form.Init()
				}
				if m.todoForm != nil {
					return m, m.todoForm.form.Init()
				}
			}
		case "f":
			m.showFreeMode = !m.showFreeMode
			m.eventCursor = 0
			m.eventListOffset = 0
			m.ensureEventSelectionValid()
		case "m":
			m.showTasksMode = !m.showTasksMode
			m.eventCursor = 0
			m.eventListOffset = 0
			m.ensureEventSelectionValid()
		case "t":
			now := time.Now().In(m.selected.Location())
			m.selected = now
			m.agendaStart = dayStart(now)
			m.weekViewportStart = calendar.StartOfWeek(now, m.weekStart())
			m.focusMain = false
			m.focusDetails = false
			m.eventCursor = 0
			m.eventListOffset = 0
			m.detailScroll = 0
		case "enter":
			m.detailScroll = 0
		case " ":
			m.detailScroll = 0
		}
		m.ensureEventSelectionValid()
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		m.width = 140
	}
	if m.height == 0 {
		m.height = 42
	}

	leftWidth := m.sidebarWidth()
	rightWidth := max(50, m.width-leftWidth-5)
	left := m.renderLeftPanel(leftWidth)
	right := m.renderMainPanel(rightWidth)

	root := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	legend := m.styles.Subtle.Render("[q] quit  [?] shortcuts")
	base := m.styles.Container.Render(lipgloss.JoinVertical(lipgloss.Left, legend, "", root))
	if m.showHelpOverlay {
		overlay := m.renderHelpOverlay(max(40, m.width*2/3), max(16, m.height*2/3))
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
	}
	return base
}

func (m Model) renderLeftPanel(width int) string {
	panelHeight := m.height - 6
	if panelHeight < 8 {
		panelHeight = 8
	}
	topHeight := panelHeight * 2 / 3
	if topHeight < 8 {
		topHeight = 8
	}
	bottomHeight := panelHeight - topHeight - 1
	if bottomHeight < 4 {
		bottomHeight = 4
		topHeight = panelHeight - bottomHeight - 1
	}
	weeks := renderWeekViewport(m.weekViewportStart, m.selected, filteredEvents(m.data.Events, m.calendarVisibility), width-2, max(3, topHeight-2), m.weekStart(), m.styles)
	calPane := m.renderCalendarListPane(width-2, bottomHeight)
	divider := m.styles.Subtle.Render(strings.Repeat("-", max(8, width-2)))
	panel := lipgloss.JoinVertical(lipgloss.Left, m.styles.PanelTitle.Render("Calendar"), weeks, divider, calPane)
	return m.styles.Sidebar.Width(width).Height(panelHeight).Render(panel)
}

func (m Model) renderMainPanel(width int) string {
	panelHeight := m.height - 6
	if panelHeight < 10 {
		panelHeight = 10
	}
	if m.eventForm != nil {
		return m.renderEventFormMainPanel(width, panelHeight)
	}
	if m.todoForm != nil {
		return m.renderTodoFormMainPanel(width, panelHeight)
	}

	items := m.agendaItems()
	available := panelHeight - 4
	if available < 8 {
		available = 8
	}
	topHeight := available * 3 / 5
	if topHeight < 5 {
		topHeight = 6
	}
	bottomHeight := available - topHeight - 1
	if bottomHeight < 4 {
		bottomHeight = 4
		topHeight = available - bottomHeight - 1
		if topHeight < 3 {
			topHeight = 3
		}
	}

	rendered := renderAgendaFromItems(items, width-2, topHeight, m.cfg.TimeFormat, m.styles, m.eventCursor, m.eventListOffset, true)
	top := lipgloss.NewStyle().Height(topHeight).MaxHeight(topHeight).Render(rendered.Text)
	detail := m.renderEventDetailsPane(width-2, bottomHeight)
	header := m.styles.PanelTitle.Render(fmt.Sprintf("Agenda from %s", m.agendaStart.Format("Mon Jan 2, 2006")))
	if m.showTasksMode {
		header += m.styles.Subtle.Render(" [TASKS]")
	}
	if m.showFreeMode {
		header += m.styles.Subtle.Render(" [SHOW-FREE]")
	}
	separator := m.styles.Subtle.Render(strings.Repeat("-", max(10, width-2)))
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", top, separator, detail)
	return m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
}

func (m Model) renderEventFormMainPanel(width, panelHeight int) string {
	if m.eventForm == nil {
		return m.styles.MainPanel.Width(width).Height(panelHeight).Render("")
	}
	header := m.styles.PanelTitle.Render("Event Form")
	if m.eventForm.mode == "edit" {
		header = m.styles.PanelTitle.Render("Edit Event")
	}
	if f := m.eventForm.form.GetFocusedField(); f != nil {
		if key := strings.TrimSpace(f.GetKey()); key != "" {
			header += m.styles.Subtle.Render("  [focused: " + key + "]")
		}
	}
	innerHeight := panelHeight - 2
	if innerHeight < 6 {
		innerHeight = 6
	}
	formView := m.eventForm.form.WithWidth(width - 2).WithHeight(innerHeight).WithShowHelp(true).WithShowErrors(true).View()
	if strings.TrimSpace(m.eventForm.errMsg) != "" {
		formView = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.eventForm.errMsg), "", formView)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", formView)
	return m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
}

func (m Model) renderTodoFormMainPanel(width, panelHeight int) string {
	if m.todoForm == nil {
		return m.styles.MainPanel.Width(width).Height(panelHeight).Render("")
	}
	header := m.styles.PanelTitle.Render("Task Form")
	if m.todoForm.mode == "edit" {
		header = m.styles.PanelTitle.Render("Edit Task")
	}
	if f := m.todoForm.form.GetFocusedField(); f != nil {
		if key := strings.TrimSpace(f.GetKey()); key != "" {
			header += m.styles.Subtle.Render("  [focused: " + key + "]")
		}
	}
	innerHeight := panelHeight - 2
	if innerHeight < 6 {
		innerHeight = 6
	}
	formView := m.todoForm.form.WithWidth(width - 2).WithHeight(innerHeight).WithShowHelp(true).WithShowErrors(true).View()
	if strings.TrimSpace(m.todoForm.errMsg) != "" {
		formView = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.todoForm.errMsg), "", formView)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", formView)
	return m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
}

func (m Model) renderEventDetailsPane(width, height int) string {
	if m.eventForm != nil {
		view := m.eventForm.form.WithWidth(width).WithHeight(height).WithShowHelp(true).WithShowErrors(true).View()
		if strings.TrimSpace(m.eventForm.errMsg) != "" {
			view = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.eventForm.errMsg), "", view)
		}
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(view)
	}
	if m.todoForm != nil {
		view := m.todoForm.form.WithWidth(width).WithHeight(height).WithShowHelp(true).WithShowErrors(true).View()
		if strings.TrimSpace(m.todoForm.errMsg) != "" {
			view = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.todoForm.errMsg), "", view)
		}
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(view)
	}

	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return lipgloss.NewStyle().Width(width).Height(height).Render(m.styles.Subtle.Render("No item selected"))
	}
	it := items[m.eventCursor]
	if it.IsFree {
		msg := fmt.Sprintf("Free time: %s - %s", it.Start.Format("2006-01-02 15:04"), it.End.Format("2006-01-02 15:04"))
		return lipgloss.NewStyle().Width(width).Height(height).Render(m.styles.Subtle.Render(msg))
	}
	if it.Event != nil {
		return m.renderEventDetailsFor(*it.Event, width, height)
	}
	if it.Todo != nil {
		return m.renderTodoDetailsFor(*it.Todo, it.Mode, width, height)
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(m.styles.Subtle.Render("No item selected"))
}

func (m Model) renderEventDetailsFor(ev calendar.Event, width, height int) string {
	calendarName := ev.DisplayName
	if calendarName == "" {
		calendarName = ev.Calendar
	}
	lines := []string{
		m.styles.Title.Render("Event Details"),
		"",
		"Title: " + ev.Summary,
		"Calendar: " + calendarName,
		"When: " + ev.Start.Format("2006-01-02 15:04") + " - " + ev.End.Format("2006-01-02 15:04"),
	}
	if ev.Location != "" {
		lines = append(lines, "Location: "+ev.Location)
	}
	if ev.Description != "" {
		lines = append(lines, "", "Description:")
		for _, raw := range strings.Split(ev.Description, "\n") {
			lines = append(lines, wrapLine(raw, max(10, width-4))...)
		}
	}
	for i := range lines {
		lines[i] = truncate(lines[i], max(10, width-2))
	}

	scroll := m.detailScroll
	if scroll > len(lines)-1 {
		scroll = len(lines) - 1
	}
	if scroll > 0 {
		lines = lines[scroll:]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	prefix := "Details"
	body := strings.Join(lines, "\n")
	block := lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render(prefix), body)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(block)
}

func (m Model) renderTodoDetailsFor(todo calendar.Todo, mode string, width, height int) string {
	calendarName := todo.DisplayName
	if calendarName == "" {
		calendarName = todo.Calendar
	}
	status := strings.TrimSpace(todo.Status)
	if status == "" {
		status = "NEEDS-ACTION"
	}
	when := "all-day"
	switch mode {
	case "todo-range":
		if todo.Start != nil && todo.Due != nil {
			when = todo.Start.Format("2006-01-02 15:04") + " - " + todo.Due.Format("2006-01-02 15:04")
		}
	case "todo-start":
		if todo.Start != nil {
			when = "start " + todo.Start.Format("2006-01-02 15:04")
		}
	case "todo-end":
		if todo.Due != nil {
			when = "due " + todo.Due.Format("2006-01-02 15:04")
		}
	}
	lines := []string{
		m.styles.Title.Render("Task Details"),
		"",
		"Title: " + todo.Summary,
		"Calendar: " + calendarName,
		"Status: " + status,
		"When: " + when,
	}
	if todo.Percent > 0 {
		lines = append(lines, fmt.Sprintf("Progress: %d%%", todo.Percent))
	}
	if strings.TrimSpace(todo.Location) != "" {
		lines = append(lines, "Location: "+todo.Location)
	}
	if todo.Priority > 0 {
		priorityLabel := todoPriorityLabel(todo.Priority)
		if strings.EqualFold(priorityLabel, "high") {
			priorityLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(priorityLabel)
		}
		lines = append(lines, "Priority: "+priorityLabel)
	}
	if todo.Description != "" {
		lines = append(lines, "", "Description:")
		for _, raw := range strings.Split(todo.Description, "\n") {
			lines = append(lines, wrapLine(raw, max(10, width-4))...)
		}
	}
	for i := range lines {
		lines[i] = truncate(lines[i], max(10, width-2))
	}

	scroll := m.detailScroll
	if scroll > len(lines)-1 {
		scroll = len(lines) - 1
	}
	if scroll > 0 {
		lines = lines[scroll:]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	prefix := "Details"
	body := strings.Join(lines, "\n")
	block := lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render(prefix), body)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(block)
}

func (m *Model) moveEventCursor(delta int) {
	items := m.agendaItems()
	if len(items) == 0 {
		m.eventCursor = 0
		m.eventListOffset = 0
		return
	}
	m.eventCursor += delta
	if m.eventCursor < 0 {
		m.eventCursor = 0
	}
	if m.eventCursor >= len(items) {
		m.eventCursor = len(items) - 1
	}

	m.selected = dayStart(items[m.eventCursor].Day)
	m.scrollForSelection()
	m.ensureEventCursorVisible()
}

func (m Model) renderCalendarListPane(width, height int) string {
	if height < 3 {
		height = 3
	}
	if len(m.calendarOrder) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(m.styles.Subtle.Render("No calendars"))
	}
	start := m.calendarOffset
	if start < 0 {
		start = 0
	}
	if start >= len(m.calendarOrder) {
		start = len(m.calendarOrder) - 1
	}
	visible := max(1, height-1)
	end := start + visible
	if end > len(m.calendarOrder) {
		end = len(m.calendarOrder)
	}
	lines := []string{m.styles.PanelTitle.Render("Calendars")}
	for i := start; i < end; i++ {
		key := m.calendarOrder[i]
		cal := m.calendarByKey(key)
		if cal == nil {
			continue
		}
		prefix := "  "
		if m.focusCalendarPane && i == m.calendarCursor {
			prefix = "> "
		}
		stateIcon := ""
		if m.calendarVisibility[key] {
			stateIcon = ""
		}
		if cal.Color != "" {
			stateIcon = styleForColor(m.styles.CalendarItem, cal.Color).Render(stateIcon)
		}
		name := cal.DisplayName
		if name == "" {
			name = cal.Name
		}
		line := fmt.Sprintf("%s%s %s", prefix, stateIcon, truncate(name, max(4, width-8)))
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func (m *Model) scrollForSelection() {
	selectedWeek := calendar.StartOfWeek(m.selected, m.weekStart())
	if m.weekViewportStart.IsZero() {
		m.weekViewportStart = selectedWeek
		return
	}
	for {
		visibleWeeks := m.visibleWeekCapacity(m.weekViewportStart)
		if visibleWeeks < 1 {
			visibleWeeks = 1
		}
		top := m.weekViewportStart
		bottom := top.AddDate(0, 0, (visibleWeeks-1)*7)
		if selectedWeek.Before(top) {
			m.weekViewportStart = selectedWeek
			continue
		}
		if selectedWeek.After(bottom) {
			m.weekViewportStart = selectedWeek.AddDate(0, 0, -(visibleWeeks-1)*7)
			continue
		}
		break
	}
}

func (m Model) monthListHeightBudget() int {
	if m.height <= 0 {
		return 30
	}
	panelHeight := m.height - 6
	if panelHeight < 8 {
		panelHeight = 8
	}
	topHeight := panelHeight * 2 / 3
	budget := topHeight - 2
	if budget < 8 {
		budget = 8
	}
	return budget
}

func (m Model) visibleWeekCapacity(top time.Time) int {
	budget := m.monthListHeightBudget()
	if budget < 3 {
		return 1
	}
	count := 0
	used := 0
	lastHeaderMonth := time.Time{}
	for i := 0; i < 104; i++ {
		week := top.AddDate(0, 0, i*7)
		headerMonth := monthStart(week.AddDate(0, 0, 3))
		need := 1
		if i == 0 || monthCompare(headerMonth, lastHeaderMonth) != 0 {
			need += 2
		}
		if used+need > budget {
			break
		}
		used += need
		count++
		lastHeaderMonth = headerMonth
	}
	if count < 1 {
		return 1
	}
	return count
}

func (m Model) calendarByKey(key string) *calendar.Calendar {
	for i := range m.data.Calendars {
		if calendarKey(m.data.Calendars[i].Source, m.data.Calendars[i].Name) == key {
			return &m.data.Calendars[i]
		}
	}
	return nil
}

func calendarKey(source, name string) string { return source + "/" + name }

func filteredEvents(events []calendar.Event, vis map[string]bool) []calendar.Event {
	out := make([]calendar.Event, 0, len(events))
	for _, ev := range events {
		if !vis[calendarKey(ev.Source, ev.Calendar)] {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) sidebarWidth() int {
	if m.cfg == nil || m.cfg.SidebarWidth <= 0 {
		return 30
	}
	if m.cfg.SidebarWidth < 18 {
		return 18
	}
	return m.cfg.SidebarWidth
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func monthCompare(a, b time.Time) int {
	a = monthStart(a)
	b = monthStart(b)
	if a.Year() < b.Year() || (a.Year() == b.Year() && a.Month() < b.Month()) {
		return -1
	}
	if a.Year() == b.Year() && a.Month() == b.Month() {
		return 0
	}
	return 1
}

func (m Model) weekStart() time.Weekday {
	if m.cfg == nil {
		return time.Monday
	}
	return m.cfg.WeekStart()
}

func (m *Model) moveCalendarCursor(delta int) {
	if len(m.calendarOrder) == 0 {
		m.calendarCursor = 0
		m.calendarOffset = 0
		return
	}
	m.calendarCursor += delta
	if m.calendarCursor < 0 {
		m.calendarCursor = 0
	}
	if m.calendarCursor >= len(m.calendarOrder) {
		m.calendarCursor = len(m.calendarOrder) - 1
	}
}

func (m *Model) ensureCalendarCursorVisible(height int) {
	if len(m.calendarOrder) == 0 {
		m.calendarCursor = 0
		m.calendarOffset = 0
		return
	}
	if m.calendarCursor < 0 {
		m.calendarCursor = 0
	}
	if m.calendarCursor >= len(m.calendarOrder) {
		m.calendarCursor = len(m.calendarOrder) - 1
	}
	visible := max(1, height-1)
	if m.calendarOffset < 0 {
		m.calendarOffset = 0
	}
	if m.calendarCursor < m.calendarOffset {
		m.calendarOffset = m.calendarCursor
	}
	if m.calendarCursor >= m.calendarOffset+visible {
		m.calendarOffset = m.calendarCursor - visible + 1
	}
}

func (m Model) renderHelpOverlay(width, height int) string {
	if width < 36 {
		width = 36
	}
	if height < 12 {
		height = 12
	}
	title := m.styles.Title.Render("Shortcuts")
	lines := []string{
		"q, ctrl+c   Quit",
		"?           Toggle help",
		"j / k       Next / previous event",
		"ctrl+f/b    Page down / page up",
		"h / l       Previous / next day",
		"ctrl+h/l    Previous / next week",
		"ctrl+j/k    Scroll description down/up",
		"f           Toggle show-free mode",
		"m           Toggle tasks-only mode",
		"c           Open calendars toggle pane",
		"n           New item form (event/task)",
		"e           Edit selected item",
		"",
		"Event form:",
		"tab/s-tab   Next / previous field",
		"ctrl+s      Save",
		"esc         Cancel",
	}
	body := strings.Join(lines, "\n")
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("245")).Padding(1, 2).Width(width).Height(height).Render(content)
	return box
}

func (m Model) calendarPaneHeight() int {
	panelHeight := m.height - 6
	if panelHeight < 8 {
		panelHeight = 8
	}
	topHeight := panelHeight * 2 / 3
	bottom := panelHeight - topHeight - 1
	if bottom < 4 {
		bottom = 4
	}
	return bottom
}

func (m *Model) ensureEventSelectionValid() {
	items := m.agendaItems()
	if len(items) == 0 {
		m.eventCursor = 0
		m.eventListOffset = 0
		return
	}
	if m.eventCursor < 0 {
		m.eventCursor = 0
	}
	if m.eventCursor >= len(items) {
		m.eventCursor = len(items) - 1
	}
	m.ensureEventCursorVisible()
	m.selected = dayStart(items[m.eventCursor].Day)
	m.scrollForSelection()
}

func (m *Model) ensureEventCursorVisible() {
	items := m.agendaItems()
	if len(items) == 0 {
		m.eventListOffset = 0
		return
	}
	if m.eventListOffset < 0 {
		m.eventListOffset = 0
	}
	if m.eventListOffset >= len(items) {
		m.eventListOffset = len(items) - 1
	}
	if m.eventCursor < m.eventListOffset {
		m.eventListOffset = m.eventCursor
	}
	maxLines := m.eventListLines()
	rendered := renderAgendaFromItems(items, m.mainInnerWidth(), maxLines, m.cfg.TimeFormat, m.styles, m.eventCursor, m.eventListOffset, true)
	if rendered.LastVisibleEventIndex >= 0 && m.eventCursor <= rendered.LastVisibleEventIndex {
		return
	}
	m.eventListOffset = m.eventCursor
	for m.eventListOffset > 0 {
		r := renderAgendaFromItems(items, m.mainInnerWidth(), maxLines, m.cfg.TimeFormat, m.styles, m.eventCursor, m.eventListOffset-1, true)
		if r.LastVisibleEventIndex < m.eventCursor {
			break
		}
		m.eventListOffset--
	}
}

func (m Model) agendaEvents() []calendar.Event {
	start := m.agendaStart
	if start.IsZero() {
		start = dayStart(m.selected)
	}
	return agendaEventsFromDay(start, filteredEvents(m.data.Events, m.calendarVisibility))
}

func (m Model) agendaItems() []AgendaListItem {
	start := m.agendaStart
	if start.IsZero() {
		start = dayStart(m.selected)
	}
	items := buildAgendaItems(
		start,
		filteredEvents(m.data.Events, m.calendarVisibility),
		filteredTodos(m.data.Todos, m.calendarVisibility),
		90,
		m.showFreeMode,
		m.showFreeMode,
	)
	if !m.showTasksMode {
		return items
	}
	out := make([]AgendaListItem, 0, len(items))
	for _, it := range items {
		if it.IsFree || it.Todo == nil {
			continue
		}
		out = append(out, it)
	}
	return out
}

func (m Model) currentSelectionHasDetails() bool {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return false
	}
	it := items[m.eventCursor]
	if it.IsFree {
		return false
	}
	return it.Event != nil || it.Todo != nil
}

func (m *Model) openEventFormNew() {
	defaultKey := ""
	for _, k := range m.calendarOrder {
		if !m.calendarVisibility[k] {
			continue
		}
		cal := m.calendarByKey(k)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		defaultKey = k
		break
	}
	if defaultKey == "" {
		for _, k := range m.calendarOrder {
			cal := m.calendarByKey(k)
			if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
				continue
			}
			defaultKey = k
			break
		}
	}
	now := time.Now().In(m.selected.Location()).Truncate(time.Minute)
	end := now.Add(time.Hour)
	m.eventForm = m.newEventFormState("create", "", calendar.Event{
		Summary:  "",
		Start:    now,
		End:      end,
		AllDay:   false,
		Source:   splitCalendarKey(defaultKey).source,
		Calendar: splitCalendarKey(defaultKey).name,
	})
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.eventForm.form.UpdateFieldPositions()
}

func (m *Model) openTodoFormEditSelected() bool {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return false
	}
	it := items[m.eventCursor]
	if it.Todo == nil || it.IsFree {
		return false
	}
	td := *it.Todo
	m.todoForm = m.newTodoFormState("edit", td.UID, td)
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.todoForm.form.UpdateFieldPositions()
	return true
}

func (m *Model) openTodoFormNew() {
	defaultKey := m.firstWritableCalendarKey()
	td := calendar.Todo{
		Summary:  "",
		Source:   splitCalendarKey(defaultKey).source,
		Calendar: splitCalendarKey(defaultKey).name,
		Priority: 5,
	}
	m.todoForm = m.newTodoFormState("create", "", td)
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.todoForm.form.UpdateFieldPositions()
}

func (m *Model) openEditFormForSelected() bool {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return false
	}
	it := items[m.eventCursor]
	if it.IsFree {
		return false
	}
	if it.Event != nil {
		return m.openEventFormEditSelected()
	}
	if it.Todo != nil {
		return m.openTodoFormEditSelected()
	}
	return false
}

func (m *Model) openEventFormEditSelected() bool {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return false
	}
	it := items[m.eventCursor]
	if it.Event == nil || it.IsFree {
		return false
	}
	ev := *it.Event
	if ev.Source == calendar.SpecialSourceBirthdays {
		return false
	}
	m.eventForm = m.newEventFormState("edit", ev.UID, ev)
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.eventForm.form.UpdateFieldPositions()
	return true
}

func (m *Model) newTodoFormState(mode, targetUID string, td calendar.Todo) *todoFormState {
	key := calendarKey(td.Source, td.Calendar)
	if key == "/" || key == "" {
		key = m.firstWritableCalendarKey()
	}
	startDate := ""
	startTime := ""
	if td.Start != nil {
		start := td.Start.In(m.selected.Location())
		startDate = start.Format("2006-01-02")
		startTime = start.Format("15:04")
	}
	dueDate := ""
	dueTime := ""
	if td.Due != nil {
		due := td.Due.In(m.selected.Location())
		dueDate = due.Format("2006-01-02")
		dueTime = due.Format("15:04")
	}
	state := &todoFormState{
		mode:          mode,
		targetUID:     targetUID,
		summary:       td.Summary,
		description:   td.Description,
		location:      td.Location,
		calendarKey:   key,
		startDate:     startDate,
		startTime:     startTime,
		dueDate:       dueDate,
		dueTime:       dueTime,
		completed:     isTodoDone(td),
		priorityLabel: todoPriorityLabel(td.Priority),
	}
	if state.priorityLabel == "" {
		state.priorityLabel = "mid"
	}
	state.form = m.buildTodoForm(state)
	return state
}

func (m *Model) buildTodoForm(s *todoFormState) *huh.Form {
	calOptions := make([]huh.Option[string], 0, len(m.calendarOrder))
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		name := cal.DisplayName
		if name == "" {
			name = cal.Name
		}
		calOptions = append(calOptions, huh.NewOption(name+" ("+cal.Source+")", key))
	}
	if len(calOptions) == 0 {
		calOptions = append(calOptions, huh.NewOption("No writable calendar", ""))
	}
	priorityOptions := []huh.Option[string]{
		huh.NewOption("Low", "low"),
		huh.NewOption("Mid", "mid"),
		huh.NewOption("High", "high"),
	}
	title := "Edit Task"
	group := huh.NewGroup(
		huh.NewInput().Key("summary").Title("Summary").Value(&s.summary).Validate(func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("summary is required")
			}
			return nil
		}),
		huh.NewText().Key("description").Title("Description").Value(&s.description).Lines(4),
		huh.NewInput().Key("location").Title("Location").Value(&s.location),
		huh.NewSelect[string]().Key("calendar").Title("Calendar").Options(calOptions...).Value(&s.calendarKey).Validate(func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("calendar is required")
			}
			return nil
		}),
		huh.NewInput().Key("start-date").Title("Start date (YYYY-MM-DD)").Value(&s.startDate).Validate(func(v string) error {
			if err := validateOptionalDateInput(v); err != nil {
				return err
			}
			if strings.TrimSpace(v) != "" && strings.TrimSpace(s.startTime) == "" {
				return errors.New("start time is required when start date is set")
			}
			return nil
		}),
		huh.NewInput().Key("start-time").Title("Start time (HH:MM)").Value(&s.startTime).Validate(func(v string) error {
			if err := validateOptionalTimeInput(v); err != nil {
				return err
			}
			if strings.TrimSpace(v) != "" && strings.TrimSpace(s.startDate) == "" {
				return errors.New("start date is required when start time is set")
			}
			return nil
		}),
		huh.NewInput().Key("due-date").Title("Due date (YYYY-MM-DD)").Value(&s.dueDate).Validate(func(v string) error {
			if err := validateOptionalDateInput(v); err != nil {
				return err
			}
			if strings.TrimSpace(v) != "" && strings.TrimSpace(s.dueTime) == "" {
				return errors.New("due time is required when due date is set")
			}
			if !todoRangeIsValid(s.startDate, s.startTime, v, s.dueTime) {
				return errors.New("due must be after start")
			}
			return nil
		}),
		huh.NewInput().Key("due-time").Title("Due time (HH:MM)").Value(&s.dueTime).Validate(func(v string) error {
			if err := validateOptionalTimeInput(v); err != nil {
				return err
			}
			if strings.TrimSpace(v) != "" && strings.TrimSpace(s.dueDate) == "" {
				return errors.New("due date is required when due time is set")
			}
			if !todoRangeIsValid(s.startDate, s.startTime, s.dueDate, v) {
				return errors.New("due must be after start")
			}
			return nil
		}),
		huh.NewConfirm().Key("completed").Title("Completed").Value(&s.completed),
		huh.NewSelect[string]().Key("priority").Title("Priority").Options(priorityOptions...).Value(&s.priorityLabel),
	).Title(title)
	return huh.NewForm(group).WithShowErrors(true).WithShowHelp(true)
}

func (m *Model) commitTodoForm() error {
	if m.todoForm == nil {
		return nil
	}
	if m.store == nil {
		return errors.New("todo store is unavailable")
	}
	s := m.todoForm
	cal := splitCalendarKey(s.calendarKey)
	if strings.TrimSpace(cal.source) == "" || strings.TrimSpace(cal.name) == "" {
		return errors.New("calendar is required")
	}
	startPtr, duePtr, err := parseTodoFormTimesOptional(*s)
	if err != nil {
		return err
	}
	status := "NEEDS-ACTION"
	if s.completed {
		status = "COMPLETED"
	}
	priority := todoPriorityFromLabel(s.priorityLabel)

	if s.mode == "edit" {
		startUpdate := startPtr
		dueUpdate := duePtr
		upd := calendar.TodoUpdate{
			Summary:     &s.summary,
			Description: &s.description,
			Location:    &s.location,
			Status:      &status,
			Priority:    &priority,
			Start:       &startUpdate,
			Due:         &dueUpdate,
		}
		if err := m.store.UpdateTodo(s.targetUID, upd); err != nil {
			return err
		}
	} else {
		td := calendar.Todo{
			Summary:     s.summary,
			Description: s.description,
			Location:    s.location,
			Status:      status,
			Priority:    priority,
			Start:       startPtr,
			Due:         duePtr,
		}
		if err := m.store.CreateTodo(cal.source, cal.name, td); err != nil {
			return err
		}
	}

	ds, err := m.store.Load()
	if err != nil {
		return err
	}
	m.data = ds
	if startPtr != nil {
		m.selected = dayStart(*startPtr)
		m.agendaStart = dayStart(*startPtr)
	}
	m.eventCursor = 0
	m.eventListOffset = 0
	return nil
}

func (m *Model) newEventFormState(mode, targetUID string, ev calendar.Event) *eventFormState {
	key := calendarKey(ev.Source, ev.Calendar)
	if key == "/" || key == "" {
		key = m.firstWritableCalendarKey()
	}
	fd := ev.Start.In(m.selected.Location())
	td := ev.End.In(m.selected.Location())
	if fd.IsZero() {
		fd = m.selected
	}
	if td.IsZero() || !td.After(fd) {
		td = fd.Add(time.Hour)
	}
	state := &eventFormState{
		mode:        mode,
		targetUID:   targetUID,
		summary:     ev.Summary,
		calendarKey: key,
		location:    ev.Location,
		description: ev.Description,
		url:         ev.URL,
		allDay:      ev.AllDay,
		fromDate:    fd.Format("2006-01-02"),
		fromTime:    fd.Format("15:04"),
		toDate:      td.Format("2006-01-02"),
		toTime:      td.Format("15:04"),
	}
	state.form = m.buildEventForm(state)
	return state
}

func (m *Model) buildEventForm(s *eventFormState) *huh.Form {
	calOptions := make([]huh.Option[string], 0, len(m.calendarOrder))
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		name := cal.DisplayName
		if name == "" {
			name = cal.Name
		}
		calOptions = append(calOptions, huh.NewOption(name+" ("+cal.Source+")", key))
	}
	if len(calOptions) == 0 {
		calOptions = append(calOptions, huh.NewOption("No writable calendar", ""))
	}

	modeTitle := "Create Event"
	if s.mode == "edit" {
		modeTitle = "Edit Event"
	}

	mainGroup := huh.NewGroup(
		huh.NewInput().Key("title").Title("Title").Value(&s.summary).Validate(func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("title is required")
			}
			return nil
		}),
		huh.NewSelect[string]().Key("calendar").Title("Calendar").Options(calOptions...).Value(&s.calendarKey).Validate(func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("calendar is required")
			}
			return nil
		}),
		huh.NewInput().Key("location").Title("Location").Value(&s.location),
		huh.NewText().Key("description").Title("Description").Value(&s.description).Lines(4),
		huh.NewInput().Key("url").Title("URL").Value(&s.url),
		huh.NewConfirm().Key("all-day").Title("All-day").Value(&s.allDay),
		huh.NewInput().Key("from-date").Title("From date (YYYY-MM-DD)").Value(&s.fromDate).Validate(validateEventDateInput),
		huh.NewInput().Key("from-time").Title("From time (HH:MM)").Description("Ignored when all-day is enabled").Value(&s.fromTime).Validate(func(v string) error {
			if s.allDay {
				return nil
			}
			return validateEventTimeInput(v)
		}),
		huh.NewInput().Key("to-date").Title("To date (YYYY-MM-DD)").Value(&s.toDate).Validate(func(v string) error {
			if err := validateEventDateInput(v); err != nil {
				return err
			}
			fromDate, err := time.Parse("2006-01-02", strings.TrimSpace(s.fromDate))
			if err != nil {
				return nil
			}
			toDate, _ := time.Parse("2006-01-02", strings.TrimSpace(v))
			if toDate.Before(fromDate) {
				return errors.New("end date cannot be before start date")
			}
			if s.allDay {
				return nil
			}
			if validateEventTimeInput(s.fromTime) != nil || validateEventTimeInput(s.toTime) != nil {
				return nil
			}
			if !eventRangeIsValid(s.fromDate, s.fromTime, v, s.toTime) {
				return errors.New("end must be after start")
			}
			return nil
		}),
		huh.NewInput().Key("to-time").Title("To time (HH:MM)").Description("Ignored when all-day is enabled").Value(&s.toTime).Validate(func(v string) error {
			if s.allDay {
				return nil
			}
			if err := validateEventTimeInput(v); err != nil {
				return err
			}
			if validateEventDateInput(s.fromDate) != nil || validateEventDateInput(s.toDate) != nil || validateEventTimeInput(s.fromTime) != nil {
				return nil
			}
			if !eventRangeIsValid(s.fromDate, s.fromTime, s.toDate, v) {
				return errors.New("end must be after start")
			}
			return nil
		}),
	).Title(modeTitle)

	return huh.NewForm(mainGroup).WithShowHelp(true).WithShowErrors(true)
}

func (m *Model) commitEventForm() error {
	if m.eventForm == nil {
		return nil
	}
	s := m.eventForm
	if m.store == nil {
		return errors.New("event store is unavailable")
	}

	cal := splitCalendarKey(s.calendarKey)
	if strings.TrimSpace(cal.source) == "" || strings.TrimSpace(cal.name) == "" {
		return errors.New("calendar is required")
	}

	start, end, err := parseEventFormTimes(*s)
	if err != nil {
		return err
	}

	if s.mode == "edit" {
		upd := calendar.EventUpdate{
			Summary:     &s.summary,
			Description: &s.description,
			Location:    &s.location,
			URL:         &s.url,
			Start:       &start,
			End:         &end,
			AllDay:      &s.allDay,
		}
		if err := m.store.UpdateEvent(s.targetUID, upd); err != nil {
			return err
		}
	} else {
		ev := calendar.Event{
			Summary:     s.summary,
			Description: s.description,
			Location:    s.location,
			URL:         s.url,
			AllDay:      s.allDay,
			Start:       start,
			End:         end,
		}
		if err := m.store.CreateEvent(cal.source, cal.name, ev); err != nil {
			return err
		}
	}

	ds, err := m.store.Load()
	if err != nil {
		return err
	}
	m.data = ds
	if s.mode == "create" {
		m.selected = dayStart(start)
		m.agendaStart = dayStart(start)
		m.eventCursor = 0
		m.eventListOffset = 0
	}
	return nil
}

type calendarKeyParts struct {
	source string
	name   string
}

func splitCalendarKey(v string) calendarKeyParts {
	parts := strings.SplitN(v, "/", 2)
	if len(parts) != 2 {
		return calendarKeyParts{}
	}
	return calendarKeyParts{source: parts[0], name: parts[1]}
}

func (m *Model) firstWritableCalendarKey() string {
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		return key
	}
	return ""
}

func parseEventFormTimes(s eventFormState) (time.Time, time.Time, error) {
	startDate, err := time.Parse("2006-01-02", strings.TrimSpace(s.fromDate))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid from date (expected YYYY-MM-DD)")
	}
	endDate, err := time.Parse("2006-01-02", strings.TrimSpace(s.toDate))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid to date (expected YYYY-MM-DD)")
	}
	if s.allDay {
		start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.Local)
		end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.Local)
		if !end.After(start) {
			end = start.Add(24 * time.Hour)
		}
		return start, end, nil
	}

	startClock, err := time.Parse("15:04", strings.TrimSpace(s.fromTime))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid from time (expected HH:MM)")
	}
	endClock, err := time.Parse("15:04", strings.TrimSpace(s.toTime))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid to time (expected HH:MM)")
	}
	start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.Local)
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), endClock.Hour(), endClock.Minute(), 0, 0, time.Local)
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("end must be after start")
	}
	return start, end, nil
}

func validateOptionalDateInput(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(v)); err != nil {
		return errors.New("invalid date (expected YYYY-MM-DD)")
	}
	return nil
}

func validateOptionalTimeInput(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if _, err := time.Parse("15:04", strings.TrimSpace(v)); err != nil {
		return errors.New("invalid time (expected HH:MM)")
	}
	return nil
}

func validateEventDateInput(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("date is required")
	}
	if _, err := time.Parse("2006-01-02", strings.TrimSpace(v)); err != nil {
		return errors.New("invalid date (expected YYYY-MM-DD)")
	}
	return nil
}

func validateEventTimeInput(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("time is required")
	}
	if _, err := time.Parse("15:04", strings.TrimSpace(v)); err != nil {
		return errors.New("invalid time (expected HH:MM)")
	}
	return nil
}

func eventRangeIsValid(fromDate, fromTime, toDate, toTime string) bool {
	fd, err := time.Parse("2006-01-02", strings.TrimSpace(fromDate))
	if err != nil {
		return true
	}
	td, err := time.Parse("2006-01-02", strings.TrimSpace(toDate))
	if err != nil {
		return true
	}
	ft, err := time.Parse("15:04", strings.TrimSpace(fromTime))
	if err != nil {
		return true
	}
	tt, err := time.Parse("15:04", strings.TrimSpace(toTime))
	if err != nil {
		return true
	}
	start := time.Date(fd.Year(), fd.Month(), fd.Day(), ft.Hour(), ft.Minute(), 0, 0, time.Local)
	end := time.Date(td.Year(), td.Month(), td.Day(), tt.Hour(), tt.Minute(), 0, 0, time.Local)
	return end.After(start)
}

func parseTodoFormTimesOptional(s todoFormState) (*time.Time, *time.Time, error) {
	startDateRaw := strings.TrimSpace(s.startDate)
	startTimeRaw := strings.TrimSpace(s.startTime)
	dueDateRaw := strings.TrimSpace(s.dueDate)
	dueTimeRaw := strings.TrimSpace(s.dueTime)

	if startDateRaw == "" && startTimeRaw == "" && dueDateRaw == "" && dueTimeRaw == "" {
		return nil, nil, nil
	}
	if (startDateRaw == "") != (startTimeRaw == "") {
		return nil, nil, errors.New("start date and time must both be set")
	}
	if (dueDateRaw == "") != (dueTimeRaw == "") {
		return nil, nil, errors.New("due date and time must both be set")
	}

	var startPtr *time.Time
	if startDateRaw != "" {
		startDate, err := time.Parse("2006-01-02", startDateRaw)
		if err != nil {
			return nil, nil, errors.New("invalid start date (expected YYYY-MM-DD)")
		}
		startClock, err := time.Parse("15:04", startTimeRaw)
		if err != nil {
			return nil, nil, errors.New("invalid start time (expected HH:MM)")
		}
		start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), startClock.Hour(), startClock.Minute(), 0, 0, time.Local)
		startPtr = &start
	}

	var duePtr *time.Time
	if dueDateRaw != "" {
		dueDate, err := time.Parse("2006-01-02", dueDateRaw)
		if err != nil {
			return nil, nil, errors.New("invalid due date (expected YYYY-MM-DD)")
		}
		dueClock, err := time.Parse("15:04", dueTimeRaw)
		if err != nil {
			return nil, nil, errors.New("invalid due time (expected HH:MM)")
		}
		due := time.Date(dueDate.Year(), dueDate.Month(), dueDate.Day(), dueClock.Hour(), dueClock.Minute(), 0, 0, time.Local)
		duePtr = &due
	}

	if startPtr != nil && duePtr != nil && !duePtr.After(*startPtr) {
		return nil, nil, errors.New("due must be after start")
	}
	return startPtr, duePtr, nil
}

func todoRangeIsValid(startDate, startTime, dueDate, dueTime string) bool {
	startDate = strings.TrimSpace(startDate)
	startTime = strings.TrimSpace(startTime)
	dueDate = strings.TrimSpace(dueDate)
	dueTime = strings.TrimSpace(dueTime)
	if startDate == "" || startTime == "" || dueDate == "" || dueTime == "" {
		return true
	}
	return eventRangeIsValid(startDate, startTime, dueDate, dueTime)
}

func todoPriorityFromLabel(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "high":
		return 1
	case "low":
		return 9
	default:
		return 5
	}
}

func todoPriorityLabel(v int) string {
	if v <= 0 {
		return "mid"
	}
	if v <= 3 {
		return "high"
	}
	if v >= 7 {
		return "low"
	}
	return "mid"
}

func (m *Model) isTodoFormFocusedFirstField() bool {
	if m.todoForm == nil || m.todoForm.form == nil || len(todoFormFieldOrder) == 0 {
		return false
	}
	f := m.todoForm.form.GetFocusedField()
	if f == nil {
		return false
	}
	return f.GetKey() == todoFormFieldOrder[0]
}

func (m *Model) isTodoFormFocusedLastField() bool {
	if m.todoForm == nil || m.todoForm.form == nil || len(todoFormFieldOrder) == 0 {
		return false
	}
	f := m.todoForm.form.GetFocusedField()
	if f == nil {
		return false
	}
	return f.GetKey() == todoFormFieldOrder[len(todoFormFieldOrder)-1]
}

func (m *Model) isEventFormFocusedFirstField() bool {
	if m.eventForm == nil || m.eventForm.form == nil || len(eventFormFieldOrder) == 0 {
		return false
	}
	f := m.eventForm.form.GetFocusedField()
	if f == nil {
		return false
	}
	return f.GetKey() == eventFormFieldOrder[0]
}

func (m *Model) isEventFormFocusedLastField() bool {
	if m.eventForm == nil || m.eventForm.form == nil || len(eventFormFieldOrder) == 0 {
		return false
	}
	f := m.eventForm.form.GetFocusedField()
	if f == nil {
		return false
	}
	return f.GetKey() == eventFormFieldOrder[len(eventFormFieldOrder)-1]
}

func filteredTodos(todos []calendar.Todo, vis map[string]bool) []calendar.Todo {
	out := make([]calendar.Todo, 0, len(todos))
	for _, td := range todos {
		if !vis[calendarKey(td.Source, td.Calendar)] {
			continue
		}
		out = append(out, td)
	}
	return out
}

func (m Model) eventListLines() int {
	panelHeight := m.height - 6
	if panelHeight < 10 {
		panelHeight = 10
	}
	available := panelHeight - 4
	if available < 8 {
		available = 8
	}
	topHeight := available * 3 / 5
	if topHeight < 6 {
		topHeight = 6
	}
	return topHeight
}

func (m Model) eventPageStep() int {
	step := m.eventListLines() - 2
	if step < 1 {
		return 1
	}
	return step
}

func (m Model) mainInnerWidth() int {
	leftWidth := m.sidebarWidth()
	rightWidth := max(50, m.width-leftWidth-5)
	if rightWidth < 8 {
		return 8
	}
	return rightWidth - 2
}

func wrapLine(s string, maxLen int) []string {
	if maxLen < 1 {
		return []string{s}
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return []string{s}
	}
	out := make([]string, 0, (len(r)/maxLen)+1)
	for len(r) > 0 {
		take := maxLen
		if len(r) < take {
			take = len(r)
		}
		out = append(out, string(r[:take]))
		r = r[take:]
	}
	return out
}
