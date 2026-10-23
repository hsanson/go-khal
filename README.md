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
- Minimap with a weekly overview: click events to jump to them, or click empty time to stage a one-hour slot for `n`
- Separate agenda and task modes, agenda page movement, and show-all mode
- Event and task create/edit/delete support from the interactive calendar, with
  confirmed destructive actions in edit forms
- Event attendees auto-completion from VCARD contacts.
- Birthday events from VCARD contacts.
- Configurable calendar and addressbook sources that point directly at vdirsyncer folders
- Per-calendar metadata (display name, color) including discovery from `displayname`/`color` files
- Per-calendar show/hide controls to include/exclude all events and todos
- Optional Nerd Font glyphs for the richest terminal rendering
- Automatic Omarchy palette support with live theme reloads on Linux Omarchy
  Quattro; application surfaces keep the terminal's default background, and
  form actions use palette-defined focus states

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

Print the application and build toolchain versions:

```bash
go-khal version
```

## Quick Start


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

The workspace contains four panes: Calendar, Agenda/Tasks, Details, and
Calendars. `Tab` moves clockwise between panes, and `Shift-Tab` moves
counterclockwise. The active pane has a highlighted border.

- Calendar: use `h/j/k/l` to move left/down/up/right, `Enter` to focus the
  selected date in Agenda/Tasks, and `n` to create an item.
- Agenda/Tasks: use `Enter` to focus Details, `e` to edit, `n` to create, and
  `h` to return to Calendar. In Tasks mode, `Space` toggles completion.
- Details: use `e` to edit and `n` to create.
- Calendars: use `j/k` to select a calendar and `Space` to hide or show it.

Open directly in task mode:

```bash
go-khal todo
```

Create a task directly in the same editor used by the interactive calendar:

```bash
go-khal todo new
```

## Omarchy Calendar

Omarchy Calendar looks for an executable named `khal`. If the original khal is
not installed, link go-khal under that name:

```bash
mkdir -p ~/.local/bin
ln -s "$(command -v go-khal)" ~/.local/bin/khal
```

Ensure `~/.local/bin` is in the environment inherited by Quickshell. The
`printformats` and JSON `list` commands implement only the argument forms used
by [omarchy-calendar](https://github.com/derekross/omarchy-calendar), not the
complete khal CLI.

## Screenshots

### Agenda with weekly minimap

![Agenda with Personal, Work, and Birthdays calendars and a weekly minimap](img/agenda-minimap.png)

### Todo view

![Todo view with tasks from Personal and Work calendars](img/todo-view.png)

### Event editor

![Event edit form](img/event-edit.png)

### Task editor

![Task edit form](img/task-edit.png)

## Configuration

The canonical configuration is `${XDG_CONFIG_HOME:-$HOME/.config}/khal/config`,
shared with khal and Omarchy Calendar:

```ini
[calendars]
[[personal]]
path = ~/.local/share/calendars/personal
color = '#4caf50'
addresses = user@example.com

[[work]]
path = ~/.local/share/calendars/work
readonly = true

[[contacts]]
path = ~/.local/share/contacts/personal
type = birthdays

[locale]
firstweekday = 0
timeformat = %H:%M
dateformat = %Y-%m-%d
longdateformat = %Y-%m-%d
datetimeformat = %Y-%m-%d %H:%M
longdatetimeformat = %Y-%m-%d %H:%M

[go-khal]
default_view = agenda
sidebar_width = 30
minimap_start_time = 08:00
minimap_end_time = 18:00
recurrence_lookback_months = 12
recurrence_lookahead_months = 24
hidden_calendar_paths =
```

Supported calendar types are `calendar`, `discover`, and `birthdays`.
`discover` expands khal-style glob paths, including `**`. Calendar `readonly`
and `addresses` settings are honored. Unused khal sections and fields are
ignored and preserved when go-khal updates the file.

`firstweekday` accepts khal values `0` (Monday) through `6` (Sunday). Consumed
date/time formats support `%%`, `%a`, `%A`, `%b`, `%B`, `%c`, `%d`, `%e`, `%F`,
`%H`, `%I`, `%j`, `%m`, `%M`, `%p`, `%r`, `%R`, `%S`, `%T`, `%x`, `%X`, `%y`,
`%Y`, `%z`, `%Z`, `%n`, and `%t`, plus supported `-`, `_`, and `0` padding
forms. Unsupported directives produce an actionable error instead of silently
rendering the wrong value. go-khal currently uses the host timezone and ignores
khal's custom `local_timezone` and `default_timezone` settings.

The former `~/.config/go-khal/config.json` format is deprecated and scheduled
for removal in a future release. When the canonical khal config is absent,
go-khal reads that JSON file without modifying it. The next config-mutating
command creates the canonical khal config and leaves the JSON file untouched.

## Notes

- Paths expand `~` and environment variables. Direct `calendar` and `birthdays`
  paths name one folder; `discover` paths expand matching calendar folders.
- `go-khal config from-vdirsyncer` upserts discovered concrete vdir folders
  without removing unmatched khal calendars.
- Calendar display name/color are read from `displayname` or `.displayname`,
  and `color` or `.color` when present.
- Hidden calendars are excluded from agenda, details, editor lists, todos, and
  Omarchy JSON output.
- Readonly calendars remain visible but are excluded from every write path.
- Events and tasks are created, updated, and deleted directly in source `.ics`
  files.
- Birthday `.vcf` files are also used for attendee suggestions.
- Notifications are written as display alarms.
