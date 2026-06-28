package cmd

import (
	"fmt"

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
	configCmd.AddCommand(newConfigAddCalendarCommand())
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
	var name string
	var path string
	var abook string
	var tz string
	var asAccount bool

	cmd := &cobra.Command{
		Use:   "add-source",
		Short: "Add vdir source folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" || path == "" {
				return fmt.Errorf("--name and --path are required")
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			cfg.Sources = append(cfg.Sources, config.Source{
				Name:          name,
				Path:          path,
				AddressBook:   abook,
				DefaultTZName: tz,
			})
			if asAccount {
				store := calendar.NewStore(cfg)
				ds, err := store.Load()
				if err == nil {
					src := cfg.SourceByName(name)
					for _, cal := range ds.Calendars {
						if cal.Source != name {
							continue
						}
						src.UpsertCalendar(config.CalendarConfig{
							Name:        cal.Name,
							Path:        cal.Path,
							DisplayName: cal.DisplayName,
							Color:       cal.Color,
							Hidden:      cal.Hidden,
						})
					}
				}
			}
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Println("source added")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "source name")
	cmd.Flags().StringVar(&path, "path", "", "path to calendar vdir")
	cmd.Flags().StringVar(&abook, "address-book", "", "path to address-book vdir")
	cmd.Flags().StringVar(&tz, "timezone", "", "default timezone")
	cmd.Flags().BoolVar(&asAccount, "account", true, "treat path as parent folder containing calendar subfolders")
	return cmd
}

func newConfigAddCalendarCommand() *cobra.Command {
	var sourceName string
	var name string
	var path string
	var displayName string
	var color string

	cmd := &cobra.Command{
		Use:   "add-calendar",
		Short: "Add or update a calendar inside a source",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sourceName == "" || name == "" {
				return fmt.Errorf("--source and --name are required")
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			src := cfg.SourceByName(sourceName)
			if src == nil {
				return fmt.Errorf("source %q not found", sourceName)
			}
			src.UpsertCalendar(config.CalendarConfig{
				Name:        name,
				Path:        path,
				DisplayName: displayName,
				Color:       color,
			})
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Println("calendar configured")
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "source name")
	cmd.Flags().StringVar(&name, "name", "", "calendar name")
	cmd.Flags().StringVar(&path, "path", "", "calendar folder path (optional, defaults to source path + name)")
	cmd.Flags().StringVar(&displayName, "display-name", "", "calendar display name")
	cmd.Flags().StringVar(&color, "color", "", "calendar color (hex like #4CAF50)")
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
				fmt.Printf("- %s\n  path: %s\n", src.Name, src.Path)
				if src.AddressBook != "" {
					fmt.Printf("  address_book: %s\n", src.AddressBook)
				}
				if src.DefaultTZName != "" {
					fmt.Printf("  timezone: %s\n", src.DefaultTZName)
				}
				if len(src.Calendars) > 0 {
					fmt.Printf("  calendars: %d configured\n", len(src.Calendars))
				}
			}
			return nil
		},
	}
}

func newConfigListCalendarsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-calendars",
		Short: "List discovered calendars with visibility/color",
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
				fmt.Printf("- %s/%s\n", cal.Source, cal.Name)
				fmt.Printf("  display: %s\n", cal.DisplayName)
				if cal.Color != "" {
					fmt.Printf("  color: %s\n", cal.Color)
				}
				fmt.Printf("  state: %s\n", visibility)
				fmt.Printf("  path: %s\n", cal.Path)
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
	var sourceName string
	var calendarName string

	cmd := &cobra.Command{
		Use:   mode + "-calendar",
		Short: mode + " all events for a calendar",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sourceName == "" || calendarName == "" {
				return fmt.Errorf("--source and --name are required")
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			store := calendar.NewStore(cfg)
			hidden := mode == "hide"
			if err := store.SetCalendarHidden(sourceName, calendarName, hidden); err != nil {
				return err
			}
			if err := config.Save(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Printf("calendar %s/%s is now %s\n", sourceName, calendarName, map[bool]string{true: "hidden", false: "shown"}[hidden])
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceName, "source", "", "source name")
	cmd.Flags().StringVar(&calendarName, "name", "", "calendar name")
	return cmd
}
