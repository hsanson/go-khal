package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func workflowChoiceView(title string, choices []editorChoice, current string, kind mouseTarget, styles Styles) (string, []mouseHit) {
	lines := []string{styles.Title.Render(title), ""}
	hits := make([]mouseHit, 0, len(choices))
	for _, choice := range choices {
		prefix := "  "
		if choice.value == current {
			prefix = "> "
		}
		line := prefix + choice.label
		if choice.value == current {
			line = styles.Accent.Render(line)
		}
		hits = append(hits, mouseHit{
			rect:  mouseRect{x: 0, y: len(lines), width: max(1, len(prefix+choice.label)), height: 1},
			kind:  kind,
			value: choice.value,
		})
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), hits
}

func deleteWorkflowView(state *deleteConfirmState, styles Styles) (string, []mouseHit) {
	if state.stage == "scope" {
		return workflowChoiceView("Delete recurring event", []editorChoice{
			{"Only this occurrence", "occurrence"},
			{"This and following occurrences", "future"},
			{"All occurrences", "all"},
		}, state.scope, mouseDeleteScope, styles)
	}

	title := "Delete " + state.kind
	if strings.TrimSpace(state.itemLabel) != "" {
		title += ": " + state.itemLabel
	}
	description := "This cannot be undone"
	if state.recurring {
		description = "Delete " + eventEditScopeLabel(state.scope) + ". " + description
	}
	no := "[ No ]"
	yes := "[ Yes ]"
	buttons := no + "  " + yes
	return strings.Join([]string{styles.Title.Render(title), description, "", buttons}, "\n"), []mouseHit{
		{rect: mouseRect{x: 0, y: 3, width: len(no), height: 1}, kind: mouseDeleteConfirm, value: "false"},
		{rect: mouseRect{x: len(no) + 2, y: 3, width: len(yes), height: 1}, kind: mouseDeleteConfirm, value: "true"},
	}
}

func (m Model) addOffsetMouseHits(hits []mouseHit, x, y int) {
	for _, hit := range hits {
		hit.rect.x += x
		hit.rect.y += y
		m.addMouseHit(hit)
	}
}

func (m Model) overlayCenteredWithHits(base, modal string, width, height int, hits []mouseHit) string {
	box := centeredOverlayBox(modal, width)
	boxX := max(0, (width-lipgloss.Width(box))/2)
	boxY := max(0, (height-lipgloss.Height(box))/2)
	m.addOffsetMouseHits(hits, m.mouseMainX+boxX+3, m.mouseMainY+boxY+2)
	return placeCenteredBox(base, box, width, height)
}
