//go:build !linux

package tui

import tea "github.com/charmbracelet/bubbletea"

type themeMonitor struct{}

func newThemeMonitor() (*themeMonitor, themeStyles) {
	return nil, defaultInteractiveTheme()
}

func (m *themeMonitor) wait() tea.Cmd { return nil }

func (m *themeMonitor) close() {}
