package tui

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type themeLoadStatus uint8

const (
	themeMissing themeLoadStatus = iota
	themeInvalid
	themeValid
)

var omarchyColorKeys = [...]string{
	"accent",
	"selection",
	"muted",
	"background",
	"dark_background",
	"darker_background",
	"lighter_background",
	"foreground",
	"dark_foreground",
	"light_foreground",
	"bright_foreground",
	"active_border_color",
	"active_tab_background",
	"red",
	"yellow",
	"orange",
	"green",
	"cyan",
	"blue",
	"magenta",
	"brown",
	"bright_red",
	"bright_yellow",
	"bright_green",
	"bright_cyan",
	"bright_blue",
	"bright_magenta",
}

type omarchyPalette map[string]string

type themeStyles struct {
	styles    Styles
	formTheme *huh.Theme
}

type themeStylesMsg struct {
	theme themeStyles
}

func defaultInteractiveTheme() themeStyles {
	return themeStyles{styles: DefaultStyles(), formTheme: huh.ThemeCharm()}
}

func readThemeStyles(path string) (themeStyles, themeLoadStatus, [sha256.Size]byte) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return themeStyles{}, themeMissing, [sha256.Size]byte{}
	}
	if err != nil {
		return themeStyles{}, themeInvalid, [sha256.Size]byte{}
	}
	palette, err := parseOmarchyPalette(data)
	if err != nil {
		return themeStyles{}, themeInvalid, [sha256.Size]byte{}
	}
	return stylesFromOmarchyPalette(palette), themeValid, sha256.Sum256(data)
}

func parseOmarchyPalette(data []byte) (omarchyPalette, error) {
	palette := make(omarchyPalette, len(omarchyColorKeys))
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid palette line %q", line)
		}
		key = strings.TrimSpace(key)
		if !isOmarchyColorKey(key) {
			continue
		}
		if _, exists := palette[key]; exists {
			return nil, fmt.Errorf("duplicate palette color %q", key)
		}
		value, err := parseOmarchyColorValue(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid palette color %q: %w", key, err)
		}
		palette[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for _, key := range omarchyColorKeys {
		if palette[key] == "" {
			return nil, fmt.Errorf("missing palette color %q", key)
		}
	}
	return palette, nil
}

func isOmarchyColorKey(key string) bool {
	for _, candidate := range omarchyColorKeys {
		if key == candidate {
			return true
		}
	}
	return false
}

func parseOmarchyColorValue(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' && raw[0] != '\'' {
		return "", errors.New("expected a quoted color")
	}
	quote := raw[0]
	end := strings.IndexByte(raw[1:], quote)
	if end < 0 {
		return "", errors.New("unterminated quoted color")
	}
	value := raw[1 : end+1]
	rest := strings.TrimSpace(raw[end+2:])
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return "", errors.New("unexpected content after color")
	}
	if len(value) != 7 || value[0] != '#' {
		return "", errors.New("expected #RRGGBB")
	}
	if _, err := strconv.ParseUint(value[1:], 16, 24); err != nil {
		return "", errors.New("expected #RRGGBB")
	}
	return value, nil
}

func stylesFromOmarchyPalette(p omarchyPalette) themeStyles {
	color := func(key string) lipgloss.Color { return lipgloss.Color(p[key]) }
	styles := Styles{
		Title:           lipgloss.NewStyle().Bold(true).Foreground(color("bright_blue")),
		Subtle:          lipgloss.NewStyle().Foreground(color("muted")),
		Accent:          lipgloss.NewStyle().Foreground(color("accent")).Bold(true),
		Hour:            lipgloss.NewStyle().Foreground(color("cyan")),
		Event:           lipgloss.NewStyle().Foreground(color("blue")),
		TodoDone:        lipgloss.NewStyle().Foreground(color("bright_green")),
		TodoOpen:        lipgloss.NewStyle().Foreground(color("yellow")),
		Container:       lipgloss.NewStyle().Foreground(color("foreground")).Padding(1, 1),
		Sidebar:         lipgloss.NewStyle().Foreground(color("foreground")).Border(lipgloss.RoundedBorder()).BorderForeground(color("active_border_color")).Padding(0, 1),
		MainPanel:       lipgloss.NewStyle().Foreground(color("foreground")).Border(lipgloss.RoundedBorder()).BorderForeground(color("active_border_color")).Padding(0, 1),
		PanelTitle:      lipgloss.NewStyle().Bold(true).Foreground(color("bright_magenta")),
		CalendarItem:    lipgloss.NewStyle().Foreground(color("blue")),
		DayHeader:       lipgloss.NewStyle().Bold(true).Foreground(color("bright_cyan")),
		GridCell:        lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(color("brown")).Padding(0, 1),
		Overlay:         lipgloss.NewStyle().Foreground(color("foreground")).Border(lipgloss.RoundedBorder()).BorderForeground(color("active_border_color")).Padding(1, 2),
		HelpOverlay:     lipgloss.NewStyle().Foreground(color("foreground")).Border(lipgloss.RoundedBorder()).BorderForeground(color("active_border_color")).Padding(1, 2),
		SelectedCell:    lipgloss.NewStyle().Background(color("selection")).Border(lipgloss.NormalBorder()).BorderForeground(color("accent")).Padding(0, 1),
		Error:           lipgloss.NewStyle().Foreground(color("red")).Bold(true),
		Success:         lipgloss.NewStyle().Foreground(color("green")).Bold(true),
		HighPriority:    lipgloss.NewStyle().Foreground(color("bright_red")),
		Value:           lipgloss.NewStyle().Foreground(color("light_foreground")),
		Disabled:        lipgloss.NewStyle().Foreground(color("dark_foreground")),
		SelectedRow:     lipgloss.NewStyle().Background(color("selection")).Foreground(color("bright_foreground")).Bold(true),
		Focus:           lipgloss.NewStyle().Background(color("accent")).Foreground(color("darker_background")).Bold(true),
		RangeEndpoint:   lipgloss.NewStyle().Background(color("active_tab_background")).Foreground(color("bright_foreground")).Bold(true),
		Range:           lipgloss.NewStyle().Background(color("lighter_background")).Foreground(color("foreground")),
		TimeCursor:      lipgloss.NewStyle().Background(color("selection")).Foreground(color("bright_foreground")).Bold(true),
		PrimaryButton:   lipgloss.NewStyle().Foreground(color("darker_background")).Background(color("orange")).Bold(true).Padding(0, 1),
		SecondaryButton: lipgloss.NewStyle().Foreground(color("foreground")).Background(color("lighter_background")).Bold(true).Padding(0, 1),
		FocusedButton:   lipgloss.NewStyle().Foreground(color("darker_background")).Background(color("active_tab_background")).Bold(true).Padding(0, 1),
	}
	return themeStyles{styles: styles, formTheme: formThemeFromOmarchyPalette(p)}
}

