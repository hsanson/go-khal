package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
