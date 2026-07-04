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
	Path        string `json:"path"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name,omitempty"`
	Color       string `json:"color,omitempty"`
	Email       string `json:"email,omitempty"`
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

func Default() *Config {
	return defaultConfig()
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
	for i := range cfg.Sources {
		src := &cfg.Sources[i]
		src.Path = strings.TrimSpace(src.Path)
		if src.Path == "" {
			return nil, fmt.Errorf("source %d path is required", i+1)
		}
		if !filepath.IsAbs(src.Path) {
			return nil, fmt.Errorf("source %d path must be absolute: %s", i+1, src.Path)
		}
		src.Path = filepath.Clean(src.Path)
		src.Type = strings.ToLower(strings.TrimSpace(src.Type))
		if src.Type != "calendar" && src.Type != "addressbook" {
			return nil, fmt.Errorf("source %d type must be calendar or addressbook", i+1)
		}
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
		if c.Sources[i].Path == name || filepath.Base(c.Sources[i].Path) == name {
			return &c.Sources[i]
		}
	}
	return nil
}
