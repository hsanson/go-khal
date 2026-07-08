package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/tui"
	"github.com/spf13/cobra"
)

func newAgendaCommand() *cobra.Command {
	var showBirthdays bool
	var maxLength int

	cmd := &cobra.Command{
		Use:   "agenda [count]",
		Short: "Show upcoming events",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			limit := 0
			if len(args) > 0 {
				n, err := strconv.Atoi(args[0])
				if err != nil || n < 1 {
					return fmt.Errorf("count must be a positive integer")
				}
				limit = n
			}
			if cmd.Flags().Changed("max-length") && maxLength < 1 {
				return fmt.Errorf("max length must be a positive integer")
			}

			cfg, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			now := parseNow()
			events := agendaEvents(calendar.FilterVisibleEvents(ds.Events), now, showBirthdays, limit)
			rendered := tui.RenderAgenda(events, now, cfg.WeekStart(), cfg.TimeFormat, tui.DefaultStyles(), maxLength)
			if rendered != "" {
				fmt.Println(rendered)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&showBirthdays, "birthdays", false, "show birthdays and anniversaries instead of events")
	cmd.Flags().IntVar(&maxLength, "max-length", 0, "maximum output line length")
	return cmd
}

func agendaEvents(events []calendar.Event, now time.Time, birthdaysOnly bool, limit int) []calendar.Event {
	out := make([]calendar.Event, 0, len(events))
	for _, ev := range events {
		if ev.End.Before(now) {
			continue
		}
		isBirthday := ev.Kind == calendar.EventKindBirthday || ev.Kind == calendar.EventKindAnniversary
		if birthdaysOnly != isBirthday {
			continue
		}
		out = append(out, ev)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
