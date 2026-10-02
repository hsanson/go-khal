package tui

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	showAllMode        bool
	showTasksMode      bool
	agendaStart        time.Time
	eventCursor        int
	eventListOffset    int
	detailScroll       int
	eventForm          *eventFormState
	todoForm           *todoFormState
	deleteConfirm      *deleteConfirmState
	showHelpOverlay    bool
	actionErr          string
}

const calendarKeySeparator = "\x1f"

type eventFormState struct {
	mode            string
	targetUID       string
	targetEvent     *calendar.Event
	editScope       string
	form            *huh.Form
	activeKey       string
	activeForm      *huh.Form
	attendeeManager *attendeeManagerState
	noNotifications bool
	backup          *eventFormSnapshot
	datePicker      *dateRangePicker
	timeEditor      *timeRangeEditor
	searchPicker    *searchPicker
	cursor          int
	summary         string
	calendarKey     string
	location        string
	description     string
	url             string
	attendees       string
	rsvp            string
	availability    string
	visibility      string
	alarms          string
	recur           bool
	recurFreq       string
	recurEvery      string
	recurWeekdays   []string
	recurMonthlyBy  string
	recurEnd        string
	recurUntil      string
	recurCount      string
	allDay          bool
	fromDate        string
	fromTime        string
	toDate          string
	toTime          string
	timezone        string
	timezoneLocal   bool
	timingDirty     bool
	overnightAuto   bool
	errMsg          string
}

type attendeeManagerState struct {
	attendees []attendeeEditorItem
	cursor    int
}

type attendeeEditorItem struct {
	attendee calendar.Attendee
	remove   bool
}

type deleteConfirmState struct {
	form      *huh.Form
	itemLabel string
	kind      string
	event     *calendar.Event
	todo      *calendar.Todo
	recurring bool
	stage     string
	confirm   bool
	scope     string
	errMsg    string
}

type todoFormState struct {
	mode          string
	targetUID     string
	form          *huh.Form
	activeKey     string
	activeForm    *huh.Form
	backup        *todoFormSnapshot
	datePicker    *dateRangePicker
	timeEditor    *timeRangeEditor
	cursor        int
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

type editorRow struct {
	key   string
	label string
	value string
}

type eventFormSnapshot struct {
	editScope      string
	summary        string
	calendarKey    string
	location       string
	description    string
	url            string
	attendees      string
	rsvp           string
	availability   string
	visibility     string
	alarms         string
	recur          bool
	recurFreq      string
	recurEvery     string
	recurWeekdays  []string
	recurMonthlyBy string
	recurEnd       string
	recurUntil     string
	recurCount     string
	allDay         bool
	fromDate       string
	fromTime       string
	toDate         string
	toTime         string
	timezone       string
	timezoneLocal  bool
	timingDirty    bool
	overnightAuto  bool
}

type todoFormSnapshot struct {
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

func NewTaskModeModel(cfg *config.Config, data calendar.Dataset, store *calendar.Store) Model {
	m := NewModel(cfg, data, store)
	m.enterTaskMode()
	return m
}

func NewTodoCreateModel(cfg *config.Config, data calendar.Dataset, store *calendar.Store, todo calendar.Todo) Model {
	m := NewTaskModeModel(cfg, data, store)
	m.openTodoFormNewWith(todo)
	return m
}

func NewEventImportModel(cfg *config.Config, data calendar.Dataset, store *calendar.Store, imported calendar.Event, existing *calendar.Event) Model {
	m := NewModel(cfg, data, store)
	mode := "create"
	targetUID := imported.UID
	if existing != nil {
		mode = "edit"
		imported.Source = existing.Source
		imported.Calendar = existing.Calendar
		imported.FilePath = existing.FilePath
	}
	m.eventForm = m.newEventFormState(mode, targetUID, imported)
	if existing != nil {
		target := *existing
		m.eventForm.targetEvent = &target
		m.eventForm.timingDirty = true
	}
	m.focusDetails = true
	m.focusMain = false
	m.eventForm.form.UpdateFieldPositions()
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
		key := msg.String()
		if m.showHelpOverlay {
			if key == "?" || key == "q" || key == "esc" {
				m.showHelpOverlay = false
			}
			return m, nil
		}

		// Popups own their shortcuts and do not open application help.
		if m.deleteConfirm != nil {
			return m.updateDeleteConfirm(msg)
		}
		if m.eventForm != nil && m.eventForm.hasActiveDialog() {
			return m.updateEventEditor(msg)
		}
		if m.todoForm != nil && m.todoForm.activeForm != nil {
			return m.updateTodoEditor(msg)
		}

		if key == "?" {
			m.showHelpOverlay = true
			return m, nil
		}
		if m.eventForm != nil {
			return m.updateEventEditor(msg)
		}
		if m.todoForm != nil {
			return m.updateTodoEditor(msg)
		}

		if m.focusDetails {
			if key == "ctrl+c" {
				return m, tea.Quit
			}
			switch key {
			case "esc", "q", "enter", " ":
				m.focusDetails = false
				m.focusMain = true
				m.detailScroll = 0
			case "e":
				if m.openEditFormForSelected() {
					if m.eventForm != nil {
						return m, m.initCurrentEventForm()
					}
					return m, m.todoForm.form.Init()
				}
			case "ctrl+d":
				if m.openDeleteConfirmForSelected() {
					return m, m.deleteConfirm.form.Init()
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
			if key == "ctrl+c" {
				return m, tea.Quit
			}
			switch key {
			case "j", "down":
				m.moveCalendarCursor(1)
			case "k", "up":
				m.moveCalendarCursor(-1)
			case " ", "enter":
				if len(m.calendarOrder) > 0 {
					calendarKey := m.calendarOrder[m.calendarCursor]
					m.calendarVisibility[calendarKey] = !m.calendarVisibility[calendarKey]
					m.ensureEventSelectionValid()
				}
			case "esc", "q", "h", "c":
				m.focusCalendarPane = false
				m.focusMain = true
			}
			m.ensureCalendarCursorVisible(m.calendarPaneHeight())
			return m, nil
		}
		if key == "q" || key == "esc" || key == "ctrl+c" {
			return m, tea.Quit
		}
		if key == "c" {
			m.focusCalendarPane = true
			m.focusMain = false
			m.ensureCalendarCursorVisible(m.calendarPaneHeight())
			m.ensureEventSelectionValid()
			return m, nil
		}

		switch key {
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
			if m.showTasksMode {
				break
			}
			m.selected = m.selected.AddDate(0, 0, -1)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "l", "right":
			if m.showTasksMode {
				break
			}
			m.selected = m.selected.AddDate(0, 0, 1)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "ctrl+h":
			if m.showTasksMode {
				break
			}
			m.selected = m.selected.AddDate(0, 0, -7)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "ctrl+l":
			if m.showTasksMode {
				break
			}
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
		case "e", "enter":
			if m.openEditFormForSelected() {
				if m.eventForm != nil {
					return m, m.initCurrentEventForm()
				}
				if m.todoForm != nil {
					return m, m.todoForm.form.Init()
				}
			}
		case "ctrl+d":
			if m.openDeleteConfirmForSelected() {
				return m, m.deleteConfirm.form.Init()
			}
		case "f":
			m.showAllMode = !m.showAllMode
			m.eventCursor = 0
			m.eventListOffset = 0
			m.ensureEventSelectionValid()
		case "x", "d":
			if m.showTasksMode {
				m.actionErr = m.toggleSelectedTodoDone()
			}
		case "p":
			if m.showTasksMode {
				m.actionErr = m.cycleSelectedTodoPriority()
			}
		case "m":
			if m.showTasksMode {
				m.exitTaskMode()
			} else {
				m.enterTaskMode()
			}
		case "t":
			now := time.Now().In(m.selected.Location())
			m.selected = now
			m.agendaStart = dayStart(now)
			m.weekViewportStart = calendar.StartOfWeek(now, m.weekStart())
			m.focusMain = true
			m.focusDetails = false
			if m.showTasksMode {
				m.jumpTaskCursorToToday(now)
			} else {
				m.eventCursor = 0
				m.eventListOffset = 0
			}
			m.detailScroll = 0
		case "v":
			if m.openViewForSelected() {
				return m, nil
			}
		case " ":
			if len(m.agendaItems()) > 0 {
				m.focusDetails = true
				m.focusMain = false
				m.detailScroll = 0
			}
		}
		m.ensureEventSelectionValid()
	}
	if m.deleteConfirm != nil {
		return m.updateDeleteConfirm(msg)
	}
	if m.eventForm != nil && m.eventForm.activeForm != nil {
		return m, m.updateActiveEventEditorForm(msg)
	}
	if m.todoForm != nil && m.todoForm.activeForm != nil {
		return m, m.updateActiveTodoEditorForm(msg)
	}
	return m, nil
}

func (m *Model) updateDeleteConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.deleteConfirm
	if s == nil {
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc", "q":
			m.closeDeleteConfirm()
			return m, nil
		case "ctrl+s":
			return m.finishDeleteConfirm()
		}
	}

	updated, cmd := s.form.Update(msg)
	if form, ok := updated.(*huh.Form); ok {
		s.form = form
	}
	s.form.UpdateFieldPositions()
	if s.form.State == huh.StateAborted {
		m.closeDeleteConfirm()
		return m, nil
	}
	if s.form.State == huh.StateCompleted {
		return m.finishDeleteConfirm()
	}
	return m, cmd
}

func (m *Model) finishDeleteConfirm() (tea.Model, tea.Cmd) {
	s := m.deleteConfirm
	if s == nil {
		return m, nil
	}
	if s.stage == "scope" {
		s.stage = "confirm"
		s.confirm = false
		s.errMsg = ""
		s.form = m.buildDeleteConfirmForm(s).WithKeyMap(deleteConfirmFormKeyMap(s))
		s.form.UpdateFieldPositions()
		return m, s.form.Init()
	}
	if err := m.commitDeleteConfirm(); err != nil {
		s.errMsg = err.Error()
		s.form = m.buildDeleteConfirmForm(s).WithKeyMap(deleteConfirmFormKeyMap(s))
		return m, s.form.Init()
	}
	m.closeDeleteConfirm()
	m.ensureEventSelectionValid()
	return m, nil
}

func (m *Model) closeDeleteConfirm() {
	m.deleteConfirm = nil
	m.focusDetails = false
	m.focusMain = true
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
	content := root
	if legend := m.shortcutsLegend(); legend != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, root, "", m.styles.Subtle.Render(legend))
	}
	base := m.styles.Container.Render(content)
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
	weeks := renderWeekViewport(m.weekViewportStart, m.selected, filteredEvents(m.data.Events, m.calendarVisibility, true), width-2, max(3, topHeight-2), m.weekStart(), m.styles)
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
	if m.deleteConfirm != nil {
		return m.renderDeleteConfirmMainPanel(width, panelHeight)
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
		header = m.styles.PanelTitle.Render("Tasks")
	}
	if m.showAllMode {
		if m.showTasksMode {
			header += m.styles.Subtle.Render(" [SHOW-COMPLETED]")
		} else {
			header += m.styles.Subtle.Render(" [SHOW-ALL]")
		}
	}
	if strings.TrimSpace(m.actionErr) != "" {
		header = lipgloss.JoinVertical(lipgloss.Left, header, errorText("Error: "+m.actionErr))
	}
	separator := m.styles.Subtle.Render(strings.Repeat("-", max(10, width-2)))
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", top, separator, detail)
	return m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
}

func (m Model) renderEventFormMainPanel(width, panelHeight int) string {
	if m.eventForm == nil {
		return m.styles.MainPanel.Width(width).Height(panelHeight).Render("")
	}
	header := m.styles.PanelTitle.Render("Event")
	if m.eventForm.mode == "edit" {
		header = m.styles.PanelTitle.Render("Edit Event")
	}
	if label := eventEditScopeLabel(m.eventForm.editScope); m.eventForm.mode == "edit" && label != "" {
		header = lipgloss.JoinHorizontal(lipgloss.Left, header, m.styles.Subtle.Render("  ["+label+"]"))
	}
	body := m.renderEventEditorList(width-2, panelHeight-2)
	if strings.TrimSpace(m.eventForm.errMsg) != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, errorText("Error: "+m.eventForm.errMsg), "", body)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body)
	panel := m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
	if m.eventForm.attendeeManager != nil {
		modal := m.renderAttendeeManager(min(78, max(36, width-8)), max(9, min(panelHeight-4, (panelHeight*2)/3)))
		return overlayCentered(panel, modal, width, panelHeight)
	}
	if m.eventForm.noNotifications {
		modal := m.renderEmptyEditorDialog("Notifications", "No notifications", min(70, max(30, width-10)), max(7, panelHeight/3))
		return overlayCentered(panel, modal, width, panelHeight)
	}
	if m.eventForm.datePicker != nil {
		return overlayCentered(panel, m.eventForm.datePicker.View(m.styles), width, panelHeight)
	}
	if m.eventForm.timeEditor != nil {
		return overlayCentered(panel, m.eventForm.timeEditor.View(m.styles), width, panelHeight)
	}
	if m.eventForm.searchPicker != nil {
		modal := m.eventForm.searchPicker.View(min(70, max(38, width-12)), m.styles)
		return overlayCentered(panel, modal, width, panelHeight)
	}
	if m.eventForm.activeForm != nil {
		formHeight := max(7, panelHeight/3)
		if m.eventForm.activeKey == "description" {
			formHeight = max(12, (panelHeight*2)/3)
		}
		modal := activeFormModalView(m.eventForm.activeForm, min(70, max(30, width-10)), formHeight, m.eventForm.errMsg)
		return overlayCentered(panel, modal, width, panelHeight)
	}
	return panel
}

func (m Model) renderTodoFormMainPanel(width, panelHeight int) string {
	if m.todoForm == nil {
		return m.styles.MainPanel.Width(width).Height(panelHeight).Render("")
	}
	header := m.styles.PanelTitle.Render("Task")
	if m.todoForm.mode == "edit" {
		header = m.styles.PanelTitle.Render("Edit Task")
	}
	body := m.renderTodoEditorList(width-2, panelHeight-2)
	if strings.TrimSpace(m.todoForm.errMsg) != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, errorText("Error: "+m.todoForm.errMsg), "", body)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", body)
	panel := m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
	if m.todoForm.datePicker != nil {
		return overlayCentered(panel, m.todoForm.datePicker.View(m.styles), width, panelHeight)
	}
	if m.todoForm.timeEditor != nil {
		return overlayCentered(panel, m.todoForm.timeEditor.View(m.styles), width, panelHeight)
	}
	if m.todoForm.activeForm != nil {
		formHeight := max(7, panelHeight/3)
		if m.todoForm.activeKey == "description" {
			formHeight = max(12, (panelHeight*2)/3)
		}
		modal := activeFormModalView(m.todoForm.activeForm, min(70, max(30, width-10)), formHeight, m.todoForm.errMsg)
		return overlayCentered(panel, modal, width, panelHeight)
	}
	return panel
}

