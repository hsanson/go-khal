package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListCommandEmitsOmarchyJSONForRequestedRange(t *testing.T) {
	root := t.TempDir()
	calendarPath := filepath.Join(root, "personal")
	if err := os.Mkdir(calendarPath, 0o755); err != nil {
		t.Fatal(err)
	}
	ics := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:daily@example.test
DTSTART:20350101T100000
DTEND:20350101T110000
RRULE:FREQ=DAILY;COUNT=2
SUMMARY:Daily meeting
STATUS:TENTATIVE
END:VEVENT
BEGIN:VEVENT
UID:holiday@example.test
DTSTART;VALUE=DATE:20350101
DTEND;VALUE=DATE:20350102
SUMMARY:Holiday
END:VEVENT
END:VCALENDAR
`
	if err := os.WriteFile(filepath.Join(calendarPath, "events.ics"), []byte(ics), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config")
	configBody := `[calendars]
[[Personal]]
path = ` + calendarPath + `
color = '#123456'

[locale]
timeformat = %H:%M
longdateformat = %Y-%m-%d
`
	if err := os.WriteFile(configPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}

	oldConfigPath := cfgPath
	cfgPath = configPath
	t.Cleanup(func() { cfgPath = oldConfigPath })

	args := make([]string, 0, len(omarchyJSONFields)*2+3)
	for _, field := range omarchyJSONFields {
		args = append(args, "--json", field)
	}
	args = append(args, "--day-format=", "2035-01-01", "2d")
	command := newListCommand()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}

	var rows []khalJSONEvent
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("decode list output: %v\n%s", err, stdout.String())
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %s", len(rows), stdout.String())
	}
	var holiday, recurring *khalJSONEvent
	for index := range rows {
		switch rows[index].UID {
		case "holiday@example.test":
			holiday = &rows[index]
		case "daily@example.test":
			recurring = &rows[index]
		}
	}
	if holiday == nil || holiday.AllDay != "True" || holiday.EndDateLong != "2035-01-01" || holiday.EndTime != "" {
		t.Fatalf("all-day output is not khal-compatible: %+v", holiday)
	}
	if recurring == nil || recurring.Status != "TENTATIVE" || recurring.RepeatPattern != "FREQ=DAILY;COUNT=2" {
		t.Fatalf("recurring output lost status/RRULE: %+v", recurring)
	}
}

func TestPrintFormatsUsesKhalLocaleFormats(t *testing.T) {
	root := t.TempDir()
	calendarPath := filepath.Join(root, "personal")
	if err := os.Mkdir(calendarPath, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config")
	body := `[calendars]
[[personal]]
path = ` + calendarPath + `

[locale]
longdatetimeformat = %Y-%m-%d %I:%M %p
datetimeformat = %Y-%m-%d %H:%M
longdateformat = %Y-%m-%d
dateformat = %d/%m/%Y
timeformat = %I:%M %p
`
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	oldConfigPath := cfgPath
	cfgPath = configPath
	t.Cleanup(func() { cfgPath = oldConfigPath })
	command := newPrintFormatsCommand()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"longdatetimeformat: 2013-12-21 09:45 PM",
		"datetimeformat: 2013-12-21 21:45",
		"longdateformat: 2013-12-21",
		"dateformat: 21/12/2013",
		"timeformat: 09:45 PM",
		"",
	}, "\n")
	if stdout.String() != want {
		t.Fatalf("printformats output:\n%s\nwant:\n%s", stdout.String(), want)
	}
}
