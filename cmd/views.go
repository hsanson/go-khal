package cmd

import (
	"fmt"

	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/tui"
	"github.com/spf13/cobra"
)

func newAgendaCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agenda",
		Short: "Show upcoming events",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			fmt.Println(tui.RenderAgenda(calendar.FilterVisibleEvents(ds.Events), parseNow(), cfg.TimeFormat, tui.DefaultStyles()))
			return nil
		},
	}
	return cmd
}
