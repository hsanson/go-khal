package calendar

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khal/internal/config"
	"github.com/teambition/rrule-go"
)

type calendarSource struct {
	sourceName string
	sourcePath string
	calendar   Calendar
	location   *time.Location
}

type Store struct {
	config *config.Config
}

func NewStore(cfg *config.Config) *Store {
	return &Store{config: cfg}
}

func (s *Store) Load() (Dataset, error) {
	if s.config == nil {
		return Dataset{}, errors.New("missing config")
	}

	var out Dataset
	birthdayEvents := make([]Event, 0, 64)
	hasAddressBook := false
	for _, src := range s.config.Sources {
		if src.Path == "" {
			continue
		}

		calendarSources, err := s.resolveCalendarSources(src)
		if err != nil {
			return Dataset{}, err
		}

		for _, resolved := range calendarSources {
			out.Calendars = append(out.Calendars, resolved.calendar)
			events, todos, err := s.loadCalendarFolder(resolved)
			if err != nil {
				continue
			}
			out.Events = append(out.Events, events...)
			out.Todos = append(out.Todos, todos...)
		}

		if src.AddressBook != "" {
			hasAddressBook = true
			events, err := s.loadAddressBook(src.AddressBook, src.Name)
			if err == nil {
				birthdayEvents = append(birthdayEvents, events...)
			}
		}
	}
	if hasAddressBook {
		out.Events = append(out.Events, birthdayEvents...)
		out.Calendars = append(out.Calendars, Calendar{
			Source:      SpecialSourceBirthdays,
			Name:        SpecialCalendarBirthdays,
			Path:        "virtual://birthdays-anniversaries",
			DisplayName: "Birthdays & Aniversaries",
			Color:       "#d49e00",
			Hidden:      false,
		})
	}

	sort.Slice(out.Calendars, func(i, j int) bool {
		if out.Calendars[i].Source == out.Calendars[j].Source {
			return out.Calendars[i].Name < out.Calendars[j].Name
		}
		return out.Calendars[i].Source < out.Calendars[j].Source
	})
	sort.Slice(out.Events, func(i, j int) bool {
		if out.Events[i].Start.Equal(out.Events[j].Start) {
			return out.Events[i].Summary < out.Events[j].Summary
		}
		return out.Events[i].Start.Before(out.Events[j].Start)
	})
	sort.Slice(out.Todos, func(i, j int) bool {
		left := out.Todos[i]
		right := out.Todos[j]
		if left.Due == nil {
			return false
		}
		if right.Due == nil {
			return true
		}
		return left.Due.Before(*right.Due)
	})

	return out, nil
}

func onlyVisibleEvents(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		if ev.Hidden {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func onlyVisibleTodos(todos []Todo) []Todo {
	out := make([]Todo, 0, len(todos))
	for _, todo := range todos {
		if todo.Hidden {
			continue
		}
		out = append(out, todo)
	}
	return out
}

func (s *Store) resolveCalendarSources(src config.Source) ([]calendarSource, error) {
	loc := time.Local
	if src.DefaultTZName != "" {
		if srcLoc, err := time.LoadLocation(src.DefaultTZName); err == nil {
			loc = srcLoc
		}
	}

	if len(src.Calendars) > 0 {
		out := make([]calendarSource, 0, len(src.Calendars))
		for _, cal := range src.Calendars {
			calPath := cal.Path
			if calPath == "" {
				calPath = filepath.Join(src.Path, cal.Name)
			}
			metaDisplay, metaColor := readCalendarMeta(calPath)
			name := cal.Name
			if name == "" {
				name = filepath.Base(calPath)
			}
			out = append(out, calendarSource{
				sourceName: src.Name,
				sourcePath: src.Path,
				location:   loc,
				calendar: Calendar{
					Source:      src.Name,
					Name:        name,
					Path:        calPath,
					DisplayName: emptyDefault(cal.DisplayName, emptyDefault(metaDisplay, name)),
					Color:       emptyDefault(cal.Color, metaColor),
					Hidden:      cal.Hidden,
				},
			})
		}
		return out, nil
	}

	entries, err := os.ReadDir(src.Path)
	if err != nil {
		return nil, fmt.Errorf("read source %s: %w", src.Path, err)
	}

	hasICSAtRoot := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) == ".ics" {
			hasICSAtRoot = true
			break
		}
	}

	var out []calendarSource
	if hasICSAtRoot {
		meta := findCalendarConfig(src, src.Name)
		metaDisplay, metaColor := readCalendarMeta(src.Path)
		out = append(out, calendarSource{
			sourceName: src.Name,
			sourcePath: src.Path,
			location:   loc,
			calendar: Calendar{
				Source:      src.Name,
				Name:        src.Name,
				Path:        src.Path,
				DisplayName: emptyDefault(meta.DisplayName, emptyDefault(metaDisplay, src.Name)),
				Color:       emptyDefault(meta.Color, metaColor),
				Hidden:      meta.Hidden,
			},
		})
		return out, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		calName := entry.Name()
		calPath := filepath.Join(src.Path, calName)
		meta := findCalendarConfig(src, calName)
		metaDisplay, metaColor := readCalendarMeta(calPath)
		out = append(out, calendarSource{
			sourceName: src.Name,
			sourcePath: src.Path,
			location:   loc,
			calendar: Calendar{
				Source:      src.Name,
				Name:        calName,
				Path:        calPath,
				DisplayName: emptyDefault(meta.DisplayName, emptyDefault(metaDisplay, calName)),
				Color:       emptyDefault(meta.Color, metaColor),
				Hidden:      meta.Hidden,
			},
		})
	}

	if len(out) == 0 {
		meta := findCalendarConfig(src, src.Name)
		metaDisplay, metaColor := readCalendarMeta(src.Path)
		out = append(out, calendarSource{
			sourceName: src.Name,
			sourcePath: src.Path,
			location:   loc,
			calendar: Calendar{
				Source:      src.Name,
				Name:        src.Name,
				Path:        src.Path,
				DisplayName: emptyDefault(meta.DisplayName, emptyDefault(metaDisplay, src.Name)),
				Color:       emptyDefault(meta.Color, metaColor),
				Hidden:      meta.Hidden,
			},
		})
	}

	return out, nil
}

func findCalendarConfig(src config.Source, name string) config.CalendarConfig {
	for _, cal := range src.Calendars {
		if cal.Name == name {
			return cal
		}
	}
	return config.CalendarConfig{Name: name}
}

func readCalendarMeta(path string) (displayName string, color string) {
	displayName = readTrimmedFile(filepath.Join(path, "displayname"))
	if displayName == "" {
		displayName = readTrimmedFile(filepath.Join(path, ".displayname"))
	}
	color = readTrimmedFile(filepath.Join(path, "color"))
	if color == "" {
		color = readTrimmedFile(filepath.Join(path, ".color"))
	}
	return displayName, color
}

func readTrimmedFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *Store) loadCalendarFolder(src calendarSource) ([]Event, []Todo, error) {
	entries, err := os.ReadDir(src.calendar.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read calendar %s: %w", src.calendar.Path, err)
	}

	var events []Event
	var todos []Todo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".ics" {
			continue
		}
		fullPath := filepath.Join(src.calendar.Path, entry.Name())
		e, t, err := s.loadICSFile(src, fullPath)
		if err != nil {
			continue
		}
		events = append(events, e...)
		todos = append(todos, t...)
	}
	return events, todos, nil
}

