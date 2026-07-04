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
	showFreeMode       bool
	showTasksMode      bool
	agendaStart        time.Time
	eventCursor        int
	eventListOffset    int
	detailScroll       int
	eventForm          *eventFormState
	todoForm           *todoFormState
	deleteConfirm      *deleteConfirmState
	showHelpOverlay    bool
}

type eventFormState struct {
	mode           string
	targetUID      string
	targetEvent    *calendar.Event
	editScope      string
	form           *huh.Form
	activeKey      string
	activeForm     *huh.Form
	backup         *eventFormSnapshot
	cursor         int
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
	errMsg         string
}

type deleteConfirmState struct {
	form      *huh.Form
	itemLabel string
	kind      string
	event     *calendar.Event
	todo      *calendar.Todo
	recurring bool
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

var eventFormFieldOrder = []string{
	"title",
	"calendar",
	"location",
	"description",
	"url",
	"attendees",
	"alarms",
	"recur",
	"recur-freq",
	"recur-every",
	"recur-end",
	"recur-until",
	"recur-count",
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
		if (msg.String() == "q" || msg.String() == "ctrl+c") && m.eventForm == nil && m.todoForm == nil && m.deleteConfirm == nil {
			return m, tea.Quit
		}

		if m.deleteConfirm != nil {
			switch msg.String() {
			case "ctrl+s", "enter":
				if err := m.commitDeleteConfirm(); err != nil {
					m.deleteConfirm.errMsg = err.Error()
					return m, nil
				}
				m.deleteConfirm = nil
				m.focusDetails = false
				m.focusMain = true
				m.ensureEventSelectionValid()
				return m, nil
			case "ctrl+c", "esc", "q":
				m.deleteConfirm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			case "tab":
				m.deleteConfirm.form.UpdateFieldPositions()
				return m, m.deleteConfirm.form.NextField()
			case "shift+tab":
				m.deleteConfirm.form.UpdateFieldPositions()
				return m, m.deleteConfirm.form.PrevField()
			}
			updated, cmd := m.deleteConfirm.form.Update(msg)
			if fm, ok := updated.(*huh.Form); ok {
				m.deleteConfirm.form = fm
			}
			m.deleteConfirm.form.UpdateFieldPositions()
			if m.deleteConfirm.form.State == huh.StateAborted {
				m.deleteConfirm = nil
				m.focusDetails = false
				m.focusMain = true
				return m, nil
			}
			if m.deleteConfirm.form.State == huh.StateCompleted {
				m.deleteConfirm.form.State = huh.StateNormal
			}
			return m, cmd
		}

		if m.eventForm != nil {
			return m.updateEventEditor(msg)
		}
		if m.todoForm != nil {
			return m.updateTodoEditor(msg)
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
					return m, m.initCurrentEventForm()
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
	if m.eventForm != nil && m.eventForm.activeForm != nil {
		return m, m.updateActiveEventEditorForm(msg)
	}
	if m.todoForm != nil && m.todoForm.activeForm != nil {
		return m, m.updateActiveTodoEditorForm(msg)
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
	header := m.styles.PanelTitle.Render("Event")
	if m.eventForm.mode == "edit" {
		header = m.styles.PanelTitle.Render("Edit Event")
	}
	if label := eventEditScopeLabel(m.eventForm.editScope); m.eventForm.mode == "edit" && label != "" {
		header = lipgloss.JoinHorizontal(lipgloss.Left, header, m.styles.Subtle.Render("  ["+label+"]"))
	}
	body := m.renderEventEditorList(width-2, panelHeight-4)
	if strings.TrimSpace(m.eventForm.errMsg) != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.eventForm.errMsg), "", body)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, m.styles.Subtle.Render("[j/k] move  [enter] edit  [ctrl+s] save  [esc] cancel"), "", body)
	panel := m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
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
	body := m.renderTodoEditorList(width-2, panelHeight-4)
	if strings.TrimSpace(m.todoForm.errMsg) != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.todoForm.errMsg), "", body)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, header, m.styles.Subtle.Render("[j/k] move  [enter] edit  [ctrl+s] save  [esc] cancel"), "", body)
	panel := m.styles.MainPanel.Width(width).Height(panelHeight).Render(content)
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
		formView = lipgloss.JoinVertical(lipgloss.Left, m.styles.Subtle.Render("Error: "+m.deleteConfirm.errMsg), "", formView)
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
	err := lipgloss.NewStyle().
		Width(width).
		Foreground(lipgloss.Color("210")).
		Bold(true).
		Render("Error: " + errMsg)
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height).
		Render(lipgloss.JoinVertical(lipgloss.Left, err, "", view))
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

	prefix := "  "
	if selected {
		prefix = " "
	}
	if row.key == "attendees-add" || row.key == "alarms-add" {
		line := prefix + lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Bold(true).
			Padding(0, 1).
			Render(editorButtonLabel(row.key))
		return editorRowStyle(selected, width).Render(line)
	}

	label := lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render(row.label)
	sep := m.styles.Subtle.Render(": ")
	valueBudget := max(1, width-lipgloss.Width(prefix)-lipgloss.Width(row.label)-2)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
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
	rows := []editorRow{
		editorSeparatorRow("󰉢 Title"),
		{"title", "Title", emptyDefault(strings.TrimSpace(s.summary), "(untitled event)")},
		{"calendar", "Calendar", m.calendarDisplayName(s.calendarKey)},
		editorSeparatorRow("󰋫 Place"),
		{"location", "Location", emptyDefault(s.location, "-")},
		{"url", "URL", emptyDefault(s.url, "-")},
		editorSeparatorRow("󰦨 Description"),
		{"description", "Description", multilineValue(s.description)},
		editorSeparatorRow("󰅺 Attendees"),
		{"attendees", "Attendees", emptyDefault(s.attendees, "-")},
		{"rsvp", "RSVP", emptyDefault(eventRSVPDisplayValue(s.rsvp), "default")},
		{"attendees-add", "", ""},
		editorSeparatorRow("󰄨 Privacy"),
		{"availability", "Availability", emptyDefault(s.availability, "default")},
		{"visibility", "Visibility", emptyDefault(s.visibility, "default")},
		editorSeparatorRow("󰥔 Time"),
		{"all-day", "All-day", yesNo(s.allDay)},
	}
	if !s.allDay {
		rows = append(rows, editorRow{"when", "When", fmt.Sprintf("%s %s - %s %s", s.fromDate, s.fromTime, s.toDate, s.toTime)})
	}
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
		editorRow{"alarms-add", "", ""},
	)
	return rows
}

