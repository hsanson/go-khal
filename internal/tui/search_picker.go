package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const searchPickerVisibleRows = 8

type searchOption struct {
	label  string
	value  string
	search string
}

type searchPicker struct {
	title     string
	options   []searchOption
	filtered  []int
	query     string
	cursor    int
	offset    int
	selected  string
	done      bool
	cancelled bool
}

func newSearchPicker(title string, options []searchOption, current string) *searchPicker {
	picker := &searchPicker{
		title:    title,
		options:  options,
		filtered: make([]int, 0, len(options)),
	}
	for i := range picker.options {
		picker.options[i].search = normalizeSearchText(picker.options[i].label)
	}
	picker.refilter()
	for i, optionIndex := range picker.filtered {
		if picker.options[optionIndex].value == current {
			picker.cursor = i
			picker.ensureVisible()
			break
		}
	}
	return picker
}

func (p *searchPicker) Update(msg tea.KeyMsg) {
	if p == nil || p.done || p.cancelled {
		return
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		p.cancelled = true
	case "down", "ctrl+j":
		if p.cursor+1 < len(p.filtered) {
			p.cursor++
			p.ensureVisible()
		}
	case "up", "ctrl+k":
		if p.cursor > 0 {
			p.cursor--
			p.ensureVisible()
		}
	case "enter":
		if len(p.filtered) > 0 {
			p.selected = p.options[p.filtered[p.cursor]].value
			p.done = true
		}
	case "backspace":
		runes := []rune(p.query)
		if len(runes) > 0 {
			p.query = string(runes[:len(runes)-1])
			p.refilter()
		}
	case "ctrl+u":
		if p.query != "" {
			p.query = ""
			p.refilter()
		}
	default:
		if msg.Type != tea.KeyRunes {
			return
		}
		changed := false
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				p.query += string(r)
				changed = true
			}
		}
		if changed {
			p.refilter()
		}
	}
}

func (p *searchPicker) refilter() {
	p.filtered = p.filtered[:0]
	query := normalizeSearchText(strings.TrimSpace(p.query))
	for i := range p.options {
		if query == "" || fuzzySubsequence(query, p.options[i].search) {
			p.filtered = append(p.filtered, i)
		}
	}
	p.cursor = 0
	p.offset = 0
}

func (p *searchPicker) ensureVisible() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+searchPickerVisibleRows {
		p.offset = p.cursor - searchPickerVisibleRows + 1
	}
}

func (p *searchPicker) View(width int, styles Styles) string {
	if p == nil {
		return ""
	}
	width = max(32, width)
	search := styles.Accent.Render("Search: ") + p.query + "█"
	lines := make([]string, 0, searchPickerVisibleRows)
	if len(p.filtered) == 0 {
		lines = append(lines, styles.Subtle.Render("No matches"))
	} else {
		end := min(len(p.filtered), p.offset+searchPickerVisibleRows)
		for i := p.offset; i < end; i++ {
			prefix := "  "
			style := lipgloss.NewStyle().Width(width)
			if i == p.cursor {
				prefix = " "
				style = style.Background(lipgloss.Color("238")).Foreground(lipgloss.Color("230")).Bold(true)
			}
			label := p.options[p.filtered[i]].label
			lines = append(lines, style.Render(prefix+truncate(label, width-2)))
		}
	}
	for len(lines) < searchPickerVisibleRows {
		lines = append(lines, "")
	}
	legend := styles.Subtle.Render("[↑/↓ ctrl-j/ctrl-k] Navigate  [enter] Select  [esc] Cancel")
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.PanelTitle.Render(p.title),
		search,
		"",
		strings.Join(lines, "\n"),
		legend,
	)
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(value)
	return strings.NewReplacer("_", " ", "/", " ", "-", " ").Replace(value)
}

func fuzzySubsequence(query, target string) bool {
	queryRunes := []rune(query)
	if len(queryRunes) == 0 {
		return true
	}
	matched := 0
	for _, r := range target {
		if r == queryRunes[matched] {
			matched++
			if matched == len(queryRunes) {
				return true
			}
		}
	}
	return false
}