func (m Model) renderDeleteConfirmMainPanel(width, panelHeight int) string {
	if m.deleteConfirm == nil {
		return m.styles.MainPanel.Width(width).Height(panelHeight).Render("")
	}
	header := m.styles.PanelTitle.Render("Confirm Delete")
	formView := m.deleteConfirm.form.WithWidth(width - 2).WithHeight(panelHeight - 2).WithShowHelp(true).WithShowErrors(true).View()
	if strings.TrimSpace(m.deleteConfirm.errMsg) != "" {
		formView = lipgloss.JoinVertical(lipgloss.Left, errorText("Error: "+m.deleteConfirm.errMsg), "", formView)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", formView)
	return m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
}

func overlayCentered(base, modal string, width, height int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(min(width-8, max(30, lipgloss.Width(modal)+4))).
		Render(modal)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func activeFormModalView(form *huh.Form, width, height int, errMsg string) string {
	if form == nil {
		return ""
	}
	if strings.TrimSpace(errMsg) == "" {
		return form.WithWidth(width).WithHeight(height).WithShowHelp(true).WithShowErrors(true).View()
	}
	formHeight := max(3, height-2)
	view := form.WithWidth(width).WithHeight(formHeight).WithShowHelp(true).WithShowErrors(true).View()
	err := lipgloss.NewStyle().Width(width).Render(errorText("Error: " + errMsg))
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, err, "", view))
}

func errorText(msg string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("210")).
		Bold(true).
		Render(msg)
}

func (m Model) renderAttendeeManager(width, height int) string {
	width = max(28, width)
	height = max(6, height)
	mgr := m.eventForm.attendeeManager
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117")).Render("Attendees")
	legend := m.styles.Subtle.Render("[j/k] Move  [space/o] Optional  [x/d] Remove  [enter] Apply  [esc/q] Cancel")
	bodyHeight := max(1, height-2)
	lines := make([]string, 0, bodyHeight)
	if mgr == nil || len(mgr.attendees) == 0 {
		lines = append(lines, m.styles.Subtle.Render("No attendees"))
	} else {
		start := 0
		if mgr.cursor >= bodyHeight {
			start = mgr.cursor - bodyHeight + 1
		}
		end := min(len(mgr.attendees), start+bodyHeight)
		for i := start; i < end; i++ {
			item := mgr.attendees[i]
			selected := i == mgr.cursor
			prefix := "  "
			if selected {
				prefix = " "
			}
			glyph := ""
			glyphColor := lipgloss.Color("65")
			if attendeeIsOptional(item.attendee) {
				glyphColor = lipgloss.Color("244")
			}
			if item.remove {
				glyph = ""
				glyphColor = lipgloss.Color("210")
			}
			state := lipgloss.NewStyle().Foreground(glyphColor).Bold(true).Render(glyph)
			label := attendeeBaseLabel(item.attendee)
			valueBudget := max(8, width-lipgloss.Width(prefix)-lipgloss.Width(state)-2)
			line := prefix + state + " " + truncate(label, valueBudget)
			lines = append(lines, editorRowStyle(selected, width).Render(line))
		}
	}
	body := lipgloss.NewStyle().Width(width).Height(bodyHeight).MaxHeight(bodyHeight).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, title, body, legend))
}

func (m Model) renderEmptyEditorDialog(title, message string, width, height int) string {
	width = max(28, width)
	height = max(5, height)
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117")).Render(title)
	legend := m.styles.Subtle.Render("[enter/esc/q] Close")
	bodyHeight := max(1, height-2)
	body := lipgloss.NewStyle().Width(width).Height(bodyHeight).MaxHeight(bodyHeight).Render(m.styles.Subtle.Render(message))
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, body, legend),
	)
}

func (m Model) renderEventEditorList(width, height int) string {
	rows := m.eventEditorRows()
	if len(rows) == 0 {
		return ""
	}
	cur := nearestSelectableEditorCursor(rows, m.eventForm.cursor)
	return m.renderEditorRows(rows, cur, width, height)
}

func (m Model) renderTodoEditorList(width, height int) string {
	rows := m.todoEditorRows()
	if len(rows) == 0 {
		return ""
	}
	cur := nearestSelectableEditorCursor(rows, m.todoForm.cursor)
	return m.renderEditorRows(rows, cur, width, height)
}

