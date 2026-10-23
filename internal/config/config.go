package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Source struct {
	Name        string   `json:"-"`
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	DisplayName string   `json:"display_name,omitempty"`
	Color       string   `json:"color,omitempty"`
	Email       string   `json:"email,omitempty"`
	Addresses   []string `json:"addresses,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
	ReadOnly    bool     `json:"readonly,omitempty"`
	Discovered  bool     `json:"-"`
}

type Config struct {
	Sources                   []Source `json:"sources"`
	DefaultView               string   `json:"default_view,omitempty"`
	WeekStartsOn              string   `json:"week_starts_on,omitempty"`
	TimeFormat                string   `json:"time_format,omitempty"`
	DateFormat                string   `json:"-"`
	LongDateFormat            string   `json:"-"`
	DateTimeFormat            string   `json:"-"`
	LongDateTimeFormat        string   `json:"-"`
	SidebarWidth              int      `json:"sidebar_width,omitempty"`
	MinimapStartTime          string   `json:"minimap_start_time,omitempty"`
	MinimapEndTime            string   `json:"minimap_end_time,omitempty"`
	RecurrenceLookbackMonths  int      `json:"recurrence_lookback_months,omitempty"`
	RecurrenceLookaheadMonths int      `json:"recurrence_lookahead_months,omitempty"`

	hiddenCalendarPaths []string
	document            *khalDocument
	original            *Config
}

func DefaultPath() string {
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join("khal", "config")
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "khal", "config")
}

func LegacyJSONPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".go-khal.json"
	}
	return filepath.Join(home, ".config", "go-khal", "config.json")
}

func defaultConfig() *Config {
	return &Config{
		DefaultView:               "agenda",
		WeekStartsOn:              "monday",
		TimeFormat:                "15:04",
		DateFormat:                "2006-01-02",
		LongDateFormat:            "2006-01-02",
		DateTimeFormat:            "2006-01-02 15:04",
		LongDateTimeFormat:        "2006-01-02 15:04",
		SidebarWidth:              30,
		MinimapStartTime:          "08:00",
		MinimapEndTime:            "18:00",
		RecurrenceLookbackMonths:  12,
		RecurrenceLookaheadMonths: 24,
	}
}

func Default() *Config {
	return defaultConfig()
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && filepath.Clean(path) == filepath.Clean(DefaultPath()) {
		legacyPath := LegacyJSONPath()
		b, err = os.ReadFile(legacyPath)
		if err == nil {
			cfg, loadErr := loadJSON(b)
			if loadErr != nil {
				return nil, fmt.Errorf("parse deprecated config %s: %w", legacyPath, loadErr)
			}
			return cfg, nil
		}
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf(
			"no configuration found; add a calendar with `go-khal config add-source` or import vdirsyncer calendars with `go-khal config from-vdirsyncer`: %w",
			os.ErrNotExist,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if strings.HasSuffix(strings.ToLower(path), ".json") || strings.HasPrefix(strings.TrimSpace(string(b)), "{") {
		cfg, err := loadJSON(b)
		if err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		return cfg, nil
	}
	cfg, err := loadKhalConfig(b)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func LoadOrDefault(path string) (*Config, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return defaultConfig(), nil
}

func loadJSON(b []byte) (*Config, error) {
	cfg := defaultConfig()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	if err := normalize(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func normalize(cfg *Config) error {
	defaults := defaultConfig()
	if cfg.DefaultView == "" {
		cfg.DefaultView = defaults.DefaultView
	}
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = defaults.TimeFormat
	}
	if cfg.DateFormat == "" {
		cfg.DateFormat = defaults.DateFormat
	}
	if cfg.LongDateFormat == "" {
		cfg.LongDateFormat = defaults.LongDateFormat
	}
	if cfg.DateTimeFormat == "" {
		cfg.DateTimeFormat = defaults.DateTimeFormat
	}
	if cfg.LongDateTimeFormat == "" {
		cfg.LongDateTimeFormat = defaults.LongDateTimeFormat
	}
	if cfg.WeekStartsOn == "" {
		cfg.WeekStartsOn = defaults.WeekStartsOn
	}
	if cfg.SidebarWidth <= 0 {
		cfg.SidebarWidth = defaults.SidebarWidth
	}
	if cfg.MinimapStartTime == "" {
		cfg.MinimapStartTime = defaults.MinimapStartTime
	}
	if cfg.MinimapEndTime == "" {
		cfg.MinimapEndTime = defaults.MinimapEndTime
	}
	startHour, err := minimapHour(cfg.MinimapStartTime)
	if err != nil {
		return fmt.Errorf("minimap_start_time: %w", err)
	}
	endHour, err := minimapHour(cfg.MinimapEndTime)
	if err != nil {
		return fmt.Errorf("minimap_end_time: %w", err)
	}
	if endHour < startHour {
		return errors.New("minimap_end_time must not be before minimap_start_time")
	}
	if cfg.RecurrenceLookbackMonths <= 0 {
		cfg.RecurrenceLookbackMonths = defaults.RecurrenceLookbackMonths
	}
	if cfg.RecurrenceLookaheadMonths <= 0 {
		cfg.RecurrenceLookaheadMonths = defaults.RecurrenceLookaheadMonths
	}
	if cfg.RecurrenceLookbackMonths > 120 {
		cfg.RecurrenceLookbackMonths = 120
	}
	if cfg.RecurrenceLookaheadMonths > 120 {
		cfg.RecurrenceLookaheadMonths = 120
	}
	for i := range cfg.Sources {
		src := &cfg.Sources[i]
		src.Path = strings.TrimSpace(src.Path)
		if src.Path == "" {
			return fmt.Errorf("source %d path is required", i+1)
		}
		if !filepath.IsAbs(src.Path) {
			return fmt.Errorf("source %d path must be absolute: %s", i+1, src.Path)
		}
		src.Path = filepath.Clean(src.Path)
		src.Type = strings.ToLower(strings.TrimSpace(src.Type))
		if src.Type != "calendar" && src.Type != "addressbook" {
			return fmt.Errorf("source %d type must be calendar or addressbook", i+1)
		}
		if len(src.Addresses) == 0 && strings.TrimSpace(src.Email) != "" {
			src.Addresses = []string{strings.TrimSpace(src.Email)}
		}
		if src.Email == "" && len(src.Addresses) > 0 {
			src.Email = src.Addresses[0]
		}
	}
	return nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		b, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("encode config: %w", err)
		}
		return writeFileAtomic(path, append(b, '\n'))
	}
	b, err := encodeKhalConfig(cfg)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, b)
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".go-khal-config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o644); err != nil {
		file.Close()
		return fmt.Errorf("set config permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func (c *Config) WeekStart() time.Weekday {
	if c == nil {
		return time.Monday
	}
	switch strings.ToLower(strings.TrimSpace(c.WeekStartsOn)) {
	case "sunday":
		return time.Sunday
	case "tuesday":
		return time.Tuesday
	case "wednesday":
		return time.Wednesday
	case "thursday":
		return time.Thursday
	case "friday":
		return time.Friday
	case "saturday":
		return time.Saturday
	default:
		return time.Monday
	}
}

func (c *Config) MinimapHours() (int, int) {
	if c == nil {
		return 8, 18
	}
	start, startErr := minimapHour(c.MinimapStartTime)
	end, endErr := minimapHour(c.MinimapEndTime)
	if startErr != nil || endErr != nil || end < start {
		return 8, 18
	}
	return start, end
}

func minimapHour(value string) (int, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil || parsed.Minute() != 0 {
		return 0, errors.New("must be a whole hour in HH:00 format")
	}
	return parsed.Hour(), nil
}

func (c *Config) SourceByName(name string) *Source {
	if c == nil {
		return nil
	}
	for i := range c.Sources {
		if c.Sources[i].Path == name || filepath.Base(c.Sources[i].Path) == name {
			return &c.Sources[i]
		}
	}
	return nil
}