func (m Model) todoEditorRows() []editorRow {
	if m.todoForm == nil {
		return nil
	}
	s := m.todoForm
	return []editorRow{
		editorSeparatorRow("󰉢 Title"),
		{"summary", "Title", emptyDefault(strings.TrimSpace(s.summary), "(untitled task)")},
		{"calendar", "Calendar", m.calendarDisplayName(s.calendarKey)},
		editorSeparatorRow("󰋫 Place"),
		{"location", "Location", emptyDefault(s.location, "-")},
		editorSeparatorRow("󰦨 Description"),
		{"description", "Description", multilineValue(s.description)},
		editorSeparatorRow("󰥔 Schedule"),
		{"start", "Start", optionalDateTimeValue(s.startDate, s.startTime)},
		{"due", "Due", optionalDateTimeValue(s.dueDate, s.dueTime)},
		editorSeparatorRow("󰄬 Status"),
		{"completed", "Completed", yesNo(s.completed)},
		{"priority", "Priority", emptyDefault(s.priorityLabel, "mid")},
	}
}

func (m *Model) updateEventEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.eventForm
	if s.activeForm != nil {
		switch msg.String() {
		case "esc", "ctrl+c", "q":
			s.cancelActive()
			return m, nil
		case "enter":
			if activeFormFiltering(s.activeForm) {
				return m, m.updateActiveEventEditorForm(msg)
			}
			return m, m.submitActiveEventForm()
		}
		return m, m.updateActiveEventEditorForm(msg)
	}
	switch msg.String() {
	case "ctrl+s":
		if err := m.commitEventForm(); err != nil {
			s.errMsg = err.Error()
			return m, nil
		}
		m.eventForm = nil
		m.focusDetails = false
		m.focusMain = true
		m.ensureEventSelectionValid()
	case "ctrl+c", "esc", "q":
		m.eventForm = nil
		m.focusDetails = false
		m.focusMain = true
	case "j", "down", "tab":
		s.cursor = moveEditorCursor(m.eventEditorRows(), s.cursor, 1)
	case "k", "up", "shift+tab":
		s.cursor = moveEditorCursor(m.eventEditorRows(), s.cursor, -1)
	case "enter":
		cmd := m.openEventEditorForm()
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateTodoEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.todoForm
	if s.activeForm != nil {
		switch msg.String() {
		case "esc", "ctrl+c", "q":
			s.cancelActive()
			return m, nil
		case "enter":
			if activeFormFiltering(s.activeForm) {
				return m, m.updateActiveTodoEditorForm(msg)
			}
			return m, m.submitActiveTodoForm()
		}
		return m, m.updateActiveTodoEditorForm(msg)
	}
	switch msg.String() {
	case "ctrl+s":
		if err := m.commitTodoForm(); err != nil {
			s.errMsg = err.Error()
			return m, nil
		}
		m.todoForm = nil
		m.focusDetails = false
		m.focusMain = true
		m.ensureEventSelectionValid()
	case "ctrl+c", "esc", "q":
		m.todoForm = nil
		m.focusDetails = false
		m.focusMain = true
	case "j", "down", "tab":
		s.cursor = moveEditorCursor(m.todoEditorRows(), s.cursor, 1)
	case "k", "up", "shift+tab":
		s.cursor = moveEditorCursor(m.todoEditorRows(), s.cursor, -1)
	case "enter":
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
	key := rows[s.cursor].key
	if isEditorSeparator(rows[s.cursor]) {
		return nil
	}
	s.activeKey = key
	s.backup = s.snapshot()
	s.activeForm = m.buildEventEditorForm(key)
	if s.activeForm == nil {
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	return s.activeForm.Init()
}

func (m *Model) openEventEditScopeForm() tea.Cmd {
	s := m.eventForm
	if s == nil {
		return nil
	}
	s.activeKey = "edit-scope"
	s.activeForm = m.buildEventEditorForm("edit-scope")
	if s.activeForm == nil {
		s.activeKey = ""
		return nil
	}
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
		} else {
			s.errMsg = ""
		}
		s.activeForm = nil
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	return cmd
}

func (m *Model) submitActiveEventForm() tea.Cmd {
	s := m.eventForm
	if s == nil || s.activeForm == nil {
		return nil
	}
	cmd := s.activeForm.NextGroup()
	if s.activeForm.State != huh.StateCompleted {
		return cmd
	}
	if err := m.applyEventEditorForm(); err != nil {
		s.errMsg = err.Error()
		s.activeForm.State = huh.StateNormal
		return cmd
	}
	s.errMsg = ""
	s.activeForm = nil
	s.activeKey = ""
	s.backup = nil
	return cmd
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
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("RSVP").Options(
			huh.NewOption("Default", ""),
			huh.NewOption("Yes", "yes"),
			huh.NewOption("No", "no"),
			huh.NewOption("Maybe", "maybe"),
		).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "availability":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Availability").Options(
			huh.NewOption("Default", ""),
			huh.NewOption("Busy", "busy"),
			huh.NewOption("Free", "free"),
		).Value(&s.availability))).WithShowHelp(true).WithShowErrors(true)
	case "visibility":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Visibility").Options(
			huh.NewOption("Default", "default"),
			huh.NewOption("Public", "public"),
			huh.NewOption("Private", "private"),
			huh.NewOption("Confidential", "confidential"),
		).Value(&s.visibility))).WithShowHelp(true).WithShowErrors(true)
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
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Repeat").Options(
			huh.NewOption("None", "none"),
			huh.NewOption("Daily", "daily"),
			huh.NewOption("Weekly", "weekly"),
			huh.NewOption("Monthly", "monthly"),
			huh.NewOption("Yearly", "yearly"),
		).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "recur-every":
		return singleInputForm("Frequency", "value", &s.recurEvery, validatePositiveInput)
	case "recur-weekdays":
		values := append([]string{}, s.recurWeekdays...)
		return huh.NewForm(huh.NewGroup(huh.NewMultiSelect[string]().Key("value").Title("Weekday").Options(weekdayOptions(values)...).Value(&values))).WithShowHelp(true).WithShowErrors(true)
	case "recur-monthly-by":
		value := s.recurMonthlyBy
		if value == "" {
			value = "month day"
		}
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("By").Options(
			huh.NewOption("On every month day", "month day"),
			huh.NewOption("On every weekday ordinal", "weekday ordinal"),
		).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "recur-end":
		value := s.recurEnd
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Until").Options(
			huh.NewOption("Forever", "forever"),
			huh.NewOption("Until date", "until"),
			huh.NewOption("Fixed count", "count"),
		).Value(&value))).WithShowHelp(true).WithShowErrors(true)
	case "recur-until":
		return singleInputForm("Repeat until (YYYY-MM-DD)", "value", &s.recurUntil, validateEventDateInput)
	case "recur-count":
		return singleInputForm("Repeat count", "value", &s.recurCount, validatePositiveInput)
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
		repeat := anyString(value)
		s.recur = repeat != "none"
		if s.recur {
			s.recurFreq = strings.ToUpper(repeat)
		}
	case "recur-weekdays":
		s.recurWeekdays = anyStringSlice(value)
	case "recur-monthly-by":
		s.recurMonthlyBy = anyString(value)
	case "recur-end":
		s.recurEnd = anyString(value)
	case "all-day":
		if v, ok := value.(bool); ok {
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
	key := rows[s.cursor].key
	if isEditorSeparator(rows[s.cursor]) {
		return nil
	}
	s.activeKey = key
	s.backup = s.snapshot()
	s.activeForm = m.buildTodoEditorForm(key)
	if s.activeForm == nil {
		s.activeKey = ""
		s.backup = nil
		return nil
	}
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
		if err := m.applyTodoEditorForm(); err != nil {
			s.errMsg = err.Error()
		} else {
			s.errMsg = ""
		}
		s.activeForm = nil
		s.activeKey = ""
		s.backup = nil
		return nil
	}
	return cmd
}