func (m Model) renderEditorRows(rows []editorRow, cur, width, height int) string {
	lines := make([]string, 0, len(rows))
	selectedLine := 0
	for i, row := range rows {
		if isEditorSeparator(row) && len(lines) > 0 {
			lines = append(lines, "")
		}
		if i == cur {
			selectedLine = len(lines)
		}
		lines = append(lines, strings.Split(m.renderEditorRow(row, i == cur, width), "\n")...)
	}
	if len(lines) > height {
		start := 0
		if selectedLine >= height {
			start = selectedLine - height + 1
		}
		lines = lines[start:min(len(lines), start+height)]
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func (m Model) renderEditorRow(row editorRow, selected bool, width int) string {
	width = max(10, width)
	if isEditorSeparator(row) {
		label := " " + row.label + " "
		ruleWidth := max(0, width-lipgloss.Width(label)-2)
		return lipgloss.NewStyle().
			Width(width).
			Foreground(lipgloss.Color("244")).
			Bold(true).
			Render(label + strings.Repeat("─", ruleWidth))
	}

	disabled := isEditorDisabled(row)
	selected = selected && !disabled
	prefix := "  "
	if selected {
		prefix = " "
	}
	key := editorRowKey(row)
	if key == "attendees-add" || key == "alarms-add" || key == "form-cancel" || key == "form-save" {
		buttonStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 1)
		if key == "form-cancel" {
			buttonStyle = buttonStyle.Background(lipgloss.Color("238"))
		}
		line := prefix + buttonStyle.Render(editorButtonLabel(key))
		return editorRowStyle(selected, width).Render(line)
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	if disabled {
		labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	}
	label := labelStyle.Render(row.label)
	sep := m.styles.Subtle.Render(": ")
	valueBudget := max(1, width-lipgloss.Width(prefix)-lipgloss.Width(row.label)-2)
	if key == "attendees" {
		lines := attendeeEditorDisplayLines(row.value, valueBudget, valueStyle)
		indent := strings.Repeat(" ", lipgloss.Width(prefix)+lipgloss.Width(row.label)+2)
		rendered := make([]string, 0, len(lines))
		for i, valueLine := range lines {
			if i == 0 {
				rendered = append(rendered, prefix+label+sep+valueLine)
				continue
			}
			rendered = append(rendered, indent+valueLine)
		}
		return editorRowStyle(selected, width).Render(strings.Join(rendered, "\n"))
	}
	displayValue := editorDisplayValue(row)
	if editorRowWraps(row) {
		wrapped := wrapEditorValue(displayValue, valueBudget)
		indent := strings.Repeat(" ", lipgloss.Width(prefix)+lipgloss.Width(row.label)+2)
		lines := make([]string, 0, len(wrapped))
		for i, valueLine := range wrapped {
			value := valueStyle.Render(valueLine)
			if i == 0 {
				lines = append(lines, prefix+label+sep+value)
				continue
			}
			lines = append(lines, indent+value)
		}
		return editorRowStyle(selected, width).Render(strings.Join(lines, "\n"))
	}
	value := valueStyle.Render(truncate(displayValue, valueBudget))
	return editorRowStyle(selected, width).Render(prefix + label + sep + value)
}

func (m Model) eventEditorRows() []editorRow {
	if m.eventForm == nil {
		return nil
	}
	s := m.eventForm
	if m.eventFormAttendeeOnly() {
		rows := []editorRow{
			editorSeparatorRow("󰉢 Event"),
			{"calendar", "Calendar", m.calendarDisplayName(s.calendarKey)},
			editorSeparatorRow(" Response"),
			{"rsvp", "RSVP", eventRSVPDisplayValue(s.rsvp)},
			editorSeparatorRow("󰛐 Privacy"),
			{"availability", "Availability", eventAvailabilityDisplay(s.availability)},
			{"visibility", "Visibility", eventVisibilityDisplay(s.visibility)},
			editorSeparatorRow("󰀠 Notifications"),
			{"alarms", "Notifications", emptyDefault(s.alarms, "-")},
			{"alarms-add", "", ""},
		}
		return appendEditorFormActions(rows, s.mode)
	}
	rows := []editorRow{
		editorSeparatorRow("󰉢 Title"),
		{"title", "Title", emptyDefault(strings.TrimSpace(s.summary), "(untitled event)")},
		{"calendar", "Calendar", m.calendarDisplayName(s.calendarKey)},
		editorSeparatorRow(" Place"),
		{"location", "Location", emptyDefault(s.location, "-")},
		{"url", "URL", emptyDefault(s.url, "-")},
		editorSeparatorRow("󰦨 Description"),
		{"description", "Description", multilineValue(s.description)},
		editorSeparatorRow(" Attendees"),
		{"attendees", "Attendees", emptyDefault(s.attendees, "-")},
		{"rsvp", "RSVP", eventRSVPDisplayValue(s.rsvp)},
		editorSeparatorRow("󰛐 Privacy"),
		{"availability", "Availability", eventAvailabilityDisplay(s.availability)},
		{"visibility", "Visibility", eventVisibilityDisplay(s.visibility)},
		editorSeparatorRow("󰥔 Time"),
		{"all-day", "All-day", yesNo(s.allDay)},
	}
	if s.mode != "view" {
		rows = append(rows[:11], append([]editorRow{{"attendees-add", "", ""}}, rows[11:]...)...)
	}
	timeKey := "time"
	timezoneKey := "timezone"
	if s.allDay {
		timeKey += editorDisabledSuffix
		timezoneKey += editorDisabledSuffix
	}
	rows = append(rows,
		editorRow{"date", "Date", fmt.Sprintf("%s → %s", s.fromDate, s.toDate)},
		editorRow{timeKey, "Time", fmt.Sprintf("%s -> %s", s.fromTime, s.toTime)},
		editorRow{timezoneKey, "Timezone", eventTimezoneDisplay(s)},
	)
	if s.editScope != string(calendar.EditRecurringOccurrence) {
		rows = append(rows,
			editorSeparatorRow("󰑖 Repeat"),
			editorRow{"recur", "Repeat", repeatValue(s)},
		)
		if s.recur {
			rows = append(rows, editorRow{"recur-every", "Frequency", emptyDefault(s.recurEvery, "1")})
			if s.recurFreq == "WEEKLY" {
				rows = append(rows, editorRow{"recur-weekdays", "Weekday", emptyDefault(strings.Join(s.recurWeekdays, ", "), "-")})
			}
			if s.recurFreq == "MONTHLY" {
				rows = append(rows, editorRow{"recur-monthly-by", "By", emptyDefault(s.recurMonthlyBy, "month day")})
			}
			rows = append(rows, editorRow{"recur-end", "Until", repeatEndValue(s)})
			if s.recurEnd == "until" {
				rows = append(rows, editorRow{"recur-until", "Repeat until (YYYY-MM-DD)", emptyDefault(s.recurUntil, "-")})
			}
			if s.recurEnd == "count" {
				rows = append(rows, editorRow{"recur-count", "Repeat count", emptyDefault(s.recurCount, "-")})
			}
		}
	}
	rows = append(rows,
		editorSeparatorRow("󰀠 Notifications"),
		editorRow{"alarms", "Notifications", emptyDefault(s.alarms, "-")},
	)
	if s.mode != "view" {
		rows = append(rows, editorRow{"alarms-add", "", ""})
	}
	return appendEditorFormActions(rows, s.mode)
}

func (m Model) eventFormAttendeeOnly() bool {
	if m.store == nil || m.eventForm == nil || m.eventForm.targetEvent == nil {
		return false
	}
	return m.eventForm.mode == "edit" && m.store.EventUserRole(*m.eventForm.targetEvent) == calendar.EventUserRoleAttendee
}

func (m Model) todoEditorRows() []editorRow {
	if m.todoForm == nil {
		return nil
	}
	s := m.todoForm
	dueTimeKey := "due-time"
	if strings.TrimSpace(s.dueDate) == "" {
		dueTimeKey += editorDisabledSuffix
	}
	startTimeKey := "start-time"
	if strings.TrimSpace(s.startDate) == "" {
		startTimeKey += editorDisabledSuffix
	}
	rows := []editorRow{
		editorSeparatorRow("󰉢 Title"),
		{"summary", "Title", emptyDefault(strings.TrimSpace(s.summary), "(untitled task)")},
		{"calendar", "Calendar", m.calendarDisplayName(s.calendarKey)},
		editorSeparatorRow(" Place"),
		{"location", "Location", emptyDefault(s.location, "-")},
		editorSeparatorRow("󰦨 Description"),
		{"description", "Description", multilineValue(s.description)},
		editorSeparatorRow("󰥔 Schedule"),
		{"due-date", "Due date", emptyDefault(s.dueDate, "-")},
		{dueTimeKey, "Due time", emptyDefault(s.dueTime, "-")},
		{"start-date", "Start date", emptyDefault(s.startDate, "-")},
		{startTimeKey, "Start time", emptyDefault(s.startTime, "-")},
		editorSeparatorRow("󰄬 Status"),
		{"completed", "Completed", yesNo(s.completed)},
		{"priority", "Priority", emptyDefault(s.priorityLabel, "mid")},
	}
	return appendEditorFormActions(rows, s.mode)
}

func (m *Model) updateEventEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.eventForm
	if s.mode == "view" {
		switch msg.String() {
		case "esc", "q":
			m.eventForm = nil
			m.focusDetails = false
			m.focusMain = true
		case "e":
			view := m.eventForm
			m.eventForm = nil
			if m.openEventFormEditSelected() {
				return m, m.initCurrentEventForm()
			}
			m.eventForm = view
		case "j", "down", "tab":
			s.cursor = moveEditorCursor(m.eventEditorRows(), s.cursor, 1)
		case "k", "up", "shift+tab":
			s.cursor = moveEditorCursor(m.eventEditorRows(), s.cursor, -1)
		}
		return m, nil
	}
	if s.noNotifications {
		switch msg.String() {
		case "enter", "esc", "ctrl+c", "q":
			s.noNotifications = false
			s.activeKey = ""
			s.backup = nil
		}
		return m, nil
	}
	if s.datePicker != nil {
		m.updateEventDatePicker(msg)
		return m, nil
	}
	if s.timeEditor != nil {
		m.updateEventTimeEditor(msg)
		return m, nil
	}
	if s.searchPicker != nil {
		m.updateEventSearchPicker(msg)
		return m, nil
	}
	if s.attendeeManager != nil {
		switch msg.String() {
		case "esc", "ctrl+c", "q":
			s.attendeeManager = nil
			s.activeKey = ""
			s.backup = nil
		case "enter":
			s.attendees = attendeesInput(attendeeManagerValues(s.attendeeManager))
			s.attendeeManager = nil
			s.activeKey = ""
			s.backup = nil
			s.errMsg = ""
		case "j", "down", "tab":
			s.attendeeManager.cursor = min(len(s.attendeeManager.attendees)-1, s.attendeeManager.cursor+1)
		case "k", "up", "shift+tab":
			s.attendeeManager.cursor = max(0, s.attendeeManager.cursor-1)
		case " ", "space", "o":
			toggleAttendeeOptional(s.attendeeManager)
		case "x", "d", "backspace":
			toggleAttendeeRemove(s.attendeeManager)
		}
		return m, nil
	}
	if s.activeForm != nil {
		if s.activeKey == "recur-every" || s.activeKey == "recur-count" {
			filtered, ok := filterRecurrenceNumberKey(msg)
			if !ok {
				return m, nil
			}
			msg = filtered
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			s.cancelActive()
			return m, nil
		}
		return m, m.updateActiveEventEditorForm(msg)
	}
	switch msg.String() {
	case "ctrl+s":
		if err := m.saveEventEditor(); err != nil {
			s.errMsg = err.Error()
		}
	case "ctrl+c", "esc", "q":
		m.closeEventEditor()
	case "j", "down", "tab":
		s.cursor = moveEditorCursor(m.eventEditorRows(), s.cursor, 1)
	case "k", "up", "shift+tab":
		s.cursor = moveEditorCursor(m.eventEditorRows(), s.cursor, -1)
	case "h", "left":
		m.cycleEventEditorValue(-1)
	case "l", "right":
		m.cycleEventEditorValue(1)
	case "enter":
		if m.toggleEventEditorBoolean() {
			return m, nil
		}
		cmd := m.openEventEditorForm()
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateTodoEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.todoForm
	if s.mode == "view" {
		switch msg.String() {
		case "esc", "q":
			m.todoForm = nil
			m.focusDetails = false
			m.focusMain = true
		case "e":
			view := m.todoForm
			m.todoForm = nil
			if m.openTodoFormEditSelected() {
				return m, m.todoForm.form.Init()
			}
			m.todoForm = view
		case "j", "down", "tab":
			s.cursor = moveEditorCursor(m.todoEditorRows(), s.cursor, 1)
		case "k", "up", "shift+tab":
			s.cursor = moveEditorCursor(m.todoEditorRows(), s.cursor, -1)
		}
		return m, nil
	}
	if s.datePicker != nil {
		m.updateTodoDatePicker(msg)
		return m, nil
	}
	if s.timeEditor != nil {
		m.updateTodoTimeEditor(msg)
		return m, nil
	}
	if s.activeForm != nil {
		switch msg.String() {
		case "esc", "ctrl+c":
			s.cancelActive()
			return m, nil
		}
		return m, m.updateActiveTodoEditorForm(msg)
	}
	switch msg.String() {
	case "ctrl+s":
		if err := m.saveTodoEditor(); err != nil {
			s.errMsg = err.Error()
		}
	case "ctrl+c", "esc", "q":
		m.closeTodoEditor()
	case "j", "down", "tab":
		s.cursor = moveEditorCursor(m.todoEditorRows(), s.cursor, 1)
	case "k", "up", "shift+tab":
		s.cursor = moveEditorCursor(m.todoEditorRows(), s.cursor, -1)
	case "h", "left":
		m.cycleTodoEditorValue(-1)
	case "l", "right":
		m.cycleTodoEditorValue(1)
	case "delete":
		m.clearTodoSchedule()
	case "enter":
		if m.toggleTodoEditorBoolean() {
			return m, nil
		}
		cmd := m.openTodoEditorForm()
		return m, cmd
	}
	return m, nil
}

func (m *Model) openEventEditorForm() tea.Cmd {
	rows := m.eventEditorRows()
	if len(rows) == 0 {
		return nil
	}
	s := m.eventForm
	s.cursor = nearestSelectableEditorCursor(rows, s.cursor)
	row := rows[s.cursor]
	if !isEditorSelectable(row) {
		return nil
	}
	key := editorRowKey(row)
	s.activeKey = key
	s.backup = s.snapshot()
	s.errMsg = ""
	if m.openCustomEventEditor(key) {
		return nil
	}
	if key == "attendees" {
		s.attendeeManager = newAttendeeManager(parseAttendeesInput(s.attendees))
		return nil
	}
	if key == "alarms" && len(splitListInput(s.alarms)) == 0 {
		s.noNotifications = true
		return nil
	}
	s.activeForm = m.buildEventEditorForm(key)
	if s.activeForm == nil {
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	s.activeForm.WithKeyMap(eventEditorFormKeyMap(key))
	return s.activeForm.Init()
}

func (m *Model) openEventEditScopeForm() tea.Cmd {
	s := m.eventForm
	if s == nil {
		return nil
	}
	s.activeKey = "edit-scope"
	s.errMsg = ""
	s.activeForm = m.buildEventEditorForm("edit-scope")
	if s.activeForm == nil {
		s.activeKey = ""
		return nil
	}
	s.activeForm.WithKeyMap(NewPreferredFormKeyMap())
	return s.activeForm.Init()
}

func (m *Model) updateActiveEventEditorForm(msg tea.Msg) tea.Cmd {
	s := m.eventForm
	if s == nil || s.activeForm == nil {
		return nil
	}
	updated, cmd := s.activeForm.Update(msg)
	if fm, ok := updated.(*huh.Form); ok {
		s.activeForm = fm
	}
	if s.activeForm.State == huh.StateAborted {
		s.cancelActive()
		return nil
	}
	if s.activeForm.State == huh.StateCompleted {
		if err := m.applyEventEditorForm(); err != nil {
			s.errMsg = err.Error()
			s.activeForm = m.buildEventEditorForm(s.activeKey)
			if s.activeForm == nil {
				return nil
			}
			s.activeForm.WithKeyMap(eventEditorFormKeyMap(s.activeKey))
			return s.activeForm.Init()
		}
		s.errMsg = ""
		s.activeForm = nil
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	return cmd
}

type editorChoice struct {
	label string
	value string
}

var (
	eventRSVPChoices = [...]editorChoice{
		{"Unspecified", ""},
		{"Yes", "yes"},
		{"No", "no"},
		{"Maybe", "maybe"},
		{"No response", "needs-action"},
	}
	eventAvailabilityChoices = [...]editorChoice{
		{"Calendar default", ""},
		{"Busy", "busy"},
		{"Free", "free"},
	}
	eventVisibilityChoices = [...]editorChoice{
		{"Calendar default", "default"},
		{"Public", "public"},
		{"Private", "private"},
		{"Confidential", "confidential"},
	}
	eventRepeatChoices = [...]editorChoice{
		{"None", "none"},
		{"Daily", "daily"},
		{"Weekly", "weekly"},
		{"Monthly", "monthly"},
		{"Yearly", "yearly"},
	}
	eventMonthlyByChoices = [...]editorChoice{
		{"On every month day", "month day"},
		{"On every weekday ordinal", "weekday ordinal"},
	}
	eventRepeatEndChoices = [...]editorChoice{
		{"Forever", "forever"},
		{"Until date", "until"},
		{"Fixed count", "count"},
	}
	todoPriorityChoices = [...]editorChoice{
		{"Low", "low"},
		{"Mid", "mid"},
		{"High", "high"},
	}
)

func editorChoiceOptions(choices []editorChoice) []huh.Option[string] {
	options := make([]huh.Option[string], len(choices))
	for i, choice := range choices {
		options[i] = huh.NewOption(choice.label, choice.value)
	}
	return options
}

func cycleEditorChoice(current string, choices []editorChoice, delta int) string {
	if len(choices) == 0 {
		return current
	}
	for i, choice := range choices {
		if choice.value != current {
			continue
		}
		next := (i + delta) % len(choices)
		if next < 0 {
			next += len(choices)
		}
		return choices[next].value
	}
	if delta < 0 {
		return choices[len(choices)-1].value
	}
	return choices[0].value
}

func setEventRepeat(s *eventFormState, repeat string) {
	s.recur = repeat != "none"
	if s.recur {
		s.recurFreq = strings.ToUpper(repeat)
	}
}

func (m *Model) buildEventEditorForm(key string) *huh.Form {
	s := m.eventForm
	switch key {
	case "edit-scope":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Edit recurring event").Options(
			huh.NewOption("Only this occurrence", string(calendar.EditRecurringOccurrence)),
			huh.NewOption("This and following occurrences", string(calendar.EditRecurringFuture)),
			huh.NewOption("All occurrences", string(calendar.EditRecurringAll)),
		).Value(&s.editScope))).WithShowHelp(true).WithShowErrors(true)
	case "title":
		return singleInputForm("Title", "value", &s.summary, func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("title is required")
			}
			return nil
		})
	case "location":
		return singleInputForm("Location", "value", &s.location, nil)
	case "description":
		return huh.NewForm(huh.NewGroup(huh.NewText().Key("value").Title("Description").Value(&s.description).Lines(12))).WithShowHelp(true).WithShowErrors(true)
	case "url":
		return singleInputForm("URL", "value", &s.url, nil)
	case "calendar":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Calendar").Options(m.calendarOptions()...).Value(&s.calendarKey))).WithShowHelp(true).WithShowErrors(true)
	case "attendees":
		values := splitListInput(s.attendees)
		return huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Key("value").Title("Attendees").Options(selectedOptions(values)...).Value(&values).WithKeyMap(attendeeMultiSelectKeyMap()))).WithShowHelp(true).WithShowErrors(true)
	case "attendees-add":
		values := []string{}
		return huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Key("value").Title("Add attendees").Options(m.attendeeOptions()...).Filterable(true).Value(&values).WithKeyMap(attendeeMultiSelectKeyMap()))).WithShowHelp(true).WithShowErrors(true)
	case "rsvp":
		value := s.rsvp
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("RSVP").Options(editorChoiceOptions(eventRSVPChoices[:])...).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "availability":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Availability").Options(editorChoiceOptions(eventAvailabilityChoices[:])...).Value(&s.availability))).WithShowHelp(true).WithShowErrors(true)
	case "visibility":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Visibility").Options(editorChoiceOptions(eventVisibilityChoices[:])...).Value(&s.visibility))).WithShowHelp(true).WithShowErrors(true)
	case "alarms":
		values := splitListInput(s.alarms)
		return huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Key("value").Title("Notifications").Options(selectedOptions(values)...).Filterable(false).Value(&values))).WithShowHelp(true).WithShowErrors(true)
	case "alarms-add":
		value := ""
		return huh.NewForm(huh.NewGroup(huh.NewInput().
			Key("value").
			Title("Add notification").
			Description("Examples: 10m before, 2h before, 10d before, 1d after").
			Value(&value).
			Validate(validateAlarmsInput),
		)).WithShowHelp(true).WithShowErrors(true)
	case "recur":
		value := repeatValue(s)
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Repeat").Options(editorChoiceOptions(eventRepeatChoices[:])...).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "recur-every":
		return singleInputForm("Frequency", "value", &s.recurEvery, validateRecurrenceNumberInput)
	case "recur-weekdays":
		values := append([]string{}, s.recurWeekdays...)
		return huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Key("value").Title("Weekday").Options(weekdayOptions(values)...).Value(&values))).WithShowHelp(true).WithShowErrors(true)
	case "recur-monthly-by":
		value := s.recurMonthlyBy
		if value == "" {
			value = "month day"
		}
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("By").Options(editorChoiceOptions(eventMonthlyByChoices[:])...).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "recur-end":
		value := s.recurEnd
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Until").Options(editorChoiceOptions(eventRepeatEndChoices[:])...).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "recur-until":
		return singleInputForm("Repeat until (YYYY-MM-DD)", "value", &s.recurUntil, validateEventDateInput)
	case "recur-count":
		return singleInputForm("Repeat count", "value", &s.recurCount, validateRecurrenceNumberInput)
	case "all-day":
		return huh.NewForm(huh.NewGroup(huh.NewConfirm().Key("value").Title("All-day").Value(&s.allDay))).WithShowHelp(true).WithShowErrors(true)
	case "when":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Key("from-date").Title("Start date").Value(&s.fromDate).Validate(validateEventDateInput),
			huh.NewInput().Key("from-time").Title("Start time").Value(&s.fromTime).Validate(validateEventTimeInput),
			huh.NewInput().Key("to-date").Title("End date").Value(&s.toDate).Validate(validateEventDateInput),
			huh.NewInput().Key("to-time").Title("End time").Value(&s.toTime).Validate(validateEventTimeInput),
		)).WithShowHelp(true).WithShowErrors(true)
	}
	return nil
}

func (m *Model) applyEventEditorForm() error {
	s := m.eventForm
	f := s.activeForm
	value := activeFormValue(f)
	switch s.activeKey {
	case "edit-scope":
		s.editScope = anyString(value)
	case "attendees":
		s.attendees = strings.Join(anyStringSlice(value), "; ")
	case "attendees-add":
		s.attendees = mergeListInput(s.attendees, anyStringSlice(value))
	case "rsvp":
		s.rsvp = anyString(value)
	case "alarms":
		s.alarms = strings.Join(anyStringSlice(value), "; ")
	case "alarms-add":
		added := strings.TrimSpace(anyString(value))
		if added != "" {
			if err := validateAlarmsInput(added); err != nil {
				return err
			}
			s.alarms = mergeListInput(s.alarms, []string{added})
		}
	case "recur":
		setEventRepeat(s, anyString(value))
	case "recur-every":
		number, err := parseRecurrenceNumberInput(anyString(value))
		if err != nil {
			return err
		}
		s.recurEvery = strconv.Itoa(number)
	case "recur-count":
		number, err := parseRecurrenceNumberInput(anyString(value))
		if err != nil {
			return err
		}
		s.recurCount = strconv.Itoa(number)
	case "recur-weekdays":
		s.recurWeekdays = anyStringSlice(value)
	case "recur-monthly-by":
		s.recurMonthlyBy = anyString(value)
	case "recur-end":
		s.recurEnd = anyString(value)
	case "all-day":
		if v, ok := value.(bool); ok {
			if s.backup != nil && s.backup.allDay != v {
				s.timingDirty = true
			}
			s.allDay = v
		}
	}
	if s.activeKey == "when" {
		if _, _, err := parseEventFormTimes(*s); err != nil {
			return err
		}
	}
	return nil
}

