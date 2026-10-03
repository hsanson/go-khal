package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/tui"
	"github.com/spf13/cobra"
)

func newImportCommand() *cobra.Command {
	var calendarName string
	cmd := &cobra.Command{
		Use:   "import [filename]",
		Short: "Import events from an iCalendar attachment",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				input = cmd.InOrStdin()
				file  *os.File
				err   error
			)
			if len(args) == 1 {
				file, err = os.Open(args[0])
				if err != nil {
					return fmt.Errorf("open iCalendar file: %w", err)
				}
				defer func() { _ = file.Close() }()
				input = file
			}

			events, err := calendar.ParseEvents(input)
			if err != nil {
				return err
			}
			cfg, store, ds, err := loadStore()
			if err != nil {
				return err
			}

			// Stdin is always batch mode. Files containing more than one event
			// are batch imports as well.
			if len(args) == 0 || len(events) > 1 {
				created, updated, err := importEventsBatch(store, ds, events, calendarName)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Imported %d event(s): %d added, %d updated\n", len(events), created, updated)
				return err
			}

			imported := events[0]
			existing := eventByUID(ds.Events, imported.UID)
			if existing == nil {
				dest, err := importCalendar(ds, calendarName)
				if err != nil {
					return err
				}
				imported.Source = dest.Source
				imported.Calendar = dest.Name
			}
			model := tui.NewEventImportModel(cfg, ds, store, imported, existing)
			defer model.Close()
			if _, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
				return fmt.Errorf("run import editor: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&calendarName, "calendar", "a", "", "calendar for newly imported events")
	return cmd
}

func importEventsBatch(store *calendar.Store, ds calendar.Dataset, events []calendar.Event, calendarName string) (created, updated int, err error) {
	dest, err := importCalendar(ds, calendarName)
	if err != nil {
		return 0, 0, err
	}
	existing := make(map[string]calendar.Event, len(ds.Events))
	for _, event := range ds.Events {
		if _, ok := existing[event.UID]; !ok {
			existing[event.UID] = event
		}
	}
	for _, event := range events {
		if current, ok := existing[event.UID]; ok {
			update := eventUpdateFromImport(event)
			if err := store.UpdateEvent(current.UID, update); err != nil {
				return created, updated, fmt.Errorf("update event %q: %w", event.UID, err)
			}
			updated++
			continue
		}
		event.Source = dest.Source
		event.Calendar = dest.Name
		if err := store.CreateEvent(dest.Source, dest.Name, event); err != nil {
			return created, updated, fmt.Errorf("add event %q: %w", event.UID, err)
		}
		existing[event.UID] = event
		created++
	}
	return created, updated, nil
}

func eventUpdateFromImport(event calendar.Event) calendar.EventUpdate {
	recurrence := event.Recurrence
	return calendar.EventUpdate{
		Summary:      &event.Summary,
		Description:  &event.Description,
		Location:     &event.Location,
		URL:          &event.URL,
		Organizer:    &event.Organizer,
		Attendees:    &event.Attendees,
		UserRSVP:     &event.UserRSVP,
		Availability: &event.Availability,
		Visibility:   &event.Visibility,
		Recurrence:   &recurrence,
		Alarms:       &event.Alarms,
		Start:        &event.Start,
		End:          &event.End,
		Timezone:     &event.Timezone,
		AllDay:       &event.AllDay,
	}
}

func importCalendar(ds calendar.Dataset, requested string) (calendar.Calendar, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		var matches []calendar.Calendar
		for _, cal := range ds.Calendars {
			if cal.Source == calendar.SpecialSourceBirthdays {
				continue
			}
			aliases := []string{
				cal.Name,
				cal.DisplayName,
				cal.Source,
				cal.Path,
				filepath.Base(cal.Path),
				cal.Source + "/" + cal.Name,
			}
			for _, alias := range aliases {
				if strings.EqualFold(strings.TrimSpace(alias), requested) {
					matches = append(matches, cal)
					break
				}
			}
		}
		switch len(matches) {
		case 0:
			return calendar.Calendar{}, fmt.Errorf("calendar %q not found", requested)
		case 1:
			return matches[0], nil
		default:
			return calendar.Calendar{}, fmt.Errorf("calendar %q is ambiguous; use its source/name or path", requested)
		}
	}
	for _, cal := range ds.Calendars {
		if cal.Source != calendar.SpecialSourceBirthdays {
			return cal, nil
		}
	}
	return calendar.Calendar{}, fmt.Errorf("no writable calendar is configured")
}

func eventByUID(events []calendar.Event, uid string) *calendar.Event {
	for i := range events {
		if events[i].UID == uid {
			event := events[i]
			return &event
		}
	}
	return nil
}
