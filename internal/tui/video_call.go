package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
)

type videoCall struct {
	provider string
	url      string
}

const (
	googleMeetProvider = "Google Meet"
	copyStatusDuration = 2 * time.Second
)

type detailActionHit struct {
	line  int
	x     int
	width int
	kind  mouseTarget
	value string
}

type urlOpenedMsg struct {
	err error
}

type videoCallCopiedMsg struct {
	id  uint64
	url string
	err error
}

type videoCallCopyStatusExpiredMsg struct {
	id uint64
}

type locationCopiedMsg struct {
	id    uint64
	value string
	err   error
}

type locationCopyStatusExpiredMsg struct {
	id uint64
}

type urlFieldCopiedMsg struct {
	id    uint64
	value string
	err   error
}

type urlFieldCopyStatusExpiredMsg struct {
	id uint64
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
	return openURL(calls[0].url, "video call")
}

func (m Model) addDetailActionMouseHits(hits []detailActionHit, width, height int) {
	topHeight, _ := workspacePaneHeights(max(8, m.height-4))
	for _, hit := range hits {
		y := hit.line - m.detailScroll
		if y < 0 || y >= height || hit.x >= width {
			continue
		}
		m.addMouseHit(mouseHit{
			rect: mouseRect{
				x:      m.mouseMainX + 2 + hit.x,
				y:      m.mouseMainY + topHeight + 2 + y,
				width:  min(hit.width, width-hit.x),
				height: 1,
			},
			kind:  hit.kind,
			value: hit.value,
		})
	}
}

func videoCallsForEvent(event calendar.Event) []videoCall {
	var calls []videoCall
	var seen map[string]struct{}
	for _, text := range []string{event.URL, event.Location, event.Description} {
		forEachWebURL(text, func(rawURL string) bool {
			provider, ok := videoCallProvider(rawURL)
			if !ok {
				return true
			}
			if seen == nil {
				seen = make(map[string]struct{})
			}
			if _, ok := seen[rawURL]; ok {
				return true
			}
			seen[rawURL] = struct{}{}
			calls = append(calls, videoCall{provider: provider, url: rawURL})
			return true
		})
	}
	return calls
}

func forEachWebURL(text string, visit func(string) bool) {
	for start := 0; start < len(text); {
		prefixLength := webURLPrefixLength(text[start:])
		if prefixLength == 0 {
			start++
			continue
		}
		end := start + prefixLength
		for end < len(text) && !urlBoundary(text[end]) {
			end++
		}
		rawURL := trimURLPunctuation(text[start:end])
		if len(rawURL) > prefixLength && !visit(rawURL) {
			return
		}
		start = end
	}
}

func webURLPrefixLength(text string) int {
	switch {
	case len(text) >= len("https://") && strings.EqualFold(text[:len("https://")], "https://"):
		return len("https://")
	case len(text) >= len("http://") && strings.EqualFold(text[:len("http://")], "http://"):
		return len("http://")
	default:
		return 0
	}
}

func firstWebURL(text string) string {
	var first string
	forEachWebURL(text, func(rawURL string) bool {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.User != nil || parsed.Hostname() == "" ||
			!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
			return true
		}
		first = rawURL
		return false
	})
	return first
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
		return googleMeetProvider, true
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

func openURL(rawURL, target string) tea.Cmd {
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
			return urlOpenedMsg{err: fmt.Errorf("open %s: unsupported platform %s", target, runtime.GOOS)}
		}
		if err := command.Run(); err != nil {
			return urlOpenedMsg{err: fmt.Errorf("open %s: %w", target, err)}
		}
		return urlOpenedMsg{}
	}
}

func copyVideoCall(rawURL string, id uint64) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(rawURL)
		return videoCallCopiedMsg{id: id, url: rawURL, err: err}
	}
}

func copyLocation(value string, id uint64) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(value)
		return locationCopiedMsg{id: id, value: value, err: err}
	}
}

func copyURLField(value string, id uint64) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(value)
		return urlFieldCopiedMsg{id: id, value: value, err: err}
	}
}
