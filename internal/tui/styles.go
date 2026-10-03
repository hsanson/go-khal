package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	Title           lipgloss.Style
	Subtle          lipgloss.Style
	Accent          lipgloss.Style
	Hour            lipgloss.Style
	Event           lipgloss.Style
	TodoDone        lipgloss.Style
	TodoOpen        lipgloss.Style
	Container       lipgloss.Style
	Sidebar         lipgloss.Style
	MainPanel       lipgloss.Style
	PanelTitle      lipgloss.Style
	CalendarItem    lipgloss.Style
	DayHeader       lipgloss.Style
	GridCell        lipgloss.Style
	SelectedCell    lipgloss.Style
	Overlay         lipgloss.Style
	HelpOverlay     lipgloss.Style
	Error           lipgloss.Style
	Success         lipgloss.Style
	HighPriority    lipgloss.Style
	Value           lipgloss.Style
	Disabled        lipgloss.Style
	SelectedRow     lipgloss.Style
	Focus           lipgloss.Style
	RangeEndpoint   lipgloss.Style
	Range           lipgloss.Style
	TimeCursor      lipgloss.Style
	PrimaryButton   lipgloss.Style
	SecondaryButton lipgloss.Style
	FocusedButton   lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		Title:           lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")),
		Subtle:          lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Accent:          lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true),
		Hour:            lipgloss.NewStyle().Foreground(lipgloss.Color("111")),
		Event:           lipgloss.NewStyle().Foreground(lipgloss.Color("253")),
		TodoDone:        lipgloss.NewStyle().Foreground(lipgloss.Color("78")),
		TodoOpen:        lipgloss.NewStyle().Foreground(lipgloss.Color("221")),
		Container:       lipgloss.NewStyle().Padding(1, 1),
		Sidebar:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		MainPanel:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1),
		PanelTitle:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("153")),
		CalendarItem:    lipgloss.NewStyle(),
		DayHeader:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("151")),
		GridCell:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1),
		Overlay:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2),
		HelpOverlay:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("245")).Padding(1, 2),
		SelectedCell:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("117")).Padding(0, 1),
		Error:           lipgloss.NewStyle().Foreground(lipgloss.Color("210")).Bold(true),
		Success:         lipgloss.NewStyle().Foreground(lipgloss.Color("65")).Bold(true),
		HighPriority:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Value:           lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Disabled:        lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		SelectedRow:     lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")).Bold(true),
		Focus:           lipgloss.NewStyle().Background(lipgloss.Color("117")).Foreground(lipgloss.Color("232")).Bold(true),
		RangeEndpoint:   lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true),
		Range:           lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")),
		TimeCursor:      lipgloss.NewStyle().Reverse(true).Bold(true),
		PrimaryButton:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Bold(true).Padding(0, 1),
		SecondaryButton: lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("238")).Bold(true).Padding(0, 1),
		FocusedButton:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("117")).Bold(true).Padding(0, 1),
	}
}
