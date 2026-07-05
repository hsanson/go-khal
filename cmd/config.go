package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage go-khal configuration",
	}
	configCmd.AddCommand(newConfigInitCommand())
	configCmd.AddCommand(newConfigAddSourceCommand())
	configCmd.AddCommand(newConfigFromVdirsyncerCommand())
	configCmd.AddCommand(newConfigListSourcesCommand())
	configCmd.AddCommand(newConfigListCalendarsCommand())
	configCmd.AddCommand(newConfigHideCalendarCommand())
	configCmd.AddCommand(newConfigShowCalendarCommand())
	return configCmd
}

func newConfigInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create config file if missing",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Printf("initialized config at %s\n", cfgPath)
			return nil
		},
	}
}

func newConfigAddSourceCommand() *cobra.Command {
	var sourceType string
	var path string
	var displayName string
	var color string
	var email string

	cmd := &cobra.Command{
		Use:   "add-source",
		Short: "Add one calendar or addressbook folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path is required")
			}
			absPath, err := absolutePath(path)
			if err != nil {
				return err
			}
			sourceType = strings.ToLower(strings.TrimSpace(sourceType))
			if sourceType != "calendar" && sourceType != "addressbook" {
				return fmt.Errorf("--type must be calendar or addressbook")
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			upsertSource(cfg, config.Source{
				Path:        absPath,
				Type:        sourceType,
				DisplayName: displayName,
				Color:       color,
				Email:       email,
			})
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Println("source added")
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "absolute path to calendar/addressbook folder")
	cmd.Flags().StringVar(&sourceType, "type", "calendar", "source type: calendar or addressbook")
	cmd.Flags().StringVar(&displayName, "display-name", "", "override display name")
	cmd.Flags().StringVar(&color, "color", "", "override color")
	cmd.Flags().StringVar(&email, "email", "", "organizer email for calendar sources")
	return cmd
}

func newConfigFromVdirsyncerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "from-vdirsyncer [path]",
		Short: "Generate go-khal config from vdirsyncer local storages",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := defaultVdirsyncerConfigPath()
			if len(args) == 1 {
				path = args[0]
			}
			path, err := expandPath(path)
			if err != nil {
				return err
			}
			sources, err := sourcesFromVdirsyncer(path)
			if err != nil {
				return err
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				cfg = config.Default()
			}
			cfg.Sources = sources
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Printf("wrote %d sources to %s\n", len(sources), cfgPath)
			return nil
		},
	}
	return cmd
}

func newConfigListSourcesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-sources",
		Short: "List configured sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			if len(cfg.Sources) == 0 {
				fmt.Println("no sources configured")
				return nil
			}
			for _, src := range cfg.Sources {
				fmt.Printf("- %s\n  type: %s\n  path: %s\n", filepath.Base(src.Path), src.Type, src.Path)
				if src.DisplayName != "" {
					fmt.Printf("  display: %s\n", src.DisplayName)
				}
				if src.Color != "" {
					fmt.Printf("  color: %s\n", src.Color)
				}
				if src.Email != "" {
					fmt.Printf("  email: %s\n", src.Email)
				}
				if src.Hidden {
					fmt.Println("  hidden: true")
				}
			}
			return nil
		},
	}
}

func newConfigListCalendarsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-calendars",
		Short: "List configured calendars with visibility/color",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			store := calendar.NewStore(cfg)
			ds, err := store.Load()
			if err != nil {
				return err
			}
			if len(ds.Calendars) == 0 {
				fmt.Println("no calendars found")
				return nil
			}
			for _, cal := range ds.Calendars {
				visibility := "shown"
				if cal.Hidden {
					visibility = "hidden"
				}
				fmt.Printf("- %s\n", cal.Path)
				fmt.Printf("  display: %s\n", cal.DisplayName)
				if cal.Color != "" {
					fmt.Printf("  color: %s\n", cal.Color)
				}
				if cal.Email != "" {
					fmt.Printf("  email: %s\n", cal.Email)
				}
				fmt.Printf("  state: %s\n", visibility)
			}
			return nil
		},
	}
}

func newConfigHideCalendarCommand() *cobra.Command {
	return newConfigSetCalendarVisibilityCommand("hide")
}

func newConfigShowCalendarCommand() *cobra.Command {
	return newConfigSetCalendarVisibilityCommand("show")
}