func (m *Model) openTodoEditorForm() tea.Cmd {
	rows := m.todoEditorRows()
	if len(rows) == 0 {
		return nil
	}
	s := m.todoForm
	s.cursor = nearestSelectableEditorCursor(rows, s.cursor)
	row := rows[s.cursor]
	if !isEditorSelectable(row) {
		return nil
	}
	key := editorRowKey(row)
	switch key {
	case "form-cancel":
		m.closeTodoEditor()
		return nil
	case "form-save":
		if err := m.saveTodoEditor(); err != nil {
			s.errMsg = err.Error()
		}
		return nil
	}
	s.activeKey = key
	s.backup = s.snapshot()
	s.errMsg = ""
	if m.openCustomTodoEditor(key) {
		return nil
	}
	s.activeForm = m.buildTodoEditorForm(key)
	if s.activeForm == nil {
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	s.activeForm.WithKeyMap(todoEditorFormKeyMap(key))
	return s.activeForm.Init()
}

func (m *Model) updateActiveTodoEditorForm(msg tea.Msg) tea.Cmd {
	s := m.todoForm
	if s == nil || s.activeForm == nil {
		return nil
	}
	updated, cmd := s.activeForm.Update(msg)
	if fm, ok := updated.(*huh.Form); ok {
		s.activeForm = fm
	}
	if s.activeForm.State == huh.StateAborted {
		s.cancelActive()
		return nil
	}
	if s.activeForm.State == huh.StateCompleted {
		s.errMsg = ""
		s.activeForm = nil
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	return cmd
}

func (m *Model) buildTodoEditorForm(key string) *huh.Form {
	s := m.todoForm
	switch key {
	case "summary":
		return singleInputForm("Title", "value", &s.summary, func(v string) error {
			if strings.TrimSpace(v) == "" {
				return errors.New("title is required")
			}
			return nil
		})
	case "description":
		return huh.NewForm(huh.NewGroup(huh.NewText().Key("value").Title("Description").Value(&s.description).Lines(12))).WithShowHelp(true).WithShowErrors(true)
	case "location":
		return singleInputForm("Location", "value", &s.location, nil)
	case "calendar":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Calendar").Options(m.calendarOptions()...).Value(&s.calendarKey))).WithShowHelp(true).WithShowErrors(true)
	case "completed":
		return huh.NewForm(huh.NewGroup(huh.NewConfirm().Key("value").Title("Completed").Value(&s.completed))).WithShowHelp(true).WithShowErrors(true)
	case "priority":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Priority").Options(editorChoiceOptions(todoPriorityChoices[:])...).Value(&s.priorityLabel))).WithShowHelp(true).WithShowErrors(true)
	}
	return nil
}

func singleInputForm(title, key string, value *string, validate func(string) error) *huh.Form {
	input := huh.NewInput().Key(key).Title(title).Value(value)
	if validate != nil {
		input.Validate(validate)
	}
	return huh.NewForm(huh.NewGroup(input)).WithShowHelp(true).WithShowErrors(true)
}

func (m Model) calendarOptionLabels() map[string]string {
	labels := make(map[string]string, len(m.calendarOrder))
	counts := map[string]int{}
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		label := cal.DisplayName
		if label == "" {
			label = cal.Name
		}
		labels[key] = label
		counts[label]++
	}
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		label := labels[key]
		if counts[label] > 1 && cal.Name != "" && cal.Name != label {
			labels[key] = label + " - " + cal.Name
		}
	}
	return labels
}

func (m Model) calendarOptions() []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(m.calendarOrder))
	labels := m.calendarOptionLabels()
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		out = append(out, huh.NewOption(labels[key], key))
	}
	if len(out) == 0 {
		out = append(out, huh.NewOption("No writable calendar", ""))
	}
	return out
}

func (m Model) cycleCalendarKey(current string, delta int) string {
	first := ""
	last := ""
	previous := ""
	useNext := false
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		if first == "" {
			first = key
		}
		last = key
		if useNext {
			return key
		}
		if key == current {
			if delta < 0 && previous != "" {
				return previous
			}
			useNext = delta > 0
		}
		previous = key
	}
	if first == "" {
		return current
	}
	if delta < 0 {
		return last
	}
	return first
}

func cycleRecurrenceValue(value string, delta int) string {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 {
		number = 1
	}
	if number > 99 {
		if delta < 0 {
			return "99"
		}
		return "1"
	}
	number += delta
	if number < 1 {
		number = 99
	} else if number > 99 {
		number = 1
	}
	return strconv.Itoa(number)
}

func (m *Model) cycleEventEditorValue(delta int) {
	rows := m.eventEditorRows()
	if len(rows) == 0 {
		return
	}
	s := m.eventForm
	s.cursor = nearestSelectableEditorCursor(rows, s.cursor)
	if !isEditorSelectable(rows[s.cursor]) {
		return
	}
	switch editorRowKey(rows[s.cursor]) {
	case "calendar":
		s.calendarKey = m.cycleCalendarKey(s.calendarKey, delta)
	case "rsvp":
		s.rsvp = cycleEditorChoice(s.rsvp, eventRSVPChoices[:], delta)
	case "availability":
		s.availability = cycleEditorChoice(s.availability, eventAvailabilityChoices[:], delta)
	case "visibility":
		s.visibility = cycleEditorChoice(s.visibility, eventVisibilityChoices[:], delta)
	case "all-day":
		toggleEventAllDay(s)
	case "recur":
		setEventRepeat(s, cycleEditorChoice(repeatValue(s), eventRepeatChoices[:], delta))
	case "recur-every":
		s.recurEvery = cycleRecurrenceValue(s.recurEvery, delta)
	case "recur-count":
		s.recurCount = cycleRecurrenceValue(s.recurCount, delta)
	case "recur-monthly-by":
		s.recurMonthlyBy = cycleEditorChoice(s.recurMonthlyBy, eventMonthlyByChoices[:], delta)
	case "recur-end":
		s.recurEnd = cycleEditorChoice(s.recurEnd, eventRepeatEndChoices[:], delta)
	}
}

func toggleEventAllDay(s *eventFormState) {
	s.allDay = !s.allDay
	s.timingDirty = true
}

func (m *Model) toggleEventEditorBoolean() bool {
	rows := m.eventEditorRows()
	if len(rows) == 0 {
		return false
	}
	s := m.eventForm
	s.cursor = nearestSelectableEditorCursor(rows, s.cursor)
	if !isEditorSelectable(rows[s.cursor]) || editorRowKey(rows[s.cursor]) != "all-day" {
		return false
	}
	toggleEventAllDay(s)
	return true
}

func (m *Model) cycleTodoEditorValue(delta int) {
	rows := m.todoEditorRows()
	if len(rows) == 0 {
		return
	}
	s := m.todoForm
	s.cursor = nearestSelectableEditorCursor(rows, s.cursor)
	if !isEditorSelectable(rows[s.cursor]) {
		return
	}
	switch editorRowKey(rows[s.cursor]) {
	case "calendar":
		s.calendarKey = m.cycleCalendarKey(s.calendarKey, delta)
	case "completed":
		s.completed = !s.completed
	case "priority":
		s.priorityLabel = cycleEditorChoice(s.priorityLabel, todoPriorityChoices[:], delta)
	}
}

func (m *Model) toggleTodoEditorBoolean() bool {
	rows := m.todoEditorRows()
	if len(rows) == 0 {
		return false
	}
	s := m.todoForm
	s.cursor = nearestSelectableEditorCursor(rows, s.cursor)
	if !isEditorSelectable(rows[s.cursor]) || editorRowKey(rows[s.cursor]) != "completed" {
		return false
	}
	s.completed = !s.completed
	return true
}

func (m Model) attendeeOptions() []huh.Option[string] {
	suggestions := m.attendeeSuggestions()
	out := make([]huh.Option[string], 0, len(suggestions))
	for _, suggestion := range suggestions {
		out = append(out, huh.NewOption(suggestion, suggestion))
	}
	if len(out) == 0 {
		out = append(out, huh.NewOption("No contacts found", ""))
	}
	return out
}

func selectedOptions(values []string) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(values))
	for _, value := range values {
		out = append(out, huh.NewOption(value, value).Selected(true))
	}
	return out
}

func weekdayOptions(selected []string) []huh.Option[string] {
	selectedMap := map[string]bool{}
	for _, v := range selected {
		selectedMap[v] = true
	}
	days := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	out := make([]huh.Option[string], 0, len(days))
	for _, day := range days {
		out = append(out, huh.NewOption(day, day).Selected(selectedMap[day]))
	}
	return out
}

func (s *eventFormState) snapshot() *eventFormSnapshot {
	if s == nil {
		return nil
	}
	return &eventFormSnapshot{
		editScope:      s.editScope,
		summary:        s.summary,
		calendarKey:    s.calendarKey,
		location:       s.location,
		description:    s.description,
		url:            s.url,
		attendees:      s.attendees,
		rsvp:           s.rsvp,
		availability:   s.availability,
		visibility:     s.visibility,
		alarms:         s.alarms,
		recur:          s.recur,
		recurFreq:      s.recurFreq,
		recurEvery:     s.recurEvery,
		recurWeekdays:  append([]string{}, s.recurWeekdays...),
		recurMonthlyBy: s.recurMonthlyBy,
		recurEnd:       s.recurEnd,
		recurUntil:     s.recurUntil,
		recurCount:     s.recurCount,
		allDay:         s.allDay,
		fromDate:       s.fromDate,
		fromTime:       s.fromTime,
		toDate:         s.toDate,
		toTime:         s.toTime,
		timezone:       s.timezone,
		timezoneLocal:  s.timezoneLocal,
		timingDirty:    s.timingDirty,
		overnightAuto:  s.overnightAuto,
	}
}

func (s *eventFormState) cancelActive() {
	if s == nil {
		return
	}
	if b := s.backup; b != nil {
		s.editScope = b.editScope
		s.summary = b.summary
		s.calendarKey = b.calendarKey
		s.location = b.location
		s.description = b.description
		s.url = b.url
		s.attendees = b.attendees
		s.rsvp = b.rsvp
		s.availability = b.availability
		s.visibility = b.visibility
		s.alarms = b.alarms
		s.recur = b.recur
		s.recurFreq = b.recurFreq
		s.recurEvery = b.recurEvery
		s.recurWeekdays = append([]string{}, b.recurWeekdays...)
		s.recurMonthlyBy = b.recurMonthlyBy
		s.recurEnd = b.recurEnd
		s.recurUntil = b.recurUntil
		s.recurCount = b.recurCount
		s.allDay = b.allDay
		s.fromDate = b.fromDate
		s.fromTime = b.fromTime
		s.toDate = b.toDate
		s.toTime = b.toTime
		s.timezone = b.timezone
		s.timezoneLocal = b.timezoneLocal
		s.timingDirty = b.timingDirty
		s.overnightAuto = b.overnightAuto
	}
	s.activeForm = nil
	s.attendeeManager = nil
	s.noNotifications = false
	s.datePicker = nil
	s.timeEditor = nil
	s.searchPicker = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (s *todoFormState) snapshot() *todoFormSnapshot {
	if s == nil {
		return nil
	}
	return &todoFormSnapshot{
		summary:       s.summary,
		description:   s.description,
		location:      s.location,
		calendarKey:   s.calendarKey,
		startDate:     s.startDate,
		startTime:     s.startTime,
		dueDate:       s.dueDate,
		dueTime:       s.dueTime,
		completed:     s.completed,
		priorityLabel: s.priorityLabel,
	}
}

func (s *todoFormState) cancelActive() {
	if s == nil {
		return
	}
	if b := s.backup; b != nil {
		s.summary = b.summary
		s.description = b.description
		s.location = b.location
		s.calendarKey = b.calendarKey
		s.startDate = b.startDate
		s.startTime = b.startTime
		s.dueDate = b.dueDate
		s.dueTime = b.dueTime
		s.completed = b.completed
		s.priorityLabel = b.priorityLabel
	}
	s.activeForm = nil
	s.datePicker = nil
	s.timeEditor = nil
	s.activeKey = ""
	s.backup = nil
	s.errMsg = ""
}

func (m Model) calendarDisplayName(key string) string {
	cal := m.calendarByKey(key)
	if cal == nil {
		return "-"
	}
	name := cal.DisplayName
	if name == "" {
		name = cal.Name
	}
	return name
}

func repeatValue(s *eventFormState) string {
	if s == nil || !s.recur {
		return "none"
	}
	switch strings.ToUpper(s.recurFreq) {
	case "DAILY":
		return "daily"
	case "WEEKLY":
		return "weekly"
	case "MONTHLY":
		return "monthly"
	case "YEARLY":
		return "yearly"
	default:
		return "daily"
	}
}

func repeatEndValue(s *eventFormState) string {
	if s == nil {
		return "forever"
	}
	switch s.recurEnd {
	case "until":
		return "until date"
	case "count":
		return "fixed count"
	default:
		return "forever"
	}
}

func eventEditScopeLabel(scope string) string {
	switch calendar.EditRecurringScope(scope) {
	case calendar.EditRecurringOccurrence:
		return "only this occurrence"
	case calendar.EditRecurringFuture:
		return "this and following"
	case calendar.EditRecurringAll:
		return "all occurrences"
	default:
		return ""
	}
}

var errInvalidRecurrenceNumber = errors.New("value must be a number from 1 to 99")

func filterRecurrenceNumberKey(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if msg.Type != tea.KeyRunes {
		return msg, true
	}
	digitCount := 0
	for _, r := range msg.Runes {
		if r >= '0' && r <= '9' {
			digitCount++
		}
	}
	if digitCount == 0 {
		return msg, false
	}
	if digitCount == len(msg.Runes) {
		return msg, true
	}
	digits := make([]rune, 0, digitCount)
	for _, r := range msg.Runes {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	msg.Runes = digits
	return msg, true
}

func parseRecurrenceNumberInput(value string) (int, error) {
	if value == "" {
		return 0, errInvalidRecurrenceNumber
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, errInvalidRecurrenceNumber
		}
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || number > 99 {
		return 0, errInvalidRecurrenceNumber
	}
	return number, nil
}

func validateRecurrenceNumberInput(value string) error {
	_, err := parseRecurrenceNumberInput(value)
	return err
}

func anyStringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return typed
	case *[]string:
		if typed == nil {
			return nil
		}
		return *typed
	default:
		return nil
	}
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func activeFormValue(form *huh.Form) any {
	if form == nil {
		return nil
	}
	field := form.GetFocusedField()
	if field == nil {
		return nil
	}
	return field.GetValue()
}

const editorDisabledSuffix = ":disabled"

func appendEditorFormActions(rows []editorRow, mode string) []editorRow {
	if mode == "view" {
		return rows
	}
	return append(rows,
		editorSeparatorRow("Actions"),
		editorRow{"form-cancel", "", ""},
		editorRow{"form-save", "", ""},
	)
}

func editorRowKey(row editorRow) string {
	return strings.TrimSuffix(row.key, editorDisabledSuffix)
}

func isEditorDisabled(row editorRow) bool {
	return strings.HasSuffix(row.key, editorDisabledSuffix)
}

func isEditorSelectable(row editorRow) bool {
	return !isEditorSeparator(row) && !isEditorDisabled(row)
}

func eventTimezoneDisplay(s *eventFormState) string {
	if s == nil {
		return "-"
	}
	at, err := time.Parse("2006-01-02 15:04", s.fromDate+" "+s.fromTime)
	if err != nil {
		at = time.Now()
	}
	label := timezoneLabel(s.timezone, at)
	if s.timezoneLocal {
		return "Local — " + label
	}
	return label
}

func editorSeparatorRow(label string) editorRow {
	return editorRow{key: "__separator:" + label, label: label}
}

func isEditorSeparator(row editorRow) bool {
	return strings.HasPrefix(row.key, "__separator:")
}

func editorRowWraps(row editorRow) bool {
	key := editorRowKey(row)
	return key == "description" || key == "attendees"
}

func editorDisplayValue(row editorRow) string {
	if editorRowKey(row) == "attendees" {
		return strings.ReplaceAll(row.value, "; ", "\n")
	}
	return row.value
}

func attendeeEditorDisplayLines(raw string, width int, valueStyle lipgloss.Style) []string {
	attendees := parseAttendeesInput(raw)
	if len(attendees) == 0 {
		return []string{valueStyle.Render("-")}
	}
	width = max(1, width)
	lines := make([]string, 0, len(attendees))
	for _, attendee := range attendees {
		glyph := ""
		glyphColor := lipgloss.Color("65")
		if attendeeIsOptional(attendee) {
			glyphColor = lipgloss.Color("244")
		}
		icon := lipgloss.NewStyle().Foreground(glyphColor).Bold(true).Render(glyph)
		label := attendeeBaseLabel(attendee)
		if label == "" {
			continue
		}
		labelBudget := max(1, width-lipgloss.Width(glyph)-1)
		lines = append(lines, icon+" "+valueStyle.Render(truncate(label, labelBudget)))
	}
	if len(lines) == 0 {
		return []string{valueStyle.Render("-")}
	}
	return lines
}

func wrapEditorValue(value string, width int) []string {
	width = max(1, width)
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{"-"}
	}
	out := make([]string, 0)
	for _, paragraph := range strings.Split(value, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		runes := []rune(paragraph)
		for len(runes) > width {
			cut := width
			for i := width; i > 0; i-- {
				if runes[i-1] == ' ' || runes[i-1] == '\t' {
					cut = i - 1
					break
				}
			}
			if cut <= 0 {
				cut = width
			}
			out = append(out, strings.TrimSpace(string(runes[:cut])))
			runes = []rune(strings.TrimSpace(string(runes[cut:])))
		}
		out = append(out, string(runes))
	}
	return out
}

