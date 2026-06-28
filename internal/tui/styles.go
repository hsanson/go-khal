package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	Title        lipgloss.Style
	Subtle       lipgloss.Style
	Accent       lipgloss.Style
	Hour         lipgloss.Style
	Event        lipgloss.Style
	TodoDone     lipgloss.Style
	TodoOpen     lipgloss.Style
	Container    lipgloss.Style
	Sidebar      lipgloss.Style
	MainPanel    lipgloss.Style
	PanelTitle   lipgloss.Style
	CalendarItem lipgloss.Style
	DayHeader    lipgloss.Style
	GridCell     lipgloss.Style
	SelectedCell lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")),
		Subtle:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Accent:       lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		Hour:         lipgloss.NewStyle().Foreground(lipgloss.Color("111")),
		Event:        lipgloss.NewStyle().Foreground(lipgloss.Color("253")),
		TodoDone:     lipgloss.NewStyle().Foreground(lipgloss.Color("78")),
		TodoOpen:     lipgloss.NewStyle().Foreground(lipgloss.Color("221")),
		Container:    lipgloss.NewStyle().Padding(1, 1),
		Sidebar:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		MainPanel:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		PanelTitle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("153")),
		CalendarItem: lipgloss.NewStyle(),
		DayHeader:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("151")),
		GridCell:     lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1),
		SelectedCell: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("117")).Padding(0, 1),
	}
}
