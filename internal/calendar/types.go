package calendar

import "time"

type Event struct {
	UID          string
	Summary      string
	Description  string
	Location     string
	URL          string
	Organizer    string
	Attendees    []Attendee
	Availability string
	Visibility   string
	Recurrence   *Recurrence
	Alarms       []Alarm
	Start        time.Time
	End          time.Time
	AllDay       bool
	Kind         string
	Recurring    bool
	HasAlarm     bool
	Source       string
	Calendar     string
	CalendarDir  string
	DisplayName  string
	Color        string
	Hidden       bool
	FilePath     string
}

type Attendee struct {
	Name   string
	Email  string
	Status string
	RSVP   bool
	Role   string
}

type Recurrence struct {
	Frequency    string
	Interval     int
	Weekdays     []string
	MonthlyBy    string
	MonthDay     int
	MonthWeekday string
	MonthWeek    int
	Until        *time.Time
	Count        int
}

type Alarm struct {
	Offset time.Duration
	Action string
}

const (
	EventKindBirthday    = "birthday"
	EventKindAnniversary = "anniversary"

	SpecialSourceBirthdays   = "__special__"
	SpecialCalendarBirthdays = "birthdays-anniversaries"
)

type EventUserRole string

const (
	EventUserRoleUnknown   EventUserRole = ""
	EventUserRoleOrganizer EventUserRole = "organizer"
	EventUserRoleAttendee  EventUserRole = "attendee"
	EventUserRoleLocal     EventUserRole = "local"
)

type Todo struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Status      string
	Priority    int
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
	Email       string
	DisplayName string
	Color       string
	Hidden      bool
}

type TodoUpdate struct {
	Summary     *string
	Description *string
	Location    *string
	Status      *string
	Priority    *int
	Start       **time.Time
	Due         **time.Time
	Percent     *int
}

type EventUpdate struct {
	Summary      *string
	Description  *string
	Location     *string
	URL          *string
	Organizer    *string
	Attendees    *[]Attendee
	Availability *string
	Visibility   *string
	Recurrence   **Recurrence
	Alarms       *[]Alarm
	Start        *time.Time
	End          *time.Time
	AllDay       *bool
}

type Contact struct {
	Name  string
	Email string
}

type DeleteRecurringScope string

const (
	DeleteRecurringAll        DeleteRecurringScope = "all"
	DeleteRecurringOccurrence DeleteRecurringScope = "occurrence"
	DeleteRecurringFuture     DeleteRecurringScope = "future"
)

type EditRecurringScope string

const (
	EditRecurringAll        EditRecurringScope = "all"
	EditRecurringOccurrence EditRecurringScope = "occurrence"
	EditRecurringFuture     EditRecurringScope = "future"
)
