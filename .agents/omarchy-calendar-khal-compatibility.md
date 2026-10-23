# Omarchy Calendar: minimum `khal` CLI compatibility

## Pinned sources and scope

This note uses the Omarchy plugin whose current source invokes both commands named in the task: [`derekross/omarchy-calendar`](https://github.com/derekross/omarchy-calendar), default-branch commit [`0c31e965037f7a388b733bfe9ba15cab36f56e7b`](https://github.com/derekross/omarchy-calendar/commit/0c31e965037f7a388b733bfe9ba15cab36f56e7b).

Original khal is pinned to [`pimutils/khal@3ba0fcef06d5244a4a7330869a8d5f18a0342ce9`](https://github.com/pimutils/khal/commit/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9). The local go-khal source is pinned to [`hsanson/go-khal@0448598cb90d1963bb4b815051620adc9bbcec7e`](https://github.com/hsanson/go-khal/commit/0448598cb90d1963bb4b815051620adc9bbcec7e).

The compatibility target is narrow:

1. an executable named `khal` discoverable through `PATH`;
2. `khal printformats`;
3. the two exact `khal list` shapes below; and
4. stdout and exit behavior that the plugin's parser accepts.

No other khal command or option is needed.

## Exact subprocess argv

The plugin does not execute `khal` directly. Every invocation has this fixed prefix, which makes a missing binary become exit status 127 ([Model.js lines 195–221](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L195-L221)):

```text
["sh", "-c",
 "command -v khal >/dev/null 2>&1 || exit 127; exec khal \"$@\"",
 "khal", ...]
```

### Format discovery

The complete process argv is:

```text
["sh", "-c", SCRIPT, "khal", "printformats"]
```

It runs when the service starts. A successful sync runs it again so changed locale settings take effect ([Service.qml lines 205–233](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L205-L233), [305–327](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L305-L327), and [397–398](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L397-L398)). No list command starts until this succeeds and its output parses.

### Upcoming agenda

For an agenda window of `N` days, the complete logical khal argv is:

```text
khal list \
  --json uid \
  --json title \
  --json calendar \
  --json calendar-color \
  --json location \
  --json all-day \
  --json start-date-long \
  --json start-time \
  --json end-date-long \
  --json end-time \
  --json description \
  --json url \
  --json organizer \
  --json status \
  --json repeat-pattern \
  --day-format "" \
  today Nd
```

`N` defaults to 7 and is clamped to 1–60. The service polls at the next minute boundary plus one second, refreshes when `agendaDays` changes, and refreshes when the panel opens or receives an IPC refresh ([Service.qml lines 57–89](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L57-L89), [348–357](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L348-L357), and [378–394](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L378-L394)).

### Browsed month

For a month grid, the same 15 `--json FIELD` pairs are followed by:

```text
--day-format "" START 48d
```

`START` is six calendar days before the month's first day (`new Date(year, month, -5)`) and is rendered in the `longdateformat` discovered from `printformats`. Forty-eight days cover every six-week grid. Only the newest pending month is launched, and successful results are cached until refresh or sync ([Model.js lines 100–133](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L100-L133), [215–231](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L215-L231), and [Service.qml lines 95–115](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L95-L115)).

The plugin never supplies `-c`, `-a`, `-d`, `--format`, `--once`, `--notstarted`, or color flags.

## Process, environment, config, and exit contract

- `sh` and an executable named exactly `khal` must be in the Quickshell service's inherited `PATH`. Installing only a binary named `go-khal` is insufficient.
- The plugin sets no environment variables and no working directory. `HOME`, `XDG_CONFIG_HOME`, locale variables, timezone state, and cwd are inherited.
- Because no `-c` is passed, original khal discovers its config through the XDG config directories; its common path is `~/.config/khal/config` ([settings.py lines 57–93](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/settings.py#L57-L93)). A compatible executable may use its own default config, but it cannot require an invocation-only config argument.
- Original khal's `local_timezone` controls displayed times and defaults to the computer timezone ([khal.spec lines 83–125](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/khal.spec#L83-L125)). The plugin parses the displayed wall time with JavaScript's current local timezone. Therefore, original khal's `local_timezone` and the desktop timezone must agree to preserve instants. The service calls `Date.timeZoneUpdated()` after successful lists so a live desktop timezone change is observed ([Service.qml lines 243–260](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L243-L260)).
- Only stdout is collected, with `waitForEnd: true`; no plugin code parses stderr.
- Exit 0 is the only success status. `printformats` failure clears known formats and prevents listing. `list` failure preserves the last successful agenda. Exit 127 uniquely means “khal unavailable”; other nonzero statuses become `khal printformats failed (exit N)` or `khal list failed (exit N)` ([Service.qml lines 205–260](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L205-L260)).
- A browsed-month list failure is silent and does not replace that month's cached data; the next pending request still runs ([Service.qml lines 263–286](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Service.qml#L263-L286)).

Original khal exits normally after echoing list rows, and turns its handled fatal list/format errors into a nonzero exit ([cli.py lines 160–204](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/cli.py#L160-L204), [410–432](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/cli.py#L410-L432)). Exact diagnostic text and stderr formatting are not consumed.

## `printformats` stdout

Original khal prints the fixed naive datetime `2013-12-21 21:45` through the configured formats, in this exact key order and with one newline-terminated line per key ([cli.py lines 410–428](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/cli.py#L410-L428); an exact output fixture is in [cli_test.py lines 385–400](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/cli_test.py#L385-L400)):

```text
longdatetimeformat: <2013-12-21 21:45 rendered with longdatetimeformat>
datetimeformat: <rendered with datetimeformat>
longdateformat: <2013-12-21 rendered with longdateformat>
dateformat: <rendered with dateformat>
timeformat: <21:45 rendered with timeformat>
```

The plugin accepts lines matching `^[a-z]+: ?(.*)$`, ignores unknown keys, and requires only nonempty `longdateformat` and `timeformat`. It compiles those two rendered samples into parsers, then verifies that they round-trip to 2013-12-21 and 21:45 ([Model.js lines 65–94](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L65-L94)). Thus:

- The smallest parser-satisfying output is those two lines. Emitting all five lines above matches original khal exactly and costs no extra compatibility surface.
- The date sample must let the plugin identify year, month, and day. Supported forms use four- or two-digit years, numeric months, or English full/abbreviated month names; English weekday names may also appear.
- The time sample may use 24-hour time or 12-hour time with plain ASCII `AM`/`PM`, optionally with seconds. Seconds are parsed but discarded.
- Non-English month names and localized AM/PM markers fail the round-trip and stop all listing. The UI instructs the user to set `%Y-%m-%d` and `%H:%M` ([Model.js lines 84–93](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L84-L93); tested forms are in [model.test.js lines 28–50 and 203–214](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/tests/model.test.js#L28-L50)).
- The list fields must use the same `longdateformat` and `timeformat`. The plugin also uses the date sample to construct the arbitrary `START` positional argument.
- The sample determines the default event display clock: presence of an AM/PM token selects 12-hour display; otherwise it selects 24-hour display. The Omarchy `timeFormat` setting may override that choice ([Model.js lines 75–81 and 440–445](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L75-L81)).

A fixed ISO/24-hour set is therefore sufficient for a minimal compatibility path, provided `list` emits ISO dates and 24-hour times consistently.

## `list` stdout

### Original khal framing

`--json` is repeatable and collects the selected fields in JSON arrays. JSON mode disables ANSI colors. Original khal queries and formats each calendar day separately, so an `N`-day range emits `N` JSON-array lines, including `[]` for an empty day, followed by a final newline. Multi-day events appear in every daily array they overlap because the invocation omits `--once` ([controllers.py lines 242–335](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/controllers.py#L242-L335), [utils.py lines 270–301](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/utils.py#L270-L301)). `--day-format ""` suppresses human headings; JSON mode suppresses them independently. The upstream docs define the accepted JSON fields and the empty day-format behavior ([usage.rst lines 219–266](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/doc/source/usage.rst#L219-L266)).

### What the plugin actually requires

The parser trims stdout by line, considers only lines beginning with `[`, parses each independently as JSON, normalizes every array member, and silently skips non-JSON lines and malformed JSON lines. It then deduplicates and sorts the combined events ([Model.js lines 234–291](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L234-L291)). Therefore empty stdout is accepted for no events; otherwise the minimum framing is one or more newline-delimited JSON arrays. A single array containing all unique events is accepted. Per-day arrays, empty-day arrays, field order, JSON whitespace, and a trailing newline are original-khal behavior but are not parser requirements.

Each requested field has the following contract:

| Field | Original khal value | Plugin use and minimum |
|---|---|---|
| `uid` | iCalendar UID string | Used with calendar and start as the stable occurrence key and for direct `.ics` detail lookup. Empty is accepted, but loses reliable identity and details. |
| `title` | Summary string | Displayed and used in notification titles; empty/missing becomes `(untitled)`. |
| `calendar` | Configured display name, otherwise calendar name | Displayed in expanded details and included in the dedupe key. Empty is accepted. |
| `calendar-color` | Configured color string, without ANSI | Only `#RRGGBB` and `#RRGGBBAA` are used; other values fall back to the Omarchy accent. Eight-digit khal RGBA is converted to QML ARGB. |
| `location` | Trimmed location string | Displayed, added to notifications, and searched for meeting/directions URLs. Empty is accepted. |
| `all-day` | The string `"True"` or `"False"`, not a JSON boolean | Exact `"True"` selects all-day parsing. An all-day row without that exact value is treated as timed and is dropped if `start-time` is empty. |
| `start-date-long` | Start/occurrence date in `longdateformat` | Required for a row to survive. |
| `start-time` | Start/occurrence wall time in `timeformat`; empty for all-day | Required for timed rows. |
| `end-date-long` | End date in `longdateformat` | Needed for correct duration and day coverage; invalid/missing falls back to start date. |
| `end-time` | End wall time in `timeformat`; empty for all-day | Needed for a timed duration; invalid/missing collapses end to start. |
| `description` | Description string | Cleaned from HTML/plain text and linkified in expanded details. Empty is accepted. |
| `url` | Event URL string | Candidate meeting/other link. Empty is accepted. |
| `organizer` | `CN (email)` when CN exists, otherwise email | Parsed for display. Empty is accepted. |
| `status` | Status string such as `CONFIRMED`, `TENTATIVE`, or `CANCELLED` | Uppercased. Cancelled events are struck through and never notified; tentative events are labeled. Empty is accepted. |
| `repeat-pattern` | Raw RRULE string when present, otherwise empty | Only nonemptiness matters; it controls the “Repeats” label. |

Original values come from the event attribute implementation ([event.py lines 605–782](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/event.py#L605-L782)) and its status suffix cleanup in the JSON formatter ([utils.py lines 281–299](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/utils.py#L281-L299)). Plugin consumers are the normalizer ([Model.js lines 259–290](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L259-L290)), notification/agenda/grid logic ([Model.js lines 294–445](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L294-L445)), and event rendering ([Panel.qml lines 1039–1164](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Panel.qml#L1039-L1164), [1138–1339](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Panel.qml#L1138-L1339)).

For seamless behavior, emit all 15 requested keys. For mere row acceptance, only `start-date-long` plus exact `all-day: "True"`, or `start-date-long` plus a parseable timed `start-time`, is mandatory; that narrower output would discard visible functionality and is not recommended.

## Date, time, all-day, and recurrence behavior

- `today Nd` means `N` local calendar days starting at today's midnight. `START 48d` means 48 local calendar days starting at the supplied date. These are the only date-range forms needed. Original khal accepts a much broader grammar, but its documented list range is `START [END | DELTA]`, and a day delta sets end to start plus that delta ([usage.rst lines 285–318](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/doc/source/usage.rst#L285-L318), [parse_datetime.py lines 327–413](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/parse_datetime.py#L327-L413)).
- Timed fields are wall times converted by original khal to its configured `local_timezone`; the plugin constructs local JavaScript dates from those components. It does not parse offsets or timezone names.
- Original khal presents an all-day end date inclusively: a one-day event has equal displayed start and end even though iCalendar `DTEND` is exclusive ([event model lines 48–59](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/event.py#L48-L59), [all-day end lines 923–946](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/event.py#L923-L946)). The plugin restores an exclusive end by adding exactly 86,400,000 ms to the parsed end-date midnight ([Model.js lines 259–271](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L259-L271)). A compatible producer must emit the inclusive last day, not raw iCalendar's exclusive `DTEND` date.
- That fixed 24-hour addition is the plugin's behavior across daylight-saving transitions; compatibility must not pre-adjust it. Timed values follow local JavaScript DST rules when converted to milliseconds.
- Original khal represents each expanded recurrence occurrence as an event instance with its actual occurrence start/end ([event.py lines 48–59](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/event.py#L48-L59)) and expands recurrence into its cache ([backend.py lines 344–380](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/backend.py#L344-L380)). Occurrences share a UID but differ by start. The plugin's key includes UID plus start, so it preserves separate occurrences while removing repeated daily copies of the same multi-day occurrence.
- Events are sorted by start, then all-day before timed when starts tie, then title. Events with an end before start are clamped to zero duration. Agenda view omits events whose exclusive end is at or before now; notifications omit all-day and cancelled events ([Model.js lines 253–305](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L253-L305), [373–423](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L373-L423)).
- Multi-day duplication is not required from a replacement: the plugin expands each normalized event across touched days for grid dots and selected-day overlap. If duplicates are emitted as original khal does, calendar + UID/title + start deduplication removes them.
- Recurrence exclusions and overrides must be reflected in the actual occurrence stream for correct month and agenda results. The plugin does no RRULE expansion itself; `repeat-pattern` is only a label.

## Adjacent non-CLI assumption for expanded details

The 15 JSON fields drive the agenda, grid, notifications, and most details. On expansion, the plugin separately searches `.ics` files by UID to recover attendees and conference metadata. That shell lookup reads `path =` entries only from `${XDG_CONFIG_HOME:-$HOME/.config}/khal/config`, falling back to `$HOME/.khal/khal.conf` ([Model.js lines 771–795](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L771-L795)).

This is not another khal subprocess and does not enlarge the `list`/`printformats` implementation. However, using only go-khal's JSON config means meeting conference data and attendee lists will remain unavailable unless a conventional khal config with matching vdir `path` entries also exists. Supplying a real UID lets this optional lookup work.

## Current go-khal gap and reusable behavior

Current go-khal registers `tui`, `agenda`, `import`, `todo`, and `config`, but no top-level `list` or `printformats` ([cmd/root.go lines 13–34](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/cmd/root.go#L13-L34)). Its `agenda` command emits fixed human-readable rows and cannot substitute for the JSON path ([cmd/views.go lines 17–43](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/cmd/views.go#L17-L43)). Its default config is `~/.config/go-khal/config.json`, which is usable because Omarchy passes no config path ([internal/config/config.go lines 34–63](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/config/config.go#L34-L63)).

The existing event model already carries UID, summary, description, location, URL, organizer, start/end, all-day, recurrence state, calendar/display name, and color ([internal/calendar/types.go lines 5–33](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/calendar/types.go#L5-L33)). Two gaps affect exact results beyond command/output wiring:

- current recurrence expansion is bounded around the current date, while the panel may request any browsed month ([store.go lines 468–510](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/calendar/store.go#L468-L510));
- cancelled VEVENTs are currently omitted and `Event` has no status field, while original khal can emit `CANCELLED` for the plugin to display ([store.go lines 291–300](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/calendar/store.go#L291-L300), [types.go lines 5–33](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/calendar/types.go#L5-L33)).

Those gaps matter only when such events or dates are requested; they do not justify implementing unrelated khal features.

## Explicitly unused khal surface

Do **not** implement these for this compatibility target unless another caller requires them:

- `calendar`, `interactive`, `new`, `import`, `search`, `printcalendars`, editing, or any other khal command;
- global `-c/--config`, `-v/--verbosity`, `-l/--logfile`, or `--color/--no-color` compatibility;
- list `-a/-d`, `--format`, `--once`, `--notstarted`, explicit end datetimes, time-only inputs, weekday names, `eod`, or `week`;
- a general khal format-template engine, human agenda rendering, ANSI styling, or day headings;
- JSON fields beyond the 15 requested above;
- exact logger text, stderr formatting, JSON object key order, daily array segmentation, empty-day `[]` lines, or duplicate multi-day rows.

The minimum clean cut is the hard-coded argv grammar and parser contract above, with occurrence-correct events for the requested local date range.

## Khal configuration compatibility

This section keeps the khal pin above and pins the first-party vdirsyncer examples to [`pimutils/vdirsyncer@ceee70d1a15bab19b9691beff86d23bf072ea7dc`](https://github.com/pimutils/vdirsyncer/commit/ceee70d1a15bab19b9691beff86d23bf072ea7dc). ConfigObj syntax references use [`DiffSK/configobj@9c8a0a80c767bf8a3d6493ed01df6c351bddca42`](https://github.com/DiffSK/configobj/commit/9c8a0a80c767bf8a3d6493ed01df6c351bddca42); khal does not version-bound that dependency.

### Discovery and precedence

Pinned khal's executable behavior is:

1. An explicit global `-c/--config PATH` is used alone and must exist.
2. Otherwise, khal checks `<dir>/khal/config` for each PyXDG config directory and returns the first existing path ([settings.py lines 57–91](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/settings.py#L57-L91), [cli_utils.py lines 123–133](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/cli_utils.py#L123-L133)). PyXDG orders these as `${XDG_CONFIG_HOME:-$HOME/.config}`, followed by the colon-separated `$XDG_CONFIG_DIRS`, or `/etc/xdg` when that variable is unset or empty ([BaseDirectory.py lines 34–43](https://github.com/takluyver/pyxdg/blob/1d23e483ae869ee9532aca43b133cc43f63626a3/xdg/BaseDirectory.py#L34-L43)).
3. There is no implemented `~/.khal/khal.conf` or `./khal.conf` fallback. The pinned documentation still claims both deprecated fallbacks, but `find_configuration_file()` does not check them ([configure.rst lines 15–23](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/doc/source/configure.rst#L15-L23)). The source controls the pinned runtime behavior.

The shared user default is therefore `${XDG_CONFIG_HOME:-$HOME/.config}/khal/config`; original khal additionally has system XDG fallbacks. Omarchy's separate `.ics` detail lookup has a different two-step search: that user-XDG file, then `$HOME/.khal/khal.conf`, only when the first file does not exist. It does not inspect `$XDG_CONFIG_DIRS`, cwd, `-c`, or go-khal's JSON file ([Model.js lines 776–787](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L776-L787)).

### Grammar and minimum valid file

Khal constructs `ConfigObj` with interpolation disabled, validates it against `khal.spec`, injects and converts defaults in memory, and aborts on syntax or validation errors ([settings.py lines 75–140](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/settings.py#L75-L140)). The relevant grammar is:

- `key = value`; `=` is the only divider. A `path: value` legacy line is invalid. Keys, scalar values, and section names may be single- or double-quoted.
- `#` starts a full-line or inline comment outside a quoted value. Commas make lists unless the scalar is quoted. Triple single or double quotes make multiline values.
- `[calendars]` is a top-level section; `[[name]]` is its calendar subsection. Bracket count, not indentation, determines nesting. Values after a subsection continue to belong to that subsection until another section marker changes the nesting.

These rules come from khal's short syntax description ([configure.rst lines 1–7](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/doc/source/configure.rst#L1-L7)) and ConfigObj's format definition ([configobj.rst lines 899–983](https://github.com/DiffSK/configobj/blob/9c8a0a80c767bf8a3d6493ed01df6c351bddca42/docs/configobj.rst#L899-L983)).

The parser-level minimum is one uniquely named calendar subsection with one `path`:

```ini
[calendars]
[[personal]]
path = ~/.calendars/personal/
```

`[calendars]`, at least one `[[name]]`, and `path` are mandatory; the remaining calendar keys are optional ([khal.spec lines 1–19](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/khal.spec#L1-L19)). Tests reject a missing calendars section and a one-level `[calendars] path = ...`, while a file containing only calendar subsections succeeds with all other sections defaulted ([settings_test.py lines 79–156](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/settings_test.py#L79-L156), [small.conf](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/configs/small.conf)). Thus the schema comment that locale formats are mandatory is stale: each has a default, and `[locale]` may be absent.

### Calendar values, paths, and discovery

| Key | Validated value and default | Runtime convention |
|---|---|---|
| subsection name | unique string | Direct calendars use it as the khal calendar/display name. |
| `path` | required scalar string | ConfigObj unquotes it; khal then expands `~`/`~user`, `$VAR`, and `${VAR}`. It neither cleans nor requires an absolute path. |
| `type` | `calendar`, `birthdays`, or `discover`; default `calendar` | `calendar` reads `.ics`; `birthdays` reads `.vcf` and forces readonly; `discover` expands a path pattern into calendar vdirs. |
| `color` | `auto` by default; empty, a supported name, `0`–`255`, or `#RGB`, `#RRGGBB`, `#RRGGBBAA` | `auto` reads the vdir's `color` metadata file; a missing/invalid/empty metadata color becomes no calendar color. A `#...` value in the config must be quoted so `#` is not parsed as a comment. |
| `priority` | integer, default `10` | Affects day-highlight color precedence, not inclusion or sort order. |
| `readonly` | ConfigObj boolean, default `false` | Excludes the calendar from writable choices and rejects insert, update, and delete; it does not hide events. |
| `addresses` | comma-separated/quoted ConfigObj list, with a scalar coerced to one item; default `[""]` | Account addresses used to identify the user's attendee status. |

The schema defines those types and defaults ([khal.spec lines 19–77](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/khal.spec#L19-L77)); custom expansion and color validation are in [utils.py lines 111–151](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/utils.py#L111-L151). Boolean spellings are `true/on/yes/1` and `false/off/no/0` ([validate.rst lines 73–93](https://github.com/DiffSK/configobj/blob/9c8a0a80c767bf8a3d6493ed01df6c351bddca42/docs/validate.rst#L73-L93)). Readonly calendars remain readable and are excluded from writable choices ([khalendar.py lines 113–141](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/khalendar.py#L113-L141)); every mutation is guarded ([khalendar.py lines 174–260](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/khalendar/khalendar.py#L174-L260)).

Globs are active only for `type = discover`. Khal applies user/environment expansion first, calls recursive Python glob with an appended `/`, and keeps matched leaf directories; `*`, `?`, character classes, and recursive `**` work. Each discovered vdir is materialized as `type=calendar`, `priority=10`, inherited `readonly`, and metadata color falling back to the discover block's color. The discover block's `priority` and `addresses` are not copied. The name comes from the vdir's `displayname` file, falls back to the directory basename, and gains repeated `1` suffixes on collision ([utils.py lines 183–217](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/utils.py#L183-L217), [utils.py lines 243–289](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/utils.py#L243-L289), [settings_test.py lines 202–345](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/settings_test.py#L202-L345)).

First-party real/generated forms establish two common layouts:

- Explicit vdirs: one `[[name]]` per concrete directory, commonly `path = ~/.calendars/name/`; optional examples add `color`, `priority`, `readonly`, and `addresses` ([simple.conf](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/configs/simple.conf), [small.conf](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/configs/small.conf)).
- Vdirsyncer collection roots: its example stores calendar collections as child directories under `~/.calendars/` and synchronizes `displayname` and `color` metadata ([config.example lines 51–64](https://github.com/pimutils/vdirsyncer/blob/ceee70d1a15bab19b9691beff86d23bf072ea7dc/config.example#L51-L64)). Khal's own wizard reads vdirsyncer filesystem storages ([configwizard.py lines 148–172](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/configwizard.py#L148-L172)) and writes each as `path = <storage path>*` plus `type = discover` ([configwizard.py lines 350–373](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/configwizard.py#L350-L373)). Vdirsyncer writes metadata to plain files named `displayname` and `color`; its `read_only` storage option is a sync policy and is not the khal `readonly` key ([tutorial.rst lines 143–173](https://github.com/pimutils/vdirsyncer/blob/ceee70d1a15bab19b9691beff86d23bf072ea7dc/docs/tutorial.rst#L143-L173), [config.rst lines 101–108](https://github.com/pimutils/vdirsyncer/blob/ceee70d1a15bab19b9691beff86d23bf072ea7dc/docs/config.rst#L101-L108)).

Khal has no persistent hidden/visibility setting. Without `-a/-d`, selection is unrestricted ([cli_utils.py lines 45–70](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/cli_utils.py#L45-L70)) and the collection includes every configured calendar, including readonly calendars ([cli_utils.py lines 160–179](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/cli_utils.py#L160-L179)). Omarchy supplies neither selector, so every khal calendar is visible to its lists.

### Locale and go-khal mappings

| Khal setting | Khal type/default | Current go-khal correspondence |
|---|---|---|
| `[locale] firstweekday` | integer `0`–`6`, Monday=`0`, default `0` | `week_starts_on`: exact for `0` → `monday` and `6` → `sunday`; current `WeekStart()` cannot represent Tuesday–Saturday starts. |
| `timeformat` | Python `strftime`, default `%X` | go-khal uses a Go time layout. Common exact conversions are `%H:%M` → `15:04` and `%I:%M %p` → `03:04 PM`; arbitrary strings cannot be copied verbatim. Omarchy `printformats`/`list` also need the original strftime semantics. |
| `dateformat`, `longdateformat` | `%x`, `%x` | No current JSON field; `longdateformat` is required by the Omarchy compatibility commands. |
| `datetimeformat`, `longdatetimeformat` | `%c`, `%c` | No current JSON field; preserve even if go-khal does not consume them outside `printformats`. |
| `default_timezone` | IANA timezone; host timezone when absent/empty | No current config field; khal uses it for new/fallback event timezones. |
| `local_timezone` | IANA timezone; host timezone when absent/empty | Current storage uses Go `time.Local`, so only the absent/host-timezone case maps exactly. It controls original khal's displayed/list wall times. |
| `unicode_symbols`, `weeknumbers` | boolean `true`; `off` | No go-khal config counterpart; unrelated to the required Omarchy commands. |

The khal types/defaults are authoritative in [khal.spec lines 83–130](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/khal.spec#L83-L130). Current go-khal's fields/defaults and source validation are in [config.go lines 13–127](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/config/config.go#L13-L127).

Calendar-field mappings also have non-lossless cases:

- A direct khal subsection name supplies the display name; go-khal currently derives the machine name from the path basename, then prefers its `display_name`, a vdir `displayname`, and finally the basename. Original khal consults `displayname` only while expanding `discover` ([go-khal store.go lines 157–185](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/calendar/store.go#L157-L185)).
- Khal `color = auto` must be resolved before feeding go-khal; the literal string `auto` is not a display color. Explicit khal colors map directly.
- Khal `addresses` is a list, while current go-khal has one `email`; choosing one address loses information. Go-khal uses that email to identify attendee/RSVP state.
- Khal `birthdays` maps closest to an addressbook source, but original khal retains one configured calendar per subsection while go-khal currently merges all addressbook birthdays into one virtual calendar ([store.go lines 87–127](https://github.com/hsanson/go-khal/blob/0448598cb90d1963bb4b815051620adc9bbcec7e/internal/calendar/store.go#L87-L127)).
- Current go-khal has no readonly field and treats every calendar source as writable, so merely parsing `readonly` without carrying it into calendar selection and mutation guards would discard a safety property. Conversely, go-khal `hidden` has no upstream khal equivalent.
- Khal `priority` and `[default] default_calendar` have no current go-khal config counterparts. Go-khal-only UI/recurrence settings (`sidebar_width`, minimap bounds, recurrence bounds, and any hidden state) likewise have no upstream equivalents.

### Unknown content and safe extension placement

Syntactically valid unknown top-level sections, calendar keys, and subsections do not invalidate a khal config. Khal returns them in the `ConfigObj` and logs warnings; `[palette]` is the sole warning exception. The pinned test explicitly accepts both an unknown calendar key and an unknown top-level section ([settings.py lines 142–152](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/settings/settings.py#L142-L152), [settings_test.py lines 160–174](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/tests/settings_test.py#L160-L174)). Consequently:

- a dedicated top-level `[go-khal]` section is accepted for global go-only settings, with an unknown-section warning;
- a go-only scalar inside `[[calendar]]`, such as hidden state, is accepted, with an unknown-key warning;
- `[palette]` is not a general extension namespace: khal applies palette-specific validation to its contents.

ConfigObj can round-trip comments and member/section order and retains unknown nodes ([configobj.rst lines 45–54](https://github.com/DiffSK/configobj/blob/9c8a0a80c767bf8a3d6493ed01df6c351bddca42/docs/configobj.rst#L45-L54)); values that came only from schema defaults are not written ([configobj.rst lines 617–637](https://github.com/DiffSK/configobj/blob/9c8a0a80c767bf8a3d6493ed01df6c351bddca42/docs/configobj.rst#L617-L637)). Original khal only reads an existing config; its wizard refuses to overwrite one and creates a new user-XDG file ([configwizard.py lines 385–427](https://github.com/pimutils/khal/blob/3ba0fcef06d5244a4a7330869a8d5f18a0342ce9/khal/configwizard.py#L385-L427)). Any go-khal command that changes the shared file therefore has to retain unknown and known-but-unused sections/keys, comments, order, quoted values, and omitted-vs-explicit defaults. Re-encoding only go-khal's understood model would destroy valid khal settings and extensions.

### Omarchy's direct `path =` extraction boundary

The `.ics` detail lookup is deliberately not a ConfigObj parser. Its shell code:

- matches every case-sensitive, unquoted key spelled `path` with optional surrounding whitespace and a literal `=`, regardless of section;
- removes everything from the first `#`, even inside quotes, trims trailing whitespace, and strips at most one leading and trailing single/double quote;
- expands only a leading `~` to `$HOME`; `$VAR`, `${VAR}`, `~user`, ConfigObj multiline values, and quoted keys are not interpreted like khal;
- preserves spaces because its field separator is newline, then applies Bash pathname expansion. `nullglob` drops unmatched patterns; `globstar` is not enabled, so `**` does not have khal's recursive meaning;
- keeps only expanded entries that are directories. A directory-valued `path` in an unrelated section is included, while file-valued paths such as `[sqlite] path` are ignored.

These constraints follow directly from [Model.js lines 776–787](https://github.com/derekross/omarchy-calendar/blob/0c31e965037f7a388b733bfe9ba15cab36f56e7b/Model.js#L776-L787). The common first-party forms—lowercase `path =`, optional simple quotes, leading `~`, and a one-level `*` discover pattern—work in both khal and this lookup. Valid ConfigObj edge forms outside that intersection may still list correctly through khal while losing Omarchy's optional attendee/conference detail lookup.