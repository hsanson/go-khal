# go-khal

`go-khal` is a terminal calendar and task manager inspired by `pimutils/khal`.
It reads calendars and todos from local vdir directories (for example synced by
`vdirsyncer`) and renders an interactive agenda with calendar visibility
controls, event details, and task management.

## Status

Use `go-khal` at your own risk, especially when editing or deleting events and
tasks. These operations write directly to local `.ics` files and bugs may result
in data loss. Keep backups or use versioned/synced calendar directories before
trying destructive operations. I use it personally with Radicale and Google calendars
without issues but cannot guarantee there won't be issues with other calendars.

## Features

- Keyboard- and mouse-driven terminal calendar with months list, agenda, details pane, and calendar toggles
- Minimap showing overview weekly events. Allows quickly spot open/occupied time slots.
- Separate agenda and task modes, agenda page movement, and show-all mode
- Event and task create/edit/delete support from the interactive calendar
- Event attendees auto-completion from VCARD contacts.
- Birthday events from VCARD contacts.
- Configurable calendar and addressbook sources that point directly at vdirsyncer folders
- Per-calendar metadata (display name, color) including discovery from `displayname`/`color` files
- Per-calendar show/hide controls to include/exclude all events and todos
- Optional Nerd Font glyphs for the richest terminal rendering
- Automatic Omarchy palette support with live theme reloads on Linux Omarchy
  Quattro; application surfaces keep the terminal's default background

## Installation

Requirements:

- Local vdir calendar/task/contact data, commonly synced by `vdirsyncer`
- A terminal that supports color and alternate screen applications
- A Nerd Font-compatible terminal font is recommended
- `$EDITOR` or `$VISUAL` is used for description editing with `ctrl+e`; if neither is set, `nano` is used

### Install From Release Binaries

Download the archive for your platform from the [GitHub releases page](https://github.com/hsanson/go-khal/releases).

Release artifacts are named by version, OS, and CPU architecture, for example `go-khal_v0.0.9_linux_amd64.tar.gz`, `go-khal_v0.0.9_darwin_arm64.tar.gz`, and `go-khal_v0.0.9_windows_amd64.zip`.

Linux x86_64 example:

```bash
curl -LO https://github.com/hsanson/go-khal/releases/download/v0.0.9/go-khal_v0.0.9_linux_amd64.tar.gz
curl -LO https://github.com/hsanson/go-khal/releases/download/v0.0.9/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf go-khal_v0.0.9_linux_amd64.tar.gz
install -m 0755 go-khal ~/.local/bin/go-khal
```

Replace `v0.0.9` with the release version you want. Release pages include checksums in `SHA256SUMS` for verification.

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

Import an iCalendar attachment non-interactively from stdin:

```bash
go-khal import < invite.ics
go-khal import -a work < invite.ics
```

This works with NeoMutt's `<pipe-entry>go-khal import<enter>`. Use
`<pipe-entry>go-khal import -a work<enter>` to choose the destination calendar.
Without `-a`, new event UIDs are added to the first configured calendar.
Existing UIDs are updated in their current calendars.

Import from a mailcap entry:

```mailcap
text/calendar; go-khal import "%s"; needsterminal
```

A file containing multiple events is imported non-interactively. For a
single-event file, go-khal opens a prefilled create form, or a prefilled edit
form when that UID already exists. The `needsterminal` flag lets NeoMutt run
that interactive form correctly. Do not use `copiousoutput`: it is intended
for commands whose text output should be displayed in NeoMutt's pager.

Launch the interactive calendar:

```bash
go-khal
```

Common agenda and task shortcuts appear in the bottom legend. Less common
navigation, paging, and detail-pane shortcuts appear in contextual shortcut
help `?`.

Open directly in task mode:

```bash
go-khal todo
```

Create a task directly in the same editor used by the interactive calendar:

```bash
go-khal todo new
```

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
  "minimap_start_time": "08:00",
  "minimap_end_time": "18:00",
  "recurrence_lookback_months": 12,
  "recurrence_lookahead_months": 24
}
```

## Notes

- Source paths must be absolute paths to folders that directly contain `.ics` or `.vcf` files.
- go-khal does not recurse into source subfolders. Configure each concrete calendar or addressbook folder as its own source.
- `go-khal config from-vdirsyncer` reads vdirsyncer local filesystem storages and adds each discovered concrete vdir folder as a source.
- Calendar display name/color are read from `displayname` or `.displayname`, and `color` or `.color` when present.
- Hidden calendars are excluded from agenda, details, editor lists, and todo listings.
- Events and tasks are created/updated/deleted directly in source `.ics` files.
- Address-book `.vcf` files are parsed for attendee suggestions.
- Notifications are written as display alarms.
