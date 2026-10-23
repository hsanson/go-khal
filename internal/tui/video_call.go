package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
)

type videoCall struct {
	provider string
	url      string
}

type videoCallLine struct {
	line int
	url  string
}

type videoCallOpenedMsg struct {
	err error
}

func (m Model) selectedVideoCalls() []videoCall {
	items := m.agendaItems()
	if len(items) == 0 || m.eventCursor < 0 || m.eventCursor >= len(items) {
		return nil
	}
	event := items[m.eventCursor].Event
	if event == nil {
		return nil
	}
	return videoCallsForEvent(*event)
}

func (m Model) hasSelectedVideoCall() bool {
	return len(m.selectedVideoCalls()) > 0
}

func (m *Model) openSelectedVideoCall() tea.Cmd {
	calls := m.selectedVideoCalls()
	if len(calls) == 0 {
		return nil
	}
	m.actionErr = ""
	return openVideoCall(calls[0].url)
}

func (m Model) addVideoCallMouseHits(lines []videoCallLine, width, height int) {
	topHeight, _ := workspacePaneHeights(max(8, m.height-4))
	for _, line := range lines {
		y := line.line - m.detailScroll
		if y < 0 || y >= height {
			continue
		}
		m.addMouseHit(mouseHit{
			rect: mouseRect{
				x:      m.mouseMainX + 2,
				y:      m.mouseMainY + topHeight + 2 + y,
				width:  width,
				height: 1,
			},
			kind:  mouseVideoCall,
			value: line.url,
		})
	}
}

func videoCallsForEvent(event calendar.Event) []videoCall {
	var calls []videoCall
	var seen map[string]struct{}
	for _, text := range []string{event.URL, event.Location, event.Description} {
		forEachHTTPSURL(text, func(rawURL string) {
			provider, ok := videoCallProvider(rawURL)
			if !ok {
				return
			}
			if seen == nil {
				seen = make(map[string]struct{})
			}
			if _, ok := seen[rawURL]; ok {
				return
			}
			seen[rawURL] = struct{}{}
			calls = append(calls, videoCall{provider: provider, url: rawURL})
		})
	}
	return calls
}

func forEachHTTPSURL(text string, visit func(string)) {
	const prefix = "https://"
	for start := 0; start+len(prefix) <= len(text); {
		if !strings.EqualFold(text[start:start+len(prefix)], prefix) {
			start++
			continue
		}
		end := start + len(prefix)
		for end < len(text) && !urlBoundary(text[end]) {
			end++
		}
		if rawURL := trimURLPunctuation(text[start:end]); len(rawURL) > len(prefix) {
			visit(rawURL)
		}
		start = end
	}
}

func urlBoundary(char byte) bool {
	return char <= ' ' || char == '<' || char == '>' || char == '"' || char == '\''
}

func trimURLPunctuation(rawURL string) string {
	for len(rawURL) > 0 {
		switch rawURL[len(rawURL)-1] {
		case '.', ',', ';', ':', '!', '?':
			rawURL = rawURL[:len(rawURL)-1]
		case ')':
			if strings.Count(rawURL, ")") <= strings.Count(rawURL, "(") {
				return rawURL
			}
			rawURL = rawURL[:len(rawURL)-1]
		case ']':
			if strings.Count(rawURL, "]") <= strings.Count(rawURL, "[") {
				return rawURL
			}
			rawURL = rawURL[:len(rawURL)-1]
		case '}':
			if strings.Count(rawURL, "}") <= strings.Count(rawURL, "{") {
				return rawURL
			}
			rawURL = rawURL[:len(rawURL)-1]
		default:
			return rawURL
		}
	}
	return rawURL
}

func videoCallProvider(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.Port() != "" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	path := parsed.EscapedPath()
	switch {
	case host == "meet.google.com" && googleMeetPath(path):
		return "Google Meet", true
	case host == "teams.microsoft.com" && strings.HasPrefix(path, "/l/meetup-join/") && len(path) > len("/l/meetup-join/"):
		return "Teams", true
	case zoomHost(host) && zoomMeetingPath(path):
		return "Zoom", true
	default:
		return "", false
	}
}

func googleMeetPath(path string) bool {
	if len(path) != 13 || path[0] != '/' || path[4] != '-' || path[9] != '-' {
		return false
	}
	for i := 1; i < len(path); i++ {
		if i == 4 || i == 9 {
			continue
		}
		char := path[i]
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
	}
	return true
}

func zoomHost(host string) bool {
	return host == "zoom.us" || strings.HasSuffix(host, ".zoom.us")
}

func zoomMeetingPath(path string) bool {
	meetingID := strings.TrimPrefix(path, "/j/")
	if meetingID == path {
		return false
	}
	meetingID = strings.TrimSuffix(meetingID, "/")
	if meetingID == "" {
		return false
	}
	for i := range meetingID {
		if meetingID[i] < '0' || meetingID[i] > '9' {
			return false
		}
	}
	return true
}

func openVideoCall(rawURL string) tea.Cmd {
	return func() tea.Msg {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", rawURL)
		case "linux":
			command = exec.Command("xdg-open", rawURL)
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
		default:
			return videoCallOpenedMsg{err: fmt.Errorf("open video call: unsupported platform %s", runtime.GOOS)}
		}
		if err := command.Run(); err != nil {
			return videoCallOpenedMsg{err: fmt.Errorf("open video call: %w", err)}
		}
		return videoCallOpenedMsg{}
	}
}
