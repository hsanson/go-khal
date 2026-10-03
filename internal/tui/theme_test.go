package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParseOmarchyPalette(t *testing.T) {
	palette, err := parseOmarchyPalette([]byte(testOmarchyPalette("#123456")))
	if err != nil {
		t.Fatalf("parse valid palette: %v", err)
	}
	if palette["accent"] != "#123456" {
		t.Fatalf("accent = %q, want #123456", palette["accent"])
	}
	theme := stylesFromOmarchyPalette(palette)
	if got := styleForeground(theme.styles.Accent); got != "#123456" {
		t.Fatalf("accent style = %q, want #123456", got)
	}
	if got := styleForeground(styleForColor(theme.styles.Event, "")); got != "#6fb8e3" {
		t.Fatalf("uncolored event = %q, want palette blue", got)
	}
	if got := styleForeground(styleForColor(theme.styles.Event, "#fedcba")); got != "#fedcba" {
		t.Fatalf("calendar-colored event = %q, want #fedcba", got)
	}

	missing := strings.Replace(testOmarchyPalette("#123456"), "accent = \"#123456\"\n", "", 1)
	if _, err := parseOmarchyPalette([]byte(missing)); err == nil {
		t.Fatal("palette without accent was accepted")
	}
	invalid := strings.Replace(testOmarchyPalette("#123456"), "accent = \"#123456\"", "accent = \"blue\"", 1)
	if _, err := parseOmarchyPalette([]byte(invalid)); err == nil {
		t.Fatal("palette with non-hex accent was accepted")
	}
}

func TestOmarchySurfacesUseTerminalDefaultBackground(t *testing.T) {
	palette, err := parseOmarchyPalette([]byte(testOmarchyPalette("#123456")))
	if err != nil {
		t.Fatal(err)
	}
	theme := stylesFromOmarchyPalette(palette)
	for name, style := range map[string]lipgloss.Style{
		"container": theme.styles.Container,
		"sidebar":   theme.styles.Sidebar,
		"main":      theme.styles.MainPanel,
		"overlay":   theme.styles.Overlay,
		"help":      theme.styles.HelpOverlay,
	} {
		if _, transparent := style.GetBackground().(lipgloss.NoColor); !transparent {
			t.Errorf("%s surface sets an explicit background", name)
		}
	}
	if _, transparent := theme.formTheme.Form.Base.GetBackground().(lipgloss.NoColor); !transparent {
		t.Error("form surface sets an explicit background")
	}
}

func styleForeground(style lipgloss.Style) string {
	color, _ := style.GetForeground().(lipgloss.Color)
	return string(color)
}

func testOmarchyPalette(accent string) string {
	return `mode = "dark"
accent = "` + accent + `"
selection = "#243d56"
muted = "#304860"
background = "#16242d"
dark_background = "#101b21"
darker_background = "#0b1216"
lighter_background = "#1b2d40"
foreground = "#d6e2ee"
dark_foreground = "#4d86b0"
light_foreground = "#d6e2ee"
bright_foreground = "#f2fcff"
active_border_color = "#f2fcff"
active_tab_background = "#6fb8e3"
red = "#4d86b0"
yellow = "#6fa4c9"
orange = "#8bc9eb"
green = "#5e95bc"
cyan = "#b4e4f6"
blue = "#6fb8e3"
magenta = "#8bc9eb"
brown = "#456475"
bright_red = "#73a6cb"
bright_yellow = "#9dcae5"
bright_green = "#86b7d8"
bright_cyan = "#d1eef8"
bright_blue = "#f2fcff"
bright_magenta = "#b1d8ee"
future_key = "ignored"
`
}
