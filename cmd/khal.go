package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
	"github.com/spf13/cobra"
)

var omarchyJSONFields = []string{
	"uid",
	"title",
	"calendar",
	"calendar-color",
	"location",
	"all-day",
	"start-date-long",
	"start-time",
	"end-date-long",
	"end-time",
	"description",
	"url",
	"organizer",
	"status",
	"repeat-pattern",
}

type khalJSONEvent struct {
	UID           string `json:"uid"`
	Title         string `json:"title"`
	Calendar      string `json:"calendar"`
	CalendarColor string `json:"calendar-color"`
	Location      string `json:"location"`
	AllDay        string `json:"all-day"`
	StartDateLong string `json:"start-date-long"`
	StartTime     string `json:"start-time"`
	EndDateLong   string `json:"end-date-long"`
	EndTime       string `json:"end-time"`
	Description   string `json:"description"`
	URL           string `json:"url"`
	Organizer     string `json:"organizer"`
	Status        string `json:"status"`
	RepeatPattern string `json:"repeat-pattern"`
}

func newPrintFormatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "printformats",
		Short: "Print khal-compatible date and time format samples",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			sample := time.Date(2013, time.December, 21, 21, 45, 0, 0, time.Local)
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"longdatetimeformat: %s\ndatetimeformat: %s\nlongdateformat: %s\ndateformat: %s\ntimeformat: %s\n",
				sample.Format(cfg.LongDateTimeFormat),
				sample.Format(cfg.DateTimeFormat),
				sample.Format(cfg.LongDateFormat),
				sample.Format(cfg.DateFormat),
				sample.Format(cfg.TimeFormat),
			)
			return err
		},
	}
}

func newListCommand() *cobra.Command {
	var fields []string
	var dayFormat string

	cmd := &cobra.Command{
		Use:   "list START DELTA",
		Short: "List events for Omarchy Calendar",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOmarchyFields(fields); err != nil {
				return err
			}
			if !cmd.Flags().Changed("day-format") || dayFormat != "" {
				return errors.New("--day-format must be supplied with an empty value")
			}
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}
			start, end, err := parseOmarchyRange(args[0], args[1], cfg.LongDateFormat)
			if err != nil {
				return err
			}
			store := calendar.NewStore(cfg)
			dataset, err := store.LoadRange(start, end)
			if err != nil {
				return err
			}
			events := calendar.EventsInRange(calendar.FilterVisibleEvents(dataset.Events), start, end)
			rows := make([]khalJSONEvent, 0, len(events))
			for _, event := range events {
				rows = append(rows, omarchyEvent(event, cfg))
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		},
	}
	cmd.Flags().StringArrayVar(&fields, "json", nil, "JSON field to emit; repeat for each field")
	cmd.Flags().StringVar(&dayFormat, "day-format", "", "day heading format")
	return cmd
}

func validateOmarchyFields(fields []string) error {
	if len(fields) != len(omarchyJSONFields) {
		return fmt.Errorf("--json must request Omarchy Calendar's %d fields", len(omarchyJSONFields))
	}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		seen[field] = true
	}
	for _, field := range omarchyJSONFields {
		if !seen[field] {
			return fmt.Errorf("--json field %q is required", field)
		}
	}
	return nil
}

func parseOmarchyRange(startValue, deltaValue, dateLayout string) (time.Time, time.Time, error) {
	var start time.Time
	var err error
	if strings.EqualFold(strings.TrimSpace(startValue), "today") {
		now := time.Now()
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	} else {
		start, err = time.ParseInLocation(dateLayout, strings.TrimSpace(startValue), time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("start date must match longdateformat %q: %w", dateLayout, err)
		}
	}
	if !strings.HasSuffix(deltaValue, "d") {
		return time.Time{}, time.Time{}, errors.New("delta must be a number of days such as 7d")
	}
	days, err := strconv.Atoi(strings.TrimSuffix(deltaValue, "d"))
	if err != nil || days < 1 || days > 60 {
		return time.Time{}, time.Time{}, errors.New("delta days must be between 1 and 60")
	}
	return start, start.AddDate(0, 0, days), nil
}

func omarchyEvent(event calendar.Event, cfg *config.Config) khalJSONEvent {
	calendarName := event.DisplayName
	if calendarName == "" {
		calendarName = event.Calendar
	}
	allDay := "False"
	startTime := event.Start.Format(cfg.TimeFormat)
	endTime := event.End.Format(cfg.TimeFormat)
	endDate := event.End
	if event.AllDay {
		allDay = "True"
		startTime = ""
		endTime = ""
		if event.End.After(event.Start) {
			endDate = event.End.Add(-time.Nanosecond)
		}
	}
	return khalJSONEvent{
		UID:           event.UID,
		Title:         event.Summary,
		Calendar:      calendarName,
		CalendarColor: event.Color,
		Location:      strings.TrimSpace(event.Location),
		AllDay:        allDay,
		StartDateLong: event.Start.Format(cfg.LongDateFormat),
		StartTime:     startTime,
		EndDateLong:   endDate.Format(cfg.LongDateFormat),
		EndTime:       endTime,
		Description:   event.Description,
		URL:           event.URL,
		Organizer:     event.Organizer,
		Status:        strings.ToUpper(strings.TrimSpace(event.Status)),
		RepeatPattern: event.RepeatPattern,
	}
}
