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
	showCalendar       bool
	dialogCursor       int
	focusMain          bool
	focusDetails       bool
	eventCursor        int
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

	now := time.Now()
	start := calendar.StartOfWeek(now, cfg.WeekStart())
	return Model{
		cfg:                cfg,
		data:               data,
		styles:             DefaultStyles(),
		selected:           now,
		weekViewportStart:  start,
		calendarVisibility: vis,
		calendarOrder:      order,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.scrollForSelection()
	case tea.KeyMsg:
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
		if m.showCalendar {
			return m.updateCalendarDialog(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "c":
			m.showCalendar = true
			m.dialogCursor = 0
		case "j", "down":
			if m.focusMain {
				m.moveEventCursor(1)
			} else {
				m.selected = m.selected.AddDate(0, 0, 7)
				m.scrollForSelection()
			}
		case "k", "up":
			if m.focusMain {
				m.moveEventCursor(-1)
			} else {
				m.selected = m.selected.AddDate(0, 0, -7)
				m.scrollForSelection()
			}
		case "h", "left":
			if m.focusMain {
				m.focusMain = false
				m.focusDetails = false
			} else {
				m.selected = m.selected.AddDate(0, 0, -1)
				m.scrollForSelection()
			}
		case "l", "right":
			if !m.focusMain {
				m.selected = m.selected.AddDate(0, 0, 1)
				m.scrollForSelection()
			}
		case "p":
			m.selected = m.selected.AddDate(0, 0, -1)
			m.scrollForSelection()
		case "n":
			m.selected = m.selected.AddDate(0, 0, 1)
			m.scrollForSelection()
		case "t":
			now := time.Now().In(m.selected.Location())
			m.selected = now
			m.weekViewportStart = calendar.StartOfWeek(now, m.weekStart())
			m.focusMain = false
			m.focusDetails = false
			m.eventCursor = 0
			m.detailScroll = 0
		case "enter":
			if !m.focusMain {
				m.focusMain = true
				m.focusDetails = false
				m.eventCursor = 0
			} else {
				events := agendaEventsFromDay(dayStart(m.selected), filteredEvents(m.data.Events, m.calendarVisibility))
				if len(events) > 0 {
					m.focusDetails = true
					m.detailScroll = 0
				}
			}
		case " ":
			if m.focusMain {
				events := agendaEventsFromDay(dayStart(m.selected), filteredEvents(m.data.Events, m.calendarVisibility))
				if len(events) > 0 {
					m.focusDetails = true
					m.detailScroll = 0
				}
			}
		}
	}
	return m, nil
}

func (m Model) updateCalendarDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "c":
		m.showCalendar = false
	case "up", "k":
		if m.dialogCursor > 0 {
			m.dialogCursor--
		}
	case "down", "j":
		if m.dialogCursor < len(m.calendarOrder)-1 {
			m.dialogCursor++
		}
	case " ", "enter":
		if len(m.calendarOrder) > 0 {
			key := m.calendarOrder[m.dialogCursor]
			m.calendarVisibility[key] = !m.calendarVisibility[key]
		}
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
	if m.focusDetails {
		help = m.styles.Subtle.Render("Details focus: [j/k] scroll details  [enter/space/esc/h] back to events")
	} else if m.focusMain {
		help = m.styles.Subtle.Render("Events list focus: [j/k] select event  [enter/space] focus details  [h] back to calendar")
	}
	base := m.styles.Container.Render(lipgloss.JoinVertical(lipgloss.Left, root, "", help))
	if !m.showCalendar {
		return base
	}
	return lipgloss.JoinVertical(lipgloss.Left, base, "", m.renderCalendarDialog())
}

func (m Model) renderLeftPanel(width int) string {
	panelHeight := m.height - 6
	if panelHeight < 8 {
		panelHeight = 8
	}
	weeks := renderWeekViewport(m.weekViewportStart, m.selected, filteredEvents(m.data.Events, m.calendarVisibility), width-2, m.monthListHeightBudget(), m.weekStart(), m.styles)
	panel := lipgloss.JoinVertical(lipgloss.Left, m.styles.PanelTitle.Render("Months"), "", weeks)
	return m.styles.Sidebar.Width(width).Height(panelHeight).MaxHeight(panelHeight).Render(panel)
}

func (m Model) renderMainPanel(width int) string {
	events := filteredEvents(m.data.Events, m.calendarVisibility)
	day := dayStart(m.selected)
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

	rendered := renderAgendaFromDay(day, events, width-2, topHeight, m.cfg.TimeFormat, m.styles, m.eventCursor, m.focusMain || m.focusDetails)
	if m.eventCursor >= rendered.EventCount && rendered.EventCount > 0 {
		m.eventCursor = rendered.EventCount - 1
	}
	if rendered.EventCount == 0 {
		m.eventCursor = 0
	}
	top := lipgloss.NewStyle().Height(topHeight).MaxHeight(topHeight).Render(rendered.Text)
	detail := m.renderEventDetailsPane(width-2, bottomHeight)
	header := m.styles.PanelTitle.Render(fmt.Sprintf("Agenda from %s", m.selected.Format("Mon Jan 2, 2006")))
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
	events := agendaEventsFromDay(dayStart(m.selected), filteredEvents(m.data.Events, m.calendarVisibility))
	if len(events) == 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(m.styles.Subtle.Render("No event selected"))
	}
	if m.eventCursor < 0 {
		return lipgloss.NewStyle().Width(width).Height(height).Render(m.styles.Subtle.Render("No event selected"))
	}
	idx := m.eventCursor
	if idx >= len(events) {
		idx = len(events) - 1
	}
	ev := events[idx]
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

func (m *Model) moveEventCursor(delta int) {
	events := agendaEventsFromDay(dayStart(m.selected), filteredEvents(m.data.Events, m.calendarVisibility))
	if len(events) == 0 {
		m.eventCursor = 0
		return
	}
	m.eventCursor += delta
	if m.eventCursor < 0 {
		m.eventCursor = 0
	}
	if m.eventCursor >= len(events) {
		m.eventCursor = len(events) - 1
	}
}

func (m Model) renderCalendarDialog() string {
	lines := []string{m.styles.Title.Render("Calendars"), m.styles.Subtle.Render("j/k move, space toggle, esc close"), ""}
	for i, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil {
			continue
		}
		prefix := "  "
		if i == m.dialogCursor {
			prefix = "> "
		}
		state := "[ ]"
		if m.calendarVisibility[key] {
			state = "[x]"
		}
		name := cal.DisplayName
		if name == "" {
			name = cal.Name
		}
		line := fmt.Sprintf("%s%s %s", prefix, state, name)
		if cal.Color != "" {
			line = styleForColor(m.styles.CalendarItem, cal.Color).Render(line)
		}
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().
		Width(max(40, m.width/3)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("117")).
		Padding(1, 2).
		Background(lipgloss.Color("236")).
		Render(strings.Join(lines, "\n"))
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
	budget := m.height - 10
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
