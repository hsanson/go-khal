package tui

import (
	"testing"

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