func newConfigSetCalendarVisibilityCommand(mode string) *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   mode + "-calendar",
		Short: mode + " all events for a calendar",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("--path is required")
			}
			absPath, err := absolutePath(path)
			if err != nil {
				return err
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			store := calendar.NewStore(cfg)
			hidden := mode == "hide"
			if err := store.SetCalendarHidden(absPath, "", hidden); err != nil {
				return err
			}
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Printf("calendar %s is now %s\n", absPath, map[bool]string{true: "hidden", false: "shown"}[hidden])
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "calendar folder path")
	return cmd
}

func upsertSource(cfg *config.Config, src config.Source) {
	for i := range cfg.Sources {
		if filepath.Clean(cfg.Sources[i].Path) == filepath.Clean(src.Path) {
			cfg.Sources[i] = src
			return
		}
	}
	cfg.Sources = append(cfg.Sources, src)
}

func defaultVdirsyncerConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/vdirsyncer/config"
	}
	return filepath.Join(home, ".config", "vdirsyncer", "config")
}

type vdirsyncerStorage struct {
	Name    string
	Type    string
	Path    string
	FileExt string
}

func sourcesFromVdirsyncer(path string) ([]config.Source, error) {
	storages, err := parseVdirsyncerStorages(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []config.Source
	for _, storage := range storages {
		if strings.ToLower(storage.Type) != "filesystem" || storage.Path == "" {
			continue
		}
		root, err := expandPath(storage.Path)
		if err != nil {
			continue
		}
		sourceType := sourceTypeFromFileExt(storage.FileExt)
		if sourceType == "" {
			sourceType = detectSourceType(root)
		}
		if sourceType == "" {
			continue
		}
		for _, folder := range concreteVdirFolders(root, sourceType) {
			if seen[folder] {
				continue
			}
			seen[folder] = true
			metaDisplay, metaColor := readVdirMeta(folder)
			out = append(out, config.Source{
				Path:        folder,
				Type:        sourceType,
				DisplayName: metaDisplay,
				Color:       metaColor,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].Path < out[j].Path
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

func parseVdirsyncerStorages(path string) ([]vdirsyncerStorage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open vdirsyncer config: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []vdirsyncerStorage
	var current *vdirsyncerStorage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil {
				out = append(out, *current)
			}
			section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			current = nil
			if strings.HasPrefix(section, "storage ") {
				current = &vdirsyncerStorage{Name: strings.Trim(strings.TrimSpace(strings.TrimPrefix(section, "storage ")), `"'`)}
			}
			continue
		}
		if current == nil {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = parseVdirValue(value)
		switch key {
		case "type":
			current.Type = value
		case "path":
			current.Path = value
		case "fileext":
			current.FileExt = value
		}
	}
	if current != nil {
		out = append(out, *current)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseVdirValue(raw string) string {
	value := strings.TrimSpace(raw)
	if idx := strings.Index(value, "#"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func sourceTypeFromFileExt(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".ics":
		return "calendar"
	case ".vcf":
		return "addressbook"
	default:
		return ""
	}
}

func detectSourceType(path string) string {
	if folderHasExt(path, ".ics") {
		return "calendar"
	}
	if folderHasExt(path, ".vcf") {
		return "addressbook"
	}
	return ""
}

func concreteVdirFolders(root, sourceType string) []string {
	ext := ".ics"
	if sourceType == "addressbook" {
		ext = ".vcf"
	}
	if folderHasExt(root, ext) {
		return []string{root}
	}
	if hasVdirMeta(root) {
		return []string{root}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if folderHasExt(path, ext) || hasVdirMeta(path) {
			out = append(out, path)
		}
	}
	return out
}

func folderHasExt(path, ext string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			return true
		}
	}
	return false
}

func hasVdirMeta(path string) bool {
	for _, name := range []string{"displayname", ".displayname", "color", ".color"} {
		if _, err := os.Stat(filepath.Join(path, name)); err == nil {
			return true
		}
	}
	return false
}

func readVdirMeta(path string) (string, string) {
	displayName := readTrimmed(filepath.Join(path, "displayname"))
	if displayName == "" {
		displayName = readTrimmed(filepath.Join(path, ".displayname"))
	}
	color := readTrimmed(filepath.Join(path, "color"))
	if color == "" {
		color = readTrimmed(filepath.Join(path, ".color"))
	}
	return displayName, color
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func absolutePath(path string) (string, error) {
	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("path must be absolute: %s", path)
	}
	return filepath.Clean(expanded), nil
}

func expandPath(path string) (string, error) {
	path, err := expandHome(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func expandHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path, nil
}