func (s *Store) loadICSFile(src calendarSource, filePath string) ([]Event, []Todo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	dec := ical.NewDecoder(f)

	var eventComponents []*ical.Component
	var allTodos []Todo
	for {
		cal, err := dec.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		for _, child := range cal.Children {
			switch child.Name {
			case ical.CompEvent:
				eventComponents = append(eventComponents, child)
			case ical.CompToDo:
				todo, ok := componentToTodo(child, src, filePath)
				if ok {
					allTodos = append(allTodos, todo)
				}
			}
		}
	}

	allEvents := s.componentsToEvents(eventComponents, src, filePath)
	return allEvents, allTodos, nil
}

func (s *Store) componentsToEvents(comps []*ical.Component, src calendarSource, filePath string) []Event {
	exdatesByUID := map[string][]time.Time{}
	for _, comp := range comps {
		uid, _ := comp.Props.Text(ical.PropUID)
		if uid == "" {
			continue
		}
		recurrenceID, err := comp.Props.DateTime(ical.PropRecurrenceID, src.location)
		if err != nil || recurrenceID.IsZero() {
			continue
		}
		exdatesByUID[uid] = append(exdatesByUID[uid], recurrenceID.In(src.location))
	}

	var out []Event
	for _, comp := range comps {
		uid, _ := comp.Props.Text(ical.PropUID)
		status, _ := comp.Props.Text(ical.PropStatus)
		if strings.EqualFold(status, "CANCELLED") {
			continue
		}
		recurrenceID, err := comp.Props.DateTime(ical.PropRecurrenceID, src.location)
		forceSingle := err == nil && !recurrenceID.IsZero()
		out = append(out, s.componentToEvents(comp, src, filePath, exdatesByUID[uid], forceSingle)...)
	}
	return dedupeEvents(out)
}

func (s *Store) componentToEvents(comp *ical.Component, src calendarSource, filePath string, extraExdates []time.Time, forceSingle bool) []Event {
	uid, err := comp.Props.Text(ical.PropUID)
	if err != nil || uid == "" {
		return nil
	}
	summary, _ := comp.Props.Text(ical.PropSummary)
	desc, _ := comp.Props.Text(ical.PropDescription)
	location, _ := comp.Props.Text(ical.PropLocation)
	urlText, _ := comp.Props.Text(ical.PropURL)
	if urlText == "" {
		if u, err := comp.Props.URI(ical.PropURL); err == nil && u != nil {
			urlText = u.String()
		}
	}
	organizer, _ := comp.Props.Text(ical.PropOrganizer)
	attendees := propsToAttendees(comp.Props)
	availability := propsToAvailability(comp.Props)
	visibility := propsToVisibility(comp.Props)
	recurrence := propsToRecurrence(comp.Props)
	alarms := componentToAlarms(comp)
	start, err := comp.Props.DateTime(ical.PropDateTimeStart, src.location)
	if err != nil {
		return nil
	}
	start = start.In(src.location)
	end, err := comp.Props.DateTime(ical.PropDateTimeEnd, src.location)
	if err != nil {
		end = start.Add(time.Hour)
	}
	end = end.In(src.location)
	duration := end.Sub(start)
	if duration <= 0 {
		duration = time.Hour
	}
	recurrenceID, recurrenceIDErr := comp.Props.DateTime(ical.PropRecurrenceID, src.location)
	if forceSingle && recurrenceIDErr == nil && !recurrenceID.IsZero() {
		start = overrideStartFromRecurrenceID(start, recurrenceID.In(src.location))
		end = start.Add(duration)
	}

	allDay := start.Hour() == 0 && start.Minute() == 0 && end.Sub(start)%24*time.Hour == 0
	recurring := comp.Props.Get(ical.PropRecurrenceRule) != nil || len(comp.Props.Values(ical.PropRecurrenceDates)) > 0
	hasAlarm := false
	for _, child := range comp.Children {
		if child != nil && child.Name == ical.CompAlarm {
			hasAlarm = true
			break
		}
	}
	base := Event{
		UID:          uid,
		Summary:      emptyDefault(summary, "(untitled event)"),
		Description:  desc,
		Location:     location,
		URL:          urlText,
		Organizer:    organizer,
		Attendees:    attendees,
		Availability: availability,
		Visibility:   visibility,
		Recurrence:   recurrence,
		Alarms:       alarms,
		AllDay:       allDay,
		Recurring:    recurring || recurrence != nil,
		HasAlarm:     hasAlarm || len(alarms) > 0,
		Source:       src.sourceName,
		Calendar:     src.calendar.Name,
		CalendarDir:  src.calendar.Path,
		DisplayName:  src.calendar.DisplayName,
		Color:        src.calendar.Color,
		Hidden:       src.calendar.Hidden,
		FilePath:     filePath,
	}

	if !recurring || forceSingle {
		base.Start = start
		base.End = end
		return []Event{base}
	}

	set, err := comp.RecurrenceSet(src.location)
	if err != nil {
		base.Start = start
		base.End = end
		return []Event{base}
	}

	lookbackMonths := 12
	lookaheadMonths := 24
	if s != nil && s.config != nil {
		lookbackMonths = s.config.RecurrenceLookbackMonths
		lookaheadMonths = s.config.RecurrenceLookaheadMonths
		if lookbackMonths <= 0 {
			lookbackMonths = 12
		}
		if lookaheadMonths <= 0 {
			lookaheadMonths = 24
		}
	}

	from := time.Now().In(src.location).AddDate(0, -lookbackMonths, 0)
	to := time.Now().In(src.location).AddDate(0, lookaheadMonths, 0)
	if start.Before(from) {
		from = start.AddDate(0, -1, 0)
	}
	occs := set.Between(from, to, true)
	if len(occs) == 0 {
		base.Start = start
		base.End = end
		return []Event{base}
	}

	result := make([]Event, 0, len(occs))
	seen := map[int64]bool{}
	for _, occ := range occs {
		occ = occ.In(src.location)
		if occurrenceExcluded(occ, extraExdates) {
			continue
		}
		key := occ.UnixNano()
		if seen[key] {
			continue
		}
		seen[key] = true
		e := base
		e.Start = occ
		e.End = occ.Add(duration).In(src.location)
		result = append(result, e)
	}
	return result
}

func occurrenceExcluded(occ time.Time, exdates []time.Time) bool {
	for _, exdate := range exdates {
		if occ.Equal(exdate) {
			return true
		}
	}
	return false
}

func dedupeEvents(events []Event) []Event {
	if len(events) < 2 {
		return events
	}
	seen := map[string]int{}
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		key := fmt.Sprintf("%s\x00%d\x00%d", ev.UID, ev.Start.UnixNano(), ev.End.UnixNano())
		if idx, ok := seen[key]; ok {
			if len(ev.Alarms) > len(out[idx].Alarms) || len(ev.Attendees) > len(out[idx].Attendees) {
				out[idx] = ev
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, ev)
	}
	return out
}

func overrideStartFromRecurrenceID(start, recurrenceID time.Time) time.Time {
	if sameDate(start, recurrenceID) {
		return start
	}
	return time.Date(
		recurrenceID.Year(),
		recurrenceID.Month(),
		recurrenceID.Day(),
		start.Hour(),
		start.Minute(),
		start.Second(),
		start.Nanosecond(),
		recurrenceID.Location(),
	)
}

