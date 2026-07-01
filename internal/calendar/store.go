package calendar

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"
	"github.com/hsanson/go-khal/internal/config"
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
			_ = s.loadAddressBook(src.AddressBook)
		}
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

	var allEvents []Event
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
				events := s.componentToEvents(child, src, filePath)
				allEvents = append(allEvents, events...)
			case ical.CompToDo:
				todo, ok := componentToTodo(child, src, filePath)
				if ok {
					allTodos = append(allTodos, todo)
				}
			}
		}
	}

	return allEvents, allTodos, nil
}

func (s *Store) componentToEvents(comp *ical.Component, src calendarSource, filePath string) []Event {
	uid, err := comp.Props.Text(ical.PropUID)
	if err != nil || uid == "" {
		return nil
	}
	summary, _ := comp.Props.Text(ical.PropSummary)
	desc, _ := comp.Props.Text(ical.PropDescription)
	location, _ := comp.Props.Text(ical.PropLocation)
	organizer, _ := comp.Props.Text(ical.PropOrganizer)
	start, err := comp.Props.DateTime(ical.PropDateTimeStart, src.location)
	if err != nil {
		return nil
	}
	end, err := comp.Props.DateTime(ical.PropDateTimeEnd, src.location)
	if err != nil {
		end = start.Add(time.Hour)
	}
	duration := end.Sub(start)
	if duration <= 0 {
		duration = time.Hour
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
		UID:         uid,
		Summary:     emptyDefault(summary, "(untitled event)"),
		Description: desc,
		Location:    location,
		Organizer:   organizer,
		AllDay:      allDay,
		Recurring:   recurring,
		HasAlarm:    hasAlarm,
		Source:      src.sourceName,
		Calendar:    src.calendar.Name,
		CalendarDir: src.calendar.Path,
		DisplayName: src.calendar.DisplayName,
		Color:       src.calendar.Color,
		Hidden:      src.calendar.Hidden,
		FilePath:    filePath,
	}

	if !recurring {
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
	for _, occ := range occs {
		e := base
		e.Start = occ
		e.End = occ.Add(duration)
		result = append(result, e)
	}
	return result
}

func componentToTodo(comp *ical.Component, src calendarSource, filePath string) (Todo, bool) {
	uid, err := comp.Props.Text(ical.PropUID)
	if err != nil || uid == "" {
		return Todo{}, false
	}

	summary, _ := comp.Props.Text(ical.PropSummary)
	desc, _ := comp.Props.Text(ical.PropDescription)
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

	return Todo{
		UID:         uid,
		Summary:     emptyDefault(summary, "(untitled todo)"),
		Description: desc,
		Status:      emptyDefault(status, "NEEDS-ACTION"),
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

func (s *Store) loadAddressBook(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".vcf" {
			continue
		}
		fullPath := filepath.Join(path, entry.Name())
		if err := readVCardFile(fullPath); err != nil {
			continue
		}
	}
	return nil
}

func readVCardFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := vcard.NewDecoder(f)
	for {
		_, err := dec.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
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
	comp.Props.SetText(ical.PropStatus, emptyDefault(t.Status, "NEEDS-ACTION"))
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
		if update.Status != nil {
			child.Props.SetText(ical.PropStatus, *update.Status)
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
