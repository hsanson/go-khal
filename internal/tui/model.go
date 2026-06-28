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

type ViewMode string

const (
	ViewDay      ViewMode = "day"
	ViewWeek     ViewMode = "week"
	ViewWorkWeek ViewMode = "workweek"
	ViewMonth    ViewMode = "month"
	ViewYear     ViewMode = "year"
)

type Model struct {
	cfg                *config.Config
	data               calendar.Dataset
	styles             Styles
	mode               ViewMode
	selected           time.Time
	width              int
	height             int
	calendarVisibility map[string]bool
	calendarOrder      []string
	cursor             int
	focusLeft          bool
}

func NewModel(cfg *config.Config, data calendar.Dataset) Model {
	mode := ViewMode(cfg.DefaultView)
	switch mode {
	case ViewDay, ViewWeek, ViewWorkWeek, ViewMonth, ViewYear:
	default:
		mode = ViewMonth
	}

	vis := map[string]bool{}
	order := make([]string, 0, len(data.Calendars))
	for _, cal := range data.Calendars {
		k := calendarKey(cal.Source, cal.Name)
		vis[k] = !cal.Hidden
		order = append(order, k)
	}
	sort.Strings(order)

	return Model{
		cfg:                cfg,
		data:               data,
		styles:             DefaultStyles(),
		mode:               mode,
		selected:           time.Now(),
		calendarVisibility: vis,
		calendarOrder:      order,
		focusLeft:          true,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focusLeft = !m.focusLeft
		case "up", "k":
			if m.focusLeft {
				if m.cursor > 0 {
					m.cursor--
				}
			} else {
				m.selected = m.selected.AddDate(0, 0, -1)
			}
		case "down", "j":
			if m.focusLeft {
				if m.cursor < len(m.calendarOrder)-1 {
					m.cursor++
				}
			} else {
				m.selected = m.selected.AddDate(0, 0, 1)
			}
		case " ", "enter":
			if m.focusLeft && len(m.calendarOrder) > 0 {
				key := m.calendarOrder[m.cursor]
				m.calendarVisibility[key] = !m.calendarVisibility[key]
			}
		case "d":
			m.mode = ViewDay
		case "w":
			m.mode = ViewWeek
		case "5":
			m.mode = ViewWorkWeek
		case "m":
			m.mode = ViewMonth
		case "y":
			m.mode = ViewYear
		case "left", "h", "p":
			m.selected = m.shift(-1)
		case "right", "l", "n":
			m.selected = m.shift(1)
		case "[":
			m.selected = m.selected.AddDate(0, -1, 0)
		case "]":
			m.selected = m.selected.AddDate(0, 1, 0)
		case "{":
			m.selected = m.selected.AddDate(-1, 0, 0)
		case "}":
			m.selected = m.selected.AddDate(1, 0, 0)
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
	help := m.styles.Subtle.Render("Views: [d] day [w] week [5] workweek [m] month [y] year | Move: [h/l] or [p/n] | Jump month [/] year { } | Toggle calendar: space")

	return m.styles.Container.Render(lipgloss.JoinVertical(lipgloss.Left, root, "", help))
}

func (m Model) renderLeftPanel(width int) string {
	month := renderMiniMonth(m.selected, filteredEvents(m.data.Events, m.calendarVisibility), width-2, m.styles)

	var calLines []string
	calLines = append(calLines, m.styles.Title.Render("Calendars"))
	for i, key := range m.calendarOrder {
		cal := m.calendarByKey(key)
		if cal == nil {
			continue
		}
		prefix := "  "
		if m.focusLeft && i == m.cursor {
			prefix = "> "
		}
		check := "[ ]"
		if m.calendarVisibility[key] {
			check = "[x]"
		}
		name := cal.DisplayName
		if name == "" {
			name = cal.Name
		}
		line := wrappedCalendarLine(prefix, check, name, width-4)
		if cal.Color != "" {
			line = styleForColor(m.styles.CalendarItem, cal.Color).Render(line)
		}
		calLines = append(calLines, line)
	}

	panel := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.PanelTitle.Render("Selected: "+m.selected.Format("2006-01-02")),
		"",
		month,
		"",
		strings.Join(calLines, "\n"),
	)

	return m.styles.Sidebar.Width(width).Height(m.height - 6).Render(panel)
}