func nearestSelectableEditorCursor(rows []editorRow, cursor int) int {
	if len(rows) == 0 {
		return 0
	}
	cursor = clamp(cursor, 0, len(rows)-1)
	if isEditorSelectable(rows[cursor]) {
		return cursor
	}
	for i := cursor + 1; i < len(rows); i++ {
		if isEditorSelectable(rows[i]) {
			return i
		}
	}
	for i := cursor - 1; i >= 0; i-- {
		if isEditorSelectable(rows[i]) {
			return i
		}
	}
	return cursor
}

func moveEditorCursor(rows []editorRow, cursor, delta int) int {
	if len(rows) == 0 || delta == 0 {
		return 0
	}
	cursor = nearestSelectableEditorCursor(rows, cursor)
	for next := cursor + delta; next >= 0 && next < len(rows); next += delta {
		if isEditorSelectable(rows[next]) {
			return next
		}
	}
	return cursor
}

func editorButtonLabel(key string) string {
	switch key {
	case "attendees-add":
		return "󰐕 Add attendee"
	case "alarms-add":
		return "󰐕 Add notification"
	case "form-cancel":
		return "Cancel"
	case "form-save":
		return "Save"
	default:
		return "󰐕 Add"
	}
}

func editorRowStyle(selected bool, width int) lipgloss.Style {
	style := lipgloss.NewStyle().Width(width)
	if selected {
		style = style.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")).Bold(true)
	}
	return style
}

// NewPreferredFormKeyMap uses j/k-first navigation labels and ctrl-enter text shortcuts.
func NewPreferredFormKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
	keymap.Select.Up.SetHelp("k", "previous")
	keymap.Select.Down.SetHelp("j", "next")
	keymap.MultiSelect.Up.SetHelp("k", "previous")
	keymap.MultiSelect.Down.SetHelp("j", "next")
	keymap.FilePicker.Up.SetHelp("k", "previous")
	keymap.FilePicker.Down.SetHelp("j", "next")
	keymap.Confirm.Toggle.SetKeys("j", "k", "h", "l", "left", "right")
	keymap.Confirm.Toggle.SetHelp("j/k", "toggle")
	keymap.Text.NewLine.SetKeys("ctrl+enter", "ctrl+j")
	keymap.Text.NewLine.SetHelp("ctrl+enter / ctrl+j", "new line")
	return keymap
}

// NewPreferredMultiFieldFormKeyMap uses ctrl+j/ctrl+k for field navigation.
// Enter still advances and submits only from the final field.
func NewPreferredMultiFieldFormKeyMap() *huh.KeyMap {
	keymap := NewPreferredFormKeyMap()

	keymap.Input.Next.SetKeys("enter", "tab", "ctrl+j")
	keymap.Input.Next.SetHelp("ctrl+j", "next")
	keymap.Input.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.Input.Prev.SetHelp("ctrl+k", "previous")

	keymap.Text.Next.SetKeys("enter", "tab", "ctrl+j")
	keymap.Text.Next.SetHelp("ctrl+j", "next")
	keymap.Text.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.Text.Prev.SetHelp("ctrl+k", "previous")
	keymap.Text.NewLine.SetKeys("ctrl+enter")
	keymap.Text.NewLine.SetHelp("ctrl+enter", "new line")

	keymap.Select.Next.SetKeys("enter", "tab", "ctrl+j")
	keymap.Select.Next.SetHelp("ctrl+j", "next")
	keymap.Select.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.Select.Prev.SetHelp("ctrl+k", "previous")
	keymap.Select.Up.SetKeys("up", "k", "ctrl+p")
	keymap.Select.Down.SetKeys("down", "j", "ctrl+n")

	keymap.MultiSelect.Next.SetKeys("enter", "tab", "ctrl+j")
	keymap.MultiSelect.Next.SetHelp("ctrl+j", "next")
	keymap.MultiSelect.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.MultiSelect.Prev.SetHelp("ctrl+k", "previous")

	keymap.Confirm.Next.SetKeys("enter", "tab", "ctrl+j")
	keymap.Confirm.Next.SetHelp("ctrl+j", "next")
	keymap.Confirm.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.Confirm.Prev.SetHelp("ctrl+k", "previous")

	keymap.Note.Next.SetKeys("enter", "tab", "ctrl+j")
	keymap.Note.Next.SetHelp("ctrl+j", "next")
	keymap.Note.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.Note.Prev.SetHelp("ctrl+k", "previous")

	keymap.FilePicker.Next.SetKeys("tab", "ctrl+j")
	keymap.FilePicker.Next.SetHelp("ctrl+j", "next")
	keymap.FilePicker.Prev.SetKeys("shift+tab", "ctrl+k")
	keymap.FilePicker.Prev.SetHelp("ctrl+k", "previous")
	keymap.FilePicker.Up.SetKeys("up", "k", "ctrl+p")
	keymap.FilePicker.Down.SetKeys("down", "j", "ctrl+n")

	return keymap
}

func eventEditorFormKeyMap(key string) *huh.KeyMap {
	if key == "when" {
		return NewPreferredMultiFieldFormKeyMap()
	}
	if key == "attendees-add" {
		return attendeeMultiSelectKeyMap()
	}
	return NewPreferredFormKeyMap()
}

func todoEditorFormKeyMap(key string) *huh.KeyMap {
	if key == "start" || key == "due" {
		return NewPreferredMultiFieldFormKeyMap()
	}
	return NewPreferredFormKeyMap()
}

func deleteConfirmFormKeyMap(_ *deleteConfirmState) *huh.KeyMap {
	return NewPreferredFormKeyMap()
}

func attendeeMultiSelectKeyMap() *huh.KeyMap {
	keymap := NewPreferredFormKeyMap()
	keymap.MultiSelect.SelectAll.Unbind()
	keymap.MultiSelect.SelectNone.Unbind()
	keymap.MultiSelect.SetFilter.SetKeys("enter")
	keymap.MultiSelect.SetFilter.SetHelp("enter", "set filter")
	return keymap
}

func mergeListInput(existing string, added []string) string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, v := range splitListInput(existing) {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, v := range added {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return strings.Join(out, "; ")
}

func (s *eventFormState) hasActiveDialog() bool {
	return s != nil && (s.activeForm != nil || s.attendeeManager != nil || s.noNotifications ||
		s.datePicker != nil || s.timeEditor != nil || s.searchPicker != nil)
}

func newAttendeeManager(attendees []calendar.Attendee) *attendeeManagerState {
	items := make([]attendeeEditorItem, 0, len(attendees))
	for _, attendee := range attendees {
		if strings.TrimSpace(attendee.Name) == "" && strings.TrimSpace(attendee.Email) == "" {
			continue
		}
		items = append(items, attendeeEditorItem{attendee: attendee})
	}
	return &attendeeManagerState{attendees: items}
}

func attendeeManagerValues(mgr *attendeeManagerState) []calendar.Attendee {
	if mgr == nil {
		return nil
	}
	out := make([]calendar.Attendee, 0, len(mgr.attendees))
	for _, item := range mgr.attendees {
		if item.remove {
			continue
		}
		out = append(out, item.attendee)
	}
	return out
}

func toggleAttendeeOptional(mgr *attendeeManagerState) {
	if mgr == nil || len(mgr.attendees) == 0 {
		return
	}
	idx := clamp(mgr.cursor, 0, len(mgr.attendees)-1)
	if attendeeIsOptional(mgr.attendees[idx].attendee) {
		mgr.attendees[idx].attendee.Role = ""
		return
	}
	mgr.attendees[idx].attendee.Role = "optional"
}

func toggleAttendeeRemove(mgr *attendeeManagerState) {
	if mgr == nil || len(mgr.attendees) == 0 {
		return
	}
	idx := clamp(mgr.cursor, 0, len(mgr.attendees)-1)
	mgr.attendees[idx].remove = !mgr.attendees[idx].remove
}

func attendeeIsOptional(attendee calendar.Attendee) bool {
	return strings.EqualFold(strings.TrimSpace(attendee.Role), "optional")
}

func (m Model) renderEventDetailsPane(width, height int) string {
	if m.eventForm != nil {
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(m.renderEventEditorList(width, height))
	}
	if m.todoForm != nil {
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(m.renderTodoEditorList(width, height))
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
	meta := []string{
		detailLine("󰉢", "Title", ev.Summary, width),
	}
	meta = append(meta, detailLine("", "RSVP", eventRSVPDisplayValue(eventRSVPValue(ev)), width))
	if ev.Organizer != "" {
		meta = append(meta, detailLine("", "Organizer", ev.Organizer, width))
	}
	if m.store != nil {
		if role := eventUserRoleDisplay(m.store.EventUserRole(ev)); role != "" {
			meta = append(meta, detailLine("", "Role", role, width))
		}
	}
	if ev.Location != "" || ev.URL != "" {
		meta = append(meta, "")
	}
	if ev.Location != "" {
		meta = append(meta, detailLine("", "Location", ev.Location, width))
	}
	if ev.URL != "" {
		meta = append(meta, detailLine("", "URL", ev.URL, width))
	}
	if len(ev.Attendees) > 0 {
		meta = append(meta, "", detailLine("", "Attendees", fmt.Sprintf("%d total", len(ev.Attendees)), width))
		meta = append(meta, detailAttendeeLines(ev.Attendees, width, 5)...)
	}
	meta = append(meta, "", detailLine("󰥔", "All-day", yesNo(ev.AllDay), width))
	meta = append(meta, detailLine("", "When", ev.Start.Format("2006-01-02 15:04")+" - "+ev.End.Format("2006-01-02 15:04"), width))
	if ev.Recurrence != nil {
		meta = append(meta, "", detailLine("󰑖", "Repeat", formatRecurrence(ev.Recurrence), width))
	} else if ev.Recurring {
		meta = append(meta, "", detailLine("󰑖", "Repeat", "yes", width))
	}
	if len(ev.Alarms) > 0 {
		meta = append(meta, "", detailLine("󰀠", "Notifications", formatAlarms(ev.Alarms), width))
	}
	return m.renderGroupedDetails("Details", meta, "", width, height)
}

func (m Model) renderTodoDetailsFor(todo calendar.Todo, mode string, width, height int) string {
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
	meta := []string{
		detailLine("󰉢", "Title", todo.Summary, width),
	}
	if strings.TrimSpace(todo.Location) != "" {
		meta = append(meta, "", detailLine("", "Location", todo.Location, width))
	}
	meta = append(meta, "", detailLine("󰥔", "When", when, width))
	meta = append(meta, "", detailLine("󰄬", "Status", strings.ToLower(status), width))
	if todo.Percent > 0 {
		meta = append(meta, detailLine("", "Progress", fmt.Sprintf("%d%%", todo.Percent), width))
	}
	if todo.Priority > 0 {
		priorityLabel := todoPriorityLabel(todo.Priority)
		if strings.EqualFold(priorityLabel, "high") {
			priorityLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(priorityLabel)
		}
		meta = append(meta, detailLine("", "Priority", priorityLabel, width))
	}
	return m.renderGroupedDetails("Details", meta, todo.Description, width, height)
}

func (m Model) renderGroupedDetails(title string, meta []string, description string, width, height int) string {
	width = max(10, width)
	height = max(1, height)
	header := m.styles.Subtle.Render(title)
	bodyHeight := max(0, height-1)
	if bodyHeight == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(header)
	}

	meta = compactBlankLines(meta)
	descLines := detailDescriptionLines(description, width)
	if len(descLines) > 0 {
		meta = compactBlankLines(append(meta, append([]string{""}, descLines...)...))
	}
	metaView := renderScrolledLines(meta, m.detailScroll, bodyHeight)
	block := lipgloss.JoinVertical(lipgloss.Left, header, metaView)
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(block)
}

func detailLine(glyph, label, value string, width int) string {
	iconWidth := 2
	labelWidth := 13
	icon := strings.Repeat(" ", iconWidth)
	if glyph != "" {
		icon = lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render(glyph)
		icon += strings.Repeat(" ", max(0, iconWidth-lipgloss.Width(icon)))
	}
	labelText := padRight(label, labelWidth)
	prefix := icon + " " + lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render(labelText) + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(": ")
	valueBudget := max(1, width-lipgloss.Width(prefix))
	return prefix + lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(truncate(value, valueBudget))
}

func detailAttendeeLines(attendees []calendar.Attendee, width, limit int) []string {
	if len(attendees) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, min(limit, len(attendees)))
	prefix := strings.Repeat(" ", 16)
	for _, attendee := range attendees[:min(limit, len(attendees))] {
		glyphColor := lipgloss.Color("65")
		if attendeeIsOptional(attendee) {
			glyphColor = lipgloss.Color("244")
		}
		icon := lipgloss.NewStyle().Foreground(glyphColor).Bold(true).Render("")
		label := attendeeBaseLabel(attendee)
		valueBudget := max(1, width-lipgloss.Width(prefix)-lipgloss.Width(icon)-1)
		out = append(out, prefix+icon+" "+lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(truncate(label, valueBudget)))
	}
	return out
}

