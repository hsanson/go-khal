package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadValidatesSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"sources":[{"path":"relative","type":"calendar"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "path must be absolute") {
		t.Fatalf("Load() error = %v, want absolute path error", err)
	}
}

func TestLoadNormalizesSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"sources":[{"path":"/tmp/calendar/../calendar","type":"Calendar"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Sources[0].Path, "/tmp/calendar"; got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].Type, "calendar"; got != want {
		t.Fatalf("source type = %q, want %q", got, want)
	}
}

func TestMinimapHourDefaultsAndValidation(t *testing.T) {
	start, end := Default().MinimapHours()
	if start != 8 || end != 18 {
		t.Fatalf("default minimap hours = %02d:00-%02d:00, want 08:00-18:00", start, end)
	}

	for name, body := range map[string]string{
		"partial hour": `{"minimap_start_time":"08:30"}`,
		"reversed":     `{"minimap_start_time":"18:00","minimap_end_time":"08:00"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load() accepted an invalid minimap hour range")
			}
		})
	}
}

func TestKhalConfigLoadsEssentialFieldsAndPreservesUnknownContent(t *testing.T) {
	root := t.TempDir()
	calendarPath := filepath.Join(root, "personal")
	if err := os.Mkdir(calendarPath, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config")
	body := `[calendars]
[[personal]]
path = ` + calendarPath + `
color = '#123456'
readonly = true
addresses = first@example.test, second@example.test
future_key = keep-me

[locale]
firstweekday = 2
timeformat = %I:%M %p
dateformat = %Y-%m-%d
longdateformat = %Y-%m-%d
datetimeformat = %Y-%m-%d %H:%M
longdatetimeformat = %Y-%m-%d %H:%M

# this block must survive go-khal writes
[default]
default_calendar = personal
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(cfg.Sources))
	}
	source := cfg.Sources[0]
	if !source.ReadOnly || len(source.Addresses) != 2 || source.Email != "first@example.test" {
		t.Fatalf("source fields were not mapped: %+v", source)
	}
	if cfg.WeekStart() != time.Wednesday || cfg.TimeFormat != "03:04 PM" {
		t.Fatalf("locale mapping = %s, %q", cfg.WeekStart(), cfg.TimeFormat)
	}

	cfg.Sources[0].Hidden = true
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(saved)
	for _, unchanged := range []string{
		"future_key = keep-me",
		"# this block must survive go-khal writes",
		"[default]\ndefault_calendar = personal",
	} {
		if !strings.Contains(text, unchanged) {
			t.Fatalf("saved config lost %q:\n%s", unchanged, text)
		}
	}
	if !strings.Contains(text, "[go-khal]") || !strings.Contains(text, "hidden_calendar_paths = "+calendarPath) {
		t.Fatalf("saved config did not persist hidden path:\n%s", text)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Sources[0].Hidden {
		t.Fatal("hidden calendar path did not round-trip")
	}
}

func TestDefaultKhalConfigFallsBackToJSONAndMigratesOnlyOnSave(t *testing.T) {
	home := t.TempDir()
	configHome := filepath.Join(home, "xdg")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	calendarPath := filepath.Join(home, "calendar")
	legacyPath := LegacyJSONPath()
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"sources":[{"path":"` + calendarPath + `","type":"calendar","display_name":"Personal"}]}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(DefaultPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read fallback created canonical config: %v", err)
	}
	if err := Save(DefaultPath(), cfg); err != nil {
		t.Fatal(err)
	}
	canonical, err := os.ReadFile(DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(canonical), "[[Personal]]") {
		t.Fatalf("migration did not create khal calendar:\n%s", canonical)
	}
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacyAfter) != legacy {
		t.Fatal("migration modified deprecated JSON")
	}

	if err := os.Remove(DefaultPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(legacyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(DefaultPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing config error = %v, want os.ErrNotExist", err)
	}
}

func TestKhalConfigDiscoversCalendarsAndBirthdays(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "calendars", "cal-personal"),
		filepath.Join(root, "calendars", "group", "cal-work"),
		filepath.Join(root, "contacts"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "config")
	body := `[calendars]
[[all]]
path = ` + filepath.Join(root, "calendars", "**", "cal-*") + `
type = discover
readonly = true
addresses = ignored-for-discover@example.test

[[birthdays]]
path = ` + filepath.Join(root, "contacts") + `
type = birthdays

[unused]
future = value
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 3 {
		t.Fatalf("sources = %d, want two discovered calendars and one birthday source: %+v", len(cfg.Sources), cfg.Sources)
	}
	var calendars, birthdays int
	for _, source := range cfg.Sources {
		switch source.Type {
		case "calendar":
			calendars++
			if !source.Discovered || !source.ReadOnly || len(source.Addresses) != 0 {
				t.Fatalf("discovered source fields = %+v", source)
			}
		case "addressbook":
			birthdays++
			if !source.ReadOnly {
				t.Fatalf("birthday source is writable: %+v", source)
			}
		}
	}
	if calendars != 2 || birthdays != 1 {
		t.Fatalf("calendar sources = %d, birthday sources = %d", calendars, birthdays)
	}
}
