package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hsanson/go-khal/internal/calendar"
)

func TestVideoCallsForEvent(t *testing.T) {
	zoom := "https://acme.zoom.us/j/123456789?pwd=secret"
	teams := "https://teams.microsoft.com/l/meetup-join/19%3ameeting_example/0?context=abc"
	meet := "https://meet.google.com/abc-defg-hij"
	event := calendar.Event{
		URL:      zoom,
		Location: "Join " + teams,
		Description: `<a href="` + meet + `">Meet</a>. Duplicate: ` + zoom + `. ` +
			`Ignore https://meet.google.com.evil.test/abc-defg-hij`,
	}

	got := videoCallsForEvent(event)
	want := []videoCall{
		{provider: "Zoom", url: zoom},
		{provider: "Teams", url: teams},
		{provider: "Google Meet", url: meet},
	}
	if len(got) != len(want) {
		t.Fatalf("video calls = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("video call %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestVideoCallProviderRejectsNonCanonicalLinks(t *testing.T) {
	for _, rawURL := range []string{
		"http://meet.google.com/abc-defg-hij",
		"https://meet.google.com/not-a-code",
		"https://meet.google.com.evil.test/abc-defg-hij",
		"https://teams.live.com/meet/123",
		"https://teams.microsoft.com/l/meetup-join/",
		"https://zoom.us/wc/join/123456789",
		"https://evilzoom.us/j/123456789",
		"https://zoom.us:8443/j/123456789",
	} {
		if provider, ok := videoCallProvider(rawURL); ok {
			t.Errorf("videoCallProvider(%q) = %q, want unsupported", rawURL, provider)
		}
	}
}

func TestFirstWebURL(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "first URL in prose",
			text: "Map: https://maps.example.test/place?id=1. Backup http://maps.example.test/other",
			want: "https://maps.example.test/place?id=1",
		},
		{
			name: "HTTP with port",
			text: "http://localhost:8080/map",
			want: "http://localhost:8080/map",
		},
		{
			name: "skip credentials",
			text: "https://user:pass@maps.example.test/private https://maps.example.test/public",
			want: "https://maps.example.test/public",
		},
		{
			name: "skip missing host",
			text: "https:///missing https://maps.example.test/valid",
			want: "https://maps.example.test/valid",
		},
		{
			name: "no web URL",
			text: "Room 4, Building A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstWebURL(tt.text); got != tt.want {
				t.Fatalf("firstWebURL(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestLocationControlsUseFirstURLAndResponsiveRows(t *testing.T) {
	const location = "Primary https://maps.example.test/one. Backup http://maps.example.test/two"
	tests := []struct {
		name     string
		width    int
		wantRows int
		wantHitY [2]int
		wantX    int
	}{
		{name: "aligned row", width: 100, wantRows: 2, wantHitY: [2]int{8, 8}, wantX: 18},
		{name: "aligned stack", width: 30, wantRows: 3, wantHitY: [2]int{8, 9}, wantX: 18},
		{name: "row-start stack", width: 20, wantRows: 3, wantHitY: [2]int{8, 9}, wantX: 0},
	}

	m := Model{styles: DefaultStyles()}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, hits := m.locationDetailLines(location, tt.width, 7)
			if len(lines) != tt.wantRows || len(hits) != 2 {
				t.Fatalf("rendered %d rows and %d controls, want %d rows and 2 controls", len(lines), len(hits), tt.wantRows)
			}
			if strings.Contains(lines[0], "\n") || lipgloss.Width(lines[0]) > tt.width {
				t.Fatalf("Location display is not one truncated line at width %d: %q", tt.width, lines[0])
			}
			if hits[0].kind != mouseLocationOpen || hits[1].kind != mouseLocationCopy {
				t.Fatalf("control kinds = %v, %v", hits[0].kind, hits[1].kind)
			}
			if hits[0].line != tt.wantHitY[0] || hits[1].line != tt.wantHitY[1] {
				t.Fatalf("control rows = %d, %d; want %d, %d", hits[0].line, hits[1].line, tt.wantHitY[0], tt.wantHitY[1])
			}
			if hits[0].x != tt.wantX || hits[1].x < tt.wantX {
				t.Fatalf("control columns = %d, %d; want first at %d", hits[0].x, hits[1].x, tt.wantX)
			}
			if hits[0].value != "https://maps.example.test/one" || hits[1].value != location {
				t.Fatalf("control values = %q, %q", hits[0].value, hits[1].value)
			}
		})
	}
}

func TestURLControlsUseFirstURLAndCopyWholeField(t *testing.T) {
	const value = "Primary https://example.test/one. Backup http://example.test/two"
	m := Model{styles: DefaultStyles()}

	lines, hits := m.urlDetailLines(value, 100, 7)
	rendered := strings.Join(lines, "\n")
	if len(lines) != 2 || !strings.Contains(rendered, "") || !strings.Contains(rendered, " Open") {
		t.Fatalf("unexpected URL section:\n%s", rendered)
	}
	if len(hits) != 2 || hits[0].kind != mouseEventURLOpen || hits[1].kind != mouseEventURLCopy {
		t.Fatalf("unexpected URL controls: %#v", hits)
	}
	if hits[0].value != "https://example.test/one" || hits[1].value != value {
		t.Fatalf("URL control values = %q, %q", hits[0].value, hits[1].value)
	}
}

func TestGoogleMeetControlsUseResponsiveRows(t *testing.T) {
	const rawURL = "https://meet.google.com/abc-defg-hij?authuser=1"
	tests := []struct {
		name     string
		width    int
		wantRows int
		wantHitY [2]int
	}{
		{name: "inline", width: 100, wantRows: 1, wantHitY: [2]int{7, 7}},
		{name: "controls below provider", width: 20, wantRows: 2, wantHitY: [2]int{8, 8}},
		{name: "stacked controls", width: 10, wantRows: 3, wantHitY: [2]int{8, 9}},
	}

	m := Model{styles: DefaultStyles()}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, hits := m.videoCallDetailLines(videoCall{provider: googleMeetProvider, url: rawURL}, tt.width, 7)
			if len(lines) != tt.wantRows {
				t.Fatalf("rendered %d rows, want %d", len(lines), tt.wantRows)
			}
			if len(hits) != 2 {
				t.Fatalf("rendered %d controls, want 2", len(hits))
			}
			if hits[0].kind != mouseVideoCall || hits[1].kind != mouseVideoCallCopy {
				t.Fatalf("control kinds = %v, %v", hits[0].kind, hits[1].kind)
			}
			if hits[0].line != tt.wantHitY[0] || hits[1].line != tt.wantHitY[1] {
				t.Fatalf("control rows = %d, %d; want %d, %d", hits[0].line, hits[1].line, tt.wantHitY[0], tt.wantHitY[1])
			}
			if hits[0].value != rawURL || hits[1].value != rawURL {
				t.Fatalf("controls do not retain exact URL: %#v", hits)
			}
		})
	}
}

func TestGoogleMeetFailureFeedbackRemainsClickableWhenStacked(t *testing.T) {
	const rawURL = "https://meet.google.com/abc-defg-hij"
	m := Model{
		styles:              DefaultStyles(),
		videoCallCopyURL:    rawURL,
		videoCallCopyFailed: true,
	}

	lines, hits := m.videoCallDetailLines(videoCall{provider: googleMeetProvider, url: rawURL}, 20, 7)
	if len(lines) != 3 || len(hits) != 2 {
		t.Fatalf("failure controls rendered %d rows and %d hits, want 3 rows and 2 hits", len(lines), len(hits))
	}
	if hits[0].line != 8 || hits[1].line != 9 {
		t.Fatalf("stacked failure control rows = %d, %d; want 8, 9", hits[0].line, hits[1].line)
	}
	if !strings.Contains(strings.Join(lines, "\n"), " Copy Failed") {
		t.Fatal("stacked failure control does not show Copy Failed")
	}
}