func detailDescriptionLines(description string, width int) []string {
	description = strings.TrimSpace(description)
	if description == "" {
		return nil
	}
	lines := []string{detailLine("󰦨", "Description", "", width)}
	for _, raw := range strings.Split(description, "\n") {
		wrapped := wrapLine(raw, max(10, width-4))
		for _, line := range wrapped {
			lines = append(lines, strings.Repeat(" ", 3)+truncate(line, max(10, width-4)))
		}
	}
	return lines
}

func renderScrolledLines(lines []string, scroll, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) == 0 {
		return lipgloss.NewStyle().Height(height).MaxHeight(height).Render("")
	}
	scroll = clamp(scroll, 0, max(0, len(lines)-1))
	if scroll > 0 {
		lines = lines[scroll:]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lipgloss.NewStyle().Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func compactBlankLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	lastBlank := true
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && lastBlank {
			continue
		}
		out = append(out, line)
		lastBlank = blank
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func padRight(value string, width int) string {
	if lipgloss.Width(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-lipgloss.Width(value))
}

func (m *Model) moveEventCursor(delta int) {
	before := m.selectedAgendaItemKey()
	items := m.agendaItems()
	if len(items) == 0 {
		m.eventCursor = 0
		m.eventListOffset = 0
		m.resetDetailScrollIfSelectionChanged(before)
		return
	}
	target := m.eventCursor + delta
	if m.showTasksMode {
		target = clamp(target, 0, len(items)-1)
		m.eventCursor = target
		m.ensureEventCursorVisible()
		m.resetDetailScrollIfSelectionChanged(before)
		return
	}
	if target < 0 {
		needed := -target
		prepended := m.moveAgendaWindowBackward(needed)
		items = m.agendaItems()
		if len(items) == 0 {
			m.eventCursor = 0
			m.eventListOffset = 0
			m.resetDetailScrollIfSelectionChanged(before)
			return
		}
		target = prepended - needed
		if target < 0 {
			target = 0
		}
	}
	if target >= len(items) {
		target = len(items) - 1
	}
	m.eventCursor = target

	m.selected = dayStart(items[m.eventCursor].Day)
	m.scrollForSelection()
	m.ensureEventCursorVisible()
	m.resetDetailScrollIfSelectionChanged(before)
}

func (m *Model) moveAgendaWindowBackward(needed int) int {
	if needed <= 0 {
		return 0
	}
	originalStart := m.agendaStart
	if originalStart.IsZero() {
		originalStart = dayStart(m.selected)
	}
	start := originalStart
	prepended := 0
	for prepended < needed {
		start = start.AddDate(0, 0, -1)
		m.agendaStart = start
		items := m.agendaItems()
		prepended = 0
		for _, item := range items {
			if !item.Day.Before(originalStart) {
				break
			}
			prepended++
		}
	}
	m.eventCursor += prepended
	m.eventListOffset += prepended
	return prepended
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

func calendarKey(source, name string) string { return source + calendarKeySeparator + name }

func filteredEvents(events []calendar.Event, vis map[string]bool, includeDeclined bool) []calendar.Event {
	out := make([]calendar.Event, 0, len(events))
	for _, ev := range events {
		if !vis[calendarKey(ev.Source, ev.Calendar)] {
			continue
		}
		if !includeDeclined && eventRSVPIsNo(ev) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func eventRSVPIsNo(ev calendar.Event) bool {
	return eventRSVPValue(ev) == "no"
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func emptyDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func multilineValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
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

func (m Model) shortcutsLegend() string {
	if m.deleteConfirm != nil || (m.eventForm != nil && m.eventForm.hasActiveDialog()) || (m.todoForm != nil && m.todoForm.activeForm != nil) {
		return ""
	}
	if (m.eventForm != nil && m.eventForm.mode == "view") || (m.todoForm != nil && m.todoForm.mode == "view") {
		return "[esc/q] Back  [j/k] Next / Previous  [e] Edit  [?] Help"
	}
	if m.eventForm != nil || m.todoForm != nil {
		return "[esc/q] Cancel  [ctrl+s] Save  [j/k] Next / Prev  [h/l/←/→] Change  [enter] Edit  [?] Help"
	}
	if m.focusCalendarPane {
		return "[esc/q] Back  [j/k] Next / Previous  [enter/spc] Hide/Show  [?] Help"
	}
	if m.focusDetails {
		return "[esc/q] Back  [j/k] Scroll  [enter/spc] Back  [e] Edit  [ctrl-d] Delete  [?] Help"
	}
	if m.showTasksMode {
		return "[esc/q] Exit  [j/k] Next / Previous  [enter] Open  [n] New  [x] Done/Undone  [p] Priority  [f] Show/hide completed  [?] Help"
	}
	return "[esc/q] Exit  [j/k] Next / Previous  [t] Today  [enter] Open  [n] New  [m] Tasks  [ctrl-d] Delete  [c] Calendars  [?] Help"
}

func (m Model) helpLines() []string {
	if (m.eventForm != nil && m.eventForm.mode == "view") || (m.todoForm != nil && m.todoForm.mode == "view") {
		return []string{
			"esc, q      Back to list",
			"j/k         Next / previous field",
			"↑/↓         Next / previous field",
			"tab         Next field",
			"shift+tab   Previous field",
			"e           Edit item",
			"?           Toggle help",
		}
	}
	if m.eventForm != nil || m.todoForm != nil {
		return []string{
			"esc, q      Cancel editor",
			"ctrl+c      Cancel editor",
			"ctrl+s      Save",
			"j/k         Next / previous field",
			"↑/↓         Next / previous field",
			"tab         Next field",
			"shift+tab   Previous field",
			"h/l, ←/→    Previous / next option",
			"enter       Edit field or toggle yes/no",
			"?           Toggle help",
		}
	}
	if m.focusCalendarPane {
		back := "events"
		if m.showTasksMode {
			back = "tasks"
		}
		return []string{
			"esc, q      Back to " + back,
			"ctrl+c      Exit",
			"j/k         Next / previous calendar",
			"↑/↓         Next / previous calendar",
			"enter, spc  Hide/show calendar",
			"c, h        Back to " + back,
			"?           Toggle help",
		}
	}
	if m.focusDetails {
		return []string{
			"esc, q      Back to list",
			"ctrl+c      Exit",
			"j/k         Scroll down/up",
			"↑/↓         Scroll down/up",
			"enter, spc  Back to list",
			"e           Edit selected item",
			"ctrl+d      Delete selected item",
			"?           Toggle help",
		}
	}
	if m.showTasksMode {
		return []string{
			"esc, q      Exit",
			"ctrl+c      Exit",
			"j/k         Next / previous task",
			"↑/↓         Next / previous task",
			"ctrl+f/b    Page down / page up",
			"t           Jump to today",
			"enter, e    Open task editor",
			"v           Open read-only details",
			"n           New task",
			"x, d        Toggle done status",
			"p           Cycle task priority",
			"f           Show/hide completed",
			"m           Open events",
			"ctrl+d      Delete selected task",
			"c           Open calendars pane",
			"spc         Focus details",
			"ctrl+j/k    Scroll details down/up",
			"?           Toggle help",
		}
	}
	return []string{
		"esc, q      Exit",
		"ctrl+c      Exit",
		"j/k         Next / previous event",
		"↑/↓         Next / previous event",
		"ctrl+f/b    Page down / page up",
		"h/l         Previous / next day",
		"←/→         Previous / next day",
		"ctrl+h/l    Previous / next week",
		"t           Today",
		"enter, e    Open event editor",
		"v           Open read-only details",
		"n           New event",
		"m           Open tasks",
		"ctrl+d      Delete selected event",
		"c           Open calendars pane",
		"f           Show/hide free and declined",
		"spc         Focus details",
		"ctrl+j/k    Scroll details down/up",
		"?           Toggle help",
	}
}

func (m Model) renderHelpOverlay(width, height int) string {
	if width < 58 {
		width = 58
	}
	lines := m.helpLines()
	minimumHeight := len(lines) + 4
	if height < minimumHeight {
		height = minimumHeight
	}
	title := m.styles.Title.Render("Shortcuts")
	body := lipgloss.NewStyle().Width(width - 4).Render(strings.Join(lines, "\n"))
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(1, 2).
		Width(width).
		Height(height).
		Render(content)
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
	before := m.selectedAgendaItemKey()
	items := m.agendaItems()
	if len(items) == 0 {
		m.eventCursor = 0
		m.eventListOffset = 0
		m.resetDetailScrollIfSelectionChanged(before)
		return
	}
	if m.eventCursor < 0 {
		m.eventCursor = 0
	}
	if m.eventCursor >= len(items) {
		m.eventCursor = len(items) - 1
	}
	m.ensureEventCursorVisible()
	if !m.showTasksMode {
		m.selected = dayStart(items[m.eventCursor].Day)
		m.scrollForSelection()
	}
	m.resetDetailScrollIfSelectionChanged(before)
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

func (m Model) selectedAgendaItemKey() string {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return ""
	}
	it := items[m.eventCursor]
	switch {
	case it.Event != nil:
		return "event:" + it.Event.UID + ":" + it.Event.Start.Format(time.RFC3339Nano)
	case it.Todo != nil:
		return "todo:" + it.Todo.UID + ":" + it.Mode
	case it.IsFree:
		return "free:" + it.Start.Format(time.RFC3339Nano) + ":" + it.End.Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("item:%s:%s:%d", it.Mode, it.Day.Format("2006-01-02"), it.Start.UnixNano())
	}
}

func (m *Model) resetDetailScrollIfSelectionChanged(before string) {
	if before != m.selectedAgendaItemKey() {
		m.detailScroll = 0
	}
}

func (m Model) agendaItems() []AgendaListItem {
	if m.showTasksMode {
		return buildTaskItems(
			filteredTodos(m.data.Todos, m.calendarVisibility),
			m.showAllMode,
			m.selected.Location(),
		)
	}
	start := m.agendaStart
	if start.IsZero() {
		start = dayStart(m.selected)
	}
	return buildAgendaItems(
		start,
		filteredEvents(m.data.Events, m.calendarVisibility, m.showAllMode),
		90,
		m.showAllMode,
	)
}

func (m *Model) enterTaskMode() {
	m.showTasksMode = true
	m.eventCursor = 0
	m.eventListOffset = 0
	m.detailScroll = 0
	m.ensureEventSelectionValid()
}

func (m *Model) exitTaskMode() {
	m.showTasksMode = false
	m.eventCursor = 0
	m.eventListOffset = 0
	m.detailScroll = 0
	m.ensureEventSelectionValid()
}

func (m *Model) jumpTaskCursorToToday(now time.Time) {
	items := m.agendaItems()
	if len(items) == 0 {
		m.eventCursor = 0
		m.eventListOffset = 0
		return
	}
	today := dayStart(now)
	target := 0
	for i, item := range items {
		target = i
		if !item.Day.Before(today) {
			break
		}
	}
	m.eventCursor = target
	m.ensureEventCursorVisible()
}

func (m *Model) selectedTodo() (calendar.Todo, bool) {
	if !m.showTasksMode {
		return calendar.Todo{}, false
	}
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) || items[m.eventCursor].Todo == nil {
		return calendar.Todo{}, false
	}
	return *items[m.eventCursor].Todo, true
}

func (m *Model) toggleSelectedTodoDone() string {
	todo, ok := m.selectedTodo()
	if !ok {
		return ""
	}
	status := "COMPLETED"
	percent := 100
	var completed *time.Time
	if isTodoDone(todo) {
		status = "NEEDS-ACTION"
		percent = 0
	} else {
		now := time.Now()
		completed = &now
	}
	return m.updateSelectedTodo(todo.UID, calendar.TodoUpdate{
		Status:    &status,
		Percent:   &percent,
		Completed: &completed,
	})
}

func (m *Model) cycleSelectedTodoPriority() string {
	todo, ok := m.selectedTodo()
	if !ok {
		return ""
	}
	priority := 1
	switch todoPriorityLabel(todo.Priority) {
	case "high":
		priority = 5
	case "mid":
		priority = 9
	}
	return m.updateSelectedTodo(todo.UID, calendar.TodoUpdate{Priority: &priority})
}

func (m *Model) updateSelectedTodo(uid string, update calendar.TodoUpdate) string {
	if m.store != nil {
		if err := m.store.UpdateTodo(uid, update); err != nil {
			return err.Error()
		}
		data, err := m.store.Load()
		if err != nil {
			return err.Error()
		}
		m.data = data
	} else {
		for i := range m.data.Todos {
			if m.data.Todos[i].UID != uid {
				continue
			}
			if update.Status != nil {
				m.data.Todos[i].Status = *update.Status
			}
			if update.Priority != nil {
				m.data.Todos[i].Priority = *update.Priority
			}
			if update.Percent != nil {
				m.data.Todos[i].Percent = *update.Percent
			}
			if update.Completed != nil {
				m.data.Todos[i].Completed = *update.Completed
			}
			break
		}
	}

	m.ensureEventSelectionValid()
	for i, item := range m.agendaItems() {
		if item.Todo != nil && item.Todo.UID == uid {
			m.eventCursor = i
			m.ensureEventCursorVisible()
			break
		}
	}
	return ""
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
	start, end := m.defaultCreationRange()
	m.eventForm = m.newEventFormState("create", "", calendar.Event{
		Summary:  "",
		Start:    start,
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

func (m *Model) openTodoFormNewWith(td calendar.Todo) {
	defaultKey := m.firstWritableCalendarKey()
	if strings.TrimSpace(td.Source) == "" || strings.TrimSpace(td.Calendar) == "" {
		parts := splitCalendarKey(defaultKey)
		td.Source = parts.source
		td.Calendar = parts.name
	}
	if td.Priority == 0 {
		td.Priority = 5
	}
	m.todoForm = m.newTodoFormState("create", "", td)
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.todoForm.form.UpdateFieldPositions()
}

func (m Model) defaultCreationRange() (time.Time, time.Time) {
	start := m.defaultCreationStart()
	return start, start.Add(time.Hour)
}

func (m Model) defaultCreationStart() time.Time {
	items := m.agendaItems()
	if len(items) > 0 && m.eventCursor >= 0 && m.eventCursor < len(items) {
		it := items[m.eventCursor]
		if !it.Start.IsZero() {
			return it.Start
		}
		if !it.Day.IsZero() {
			return dayStart(it.Day)
		}
	}
	if !m.selected.IsZero() {
		return dayStart(m.selected)
	}
	return time.Now().Truncate(time.Minute)
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

func (m *Model) openViewForSelected() bool {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return false
	}
	it := items[m.eventCursor]
	if it.IsFree {
		return false
	}
	if it.Event != nil {
		m.eventForm = m.newEventFormState("view", it.Event.UID, *it.Event)
		m.focusDetails = true
		m.focusMain = false
		m.detailScroll = 0
		return true
	}
	if it.Todo != nil {
		m.todoForm = m.newTodoFormState("view", it.Todo.UID, *it.Todo)
		m.focusDetails = true
		m.focusMain = false
		m.detailScroll = 0
		return true
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
	if ev.Recurring {
		m.openEventEditScopeForm()
	}
	return true
}

func (m *Model) initCurrentEventForm() tea.Cmd {
	if m.eventForm == nil {
		return nil
	}
	if m.eventForm.activeForm != nil {
		return m.eventForm.activeForm.Init()
	}
	if m.eventForm.form != nil {
		return m.eventForm.form.Init()
	}
	return nil
}

func (m *Model) newTodoFormState(mode, targetUID string, td calendar.Todo) *todoFormState {
	key := calendarKey(td.Source, td.Calendar)
	if strings.TrimSpace(td.Source) == "" || strings.TrimSpace(td.Calendar) == "" {
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
	labels := m.calendarOptionLabels()
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		calOptions = append(calOptions, huh.NewOption(labels[key], key))
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
	percent := 0
	var completed *time.Time
	if s.completed {
		status = "COMPLETED"
		percent = 100
		now := time.Now()
		completed = &now
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
			Completed:   &completed,
			Percent:     &percent,
			Start:       &startUpdate,
			Due:         &dueUpdate,
		}
		if err := m.store.UpdateTodo(s.targetUID, upd); err != nil {
			return err
		}
		if s.targetUID != "" {
			if err := m.store.MoveTodo(s.targetUID, cal.source, cal.name); err != nil {
				return err
			}
		}
	} else {
		td := calendar.Todo{
			Summary:     s.summary,
			Description: s.description,
			Location:    s.location,
			Status:      status,
			Priority:    priority,
			Completed:   completed,
			Percent:     percent,
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
	if strings.TrimSpace(ev.Source) == "" || strings.TrimSpace(ev.Calendar) == "" {
		key = m.firstWritableCalendarKey()
	}
	timezone, timezoneLocal := eventFormTimezone(ev, mode)
	displayLocation, err := time.LoadLocation(timezone)
	if err != nil {
		displayLocation = time.Local
	}
	fd := ev.Start.In(displayLocation)
	td := ev.End.In(displayLocation)
	if fd.IsZero() {
		fd = m.selected.In(displayLocation)
	}
	if td.IsZero() || !td.After(fd) {
		td = fd.Add(time.Hour)
	}
	if ev.AllDay && td.After(fd) {
		td = td.AddDate(0, 0, -1)
	}
	state := &eventFormState{
		mode:           mode,
		targetUID:      targetUID,
		editScope:      string(calendar.EditRecurringAll),
		summary:        ev.Summary,
		calendarKey:    key,
		location:       ev.Location,
		description:    ev.Description,
		url:            ev.URL,
		attendees:      attendeesInput(ev.Attendees),
		rsvp:           eventRSVPValue(ev),
		availability:   ev.Availability,
		visibility:     eventVisibilityValue(ev.Visibility),
		alarms:         alarmsInput(ev.Alarms),
		recur:          ev.Recurrence != nil,
		recurFreq:      recurrenceFrequencyValue(ev.Recurrence),
		recurEvery:     recurrenceIntervalValue(ev.Recurrence),
		recurWeekdays:  recurrenceWeekdayValues(ev.Recurrence),
		recurMonthlyBy: recurrenceMonthlyByValue(ev.Recurrence),
		recurEnd:       recurrenceEndValue(ev.Recurrence),
		recurUntil:     recurrenceUntilValue(ev.Recurrence),
		recurCount:     recurrenceCountValue(ev.Recurrence),
		allDay:         ev.AllDay,
		fromDate:       fd.Format("2006-01-02"),
		fromTime:       fd.Format("15:04"),
		toDate:         td.Format("2006-01-02"),
		toTime:         td.Format("15:04"),
		timezone:       timezone,
		timezoneLocal:  timezoneLocal,
	}
	if ev.AllDay {
		state.fromTime = "09:00"
		state.toTime = "10:00"
	}
	if mode == "edit" {
		target := ev
		state.targetEvent = &target
		if ev.Recurring {
			state.editScope = string(calendar.EditRecurringOccurrence)
		}
	}
	state.form = m.buildEventForm(state)
	return state
}

func (m *Model) buildEventForm(s *eventFormState) *huh.Form {
	calOptions := make([]huh.Option[string], 0, len(m.calendarOrder))
	labels := m.calendarOptionLabels()
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		calOptions = append(calOptions, huh.NewOption(labels[key], key))
	}
	if len(calOptions) == 0 {
		calOptions = append(calOptions, huh.NewOption("No writable calendar", ""))
	}
	attendeeSuggestions := m.attendeeSuggestions()
	frequencyOptions := []huh.Option[string]{
		huh.NewOption("Daily", "DAILY"),
		huh.NewOption("Weekly", "WEEKLY"),
		huh.NewOption("Monthly", "MONTHLY"),
		huh.NewOption("Yearly", "YEARLY"),
	}
	endOptions := []huh.Option[string]{
		huh.NewOption("Forever", "forever"),
		huh.NewOption("Until date", "until"),
		huh.NewOption("Fixed count", "count"),
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
		huh.NewInput().Key("attendees").Title("Attendees").Description("Separate attendees with commas or semicolons").Value(&s.attendees).Suggestions(attendeeSuggestions),
		huh.NewSelect[string]().Key("rsvp").Title("RSVP").Options(
			huh.NewOption("Unspecified", ""),
			huh.NewOption("Yes", "yes"),
			huh.NewOption("No", "no"),
			huh.NewOption("Maybe", "maybe"),
			huh.NewOption("No response", "needs-action"),
		).Value(&s.rsvp),
		huh.NewSelect[string]().Key("availability").Title("Availability").Options(
			huh.NewOption("Calendar default", ""),
			huh.NewOption("Busy", "busy"),
			huh.NewOption("Free", "free"),
		).Value(&s.availability),
		huh.NewSelect[string]().Key("visibility").Title("Visibility").Options(
			huh.NewOption("Calendar default", "default"),
			huh.NewOption("Public", "public"),
			huh.NewOption("Private", "private"),
			huh.NewOption("Confidential", "confidential"),
		).Value(&s.visibility),
		huh.NewInput().Key("alarms").Title("Notifications").Description("Examples: 10m before, 2h before, 1d after").Value(&s.alarms).Validate(validateAlarmsInput),
		huh.NewConfirm().Key("recur").Title("Repeat").Value(&s.recur),
		huh.NewSelect[string]().Key("recur-freq").Title("Repeat frequency").Options(frequencyOptions...).Value(&s.recurFreq),
		huh.NewInput().Key("recur-every").Title("Repeat every").Value(&s.recurEvery).Validate(func(v string) error {
			if !s.recur {
				return nil
			}
			n, err := parsePositiveIntDefault(v, 1)
			if err != nil || n <= 0 {
				return errors.New("repeat interval must be a positive number")
			}
			return nil
		}),
		huh.NewSelect[string]().Key("recur-end").Title("Repeat ending").Options(endOptions...).Value(&s.recurEnd),
		huh.NewInput().Key("recur-until").Title("Repeat until (YYYY-MM-DD)").Value(&s.recurUntil).Validate(func(v string) error {
			if !s.recur || s.recurEnd != "until" {
				return nil
			}
			return validateEventDateInput(v)
		}),
		huh.NewInput().Key("recur-count").Title("Repeat count").Value(&s.recurCount).Validate(func(v string) error {
			if !s.recur || s.recurEnd != "count" {
				return nil
			}
			n, err := parsePositiveIntDefault(v, 0)
			if err != nil || n <= 0 {
				return errors.New("repeat count must be a positive number")
			}
			return nil
		}),
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
	attendees := parseAttendeesInput(s.attendees)
	if normalizeRSVPValue(s.rsvp) != "" {
		applyRSVPToAttendees(attendees, s.rsvp)
	} else if s.targetEvent != nil {
		preserveAttendeeRSVP(attendees, s.targetEvent.Attendees)
	}
	alarms, err := parseAlarmsInput(s.alarms)
	if err != nil {
		return err
	}
	recurrence, err := parseRecurrenceInput(*s)
	if err != nil {
		return err
	}

	if s.mode == "edit" {
		var recurrenceUpdate *calendar.Recurrence
		var recurrenceUpdatePtr **calendar.Recurrence
		if s.editScope != string(calendar.EditRecurringOccurrence) {
			recurrenceUpdate = recurrence
			recurrenceUpdatePtr = &recurrenceUpdate
		}
		upd := calendar.EventUpdate{
			Summary:      &s.summary,
			Description:  &s.description,
			Location:     &s.location,
			URL:          &s.url,
			Attendees:    &attendees,
			UserRSVP:     &s.rsvp,
			Availability: &s.availability,
			Visibility:   &s.visibility,
			Recurrence:   recurrenceUpdatePtr,
			Alarms:       &alarms,
		}
		if s.timingDirty {
			timezone := s.timezone
			if s.allDay {
				timezone = ""
			}
			upd.Start = &start
			upd.End = &end
			upd.Timezone = &timezone
			upd.AllDay = &s.allDay
		}
		if s.targetEvent != nil {
			var organizerUpdate *string
			if !m.eventFormAttendeeOnly() && strings.TrimSpace(s.targetEvent.Organizer) == "" && len(attendees) > 0 {
				organizer := m.store.CalendarUserEmail(cal.source, cal.name)
				if organizer != "" {
					organizerUpdate = &organizer
				}
			}
			upd.Organizer = organizerUpdate
			if err := m.store.UpdateEventScoped(*s.targetEvent, upd, calendar.EditRecurringScope(s.editScope)); err != nil {
				return err
			}
			if err := m.store.MoveEvent(*s.targetEvent, cal.source, cal.name); err != nil {
				return err
			}
		} else if err := m.store.UpdateEvent(s.targetUID, upd); err != nil {
			return err
		} else if s.targetUID != "" {
			if ev, err := m.store.FindEvent(s.targetUID); err == nil {
				if err := m.store.MoveEvent(ev, cal.source, cal.name); err != nil {
					return err
				}
			}
		}
	} else {
		ev := calendar.Event{
			UID:          s.targetUID,
			Summary:      s.summary,
			Description:  s.description,
			Location:     s.location,
			URL:          s.url,
			Attendees:    attendees,
			Availability: s.availability,
			Visibility:   s.visibility,
			Recurrence:   recurrence,
			Alarms:       alarms,
			Timezone:     s.timezone,
			AllDay:       s.allDay,
			Start:        start,
			End:          end,
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

func (m *Model) openDeleteConfirmForSelected() bool {
	if m.store == nil {
		return false
	}
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return false
	}
	it := items[m.eventCursor]
	if it.IsFree {
		return false
	}
	state := &deleteConfirmState{confirm: false, scope: string(calendar.DeleteRecurringOccurrence), stage: "confirm"}
	if it.Event != nil {
		ev := *it.Event
		if ev.Source == calendar.SpecialSourceBirthdays {
			return false
		}
		state.kind = "event"
		state.event = &ev
		state.recurring = ev.Recurring
		if state.recurring {
			state.stage = "scope"
		}
		state.itemLabel = ev.Summary
	} else if it.Todo != nil {
		td := *it.Todo
		state.kind = "task"
		state.todo = &td
		state.itemLabel = td.Summary
	} else {
		return false
	}
	state.form = m.buildDeleteConfirmForm(state).WithKeyMap(deleteConfirmFormKeyMap(state))
	m.deleteConfirm = state
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.deleteConfirm.form.UpdateFieldPositions()
	return true
}

func (m *Model) buildDeleteConfirmForm(s *deleteConfirmState) *huh.Form {
	if s.stage == "scope" {
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().
			Key("scope").
			Title("Delete recurring event").
			Options(
				huh.NewOption("Only this occurrence", string(calendar.DeleteRecurringOccurrence)),
				huh.NewOption("This and following occurrences", string(calendar.DeleteRecurringFuture)),
				huh.NewOption("All occurrences", string(calendar.DeleteRecurringAll)),
			).
			Value(&s.scope)).Title("Delete")).WithShowHelp(true).WithShowErrors(true)
	}
	title := "Delete " + s.kind
	if strings.TrimSpace(s.itemLabel) != "" {
		title += ": " + s.itemLabel
	}
	description := "This cannot be undone"
	if s.recurring {
		description = "Delete " + eventEditScopeLabel(s.scope) + ". " + description
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Key("confirm").Title(title).Description(description).Value(&s.confirm),
	).Title("Confirm deletion")).WithShowHelp(true).WithShowErrors(true)
}

func (m *Model) commitDeleteConfirm() error {
	if m.deleteConfirm == nil {
		return nil
	}
	if !m.deleteConfirm.confirm {
		return nil
	}
	if m.store == nil {
		return errors.New("calendar store is unavailable")
	}
	if m.deleteConfirm.todo != nil {
		if err := m.store.DeleteTodo(m.deleteConfirm.todo.UID); err != nil {
			return err
		}
	} else if m.deleteConfirm.event != nil {
		scope := calendar.DeleteRecurringAll
		if m.deleteConfirm.recurring {
			scope = calendar.DeleteRecurringScope(m.deleteConfirm.scope)
		}
		if err := m.store.DeleteEvent(*m.deleteConfirm.event, scope); err != nil {
			return err
		}
	}
	ds, err := m.store.Load()
	if err != nil {
		return err
	}
	m.data = ds
	m.eventCursor = 0
	m.eventListOffset = 0
	return nil
}

type calendarKeyParts struct {
	source string
	name   string
}

func splitCalendarKey(v string) calendarKeyParts {
	parts := strings.SplitN(v, calendarKeySeparator, 2)
	if len(parts) != 2 {
		parts = strings.SplitN(v, "/", 2)
		if len(parts) == 2 && parts[0] != "" {
			return calendarKeyParts{source: parts[0], name: parts[1]}
		}
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
		return time.Time{}, time.Time{}, errors.New("invalid start date (expected YYYY-MM-DD)")
	}
	endDate, err := time.Parse("2006-01-02", strings.TrimSpace(s.toDate))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid end date (expected YYYY-MM-DD)")
	}
	if endDate.Before(startDate) {
		return time.Time{}, time.Time{}, errors.New("end date cannot be before start date")
	}
	if s.allDay {
		start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.Local)
		end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
		return start, end, nil
	}

	loc, err := time.LoadLocation(s.timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timezone %q", s.timezone)
	}
	startClock, err := time.Parse("15:04", strings.TrimSpace(s.fromTime))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid start time (expected HH:mm)")
	}
	endClock, err := time.Parse("15:04", strings.TrimSpace(s.toTime))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid end time (expected HH:mm)")
	}
	start := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), startClock.Hour(), startClock.Minute(), 0, 0, loc)
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), endClock.Hour(), endClock.Minute(), 0, 0, loc)
	if sameDate(startDate, endDate) && !end.After(start) {
		end = end.AddDate(0, 0, 1)
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("end must be after start")
	}
	return start, end, nil
}

func (m Model) attendeeSuggestions() []string {
	if m.store == nil {
		return nil
	}
	contacts, err := m.store.Contacts()
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		if strings.TrimSpace(contact.Email) == "" {
			continue
		}
		if strings.TrimSpace(contact.Name) == "" || contact.Name == contact.Email {
			out = append(out, contact.Email)
			continue
		}
		out = append(out, fmt.Sprintf("%s <%s>", contact.Name, contact.Email))
	}
	return out
}