func (m *Model) submitActiveTodoForm() tea.Cmd {
	s := m.todoForm
	if s == nil || s.activeForm == nil {
		return nil
	}
	cmd := s.activeForm.NextGroup()
	if s.activeForm.State != huh.StateCompleted {
		return cmd
	}
	if err := m.applyTodoEditorForm(); err != nil {
		s.errMsg = err.Error()
		s.activeForm.State = huh.StateNormal
		return cmd
	}
	s.errMsg = ""
	s.activeForm = nil
	s.activeKey = ""
	s.backup = nil
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
	case "start":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Key("start-date").Title("Start date").Value(&s.startDate).Validate(validateOptionalDateInput),
			huh.NewInput().Key("start-time").Title("Start time").Value(&s.startTime).Validate(validateOptionalTimeInput),
		)).WithShowHelp(true).WithShowErrors(true)
	case "due":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Key("due-date").Title("Due date").Value(&s.dueDate).Validate(validateOptionalDateInput),
			huh.NewInput().Key("due-time").Title("Due time").Value(&s.dueTime).Validate(validateOptionalTimeInput),
		)).WithShowHelp(true).WithShowErrors(true)
	case "completed":
		return huh.NewForm(huh.NewGroup(huh.NewConfirm().Key("value").Title("Completed").Value(&s.completed))).WithShowHelp(true).WithShowErrors(true)
	case "priority":
		return huh.NewForm(huh.NewGroup(huh.NewSelect[string]().Key("value").Title("Priority").Options(
			huh.NewOption("Low", "low"),
			huh.NewOption("Mid", "mid"),
			huh.NewOption("High", "high"),
		).Value(&s.priorityLabel))).WithShowHelp(true).WithShowErrors(true)
	}
	return nil
}

