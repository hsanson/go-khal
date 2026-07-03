# go-khal

`go-khal` is a terminal calendar and task manager inspired by `pimutils/khal`.
It reads calendars and todos from local vdir directories (for example synced by `vdirsyncer`) and renders an interactive agenda with calendar visibility controls, event details, and task management.

## Status

`go-khal` is still under active development. Use it at your own risk, especially when editing or deleting events and tasks. These operations write directly to local `.ics` files and bugs may result in data loss. Keep backups or use versioned/synced calendar directories before trying write operations.

This project is fully vibe coded using Codex.

## Features

- Cobra-based CLI with agenda, TUI, config, and todo workflows
- Bubble Tea TUI with keyboard-driven agenda navigation and calendar toggles
- Google Calendar-like TUI layout with week overview, agenda list, and details pane
- Infinite backward/forward agenda navigation, page movement, and tasks-only/free-time modes
- Event and VTODO create/edit/delete support from the TUI
- Itemized event and task editors with compact popup controls
- Event attendees, notifications, recurrence, all-day, URL, location, and description editing
- VTODO list/show/create/edit support from the CLI
- Configurable multiple sources as either account parent folder or single calendar folder
- Per-calendar metadata (display name, color) including discovery from `displayname`/`color` files
- Per-calendar show/hide controls to include/exclude all events and todos
- iCalendar parsing/writing via `github.com/emersion/go-ical`
- vCard parsing integration via `github.com/emersion/go-vcard`
- Interactive forms using Charm `bubbletea`, `bubbles`, `huh`, and `lipgloss`
- Optional Nerd Font glyphs for the richest TUI rendering

## Installation

Requirements:

- Go 1.24.2 or newer
- A terminal that supports color and alternate screen applications
- A Nerd Font-compatible terminal font is recommended
- `$EDITOR` or `$VISUAL` is used for description editing with `ctrl+e`; if neither is set, `nano` is used
- Local vdir calendar/task data, commonly synced by `vdirsyncer`

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

- `?`: toggle shortcut help
- `q`, `ctrl+c`: quit
- `j/k`, `up/down`: move through agenda items
- `ctrl+f`, `ctrl+b`: page down/up through agenda items
- `h/l`, `left/right`: previous/next day
- `ctrl+h`, `ctrl+l`: previous/next week
- `t`: jump to today
- `enter`, `space`: focus/unfocus details
- `ctrl+j`, `ctrl+k`: scroll details down/up
- `f`: toggle free-time rows
- `m`: toggle tasks-only mode
- `c`: open the calendar visibility pane
- `n`: create a new event, or a new task when tasks-only mode is active
- `e`: edit the selected event or task
- `ctrl+d`: delete the selected event or task

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
  "sidebar_width": 30,
  "recurrence_lookback_months": 12,
  "recurrence_lookahead_months": 24
}
```

## TUI Editing

The TUI uses itemized editors for events and tasks. Move with `j/k` or `tab`/`shift+tab`, press `enter` to edit the selected item in a popup, and press `ctrl+s` to save the whole event or task. `esc`, `q`, or `ctrl+c` cancel the editor or active popup.

Event editing supports:

- Title and calendar
- Location, URL, and description
- Attendees, including fuzzy add/search from address-book contacts
- Notifications such as `10m before`, `2h before`, `10d before`, or `1d after`
- Recurrence: daily, weekly, monthly, yearly, interval, weekdays, monthly mode, until date, and fixed count
- All-day and timed start/end values

Task editing supports title, calendar, description, location, start/due times, completion, and priority.

In description popups, `ctrl+e` opens `$EDITOR`/`$VISUAL` for larger edits. In multiselect popups, `space` or `x` toggles selections. Attendee add/search supports `/` filtering and `enter` applies the filter.

## Notes

- If a source path has calendar subfolders, each subfolder is treated as a separate calendar.
- If a source path contains `.ics` files directly, it is treated as a single calendar.
- Calendar display name/color are read from `displayname` or `.displayname`, and `color` or `.color` when present.
- Hidden calendars are excluded from agenda, details, editor lists, and todo listings.
- VEVENT and VTODO entries are created/updated/deleted directly in source `.ics` files.
- Address-book `.vcf` files are parsed for attendee suggestions.
- Notifications are written as display alarms.