func attendeesInput(attendees []calendar.Attendee) string {
	parts := make([]string, 0, len(attendees))
	for _, attendee := range attendees {
		if label := attendeeLabel(attendee); label != "" {
			parts = append(parts, label)
		}
	}
	return strings.Join(parts, "; ")
}

func parseAttendeesInput(raw string) []calendar.Attendee {
	fields := splitListInput(raw)
	out := make([]calendar.Attendee, 0, len(fields))
	for _, field := range fields {
		field, role := parseAttendeeRoleMarker(field)
		name := ""
		email := strings.TrimSpace(field)
		if left := strings.LastIndex(field, "<"); left >= 0 && strings.HasSuffix(strings.TrimSpace(field), ">") {
			name = strings.TrimSpace(field[:left])
			email = strings.TrimSuffix(strings.TrimSpace(field[left+1:]), ">")
		}
		email = strings.TrimPrefix(strings.TrimSpace(email), "mailto:")
		if name == "" {
			name = email
		}
		if name == "" && email == "" {
			continue
		}
		out = append(out, calendar.Attendee{Name: name, Email: email, Role: role})
	}
	return out
}

func parseAttendeeRoleMarker(raw string) (string, string) {
	field := strings.TrimSpace(raw)
	for _, marker := range []string{"[optional]", "(optional)"} {
		if strings.HasSuffix(strings.ToLower(field), marker) {
			return strings.TrimSpace(field[:len(field)-len(marker)]), "optional"
		}
	}
	for _, marker := range []string{"[required]", "(required)"} {
		if strings.HasSuffix(strings.ToLower(field), marker) {
			return strings.TrimSpace(field[:len(field)-len(marker)]), ""
		}
	}
	return field, ""
}