func formThemeFromOmarchyPalette(p omarchyPalette) *huh.Theme {
	color := func(key string) lipgloss.Color { return lipgloss.Color(p[key]) }
	theme := huh.ThemeBase()

	theme.Form.Base = theme.Form.Base.Foreground(color("foreground"))
	theme.Focused.Base = theme.Focused.Base.BorderForeground(color("active_border_color"))
	theme.Focused.Card = theme.Focused.Base
	theme.Focused.Title = theme.Focused.Title.Foreground(color("accent")).Bold(true)
	theme.Focused.NoteTitle = theme.Focused.NoteTitle.Foreground(color("bright_magenta")).Bold(true)
	theme.Focused.Description = theme.Focused.Description.Foreground(color("muted"))
	theme.Focused.ErrorIndicator = theme.Focused.ErrorIndicator.Foreground(color("red"))
	theme.Focused.ErrorMessage = theme.Focused.ErrorMessage.Foreground(color("red"))
	theme.Focused.SelectSelector = theme.Focused.SelectSelector.Foreground(color("accent"))
	theme.Focused.NextIndicator = theme.Focused.NextIndicator.Foreground(color("accent"))
	theme.Focused.PrevIndicator = theme.Focused.PrevIndicator.Foreground(color("accent"))
	theme.Focused.Option = theme.Focused.Option.Foreground(color("foreground"))
	theme.Focused.Directory = theme.Focused.Directory.Foreground(color("cyan"))
	theme.Focused.File = theme.Focused.File.Foreground(color("foreground"))
	theme.Focused.MultiSelectSelector = theme.Focused.MultiSelectSelector.Foreground(color("accent"))
	theme.Focused.SelectedOption = theme.Focused.SelectedOption.Foreground(color("green"))
	theme.Focused.SelectedPrefix = theme.Focused.SelectedPrefix.Foreground(color("bright_green"))
	theme.Focused.UnselectedOption = theme.Focused.UnselectedOption.Foreground(color("foreground"))
	theme.Focused.UnselectedPrefix = theme.Focused.UnselectedPrefix.Foreground(color("muted"))
	theme.Focused.FocusedButton = theme.Focused.FocusedButton.Foreground(color("darker_background")).Background(color("active_tab_background")).Bold(true)
	theme.Focused.BlurredButton = theme.Focused.BlurredButton.Foreground(color("foreground")).Background(color("lighter_background"))
	theme.Focused.Next = theme.Focused.FocusedButton
	theme.Focused.TextInput.Cursor = theme.Focused.TextInput.Cursor.Foreground(color("bright_yellow"))
	theme.Focused.TextInput.CursorText = theme.Focused.TextInput.CursorText.Foreground(color("darker_background"))
	theme.Focused.TextInput.Placeholder = theme.Focused.TextInput.Placeholder.Foreground(color("muted"))
	theme.Focused.TextInput.Prompt = theme.Focused.TextInput.Prompt.Foreground(color("accent"))
	theme.Focused.TextInput.Text = theme.Focused.TextInput.Text.Foreground(color("foreground"))

	theme.Blurred = theme.Focused
	theme.Blurred.Base = theme.Blurred.Base.BorderStyle(lipgloss.HiddenBorder()).BorderForeground(color("muted"))
	theme.Blurred.Card = theme.Blurred.Base
	theme.Blurred.Title = theme.Blurred.Title.Foreground(color("dark_foreground"))
	theme.Blurred.NoteTitle = theme.Blurred.NoteTitle.Foreground(color("dark_foreground"))
	theme.Blurred.NextIndicator = lipgloss.NewStyle()
	theme.Blurred.PrevIndicator = lipgloss.NewStyle()
	theme.Group.Base = theme.Group.Base.Foreground(color("foreground"))
	theme.Group.Title = theme.Focused.Title
	theme.Group.Description = theme.Focused.Description
	return theme
}
