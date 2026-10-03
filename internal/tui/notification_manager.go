package tui

import "strings"

type notificationManagerState struct {
	items  []notificationEditorItem
	cursor int
}

type notificationEditorItem struct {
	value  string
	remove bool
}

func newNotificationManager(values []string) *notificationManagerState {
	items := make([]notificationEditorItem, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			items = append(items, notificationEditorItem{value: value})
		}
	}
	return &notificationManagerState{items: items}
}

func (m *notificationManagerState) cycle(index int) {
	if m == nil || index < 0 || index >= len(m.items) {
		return
	}
	m.cursor = index
	m.items[index].remove = !m.items[index].remove
}

func (m *notificationManagerState) values() []string {
	if m == nil {
		return nil
	}
	values := make([]string, 0, len(m.items))
	for _, item := range m.items {
		if !item.remove {
			values = append(values, item.value)
		}
	}
	return values
}

func (m *notificationManagerState) render(width, height int, styles Styles) (string, []mouseHit) {
	width = max(28, width)
	height = max(6, height)
	lines := []string{styles.PanelTitle.Render("Notifications"), ""}
	hits := make([]mouseHit, 0, len(m.items))
	if len(m.items) == 0 {
		lines = append(lines, styles.Subtle.Render("No notifications"))
	} else {
		bodyHeight := max(1, height-4)
		start := 0
		if m.cursor >= bodyHeight {
			start = m.cursor - bodyHeight + 1
		}
		end := min(len(m.items), start+bodyHeight)
		for i := start; i < end; i++ {
			item := m.items[i]
			prefix := "  "
			if i == m.cursor {
				prefix = " "
			}
			glyph := styles.Success.Render("󰀠")
			if item.remove {
				glyph = styles.Error.Render("")
			}
			line := prefix + glyph + " " + truncate(item.value, max(8, width-5))
			if i == m.cursor {
				line = editorRowStyle(styles, true, width).Render(line)
			}
			hits = append(hits, mouseHit{rect: mouseRect{x: 0, y: len(lines), width: width, height: 1}, kind: mouseNotificationRow, index: i})
			lines = append(lines, line)
		}
	}
	active := styles.Success.Render("󰀠")
	removed := styles.Error.Render("")
	lines = append(lines, "", styles.Subtle.Render(" Focused  ")+active+styles.Subtle.Render(" Active  ")+removed+styles.Subtle.Render(" Removed"))
	return strings.Join(lines, "\n"), hits
}
