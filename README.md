# go-khal

`go-khal` is a terminal calendar and task manager inspired by `pimutils/khal`.
It reads calendars and todos from local vdir directories (for example synced by `vdirsyncer`) and renders an interactive agenda with calendar visibility controls, event details, and task management.

## Status

`go-khal` is still under active development. Use it at your own risk, especially when editing or deleting events and tasks. These operations write directly to local `.ics` files and bugs may result in data loss. Keep backups or use versioned/synced calendar directories before trying write operations.

This project is fully vibe coded using Codex.

## Features

- Keyboard-driven terminal calendar with agenda, week overview, details pane, and calendar toggles
- Separate agenda and task modes, agenda page movement, and show-all mode
- Event and task create/edit/delete support from the interactive calendar
- Itemized event and task editors with compact popup controls
- Event attendees, notifications, recurrence, all-day, URL, location, and description editing
- Task list/show/create/edit support from the CLI
- Configurable calendar and addressbook sources that point directly at vdir folders
- Per-calendar metadata (display name, color) including discovery from `displayname`/`color` files
- Per-calendar show/hide controls to include/exclude all events and todos
- Optional Nerd Font glyphs for the richest terminal rendering

## Installation

Requirements:

- Local vdir calendar/task data, commonly synced by `vdirsyncer`
- A terminal that supports color and alternate screen applications
- A Nerd Font-compatible terminal font is recommended
- `$EDITOR` or `$VISUAL` is used for description editing with `ctrl+e`; if neither is set, `nano` is used

### Install From Release Binaries

Download the archive for your platform from the [GitHub releases page](https://github.com/hsanson/go-khal/releases).

Release artifacts are named by version, OS, and CPU architecture, for example `go-khal_v0.0.1_linux_amd64.tar.gz`, `go-khal_v0.0.1_darwin_arm64.tar.gz`, and `go-khal_v0.0.1_windows_amd64.zip`.

Linux x86_64 example:

```bash
curl -LO https://github.com/hsanson/go-khal/releases/download/v0.0.1/go-khal_v0.0.1_linux_amd64.tar.gz
curl -LO https://github.com/hsanson/go-khal/releases/download/v0.0.1/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf go-khal_v0.0.1_linux_amd64.tar.gz
install -m 0755 go-khal ~/.local/bin/go-khal
```

Replace `v0.0.1` with the release version you want. Release pages include checksums in `SHA256SUMS` for verification.

### Install From Source

Source installs require Go 1.24.2 or newer.

```bash
go install github.com/hsanson/go-khal@latest
```

Make sure your Go binary directory, usually `~/go/bin`, is in your `PATH`.

## Quick Start

Initialize config:

```bash
go-khal config init
```

Generate config sources from local vdirsyncer storages:

```bash
go-khal config from-vdirsyncer
```

Pass a config path when vdirsyncer uses a non-default location:

```bash
go-khal config from-vdirsyncer /path/to/vdirsyncer/config
```

Add a calendar or addressbook source manually:

```bash
go-khal config add-source --path /path/to/vdir/calendar --type calendar --display-name "Personal" --color "#4caf50" --email user@example.com
go-khal config add-source --path /path/to/vdir/addressbook --type addressbook --display-name "Contacts"
```

List calendars and visibility:

```bash
go-khal config list-calendars
```

Hide/show one calendar:

```bash
go-khal config hide-calendar --path /path/to/vdir/calendar
go-khal config show-calendar --path /path/to/vdir/calendar
```

Show agenda in plain text:

```bash
go-khal agenda
go-khal agenda 10
go-khal agenda --birthdays 10
go-khal agenda --max-length 80
```

Launch the interactive calendar:

```bash
go-khal
```

Keyboard shortcuts:

- `?`: toggle shortcut help
- `q`, `ctrl+c`: quit
- `j/k`, `up/down`: move through agenda items
- `ctrl+f`, `ctrl+b`: page down/up through agenda items
- `h/l`, `left/right`: previous/next day
- `ctrl+h`, `ctrl+l`: previous/next week
- `t`: jump to today
- `enter`, `space`: focus/unfocus details
- `ctrl+j`, `ctrl+k`: scroll details down/up
- `f`: toggle show-all mode with free slots and declined events in agenda mode; completed tasks in task mode
- `m`: toggle task mode
- `c`: open the calendar visibility pane
- `n`: create a new event, or a new task when task mode is active
- `e`: edit the selected event or task
- `ctrl+d`: delete the selected event or task

Open directly in task mode:

```bash
go-khal todo
```

Create a task directly in the same editor used by the interactive calendar:

```bash
go-khal todo new
```

Use `j/k` to move between task fields, `enter` to edit the selected field, `ctrl+s` to save, and `esc`, `q`, or `ctrl+c` to cancel. After saving or canceling, go-khal remains in task mode so the created task can be reviewed or edited.

## Configuration

Default config path: `~/.config/go-khal/config.json`

Example:

```json
{
  "sources": [
    {
      "path": "/home/user/.local/share/calendars/personal",
      "type": "calendar",
      "display_name": "Personal",
      "color": "#4caf50",
      "email": "user@example.com"
    },
    {
      "path": "/home/user/.local/share/calendars/birthdays",
      "type": "calendar",
      "display_name": "Birthdays",
      "color": "#ff9800",
      "hidden": true
    },
    {
      "path": "/home/user/.local/share/contacts/personal",
      "type": "addressbook",
      "display_name": "Contacts"
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

## Editing Events And Tasks

The interactive calendar uses itemized editors for events and tasks. Move with `j/k` or `tab`/`shift+tab`, press `enter` to edit the selected item in a popup, and press `ctrl+s` to save the whole event or task. `esc`, `q`, or `ctrl+c` cancel the editor or active popup.

Event editing supports:

- Title and calendar
- Location, URL, and description
- Attendees, including required/optional roles and fuzzy add/search from address-book contacts
- RSVP, availability, and visibility
- Notifications such as `10m before`, `2h before`, `10d before`, or `1d after`
- Recurrence: daily, weekly, monthly, yearly, interval, weekdays, monthly mode, until date, and fixed count
- All-day and timed start/end values

When an event has attendees, go-khal uses the calendar source `email` as the iCalendar `ORGANIZER`. If `email` is omitted, calendar names that look like email addresses are used as a fallback. Events where the configured email is an attendee but not the organizer are treated as attendee-owned: only local calendar placement, RSVP, availability/visibility, and notifications are editable.

Task editing supports title, calendar, description, location, start/due times, completion, and priority.

In description popups, `ctrl+e` opens `$EDITOR`/`$VISUAL` for larger edits. In multiselect popups, `space` or `x` toggles selections. Attendee add/search supports `/` filtering and `enter` applies the filter.

## Notes

- Source paths must be absolute paths to folders that directly contain `.ics` or `.vcf` files.
- go-khal does not recurse into source subfolders. Configure each concrete calendar or addressbook folder as its own source.
- `go-khal config from-vdirsyncer` reads vdirsyncer local filesystem storages and adds each discovered concrete vdir folder as a source.
- Calendar display name/color are read from `displayname` or `.displayname`, and `color` or `.color` when present.
- Hidden calendars are excluded from agenda, details, editor lists, and todo listings.
- Events and tasks are created/updated/deleted directly in source `.ics` files.
- Address-book `.vcf` files are parsed for attendee suggestions.
- Notifications are written as display alarms.