func wrappedCalendarLine(prefix, check, name string, width int) string {
	if width < 8 {
		width = 8
	}
	firstPrefix := fmt.Sprintf("%s%s ", prefix, check)
	continuationPrefix := "   "
	maxFirst := width - len([]rune(firstPrefix))
	if maxFirst < 1 {
		maxFirst = 1
	}
	maxNext := width - len([]rune(continuationPrefix))
	if maxNext < 1 {
		maxNext = 1
	}

	runes := []rune(name)
	if len(runes) <= maxFirst {
		return firstPrefix + name
	}

	parts := make([]string, 0, 4)
	parts = append(parts, firstPrefix+string(runes[:maxFirst]))
	runes = runes[maxFirst:]
	for len(runes) > 0 {
		take := maxNext
		if len(runes) < take {
			take = len(runes)
		}
		parts = append(parts, continuationPrefix+string(runes[:take]))
		runes = runes[take:]
	}
	return strings.Join(parts, "\n")
}

func (m Model) renderMainPanel(width int) string {
	events := filteredEvents(m.data.Events, m.calendarVisibility)
	var body string
	var title string
	switch m.mode {
	case ViewDay:
		title = "Day"
		body = renderDayColumns([]time.Time{dayStart(m.selected)}, events, width-2, m.cfg.TimeFormat, m.styles)
	case ViewWeek:
		title = "Week"
		start := calendar.StartOfWeek(m.selected, m.weekStart())
		days := make([]time.Time, 7)
		for i := 0; i < 7; i++ {
			days[i] = start.AddDate(0, 0, i)
		}
		body = renderDayColumns(days, events, width-2, m.cfg.TimeFormat, m.styles)
	case ViewWorkWeek:
		title = "Work Week"
		start := calendar.StartOfWeek(m.selected, m.weekStart())
		days := make([]time.Time, 5)
		for i := 0; i < 5; i++ {
			days[i] = start.AddDate(0, 0, i)
		}
		body = renderDayColumns(days, events, width-2, m.cfg.TimeFormat, m.styles)
	case ViewYear:
		title = "Year"
		body = renderYearGrid(m.selected.Year(), events, width-2, m.styles)
	default:
		title = "Month"
		body = renderMonthGrid(m.selected, events, width-2, m.styles)
	}

	header := m.styles.PanelTitle.Render(fmt.Sprintf("%s View - %s", title, m.selected.Format("Mon Jan 2, 2006")))
	return m.styles.MainPanel.Width(width).Height(m.height - 6).Render(lipgloss.JoinVertical(lipgloss.Left, header, "", body))
}

func (m Model) shift(delta int) time.Time {
	switch m.mode {
	case ViewDay:
		return m.selected.AddDate(0, 0, delta)
	case ViewWeek, ViewWorkWeek:
		return m.selected.AddDate(0, 0, 7*delta)
	case ViewMonth:
		return m.selected.AddDate(0, delta, 0)
	case ViewYear:
		return m.selected.AddDate(delta, 0, 0)
	default:
		return m.selected.AddDate(0, 0, delta)
	}
}

func (m Model) calendarByKey(key string) *calendar.Calendar {
	for i := range m.data.Calendars {
		if calendarKey(m.data.Calendars[i].Source, m.data.Calendars[i].Name) == key {
			return &m.data.Calendars[i]
		}
	}
	return nil
}

func calendarKey(source, name string) string {
	return source + "/" + name
}

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

func (m Model) weekStart() time.Weekday {
	if m.cfg == nil {
		return time.Monday
	}
	return m.cfg.WeekStart()
}