func (m *Model) applyTodoEditorForm() error {
	if m.todoForm.activeKey == "start" || m.todoForm.activeKey == "due" {
		_, _, err := parseTodoFormTimesOptional(*m.todoForm)
		return err
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

func (m Model) calendarOptions() []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(m.calendarOrder))
	for _, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil || cal.Source == calendar.SpecialSourceBirthdays {
			continue
		}
		name := cal.DisplayName
		if name == "" {
			name = cal.Name
		}
		out = append(out, huh.NewOption(name+" ("+cal.Source+")", key))
	}
	if len(out) == 0 {
		out = append(out, huh.NewOption("No writable calendar", ""))
	}
	return out
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
	if len(out) == 0 {
		out = append(out, huh.NewOption("None", ""))
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
	}
	s.activeForm = nil
	s.activeKey = ""
	s.backup = nil
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
	s.activeKey = ""
	s.backup = nil
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
	if cal.Source != "" {
		return name + " (" + cal.Source + ")"
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

func validatePositiveInput(v string) error {
	n, err := parsePositiveIntDefault(v, 0)
	if err != nil || n <= 0 {
		return errors.New("value must be a positive number")
	}
	return nil
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

func activeFormFiltering(form *huh.Form) bool {
	if form == nil {
		return false
	}
	field := form.GetFocusedField()
	if field == nil {
		return false
	}
	filtering, ok := field.(interface{ GetFiltering() bool })
	return ok && filtering.GetFiltering()
}

func editorSeparatorRow(label string) editorRow {
	return editorRow{key: "__separator:" + label, label: label}
}

func isEditorSeparator(row editorRow) bool {
	return strings.HasPrefix(row.key, "__separator:")
}

func editorRowWraps(row editorRow) bool {
	return row.key == "description" || row.key == "attendees"
}

func editorDisplayValue(row editorRow) string {
	if row.key == "attendees" {
		return strings.ReplaceAll(row.value, "; ", "\n")
	}
	return row.value
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
	if !isEditorSeparator(rows[cursor]) {
		return cursor
	}
	for i := cursor + 1; i < len(rows); i++ {
		if !isEditorSeparator(rows[i]) {
			return i
		}
	}
	for i := cursor - 1; i >= 0; i-- {
		if !isEditorSeparator(rows[i]) {
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
		if !isEditorSeparator(rows[next]) {
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

func attendeeMultiSelectKeyMap() *huh.KeyMap {
	keymap := huh.NewDefaultKeyMap()
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
	if len(ev.Attendees) > 0 {
		lines = append(lines, "Attendees: "+formatAttendees(ev.Attendees))
	}
	lines = append(lines,
		"RSVP: "+attendeeRSVPDisplay(ev.Attendees),
		"Availability: "+eventAvailabilityDisplay(ev.Availability),
		"Visibility: "+eventVisibilityDisplay(ev.Visibility),
	)
	if ev.Recurrence != nil {
		lines = append(lines, "Repeats: "+formatRecurrence(ev.Recurrence))
	} else if ev.Recurring {
		lines = append(lines, "Repeats: yes")
	}
	if len(ev.Alarms) > 0 {
		lines = append(lines, "Notifications: "+formatAlarms(ev.Alarms))
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
	target := m.eventCursor + delta
	if target < 0 {
		needed := -target
		prepended := m.moveAgendaWindowBackward(needed)
		items = m.agendaItems()
		if len(items) == 0 {
			m.eventCursor = 0
			m.eventListOffset = 0
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

func singleLineValue(v string) string {
	v = strings.TrimSpace(strings.ReplaceAll(v, "\n", " "))
	if v == "" {
		return "-"
	}
	return v
}

func multilineValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func optionalDateTimeValue(date, clock string) string {
	date = strings.TrimSpace(date)
	clock = strings.TrimSpace(clock)
	if date == "" && clock == "" {
		return "-"
	}
	return strings.TrimSpace(date + " " + clock)
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
	if width < 78 {
		width = 78
	}
	if height < 22 {
		height = 22
	}
	title := m.styles.Title.Render("Shortcuts")
	left := []string{
		"q, ctrl+c   Quit",
		"?           Toggle help",
		"",
		"Agenda:",
		"j/k, ↑/↓    Next / previous item",
		"ctrl+f/b    Page down / page up",
		"h/l, ←/→    Previous / next day",
		"ctrl+h/l    Previous / next week",
		"t           Today",
		"enter, spc  Focus / unfocus details",
		"ctrl+j/k    Scroll details down/up",
		"f           Toggle show-free mode",
		"m           Toggle tasks-only mode",
		"c           Open calendars toggle pane",
		"n           New event / task",
		"e           Edit selected item",
		"ctrl+d      Delete selected item",
		"",
		"Calendar pane:",
		"j/k, ↑/↓    Move calendar cursor",
		"enter, spc  Toggle calendar visibility",
		"h, esc      Close calendar pane",
	}
	right := []string{
		"Event / task editor:",
		"j/k, tab    Move between items",
		"enter       Edit selected item / submit popup",
		"ctrl+s      Save item",
		"esc, q      Cancel editor or popup",
		"ctrl+c      Cancel popup/editor",
		"ctrl+e      Open $EDITOR in description popup",
		"/           Filter attendee picker",
		"space, x    Toggle multiselect choice",
		"",
		"Delete confirm:",
		"enter       Confirm selected delete action",
		"esc, q      Cancel delete",
	}
	columnWidth := (width - 8) / 2
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		lipgloss.NewStyle().Width(columnWidth).Render(strings.Join(left, "\n")),
		lipgloss.NewStyle().Width(4).Render(""),
		lipgloss.NewStyle().Width(columnWidth).Render(strings.Join(right, "\n")),
	)
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
		mode:           mode,
		targetUID:      targetUID,
		editScope:      string(calendar.EditRecurringAll),
		summary:        ev.Summary,
		calendarKey:    key,
		location:       ev.Location,
		description:    ev.Description,
		url:            ev.URL,
		attendees:      attendeesInput(ev.Attendees),
		rsvp:           attendeeRSVPValue(ev.Attendees),
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
			huh.NewOption("Default", ""),
			huh.NewOption("Yes", "yes"),
			huh.NewOption("No", "no"),
			huh.NewOption("Maybe", "maybe"),
		).Value(&s.rsvp),
		huh.NewSelect[string]().Key("availability").Title("Availability").Options(
			huh.NewOption("Default", ""),
			huh.NewOption("Busy", "busy"),
			huh.NewOption("Free", "free"),
		).Value(&s.availability),
		huh.NewSelect[string]().Key("visibility").Title("Visibility").Options(
			huh.NewOption("Default", "default"),
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
	applyRSVPToAttendees(attendees, s.rsvp)
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
			Availability: &s.availability,
			Visibility:   &s.visibility,
			Recurrence:   recurrenceUpdatePtr,
			Alarms:       &alarms,
			Start:        &start,
			End:          &end,
			AllDay:       &s.allDay,
		}
		if s.targetEvent != nil {
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
			Summary:      s.summary,
			Description:  s.description,
			Location:     s.location,
			URL:          s.url,
			Attendees:    attendees,
			Availability: s.availability,
			Visibility:   s.visibility,
			Recurrence:   recurrence,
			Alarms:       alarms,
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
	state := &deleteConfirmState{confirm: false, scope: string(calendar.DeleteRecurringOccurrence)}
	if it.Event != nil {
		ev := *it.Event
		if ev.Source == calendar.SpecialSourceBirthdays {
			return false
		}
		state.kind = "event"
		state.event = &ev
		state.recurring = ev.Recurring
		state.itemLabel = ev.Summary
	} else if it.Todo != nil {
		td := *it.Todo
		state.kind = "task"
		state.todo = &td
		state.itemLabel = td.Summary
	} else {
		return false
	}
	state.form = m.buildDeleteConfirmForm(state)
	m.deleteConfirm = state
	m.focusDetails = true
	m.focusMain = false
	m.detailScroll = 0
	m.deleteConfirm.form.UpdateFieldPositions()
	return true
}

func (m *Model) buildDeleteConfirmForm(s *deleteConfirmState) *huh.Form {
	title := "Delete " + s.kind
	if strings.TrimSpace(s.itemLabel) != "" {
		title += ": " + s.itemLabel
	}
	fields := []huh.Field{
		huh.NewConfirm().Key("confirm").Title(title).Description("This cannot be undone").Value(&s.confirm),
	}
	if s.recurring {
		fields = append(fields, huh.NewSelect[string]().
			Key("scope").
			Title("Recurring event").
			Options(
				huh.NewOption("Only this occurrence", string(calendar.DeleteRecurringOccurrence)),
				huh.NewOption("This and following occurrences", string(calendar.DeleteRecurringFuture)),
				huh.NewOption("All occurrences", string(calendar.DeleteRecurringAll)),
			).
			Value(&s.scope))
	}
	return huh.NewForm(huh.NewGroup(fields...).Title("Delete")).WithShowHelp(true).WithShowErrors(true)
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
		if attendee.Name != "" && attendee.Email != "" && attendee.Name != attendee.Email {
			parts = append(parts, fmt.Sprintf("%s <%s>", attendee.Name, attendee.Email))
		} else if attendee.Email != "" {
			parts = append(parts, attendee.Email)
		} else if attendee.Name != "" {
			parts = append(parts, attendee.Name)
		}
	}
	return strings.Join(parts, "; ")
}

func parseAttendeesInput(raw string) []calendar.Attendee {
	fields := splitListInput(raw)
	out := make([]calendar.Attendee, 0, len(fields))
	for _, field := range fields {
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
		out = append(out, calendar.Attendee{Name: name, Email: email})
	}
	return out
}

func applyRSVPToAttendees(attendees []calendar.Attendee, rsvp string) {
	status := normalizeRSVPValue(rsvp)
	for i := range attendees {
		attendees[i].Status = status
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

func eventRSVPDisplayValue(rsvp string) string {
	switch normalizeRSVPValue(rsvp) {
	case "yes":
		return "yes"
	case "no":
		return "no"
	case "maybe":
		return "maybe"
	default:
		return "default"
	}
}

func attendeeRSVPDisplay(attendees []calendar.Attendee) string {
	counts := map[string]int{}
	for _, attendee := range attendees {
		status := normalizeRSVPValue(attendee.Status)
		if status == "" {
			status = "default"
		}
		counts[status]++
	}
	if len(counts) == 0 {
		return "default"
	}
	if len(counts) == 1 {
		for status := range counts {
			return eventRSVPDisplayValue(status)
		}
	}
	order := []string{"yes", "no", "maybe", "default"}
	parts := make([]string, 0, len(counts))
	for _, status := range order {
		if n := counts[status]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", eventRSVPDisplayValue(status), n))
		}
	}
	return strings.Join(parts, ", ")
}

func normalizeRSVPValue(rsvp string) string {
	switch strings.ToLower(strings.TrimSpace(rsvp)) {
	case "yes", "accepted":
		return "yes"
	case "no", "declined":
		return "no"
	case "maybe", "tentative":
		return "maybe"
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
		return "default"
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
	return eventVisibilityValue(visibility)
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

func formatAttendees(attendees []calendar.Attendee) string {
	return truncate(strings.Join(attendeeLabels(attendees), ", "), 160)
}

func attendeeLabels(attendees []calendar.Attendee) []string {
	out := make([]string, 0, len(attendees))
	for _, attendee := range attendees {
		if attendee.Name != "" && attendee.Email != "" && attendee.Name != attendee.Email {
			out = append(out, attendee.Name+" <"+attendee.Email+">")
		} else if attendee.Name != "" {
			out = append(out, attendee.Name)
		} else if attendee.Email != "" {
			out = append(out, attendee.Email)
		}
	}
	return out
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
