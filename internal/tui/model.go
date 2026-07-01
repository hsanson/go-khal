package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

type Model struct {
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
	agendaStart        time.Time
	eventCursor        int
	eventListOffset    int
	detailScroll       int
}

func NewModel(cfg *config.Config, data calendar.Dataset) Model {
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
		cfg:                cfg,
		data:               data,
		styles:             DefaultStyles(),
		selected:           now,
		agendaStart:        dayStart(now),
		weekViewportStart:  start,
		calendarVisibility: vis,
		calendarOrder:      order,
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
		if msg.String() == "c" {
			m.focusCalendarPane = true
			m.focusMain = false
			m.focusDetails = false
			m.ensureCalendarCursorVisible(m.calendarPaneHeight())
			m.ensureEventSelectionValid()
			return m, nil
		}
		if m.focusDetails {
			switch msg.String() {
			case "esc", "h", "enter", " ":
				m.focusDetails = false
				m.focusMain = true
				m.detailScroll = 0
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
			case "esc", "h", "q":
				m.focusCalendarPane = false
			}
			m.ensureCalendarCursorVisible(m.calendarPaneHeight())
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.focusMain {
				m.moveEventCursor(1)
			} else {
				m.selected = m.selected.AddDate(0, 0, 7)
				m.agendaStart = dayStart(m.selected)
				m.eventCursor = 0
				m.eventListOffset = 0
				m.scrollForSelection()
			}
		case "k", "up":
			if m.focusMain {
				m.moveEventCursor(-1)
			} else {
				m.selected = m.selected.AddDate(0, 0, -7)
				m.agendaStart = dayStart(m.selected)
				m.eventCursor = 0
				m.eventListOffset = 0
				m.scrollForSelection()
			}
		case "h", "left":
			if m.focusMain {
				m.focusMain = false
				m.focusDetails = false
			} else {
				m.selected = m.selected.AddDate(0, 0, -1)
				m.agendaStart = dayStart(m.selected)
				m.eventCursor = 0
				m.eventListOffset = 0
				m.scrollForSelection()
			}
		case "l", "right":
			if !m.focusMain {
				m.selected = m.selected.AddDate(0, 0, 1)
				m.agendaStart = dayStart(m.selected)
				m.eventCursor = 0
				m.eventListOffset = 0
				m.scrollForSelection()
			}
		case "p":
			m.selected = m.selected.AddDate(0, 0, -1)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "n":
			m.selected = m.selected.AddDate(0, 0, 1)
			m.agendaStart = dayStart(m.selected)
			m.eventCursor = 0
			m.eventListOffset = 0
			m.scrollForSelection()
		case "f":
			m.showFreeMode = !m.showFreeMode
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
			if !m.focusMain {
				m.focusMain = true
				m.focusDetails = false
				m.eventCursor = 0
				m.eventListOffset = 0
			} else {
				if m.currentSelectionHasDetails() {
					m.focusDetails = true
					m.detailScroll = 0
				}
			}
		case " ":
			if m.focusMain {
				if m.currentSelectionHasDetails() {
					m.focusDetails = true
					m.detailScroll = 0
				}
			}
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
	help := m.styles.Subtle.Render("Navigate: [h/l] left/right day  [j/k] up/down week-row  [p/n] prev/next day  [t] today  [c] calendars  [q] quit")
	if m.focusCalendarPane {
		help = m.styles.Subtle.Render("Calendars focus: [j/k] select calendar  [space/enter] toggle  [h/esc/q] back")
	}
	if m.focusDetails {
		help = m.styles.Subtle.Render("Details focus: [j/k] scroll details  [enter/space/esc/h] back to events")
	} else if m.focusMain {
		help = m.styles.Subtle.Render("Events list focus: [j/k] select  [enter/space] details  [f] toggle show-free  [h] back")
	}
	base := m.styles.Container.Render(lipgloss.JoinVertical(lipgloss.Left, root, "", help))
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
	return m.styles.Sidebar.Width(width).Height(panelHeight).MaxHeight(panelHeight).Render(panel)
}

func (m Model) renderMainPanel(width int) string {
	items := m.agendaItems()
	panelHeight := m.height - 6
	if panelHeight < 10 {
		panelHeight = 10
	}
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
	if m.showFreeMode {
		header += m.styles.Subtle.Render(" [SHOW-FREE]")
	}
	if m.focusMain {
		header += m.styles.Subtle.Render(" (focus)")
	} else if m.focusDetails {
		header += m.styles.Subtle.Render(" (details)")
	}
	separator := m.styles.Subtle.Render(strings.Repeat("-", max(10, width-2)))
	content := lipgloss.JoinVertical(lipgloss.Left, header, "", top, separator, detail)
	return m.styles.MainPanel.Width(width).Height(panelHeight).MaxHeight(panelHeight).Render(content)
}

func (m Model) renderEventDetailsPane(width, height int) string {
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
	lines = append(lines, "", m.styles.Subtle.Render("enter/space focuses details, j/k scroll when focused"))
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
	if m.focusDetails {
		prefix = "Details (focus)"
	}
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
	if todo.Description != "" {
		lines = append(lines, "", "Description:")
		for _, raw := range strings.Split(todo.Description, "\n") {
			lines = append(lines, wrapLine(raw, max(10, width-4))...)
		}
	}
	lines = append(lines, "", m.styles.Subtle.Render("enter/space focuses details, j/k scroll when focused"))
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
	if m.focusDetails {
		prefix = "Details (focus)"
	}
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
	return buildAgendaItems(
		start,
		filteredEvents(m.data.Events, m.calendarVisibility),
		filteredTodos(m.data.Todos, m.calendarVisibility),
		90,
		m.showFreeMode,
		m.showFreeMode,
	)
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
