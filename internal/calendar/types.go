package calendar

import "time"

type Event struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Organizer   string
	Start       time.Time
	End         time.Time
	AllDay      bool
	Source      string
	Calendar    string
	CalendarDir string
	DisplayName string
	Color       string
	Hidden      bool
	FilePath    string
}

type Todo struct {
	UID         string
	Summary     string
	Description string
	Status      string
	Start       *time.Time
	Due         *time.Time
	Completed   *time.Time
	Percent     int
	Source      string
	Calendar    string
	CalendarDir string
	DisplayName string
	Color       string
	Hidden      bool
	FilePath    string
}

type Dataset struct {
	Events    []Event
	Todos     []Todo
	Calendars []Calendar
}

type Calendar struct {
	Source      string
	Name        string
	Path        string
	DisplayName string
	Color       string
	Hidden      bool
}

type TodoUpdate struct {
	Summary     *string
	Description *string
	Status      *string
	Start       **time.Time
	Due         **time.Time
	Percent     *int
}