func sameDate(a, b time.Time) bool {
	a = a.In(b.Location())
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func propsToAttendees(props ical.Props) []Attendee {
	values := props.Values(ical.PropAttendee)
	if len(values) == 0 {
		return nil
	}
	out := make([]Attendee, 0, len(values))
	for _, prop := range values {
		raw := strings.TrimSpace(prop.Value)
		raw = strings.TrimPrefix(raw, "mailto:")
		raw = strings.TrimPrefix(raw, "MAILTO:")
		name := strings.TrimSpace(prop.Params.Get("CN"))
		email := strings.TrimSpace(raw)
		if name == "" {
			name = email
		}
		if email == "" && name == "" {
			continue
		}
		status := normalizeAttendeeStatus(prop.Params.Get(ical.ParamParticipationStatus))
		out = append(out, Attendee{Name: name, Email: email, Status: status})
	}
	return out
}

func propsToAvailability(props ical.Props) string {
	value, _ := props.Text(ical.PropTransparency)
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TRANSPARENT":
		return "free"
	case "OPAQUE":
		return "busy"
	default:
		return ""
	}
}

func propsToVisibility(props ical.Props) string {
	value, _ := props.Text(ical.PropClass)
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PUBLIC":
		return "public"
	case "PRIVATE":
		return "private"
	case "CONFIDENTIAL":
		return "confidential"
	default:
		return "default"
	}
}

func propsToRecurrence(props ical.Props) *Recurrence {
	opt, err := props.RecurrenceRule()
	if err != nil || opt == nil {
		return nil
	}
	interval := opt.Interval
	if interval <= 0 {
		interval = 1
	}
	rec := &Recurrence{
		Frequency: opt.Freq.String(),
		Interval:  interval,
		Count:     opt.Count,
	}
	for _, weekday := range opt.Byweekday {
		rec.Weekdays = append(rec.Weekdays, weekday.String())
	}
	if len(opt.Bymonthday) > 0 {
		rec.MonthlyBy = "month day"
		rec.MonthDay = opt.Bymonthday[0]
	}
	if len(opt.Byweekday) > 0 && strings.EqualFold(rec.Frequency, "MONTHLY") {
		rec.MonthlyBy = "weekday ordinal"
		rec.MonthWeekday = opt.Byweekday[0].String()
		rec.MonthWeek = monthWeekFromWeekdayString(rec.MonthWeekday)
	}
	if !opt.Until.IsZero() {
		until := opt.Until
		rec.Until = &until
	}
	return rec
}

func componentToAlarms(comp *ical.Component) []Alarm {
	var out []Alarm
	for _, child := range comp.Children {
		if child == nil || child.Name != ical.CompAlarm {
			continue
		}
		trigger := child.Props.Get(ical.PropTrigger)
		if trigger == nil {
			continue
		}
		offset, err := trigger.Duration()
		if err != nil {
			continue
		}
		action, _ := child.Props.Text(ical.PropAction)
		if action == "" {
			action = "DISPLAY"
		}
		out = append(out, Alarm{Offset: offset, Action: action})
	}
	return out
}

func componentToTodo(comp *ical.Component, src calendarSource, filePath string) (Todo, bool) {
	uid, err := comp.Props.Text(ical.PropUID)
	if err != nil || uid == "" {
		return Todo{}, false
	}

	summary, _ := comp.Props.Text(ical.PropSummary)
	desc, _ := comp.Props.Text(ical.PropDescription)
	location, _ := comp.Props.Text(ical.PropLocation)
	status, _ := comp.Props.Text(ical.PropStatus)

	var start *time.Time
	if st, err := comp.Props.DateTime(ical.PropDateTimeStart, src.location); err == nil {
		if !st.IsZero() {
			start = &st
		}
	}

	var due *time.Time
	if dt, err := comp.Props.DateTime(ical.PropDue, src.location); err == nil {
		if !dt.IsZero() {
			due = &dt
		}
	}

	var completed *time.Time
	if ct, err := comp.Props.DateTime(ical.PropCompleted, src.location); err == nil {
		if !ct.IsZero() {
			completed = &ct
		}
	}

	percent := 0
	if prop := comp.Props.Get(ical.PropPercentComplete); prop != nil {
		if p, err := prop.Int(); err == nil {
			percent = p
		}
	}
	priority := 0
	if prop := comp.Props.Get(ical.PropPriority); prop != nil {
		if p, err := prop.Int(); err == nil {
			priority = p
		}
	}

	return Todo{
		UID:         uid,
		Summary:     emptyDefault(summary, "(untitled todo)"),
		Description: desc,
		Location:    location,
		Status:      emptyDefault(status, "NEEDS-ACTION"),
		Priority:    priority,
		Start:       start,
		Due:         due,
		Completed:   completed,
		Percent:     percent,
		Source:      src.sourceName,
		Calendar:    src.calendar.Name,
		CalendarDir: src.calendar.Path,
		DisplayName: src.calendar.DisplayName,
		Color:       src.calendar.Color,
		Hidden:      src.calendar.Hidden,
		FilePath:    filePath,
	}, true
}