func applyRSVPToAttendees(attendees []calendar.Attendee, rsvp string) {
	status := normalizeRSVPValue(rsvp)
	for i := range attendees {
		attendees[i].Status = status
	}
}

func preserveAttendeeRSVP(attendees []calendar.Attendee, existing []calendar.Attendee) {
	statusByEmail := map[string]calendar.Attendee{}
	for _, attendee := range existing {
		email := strings.ToLower(strings.TrimSpace(attendee.Email))
		if email == "" {
			continue
		}
		statusByEmail[email] = attendee
	}
	for i := range attendees {
		email := strings.ToLower(strings.TrimSpace(attendees[i].Email))
		if email == "" {
			continue
		}
		if previous, ok := statusByEmail[email]; ok {
			attendees[i].Status = previous.Status
			attendees[i].RSVP = previous.RSVP
		}
	}
}

func attendeeRSVPValue(attendees []calendar.Attendee) string {
	status := ""
	for _, attendee := range attendees {
		next := normalizeRSVPValue(attendee.Status)
		if next == "" {
			continue
		}
		if status == "" {
			status = next
			continue
		}
		if status != next {
			return ""
		}
	}
	return status
}

func eventRSVPValue(ev calendar.Event) string {
	if status := normalizeRSVPValue(ev.UserRSVP); status != "" {
		return status
	}
	return attendeeRSVPValue(ev.Attendees)
}

func eventRSVPDisplayValue(rsvp string) string {
	switch normalizeRSVPValue(rsvp) {
	case "yes":
		return "yes"
	case "no":
		return "no"
	case "maybe":
		return "maybe"
	case "needs-action":
		return "no response"
	default:
		return "-"
	}
}

func normalizeRSVPValue(rsvp string) string {
	switch strings.ToLower(strings.TrimSpace(rsvp)) {
	case "yes", "accepted":
		return "yes"
	case "no", "declined":
		return "no"
	case "maybe", "tentative":
		return "maybe"
	case "needs-action", "needs action", "needs_action":
		return "needs-action"
	default:
		return ""
	}
}

func eventAvailabilityDisplay(availability string) string {
	switch strings.ToLower(strings.TrimSpace(availability)) {
	case "busy":
		return "busy"
	case "free":
		return "free"
	default:
		return "calendar default"
	}
}

func eventVisibilityValue(visibility string) string {
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case "public", "private", "confidential":
		return strings.ToLower(strings.TrimSpace(visibility))
	default:
		return "default"
	}
}

func eventVisibilityDisplay(visibility string) string {
	if eventVisibilityValue(visibility) == "default" {
		return "calendar default"
	}
	return eventVisibilityValue(visibility)
}

func eventUserRoleDisplay(role calendar.EventUserRole) string {
	switch role {
	case calendar.EventUserRoleOrganizer:
		return "organizer"
	case calendar.EventUserRoleAttendee:
		return "attendee"
	case calendar.EventUserRoleLocal:
		return "local"
	default:
		return ""
	}
}

func alarmsInput(alarms []calendar.Alarm) string {
	parts := make([]string, 0, len(alarms))
	for _, alarm := range alarms {
		when := "before"
		offset := alarm.Offset
		if offset > 0 {
			when = "after"
		} else {
			offset = -offset
		}
		parts = append(parts, formatDurationShort(offset)+" "+when)
	}
	return strings.Join(parts, "; ")
}

func parseAlarmsInput(raw string) ([]calendar.Alarm, error) {
	fields := splitListInput(raw)
	out := make([]calendar.Alarm, 0, len(fields))
	for _, field := range fields {
		alarm, err := parseAlarmInput(field)
		if err != nil {
			return nil, err
		}
		out = append(out, alarm)
	}
	return out, nil
}

func parseAlarmInput(raw string) (calendar.Alarm, error) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(parts) == 0 {
		return calendar.Alarm{}, errors.New("notification cannot be empty")
	}
	dur, err := parseDurationShort(parts[0])
	if err != nil {
		return calendar.Alarm{}, fmt.Errorf("invalid notification %q", raw)
	}
	offset := -dur
	if len(parts) > 1 {
		switch parts[1] {
		case "before":
			offset = -dur
		case "after":
			offset = dur
		default:
			return calendar.Alarm{}, fmt.Errorf("notification %q must use before or after", raw)
		}
	}
	return calendar.Alarm{Offset: offset, Action: "DISPLAY"}, nil
}

func validateAlarmsInput(v string) error {
	_, err := parseAlarmsInput(v)
	return err
}

func parseDurationShort(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, errors.New("empty duration")
	}
	unit := raw[len(raw)-1]
	nRaw := raw[:len(raw)-1]
	if unit >= '0' && unit <= '9' {
		unit = 'm'
		nRaw = raw
	}
	n, err := parsePositiveIntDefault(nRaw, 0)
	if err != nil || n <= 0 {
		return 0, errors.New("duration must be positive")
	}
	switch unit {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 's':
		return time.Duration(n) * time.Second, nil
	default:
		return 0, errors.New("duration unit must be d, h, m, or s")
	}
}

func formatDurationShort(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int(d/time.Second))
}

func parseRecurrenceInput(s eventFormState) (*calendar.Recurrence, error) {
	if !s.recur {
		return nil, nil
	}
	interval, err := parsePositiveIntDefault(s.recurEvery, 1)
	if err != nil || interval <= 0 {
		return nil, errors.New("repeat interval must be a positive number")
	}
	rec := &calendar.Recurrence{
		Frequency: strings.ToUpper(strings.TrimSpace(s.recurFreq)),
		Interval:  interval,
		Weekdays:  weekdayCodesFromLabels(s.recurWeekdays),
		MonthlyBy: s.recurMonthlyBy,
	}
	if rec.Frequency == "" {
		rec.Frequency = "DAILY"
	}
	if rec.Frequency == "MONTHLY" {
		if rec.MonthlyBy == "" {
			rec.MonthlyBy = "month day"
		}
	}
	switch s.recurEnd {
	case "", "forever":
	case "until":
		untilDate, err := time.Parse("2006-01-02", strings.TrimSpace(s.recurUntil))
		if err != nil {
			return nil, errors.New("invalid repeat until date (expected YYYY-MM-DD)")
		}
		until := time.Date(untilDate.Year(), untilDate.Month(), untilDate.Day(), 23, 59, 59, 0, time.Local)
		rec.Until = &until
	case "count":
		count, err := parsePositiveIntDefault(s.recurCount, 0)
		if err != nil || count <= 0 {
			return nil, errors.New("repeat count must be a positive number")
		}
		rec.Count = count
	default:
		return nil, errors.New("invalid repeat ending")
	}
	return rec, nil
}

func recurrenceFrequencyValue(rec *calendar.Recurrence) string {
	if rec == nil || strings.TrimSpace(rec.Frequency) == "" {
		return "DAILY"
	}
	return strings.ToUpper(rec.Frequency)
}

func recurrenceIntervalValue(rec *calendar.Recurrence) string {
	if rec == nil || rec.Interval <= 0 {
		return "1"
	}
	return fmt.Sprintf("%d", rec.Interval)
}

func recurrenceWeekdayValues(rec *calendar.Recurrence) []string {
	if rec == nil {
		return nil
	}
	out := make([]string, 0, len(rec.Weekdays))
	for _, day := range rec.Weekdays {
		out = append(out, weekdayLabel(day))
	}
	return out
}

func recurrenceMonthlyByValue(rec *calendar.Recurrence) string {
	if rec == nil || rec.MonthlyBy == "" {
		return "month day"
	}
	return rec.MonthlyBy
}

func weekdayCodesFromLabels(labels []string) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "mo", "mon", "monday":
			out = append(out, "MO")
		case "tu", "tue", "tuesday":
			out = append(out, "TU")
		case "we", "wed", "wednesday":
			out = append(out, "WE")
		case "th", "thu", "thursday":
			out = append(out, "TH")
		case "fr", "fri", "friday":
			out = append(out, "FR")
		case "sa", "sat", "saturday":
			out = append(out, "SA")
		case "su", "sun", "sunday":
			out = append(out, "SU")
		}
	}
	return out
}

func weekdayLabel(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) > 2 {
		code = code[len(code)-2:]
	}
	switch code {
	case "MO":
		return "Mo"
	case "TU":
		return "Tu"
	case "WE":
		return "We"
	case "TH":
		return "Th"
	case "FR":
		return "Fr"
	case "SA":
		return "Sa"
	case "SU":
		return "Su"
	default:
		return code
	}
}

func recurrenceEndValue(rec *calendar.Recurrence) string {
	if rec == nil {
		return "forever"
	}
	if rec.Until != nil {
		return "until"
	}
	if rec.Count > 0 {
		return "count"
	}
	return "forever"
}

func recurrenceUntilValue(rec *calendar.Recurrence) string {
	if rec == nil || rec.Until == nil {
		return ""
	}
	return rec.Until.Format("2006-01-02")
}

func recurrenceCountValue(rec *calendar.Recurrence) string {
	if rec == nil || rec.Count <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", rec.Count)
}

func parsePositiveIntDefault(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func splitListInput(raw string) []string {
	raw = strings.ReplaceAll(raw, "\n", ";")
	splitter := func(r rune) bool {
		return r == ';' || r == ','
	}
	fields := strings.FieldsFunc(raw, splitter)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func attendeeLabel(attendee calendar.Attendee) string {
	label := attendeeBaseLabel(attendee)
	if label == "" {
		return ""
	}
	if attendeeIsOptional(attendee) {
		label += " [optional]"
	}
	return label
}

func attendeeBaseLabel(attendee calendar.Attendee) string {
	if attendee.Name != "" && attendee.Email != "" && attendee.Name != attendee.Email {
		return attendee.Name + " <" + attendee.Email + ">"
	} else if attendee.Name != "" {
		return attendee.Name
	} else if attendee.Email != "" {
		return attendee.Email
	}
	return ""
}

func formatRecurrence(rec *calendar.Recurrence) string {
	if rec == nil {
		return ""
	}
	freq := strings.ToLower(rec.Frequency)
	if freq == "" {
		freq = "daily"
	}
	interval := rec.Interval
	if interval <= 0 {
		interval = 1
	}
	text := freq
	if interval > 1 {
		text = fmt.Sprintf("every %d %s", interval, freq)
	}
	if rec.Until != nil {
		text += " until " + rec.Until.Format("2006-01-02")
	} else if rec.Count > 0 {
		text += fmt.Sprintf(" for %d times", rec.Count)
	}
	return text
}

func formatAlarms(alarms []calendar.Alarm) string {
	parts := make([]string, 0, len(alarms))
	for _, alarm := range alarms {
		when := "before"
		offset := alarm.Offset
		if offset > 0 {
			when = "after"
		} else {
			offset = -offset
		}
		parts = append(parts, formatDurationShort(offset)+" "+when)
	}
	return strings.Join(parts, ", ")
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
