# go-khal

`go-khal` is a terminal calendar and task manager inspired by `pimutils/khal`.
It reads calendars and todos from local vdir directories (for example synced by `vdirsyncer`) and renders agenda/day/week/month/year views.

## Features

- Cobra-based CLI with commands for agenda, day, week, month, year and todo workflows
- Bubble Tea TUI with keyboard-driven navigation between views
- Google Calendar-like TUI layout with left sidebar and main view area
- 24-hour day and week timelines with events placed by start hour
- Week and 5-day workweek panes (7-day and weekday-only)
- VTODO list/show/create/edit support
- Configurable multiple sources as either account parent folder or single calendar folder
- Per-calendar metadata (display name, color) including discovery from `displayname`/`color` files
- Per-calendar show/hide controls to include/exclude all events and todos
- iCalendar parsing/writing via `github.com/emersion/go-ical`
- vCard parsing integration via `github.com/emersion/go-vcard`
- Interactive forms for todo creation/editing using `huh` and `bubbles`

## Installation

```bash
go build ./...
```

## Quick Start

Initialize config:

```bash
go run . config init
```

Add a vdir source (account folder containing multiple calendar folders):

```bash
go run . config add-source --name personal --path /path/to/vdir/account --address-book /path/to/vdir/contacts
```

Add or override one specific calendar config:

```bash
go run . config add-calendar --source personal --name birthdays --display-name "Birthdays" --color "#ff9800"
```

List calendars and visibility:

```bash
go run . config list-calendars
```

Hide/show one calendar:

```bash
go run . config hide-calendar --source personal --name birthdays
go run . config show-calendar --source personal --name birthdays
```

Show agenda:

```bash
go run . agenda
```

Launch TUI:

```bash
go run . tui
```

TUI shortcuts:

- Views: `d` day, `w` week, `5` workweek, `m` month, `y` year
- Move previous/next period: `h/l` or `p/n`
- Jump previous/next month: `[` / `]`
- Jump previous/next year: `{` / `}`
- Toggle focus between sidebar/main: `tab`
- Toggle calendar show/hide in sidebar: `space` or `enter`

Create a todo interactively:

```bash
go run . todo new
```

## Configuration

Default config path: `~/.config/go-khal/config.json`

Example:

```json
{
  "sources": [
    {
      "name": "personal",
      "path": "/home/user/.local/share/calendars/personal",
      "address_book": "/home/user/.local/share/contacts/personal",
      "default_timezone": "Europe/Paris",
      "calendars": [
        {
          "name": "birthdays",
          "display_name": "Birthdays",
          "color": "#ff9800",
          "hidden": true
        },
        {
          "name": "calendar",
          "display_name": "Personal",
          "color": "#4caf50"
        }
      ]
    },
    {
      "name": "work",
      "path": "/home/user/.local/share/calendars/work"
    }
  ],
  "default_view": "agenda",
  "week_starts_on": "monday",
  "time_format": "15:04",
  "sidebar_width": 30
}
```

## Notes

- If a source path has calendar subfolders, each subfolder is treated as a separate calendar.
- If a source path contains `.ics` files directly, it is treated as a single calendar.
- Calendar display name/color are read from `displayname` or `.displayname`, and `color` or `.color` when present.
- Hidden calendars are excluded from all views and todo listings.
- VTODO entries are created/updated directly in source `.ics` files.
- Address-book `.vcf` files are parsed for integration readiness.