func (s *Store) loadAddressBook(path string, sourceName string) ([]Event, error) {
	out := make([]Event, 0, 128)
	err := filepath.WalkDir(path, func(fullPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".vcf" {
			return nil
		}
		events, err := readVCardFile(fullPath, sourceName, s.config)
		if err != nil {
			return nil
		}
		out = append(out, events...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func readVCardFile(path string, sourceName string, cfg *config.Config) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := vcard.NewDecoder(f)
	out := make([]Event, 0, 8)
	for {
		card, err := dec.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, birthdayEventsFromCard(card, sourceName, path, cfg)...)
	}
	return out, nil
}

func birthdayEventsFromCard(card vcard.Card, sourceName string, filePath string, cfg *config.Config) []Event {
	name := strings.TrimSpace(card.Value(vcard.FieldFormattedName))
	if name == "" {
		if n := card.Name(); n != nil {
			joined := strings.TrimSpace(strings.Join([]string{n.GivenName, n.AdditionalName, n.FamilyName}, " "))
			if joined != "" {
				name = joined
			}
		}
	}
	if name == "" {
		name = "Contact"
	}

	out := make([]Event, 0, 16)
	for i, v := range card.Values(vcard.FieldBirthday) {
		out = append(out, recurringAllDayFromDateValue(v, EventKindBirthday, name, sourceName, filePath, i, cfg)...)
	}
	annValues := append([]string{}, card.Values(vcard.FieldAnniversary)...)
	annValues = append(annValues, card.Values("X-ANNIVERSARY")...)
	for i, v := range annValues {
		out = append(out, recurringAllDayFromDateValue(v, EventKindAnniversary, name, sourceName, filePath, i, cfg)...)
	}
	return out
}

func recurringAllDayFromDateValue(raw string, kind string, contactName string, sourceName string, filePath string, index int, cfg *config.Config) []Event {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	t, hasYear, ok := parseVCardDate(raw)
	if !ok {
		return nil
	}

	summary := contactName + " Birthday"
	if kind == EventKindAnniversary {
		summary = contactName + " Anniversary"
	}

	uidPrefix := "bday"
	if kind == EventKindAnniversary {
		uidPrefix = "anni"
	}
	uidBase := fmt.Sprintf("%s:%s:%s:%d:%02d%02d", uidPrefix, sourceName, filePath, index, t.Month(), t.Day())
	from, to := birthdayWindow(time.Now(), cfg)
	events := makeBirthdayInstances(uidBase, summary, kind, filePath, t.Month(), t.Day(), hasYear, t.Year(), from, to)
	return events
}

func parseVCardDate(v string) (time.Time, bool, bool) {
	v = strings.TrimSpace(v)
	if len(v) >= 10 {
		if t, err := time.Parse("2006-01-02", v[:10]); err == nil {
			return t, true, true
		}
	}
	if len(v) == 8 {
		if t, err := time.Parse("20060102", v); err == nil {
			return t, true, true
		}
	}
	if len(v) == 5 && v[0] == '-' && v[1] == '-' {
		if t, err := time.Parse("--01-02", v); err == nil {
			return t, false, true
		}
	}
	if len(v) == 6 && strings.HasPrefix(v, "--") {
		if t, err := time.Parse("--0102", v); err == nil {
			return t, false, true
		}
	}
	if len(v) == 4 {
		if t, err := time.Parse("0102", v); err == nil {
			return t, false, true
		}
	}
	return time.Time{}, false, false
}

func birthdayWindow(now time.Time, cfg *config.Config) (time.Time, time.Time) {
	lookbackMonths := 12
	lookaheadMonths := 24
	if cfg != nil {
		if cfg.RecurrenceLookbackMonths > 0 {
			lookbackMonths = cfg.RecurrenceLookbackMonths
		}
		if cfg.RecurrenceLookaheadMonths > 0 {
			lookaheadMonths = cfg.RecurrenceLookaheadMonths
		}
	}
	if lookbackMonths > 120 {
		lookbackMonths = 120
	}
	if lookaheadMonths > 120 {
		lookaheadMonths = 120
	}
	from := now.AddDate(0, -lookbackMonths, 0)
	to := now.AddDate(0, lookaheadMonths, 0)
	return from, to
}

func makeBirthdayInstances(uidBase, summary, kind, filePath string, month time.Month, day int, hasYear bool, originalYear int, from, to time.Time) []Event {
	out := make([]Event, 0, 8)
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.Local)

	for year := from.Year() - 1; year <= to.Year()+1; year++ {
		start := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
		if start.Month() != month || start.Day() != day {
			continue
		}
		if start.Before(from) || start.After(to) {
			continue
		}

		desc := "Generated from vCard"
		if hasYear && originalYear > 0 {
			years := year - originalYear
			if years >= 0 {
				desc = fmt.Sprintf("Generated from vCard (%d years)", years)
			}
		}

		eventSummary := summary
		if hasYear && originalYear > 0 {
			years := year - originalYear
			if years > 0 {
				if kind == EventKindBirthday {
					eventSummary = strings.TrimSuffix(summary, " Birthday") + " " + ordinal(years) + " Birthday"
				} else if kind == EventKindAnniversary {
					eventSummary = strings.TrimSuffix(summary, " Anniversary") + " " + ordinal(years) + " Anniversary"
				}
			}
		}

		out = append(out, Event{
			UID:         fmt.Sprintf("%s:%d", uidBase, year),
			Summary:     eventSummary,
			Description: desc,
			Start:       start,
			End:         start.Add(24 * time.Hour),
			AllDay:      true,
			Kind:        kind,
			Recurring:   true,
			Source:      SpecialSourceBirthdays,
			Calendar:    SpecialCalendarBirthdays,
			DisplayName: "Birthdays & Aniversaries",
			Color:       "#d49e00",
			Hidden:      false,
			FilePath:    filePath,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

func ordinal(n int) string {
	mod10 := n % 10
	mod100 := n % 100
	suffix := "th"
	if mod100 < 11 || mod100 > 13 {
		switch mod10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func (s *Store) CreateTodo(sourceName, calendarName string, t Todo) error {
	cal, loc, err := s.findWritableCalendar(sourceName, calendarName)
	if err != nil {
		return err
	}

	if t.UID == "" {
		t.UID = fmt.Sprintf("todo-%d@go-khal", time.Now().UnixNano())
	}

	comp := ical.NewComponent(ical.CompToDo)
	comp.Props.SetText(ical.PropUID, t.UID)
	comp.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	comp.Props.SetText(ical.PropSummary, emptyDefault(t.Summary, "(untitled todo)"))
	if t.Description != "" {
		comp.Props.SetText(ical.PropDescription, t.Description)
	}
	if t.Location != "" {
		comp.Props.SetText(ical.PropLocation, t.Location)
	}
	comp.Props.SetText(ical.PropStatus, emptyDefault(t.Status, "NEEDS-ACTION"))
	if t.Priority > 0 {
		propPriority := ical.NewProp(ical.PropPriority)
		propPriority.Value = fmt.Sprintf("%d", t.Priority)
		comp.Props.Set(propPriority)
	}
	if t.Start != nil {
		comp.Props.SetDateTime(ical.PropDateTimeStart, t.Start.In(loc))
	}
	if t.Due != nil {
		comp.Props.SetDateTime(ical.PropDue, t.Due.In(loc))
	}
	propPercent := ical.NewProp(ical.PropPercentComplete)
	propPercent.Value = fmt.Sprintf("%d", t.Percent)
	comp.Props.Set(propPercent)

	newCal := ical.NewCalendar()
	newCal.Props.SetText(ical.PropVersion, "2.0")
	newCal.Props.SetText(ical.PropProductID, "-//go-khal//EN")
	newCal.Children = append(newCal.Children, comp)

	filePath := filepath.Join(cal.Path, t.UID+".ics")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create todo file: %w", err)
	}
	defer f.Close()
	if err := ical.NewEncoder(f).Encode(newCal); err != nil {
		return fmt.Errorf("encode todo: %w", err)
	}
	return nil
}

func (s *Store) UpdateTodo(uid string, update TodoUpdate) error {
	todo, err := s.FindTodo(uid)
	if err != nil {
		return err
	}

	f, err := os.Open(todo.FilePath)
	if err != nil {
		return err
	}
	dec := ical.NewDecoder(f)
	cal, err := dec.Decode()
	f.Close()
	if err != nil {
		return err
	}

	for _, child := range cal.Children {
		if child.Name != ical.CompToDo {
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID != uid {
			continue
		}
		if update.Summary != nil {
			child.Props.SetText(ical.PropSummary, *update.Summary)
		}
		if update.Description != nil {
			child.Props.SetText(ical.PropDescription, *update.Description)
		}
		if update.Location != nil {
			child.Props.Del(ical.PropLocation)
			if strings.TrimSpace(*update.Location) != "" {
				child.Props.SetText(ical.PropLocation, *update.Location)
			}
		}
		if update.Status != nil {
			child.Props.SetText(ical.PropStatus, *update.Status)
		}
		if update.Priority != nil {
			child.Props.Del(ical.PropPriority)
			if *update.Priority > 0 {
				prop := ical.NewProp(ical.PropPriority)
				prop.Value = fmt.Sprintf("%d", *update.Priority)
				child.Props.Set(prop)
			}
		}
		if update.Start != nil {
			child.Props.Del(ical.PropDateTimeStart)
			if *update.Start != nil {
				child.Props.SetDateTime(ical.PropDateTimeStart, **update.Start)
			}
		}
		if update.Due != nil {
			child.Props.Del(ical.PropDue)
			if *update.Due != nil {
				child.Props.SetDateTime(ical.PropDue, **update.Due)
			}
		}
		if update.Percent != nil {
			prop := ical.NewProp(ical.PropPercentComplete)
			prop.Value = fmt.Sprintf("%d", *update.Percent)
			child.Props.Set(prop)
		}
	}

	out, err := os.Create(todo.FilePath)
	if err != nil {
		return err
	}
	defer out.Close()
	return ical.NewEncoder(out).Encode(cal)
}

func (s *Store) MoveTodo(uid, sourceName, calendarName string) error {
	todo, err := s.FindTodo(uid)
	if err != nil {
		return err
	}
	target, _, err := s.findWritableCalendar(sourceName, calendarName)
	if err != nil {
		return err
	}
	if sameFilePath(todo.CalendarDir, target.Path) {
		return nil
	}
	return moveCalendarComponents(todo.FilePath, target.Path, ical.CompToDo, uid)
}

func (s *Store) FindTodo(uid string) (Todo, error) {
	ds, err := s.Load()
	if err != nil {
		return Todo{}, err
	}
	for _, todo := range ds.Todos {
		if todo.UID == uid {
			return todo, nil
		}
	}
	return Todo{}, fmt.Errorf("todo with uid %q not found", uid)
}

func (s *Store) FindCalendar(sourceName, calendarName string) (Calendar, error) {
	ds, err := s.Load()
	if err != nil {
		return Calendar{}, err
	}
	for _, cal := range ds.Calendars {
		if cal.Source == sourceName && cal.Name == calendarName {
			return cal, nil
		}
	}
	return Calendar{}, fmt.Errorf("calendar %s/%s not found", sourceName, calendarName)
}

func (s *Store) FindEvent(uid string) (Event, error) {
	ds, err := s.Load()
	if err != nil {
		return Event{}, err
	}
	for _, ev := range ds.Events {
		if ev.UID == uid {
			return ev, nil
		}
	}
	return Event{}, fmt.Errorf("event with uid %q not found", uid)
}

func (s *Store) Contacts() ([]Contact, error) {
	if s == nil || s.config == nil {
		return nil, errors.New("missing config")
	}
	seen := map[string]bool{}
	var out []Contact
	for _, src := range s.config.Sources {
		if strings.TrimSpace(src.AddressBook) == "" {
			continue
		}
		contacts, err := readContacts(src.AddressBook)
		if err != nil {
			continue
		}
		for _, contact := range contacts {
			key := strings.ToLower(contact.Name + "\x00" + contact.Email)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, contact)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Email < out[j].Email
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func readContacts(path string) ([]Contact, error) {
	out := make([]Contact, 0, 128)
	err := filepath.WalkDir(path, func(fullPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || strings.ToLower(filepath.Ext(d.Name())) != ".vcf" {
			return nil
		}
		f, err := os.Open(fullPath)
		if err != nil {
			return nil
		}
		defer f.Close()
		dec := vcard.NewDecoder(f)
		for {
			card, err := dec.Decode()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil
			}
			name := strings.TrimSpace(card.Value(vcard.FieldFormattedName))
			if name == "" {
				if n := card.Name(); n != nil {
					name = strings.TrimSpace(strings.Join([]string{n.GivenName, n.AdditionalName, n.FamilyName}, " "))
				}
			}
			for _, email := range card.Values(vcard.FieldEmail) {
				email = strings.TrimSpace(email)
				if email == "" {
					continue
				}
				contactName := name
				if contactName == "" {
					contactName = email
				}
				out = append(out, Contact{Name: contactName, Email: email})
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) DeleteTodo(uid string) error {
	todo, err := s.FindTodo(uid)
	if err != nil {
		return err
	}
	cal, err := readCalendarFile(todo.FilePath)
	if err != nil {
		return err
	}
	var removed bool
	children := cal.Children[:0]
	for _, child := range cal.Children {
		if child == nil || child.Name != ical.CompToDo {
			children = append(children, child)
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID == uid {
			removed = true
			continue
		}
		children = append(children, child)
	}
	if !removed {
		return fmt.Errorf("todo with uid %q not found", uid)
	}
	cal.Children = children
	if len(cal.Children) == 0 {
		return os.Remove(todo.FilePath)
	}
	return writeCalendarFile(todo.FilePath, cal)
}

func (s *Store) DeleteEvent(ev Event, scope DeleteRecurringScope) error {
	if ev.Source == SpecialSourceBirthdays {
		return errors.New("birthday and anniversary events are generated from contacts")
	}
	if scope == "" {
		scope = DeleteRecurringAll
	}
	current, err := s.FindEvent(ev.UID)
	if err != nil {
		return err
	}
	cal, err := readCalendarFile(current.FilePath)
	if err != nil {
		return err
	}
	var removed bool
	for _, child := range cal.Children {
		if child == nil || child.Name != ical.CompEvent {
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID != ev.UID {
			continue
		}
		switch scope {
		case DeleteRecurringOccurrence:
			if !current.Recurring {
				removed = true
				break
			}
			addEventExceptionDate(child, ev.Start, ev.AllDay)
			removed = true
		case DeleteRecurringFuture:
			if !current.Recurring || !ev.Start.After(current.Start) {
				removed = true
				break
			}
			truncateEventRecurrence(child, ev.Start.Add(-time.Second))
			removed = true
		default:
			removed = true
		}
		break
	}
	if !removed {
		return fmt.Errorf("event with uid %q not found", ev.UID)
	}
	if scope == DeleteRecurringAll || !current.Recurring || (scope == DeleteRecurringFuture && !ev.Start.After(current.Start)) {
		children := cal.Children[:0]
		for _, child := range cal.Children {
			if child == nil || child.Name != ical.CompEvent {
				children = append(children, child)
				continue
			}
			entryUID, _ := child.Props.Text(ical.PropUID)
			if entryUID == ev.UID {
				continue
			}
			children = append(children, child)
		}
		cal.Children = children
		if len(cal.Children) == 0 {
			return os.Remove(current.FilePath)
		}
	}
	return writeCalendarFile(current.FilePath, cal)
}

func (s *Store) CreateEvent(sourceName, calendarName string, e Event) error {
	cal, loc, err := s.findWritableCalendar(sourceName, calendarName)
	if err != nil {
		return err
	}

	if e.UID == "" {
		e.UID = fmt.Sprintf("event-%d@go-khal", time.Now().UnixNano())
	}
	start := e.Start
	end := e.End
	if start.IsZero() {
		start = time.Now().In(loc).Truncate(time.Minute)
	}
	if end.IsZero() || !end.After(start) {
		end = start.Add(time.Hour)
	}
	if e.AllDay {
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
		end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
		if !end.After(start) {
			end = start.Add(24 * time.Hour)
		}
	}

	comp := ical.NewComponent(ical.CompEvent)
	comp.Props.SetText(ical.PropUID, e.UID)
	comp.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	comp.Props.SetText(ical.PropSummary, emptyDefault(e.Summary, "(untitled event)"))
	if strings.TrimSpace(e.Location) != "" {
		comp.Props.SetText(ical.PropLocation, e.Location)
	}
	if strings.TrimSpace(e.Description) != "" {
		comp.Props.SetText(ical.PropDescription, e.Description)
	}
	if strings.TrimSpace(e.URL) != "" {
		setEventURL(comp, e.URL)
	}
	setEventTimeProps(comp, start.In(loc), end.In(loc), e.AllDay)
	setEventAttendees(comp, e.Attendees)
	setEventAvailability(comp, e.Availability)
	setEventVisibility(comp, e.Visibility)
	setEventRecurrence(comp, e.Recurrence, start.In(loc))
	setEventAlarms(comp, e.Alarms)

	newCal := ical.NewCalendar()
	newCal.Props.SetText(ical.PropVersion, "2.0")
	newCal.Props.SetText(ical.PropProductID, "-//go-khal//EN")
	newCal.Props.SetText(ical.PropCalendarScale, "GREGORIAN")
	newCal.Props.SetText(ical.PropMethod, "PUBLISH")
	newCal.Children = append(newCal.Children, comp)

	filePath := filepath.Join(cal.Path, e.UID+".ics")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create event file: %w", err)
	}
	defer f.Close()
	if err := ical.NewEncoder(f).Encode(newCal); err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	return nil
}

func (s *Store) UpdateEvent(uid string, update EventUpdate) error {
	ev, err := s.FindEvent(uid)
	if err != nil {
		return err
	}
	return s.UpdateEventScoped(ev, update, EditRecurringAll)
}

func (s *Store) UpdateEventScoped(ev Event, update EventUpdate, scope EditRecurringScope) error {
	if scope == "" {
		scope = EditRecurringAll
	}
	if !ev.Recurring || scope == EditRecurringAll {
		return s.updateEventAll(ev, update)
	}
	switch scope {
	case EditRecurringOccurrence:
		return s.updateEventOccurrence(ev, update)
	case EditRecurringFuture:
		return s.updateEventFuture(ev, update)
	default:
		return s.updateEventAll(ev, update)
	}
}

func (s *Store) MoveEvent(ev Event, sourceName, calendarName string) error {
	if ev.Source == SpecialSourceBirthdays {
		return errors.New("birthday and anniversary events are generated from contacts")
	}
	if ev.UID == "" {
		return errors.New("event uid is required")
	}
	current, err := s.FindEvent(ev.UID)
	if err != nil {
		return err
	}
	target, _, err := s.findWritableCalendar(sourceName, calendarName)
	if err != nil {
		return err
	}
	if sameFilePath(current.CalendarDir, target.Path) {
		return nil
	}
	return moveCalendarComponents(current.FilePath, target.Path, ical.CompEvent, ev.UID)
}

func (s *Store) updateEventAll(ev Event, update EventUpdate) error {
	f, err := os.Open(ev.FilePath)
	if err != nil {
		return err
	}
	dec := ical.NewDecoder(f)
	cal, err := dec.Decode()
	f.Close()
	if err != nil {
		return err
	}

	updated := false
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID != ev.UID {
			continue
		}
		if _, hasRID := eventRecurrenceID(child, ev.Start.Location()); hasRID {
			continue
		}
		if updated {
			continue
		}
		applyEventUpdateToComponent(child, ev, update)
		updated = true
	}
	if update.Recurrence != nil && *update.Recurrence == nil {
		cal.Children = removeEventOverrides(cal.Children, ev.UID)
	}

	out, err := os.Create(ev.FilePath)
	if err != nil {
		return err
	}
	defer out.Close()
	return ical.NewEncoder(out).Encode(cal)
}

func (s *Store) updateEventOccurrence(ev Event, update EventUpdate) error {
	cal, err := readCalendarFile(ev.FilePath)
	if err != nil {
		return err
	}
	var master *ical.Component
	var override *ical.Component
	for _, child := range cal.Children {
		if child == nil || child.Name != ical.CompEvent {
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID != ev.UID {
			continue
		}
		rid, hasRID := eventRecurrenceID(child, ev.Start.Location())
		if hasRID {
			if sameOccurrence(rid, ev.Start, ev.AllDay) {
				override = child
			}
			continue
		}
		if master == nil {
			master = child
		}
	}
	if master == nil {
		return fmt.Errorf("event with uid %q not found", ev.UID)
	}
	if override == nil {
		override = ical.NewComponent(ical.CompEvent)
		override.Props.SetText(ical.PropUID, ev.UID)
		setRecurrenceID(override, ev.Start, ev.AllDay)
		cal.Children = append(cal.Children, override)
	}
	occ := ev
	occ.Recurrence = nil
	occ.Recurring = false
	applyEventUpdateToComponent(override, occ, update)
	removeEventRecurrenceProps(override)
	setRecurrenceID(override, ev.Start, ev.AllDay)
	return writeCalendarFile(ev.FilePath, cal)
}

func (s *Store) updateEventFuture(ev Event, update EventUpdate) error {
	cal, err := readCalendarFile(ev.FilePath)
	if err != nil {
		return err
	}
	var master *ical.Component
	for _, child := range cal.Children {
		if child == nil || child.Name != ical.CompEvent {
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID != ev.UID {
			continue
		}
		if _, hasRID := eventRecurrenceID(child, ev.Start.Location()); hasRID {
			continue
		}
		master = child
		break
	}
	if master == nil {
		return fmt.Errorf("event with uid %q not found", ev.UID)
	}
	if !ev.Start.After(masterEventStart(master, ev.Start.Location(), ev.Start)) {
		applyEventUpdateToComponent(master, ev, update)
		return writeCalendarFile(ev.FilePath, cal)
	}
	truncateEventRecurrence(master, ev.Start.Add(-time.Second))
	cal.Children = removeEventOverridesFrom(cal.Children, ev.UID, ev.Start, ev.AllDay)

	next := ical.NewComponent(ical.CompEvent)
	future := ev
	future.UID = fmt.Sprintf("event-%d@go-khal", time.Now().UnixNano())
	next.Props.SetText(ical.PropUID, future.UID)
	next.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	applyEventUpdateToComponent(next, future, update)
	cal.Children = append(cal.Children, next)
	return writeCalendarFile(ev.FilePath, cal)
}

func applyEventUpdateToComponent(comp *ical.Component, base Event, update EventUpdate) {
	if comp.Props.Get(ical.PropUID) == nil {
		comp.Props.SetText(ical.PropUID, base.UID)
	}
	if comp.Props.Get(ical.PropDateTimeStamp) == nil {
		comp.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	}

	summary := base.Summary
	if update.Summary != nil {
		summary = *update.Summary
	}
	comp.Props.SetText(ical.PropSummary, emptyDefault(summary, "(untitled event)"))

	location := base.Location
	if update.Location != nil {
		location = *update.Location
	}
	comp.Props.Del(ical.PropLocation)
	if strings.TrimSpace(location) != "" {
		comp.Props.SetText(ical.PropLocation, location)
	}

	description := base.Description
	if update.Description != nil {
		description = *update.Description
	}
	comp.Props.Del(ical.PropDescription)
	if strings.TrimSpace(description) != "" {
		comp.Props.SetText(ical.PropDescription, description)
	}

	url := base.URL
	if update.URL != nil {
		url = *update.URL
	}
	comp.Props.Del(ical.PropURL)
	if strings.TrimSpace(url) != "" {
		setEventURL(comp, url)
	}

	attendees := base.Attendees
	if update.Attendees != nil {
		attendees = *update.Attendees
	}
	setEventAttendees(comp, attendees)

	availability := base.Availability
	if update.Availability != nil {
		availability = *update.Availability
	}
	setEventAvailability(comp, availability)

	visibility := base.Visibility
	if update.Visibility != nil {
		visibility = *update.Visibility
	}
	setEventVisibility(comp, visibility)

	alarms := base.Alarms
	if update.Alarms != nil {
		alarms = *update.Alarms
	}
	setEventAlarms(comp, alarms)

	nextStart := base.Start
	nextEnd := base.End
	if update.Start != nil {
		nextStart = *update.Start
	}
	if update.End != nil {
		nextEnd = *update.End
	}
	allDay := base.AllDay
	if update.AllDay != nil {
		allDay = *update.AllDay
	}
	if allDay {
		nextStart = time.Date(nextStart.Year(), nextStart.Month(), nextStart.Day(), 0, 0, 0, 0, nextStart.Location())
		nextEnd = time.Date(nextEnd.Year(), nextEnd.Month(), nextEnd.Day(), 0, 0, 0, 0, nextEnd.Location())
		if !nextEnd.After(nextStart) {
			nextEnd = nextStart.Add(24 * time.Hour)
		}
	} else if !nextEnd.After(nextStart) {
		nextEnd = nextStart.Add(time.Hour)
	}
	setEventTimeProps(comp, nextStart, nextEnd, allDay)

	rec := base.Recurrence
	if update.Recurrence != nil {
		rec = *update.Recurrence
	}
	setEventRecurrence(comp, rec, nextStart)
}

func setEventAttendees(comp *ical.Component, attendees []Attendee) {
	comp.Props.Del(ical.PropAttendee)
	for _, attendee := range attendees {
		email := strings.TrimSpace(attendee.Email)
		name := strings.TrimSpace(attendee.Name)
		if email == "" && name == "" {
			continue
		}
		value := email
		if value != "" && !strings.Contains(value, ":") {
			value = "mailto:" + value
		}
		if value == "" {
			value = name
		}
		prop := ical.NewProp(ical.PropAttendee)
		prop.Value = value
		if name != "" {
			prop.Params.Set("CN", name)
		}
		if status := attendeePartstat(attendee.Status); status != "" {
			prop.Params.Set(ical.ParamParticipationStatus, status)
		}
		comp.Props.Add(prop)
	}
}

func setEventAvailability(comp *ical.Component, availability string) {
	comp.Props.Del(ical.PropTransparency)
	switch strings.ToLower(strings.TrimSpace(availability)) {
	case "free":
		comp.Props.SetText(ical.PropTransparency, "TRANSPARENT")
	case "busy":
		comp.Props.SetText(ical.PropTransparency, "OPAQUE")
	}
}

func setEventVisibility(comp *ical.Component, visibility string) {
	comp.Props.Del(ical.PropClass)
	switch strings.ToLower(strings.TrimSpace(visibility)) {
	case "public":
		comp.Props.SetText(ical.PropClass, "PUBLIC")
	case "private":
		comp.Props.SetText(ical.PropClass, "PRIVATE")
	case "confidential":
		comp.Props.SetText(ical.PropClass, "CONFIDENTIAL")
	}
}

func normalizeAttendeeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ACCEPTED":
		return "yes"
	case "DECLINED":
		return "no"
	case "TENTATIVE":
		return "maybe"
	default:
		return ""
	}
}

func attendeePartstat(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "yes":
		return "ACCEPTED"
	case "no":
		return "DECLINED"
	case "maybe":
		return "TENTATIVE"
	default:
		return ""
	}
}

func setEventRecurrence(comp *ical.Component, rec *Recurrence, start time.Time) {
	removeEventRecurrenceProps(comp)
	if rec == nil || strings.TrimSpace(rec.Frequency) == "" || strings.EqualFold(rec.Frequency, "NONE") {
		return
	}
	freq, err := rrule.StrToFreq(strings.ToUpper(strings.TrimSpace(rec.Frequency)))
	if err != nil {
		return
	}
	interval := rec.Interval
	if interval <= 0 {
		interval = 1
	}
	opt := &rrule.ROption{
		Freq:     freq,
		Dtstart:  start,
		Interval: interval,
		Count:    rec.Count,
	}
	for _, weekday := range rec.Weekdays {
		if wd, ok := rruleWeekday(weekday, 0); ok {
			opt.Byweekday = append(opt.Byweekday, wd)
		}
	}
	if strings.EqualFold(rec.Frequency, "MONTHLY") {
		switch rec.MonthlyBy {
		case "month day":
			day := rec.MonthDay
			if day <= 0 {
				day = start.Day()
			}
			opt.Bymonthday = []int{day}
			opt.Byweekday = nil
		case "weekday ordinal":
			week := rec.MonthWeek
			if week == 0 {
				week = (start.Day()-1)/7 + 1
			}
			weekday := rec.MonthWeekday
			if weekday == "" {
				weekday = weekdayCode(start.Weekday())
			}
			if wd, ok := rruleWeekday(weekday, week); ok {
				opt.Byweekday = []rrule.Weekday{wd}
			}
			opt.Bymonthday = nil
		}
	}
	if rec.Until != nil {
		opt.Until = *rec.Until
	}
	comp.Props.SetRecurrenceRule(opt)
}

func removeEventRecurrenceProps(comp *ical.Component) {
	comp.Props.Del(ical.PropRecurrenceRule)
	comp.Props.Del(ical.PropRecurrenceDates)
	comp.Props.Del(ical.PropExceptionDates)
}

func eventRecurrenceID(comp *ical.Component, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	rid, err := comp.Props.DateTime(ical.PropRecurrenceID, loc)
	if err != nil || rid.IsZero() {
		return time.Time{}, false
	}
	return rid.In(loc), true
}

func setRecurrenceID(comp *ical.Component, start time.Time, allDay bool) {
	comp.Props.Del(ical.PropRecurrenceID)
	prop := ical.NewProp(ical.PropRecurrenceID)
	if allDay {
		prop.SetDate(start)
	} else {
		prop.SetDateTime(start.UTC())
	}
	comp.Props.Add(prop)
}

func sameOccurrence(a, b time.Time, allDay bool) bool {
	if allDay {
		return sameDate(a, b)
	}
	return a.Equal(b)
}

func masterEventStart(comp *ical.Component, loc *time.Location, fallback time.Time) time.Time {
	start, err := comp.Props.DateTime(ical.PropDateTimeStart, loc)
	if err != nil || start.IsZero() {
		return fallback
	}
	if loc != nil {
		return start.In(loc)
	}
	return start
}

func removeEventOverrides(children []*ical.Component, uid string) []*ical.Component {
	out := children[:0]
	for _, child := range children {
		if child == nil || child.Name != ical.CompEvent {
			out = append(out, child)
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID == uid {
			if _, hasRID := eventRecurrenceID(child, time.Local); hasRID {
				continue
			}
		}
		out = append(out, child)
	}
	return out
}

func removeEventOverridesFrom(children []*ical.Component, uid string, start time.Time, allDay bool) []*ical.Component {
	out := children[:0]
	for _, child := range children {
		if child == nil || child.Name != ical.CompEvent {
			out = append(out, child)
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID == uid {
			if rid, hasRID := eventRecurrenceID(child, start.Location()); hasRID {
				if sameOccurrence(rid, start, allDay) || rid.After(start) {
					continue
				}
			}
		}
		out = append(out, child)
	}
	return out
}

func rruleWeekday(raw string, nth int) (rrule.Weekday, bool) {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if len(raw) > 2 {
		raw = raw[len(raw)-2:]
	}
	var wd rrule.Weekday
	switch raw {
	case "MO":
		wd = rrule.MO
	case "TU":
		wd = rrule.TU
	case "WE":
		wd = rrule.WE
	case "TH":
		wd = rrule.TH
	case "FR":
		wd = rrule.FR
	case "SA":
		wd = rrule.SA
	case "SU":
		wd = rrule.SU
	default:
		return rrule.Weekday{}, false
	}
	if nth != 0 {
		wd = wd.Nth(nth)
	}
	return wd, true
}

func weekdayCode(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "MO"
	case time.Tuesday:
		return "TU"
	case time.Wednesday:
		return "WE"
	case time.Thursday:
		return "TH"
	case time.Friday:
		return "FR"
	case time.Saturday:
		return "SA"
	default:
		return "SU"
	}
}

func monthWeekFromWeekdayString(raw string) int {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	if len(raw) <= 2 {
		return 0
	}
	n, err := strconv.Atoi(raw[:len(raw)-2])
	if err != nil {
		return 0
	}
	return n
}

func setEventAlarms(comp *ical.Component, alarms []Alarm) {
	children := comp.Children[:0]
	for _, child := range comp.Children {
		if child != nil && child.Name == ical.CompAlarm {
			continue
		}
		children = append(children, child)
	}
	comp.Children = children
	for _, alarm := range alarms {
		action := strings.TrimSpace(alarm.Action)
		if action == "" {
			action = "DISPLAY"
		}
		child := ical.NewComponent(ical.CompAlarm)
		child.Props.SetText(ical.PropAction, action)
		trigger := ical.NewProp(ical.PropTrigger)
		trigger.SetValueType(ical.ValueDuration)
		trigger.Value = formatICalDuration(alarm.Offset)
		child.Props.Set(trigger)
		if action == "DISPLAY" {
			child.Props.SetText(ical.PropDescription, "This is an event reminder")
		}
		comp.Children = append(comp.Children, child)
	}
}

func formatICalDuration(d time.Duration) string {
	neg := d < 0
	if neg {
		d = -d
	}
	total := int64(d / time.Second)
	days := total / int64((24 * time.Hour / time.Second))
	total -= days * int64((24*time.Hour)/time.Second)
	hours := total / int64((time.Hour / time.Second))
	total -= hours * int64((time.Hour)/time.Second)
	minutes := total / int64((time.Minute / time.Second))
	total -= minutes * int64((time.Minute)/time.Second)
	seconds := total

	prefix := ""
	if neg {
		prefix = "-"
	}
	return fmt.Sprintf("%sP%dDT%dH%dM%dS", prefix, days, hours, minutes, seconds)
}

func addEventExceptionDate(comp *ical.Component, start time.Time, allDay bool) {
	prop := ical.NewProp(ical.PropExceptionDates)
	if allDay {
		prop.SetDate(start)
	} else {
		prop.SetDateTime(start.UTC())
	}
	comp.Props.Add(prop)
}

func truncateEventRecurrence(comp *ical.Component, until time.Time) {
	opt, err := comp.Props.RecurrenceRule()
	if err != nil || opt == nil {
		return
	}
	opt.Until = until.UTC()
	opt.Count = 0
	comp.Props.SetRecurrenceRule(opt)
}

func readCalendarFile(path string) (*ical.Calendar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cal, err := ical.NewDecoder(f).Decode()
	if err != nil {
		return nil, err
	}
	return cal, nil
}

func writeCalendarFile(path string, cal *ical.Calendar) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	return ical.NewEncoder(out).Encode(cal)
}

func moveCalendarComponents(sourcePath, targetDir, componentName, uid string) error {
	if strings.TrimSpace(sourcePath) == "" {
		return errors.New("source calendar file is required")
	}
	if strings.TrimSpace(targetDir) == "" {
		return errors.New("target calendar directory is required")
	}
	sourceCal, err := readCalendarFile(sourcePath)
	if err != nil {
		return err
	}

	moved := make([]*ical.Component, 0, 1)
	remaining := sourceCal.Children[:0]
	for _, child := range sourceCal.Children {
		if child == nil || child.Name != componentName {
			remaining = append(remaining, child)
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID != uid {
			remaining = append(remaining, child)
			continue
		}
		moved = append(moved, child)
	}
	if len(moved) == 0 {
		return fmt.Errorf("component with uid %q not found", uid)
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
	if strings.TrimSpace(filepath.Base(sourcePath)) == "" || filepath.Ext(targetPath) == "" {
		targetPath = filepath.Join(targetDir, uid+".ics")
	}
	if sameFilePath(sourcePath, targetPath) {
		return nil
	}

	targetCal := cloneCalendarShell(sourceCal)
	if existing, err := readCalendarFile(targetPath); err == nil {
		targetCal = existing
		targetCal.Children = removeComponentsByUID(targetCal.Children, componentName, uid)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	targetCal.Children = append(targetCal.Children, moved...)
	if err := writeCalendarFile(targetPath, targetCal); err != nil {
		return err
	}

	sourceCal.Children = remaining
	if len(sourceCal.Children) == 0 {
		return os.Remove(sourcePath)
	}
	return writeCalendarFile(sourcePath, sourceCal)
}

func cloneCalendarShell(cal *ical.Calendar) *ical.Calendar {
	out := ical.NewCalendar()
	if cal != nil {
		out.Props = cal.Props
	}
	if out.Props.Get(ical.PropVersion) == nil {
		out.Props.SetText(ical.PropVersion, "2.0")
	}
	if out.Props.Get(ical.PropProductID) == nil {
		out.Props.SetText(ical.PropProductID, "-//go-khal//EN")
	}
	return out
}

func removeComponentsByUID(children []*ical.Component, componentName, uid string) []*ical.Component {
	out := children[:0]
	for _, child := range children {
		if child == nil || child.Name != componentName {
			out = append(out, child)
			continue
		}
		entryUID, _ := child.Props.Text(ical.PropUID)
		if entryUID == uid {
			continue
		}
		out = append(out, child)
	}
	return out
}

func sameFilePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA == nil {
		a = aa
	}
	if errB == nil {
		b = bb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func setEventURL(comp *ical.Component, raw string) {
	comp.Props.Del(ical.PropURL)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return
	}
	if u, err := url.Parse(trimmed); err == nil {
		comp.Props.SetURI(ical.PropURL, u)
		return
	}
	comp.Props.SetText(ical.PropURL, trimmed)
}

func setEventTimeProps(comp *ical.Component, start, end time.Time, allDay bool) {
	comp.Props.Del(ical.PropDateTimeStart)
	comp.Props.Del(ical.PropDateTimeEnd)
	if allDay {
		startDate := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
		endDate := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
		if !endDate.After(startDate) {
			endDate = startDate.Add(24 * time.Hour)
		}
		comp.Props.SetDate(ical.PropDateTimeStart, startDate)
		comp.Props.SetDate(ical.PropDateTimeEnd, endDate)
		return
	}
	comp.Props.SetDateTime(ical.PropDateTimeStart, start.UTC())
	comp.Props.SetDateTime(ical.PropDateTimeEnd, end.UTC())
}

func (s *Store) SetCalendarHidden(sourceName, calendarName string, hidden bool) error {
	if s.config == nil {
		return errors.New("missing config")
	}
	src := s.config.SourceByName(sourceName)
	if src == nil {
		return fmt.Errorf("source %q not found", sourceName)
	}
	src.UpsertCalendar(config.CalendarConfig{Name: calendarName, Hidden: hidden})
	return nil
}

func (s *Store) findWritableCalendar(sourceName, calendarName string) (Calendar, *time.Location, error) {
	if len(s.config.Sources) == 0 {
		return Calendar{}, nil, errors.New("no sources configured")
	}

	selectedSource := s.config.Sources[0]
	if sourceName != "" {
		src := s.config.SourceByName(sourceName)
		if src == nil {
			return Calendar{}, nil, fmt.Errorf("source %q not found", sourceName)
		}
		selectedSource = *src
	}

	loc := time.Local
	if selectedSource.DefaultTZName != "" {
		if srcLoc, err := time.LoadLocation(selectedSource.DefaultTZName); err == nil {
			loc = srcLoc
		}
	}

	resolved, err := s.resolveCalendarSources(selectedSource)
	if err != nil {
		return Calendar{}, nil, err
	}
	if len(resolved) == 0 {
		return Calendar{}, nil, fmt.Errorf("source %q has no writable calendar", selectedSource.Name)
	}

	if calendarName == "" {
		return resolved[0].calendar, loc, nil
	}
	for _, cal := range resolved {
		if cal.calendar.Name == calendarName {
			return cal.calendar, loc, nil
		}
	}
	return Calendar{}, nil, fmt.Errorf("calendar %q not found in source %q", calendarName, selectedSource.Name)
}

func emptyDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}
