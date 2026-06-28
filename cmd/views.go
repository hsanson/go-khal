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

func newDayCommand() *cobra.Command {
	var dateStr string
	cmd := &cobra.Command{
		Use:   "day",
		Short: "Show 24-hour timeline for one day",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			visible := calendar.FilterVisibleEvents(ds.Events)
			day, err := parseDateArg(dateStr)
			if err != nil {
				return err
			}
			fmt.Println(tui.RenderDayTimeline(calendar.EventsOnDay(visible, day), day, cfg.TimeFormat, tui.DefaultStyles()))
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "date to show (YYYY-MM-DD)")
	return cmd
}

func newWeekCommand() *cobra.Command {
	var dateStr string
	cmd := &cobra.Command{
		Use:   "week",
		Short: "Show week timeline with 24-hour days",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			visible := calendar.FilterVisibleEvents(ds.Events)
			day, err := parseDateArg(dateStr)
			if err != nil {
				return err
			}
			start := calendar.StartOfWeek(day, cfg.WeekStart())
			end := start.AddDate(0, 0, 7)
			rangeEvents := calendar.EventsInRange(visible, start, end)
			fmt.Println(tui.RenderWeekTimeline(rangeEvents, start, cfg.TimeFormat, tui.DefaultStyles()))
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "date inside desired week (YYYY-MM-DD)")
	return cmd
}

func newMonthCommand() *cobra.Command {
	var dateStr string
	cmd := &cobra.Command{
		Use:   "month",
		Short: "Show month calendar",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			day, err := parseDateArg(dateStr)
			if err != nil {
				return err
			}
			fmt.Println(tui.RenderMonth(calendar.FilterVisibleEvents(ds.Events), day, tui.DefaultStyles()))
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "month to show, any date inside month (YYYY-MM-DD)")
	return cmd
}

func newYearCommand() *cobra.Command {
	var year int
	cmd := &cobra.Command{
		Use:   "year",
		Short: "Show full year calendar",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _, ds, err := loadStore()
			if err != nil {
				return err
			}
			if year == 0 {
				year = parseNow().Year()
			}
			fmt.Println(tui.RenderYear(calendar.FilterVisibleEvents(ds.Events), year, tui.DefaultStyles()))
			return nil
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "year to show")
	return cmd
}
