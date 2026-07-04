package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Source struct {
	Name          string           `json:"name"`
	Path          string           `json:"path"`
	Email         string           `json:"email,omitempty"`
	AddressBook   string           `json:"address_book,omitempty"`
	DefaultTZName string           `json:"default_timezone,omitempty"`
	Calendars     []CalendarConfig `json:"calendars,omitempty"`
}

type CalendarConfig struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Color       string `json:"color,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

type Config struct {
	Sources                   []Source `json:"sources"`
	DefaultView               string   `json:"default_view,omitempty"`
	WeekStartsOn              string   `json:"week_starts_on,omitempty"`
	TimeFormat                string   `json:"time_format,omitempty"`
	SidebarWidth              int      `json:"sidebar_width,omitempty"`
	RecurrenceLookbackMonths  int      `json:"recurrence_lookback_months,omitempty"`
	RecurrenceLookaheadMonths int      `json:"recurrence_lookahead_months,omitempty"`
}

func DefaultPath() string {
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
		SidebarWidth:              30,
		RecurrenceLookbackMonths:  12,
		RecurrenceLookaheadMonths: 24,
	}
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultConfig(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.DefaultView == "" {
		cfg.DefaultView = "agenda"
	}
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = "15:04"
	}
	if cfg.WeekStartsOn == "" {
		cfg.WeekStartsOn = "monday"
	}
	if cfg.SidebarWidth <= 0 {
		cfg.SidebarWidth = 30
	}
	if cfg.RecurrenceLookbackMonths <= 0 {
		cfg.RecurrenceLookbackMonths = 12
	}
	if cfg.RecurrenceLookaheadMonths <= 0 {
		cfg.RecurrenceLookaheadMonths = 24
	}
	if cfg.RecurrenceLookbackMonths > 120 {
		cfg.RecurrenceLookbackMonths = 120
	}
	if cfg.RecurrenceLookaheadMonths > 120 {
		cfg.RecurrenceLookaheadMonths = 120
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) WeekStart() time.Weekday {
	if c != nil && c.WeekStartsOn == "sunday" {
		return time.Sunday
	}
	return time.Monday
}

func (c *Config) SourceByName(name string) *Source {
	if c == nil {
		return nil
	}
	for i := range c.Sources {
		if c.Sources[i].Name == name {
			return &c.Sources[i]
		}
	}
	return nil
}

func (s *Source) UpsertCalendar(cal CalendarConfig) {
	if s == nil || cal.Name == "" {
		return
	}
	for i := range s.Calendars {
		if s.Calendars[i].Name != cal.Name {
			continue
		}
		if cal.Path != "" {
			s.Calendars[i].Path = cal.Path
		}
		if cal.Email != "" {
			s.Calendars[i].Email = cal.Email
		}
		if cal.DisplayName != "" {
			s.Calendars[i].DisplayName = cal.DisplayName
		}
		if cal.Color != "" {
			s.Calendars[i].Color = cal.Color
		}
		s.Calendars[i].Hidden = cal.Hidden
		return
	}
	s.Calendars = append(s.Calendars, cal)
}
