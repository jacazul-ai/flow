# Dates and Timestamps

How `flow` stores, compares and shows time, what the Taskwarrior model it
migrates from does, and the direction agreed for storage order
([#15](https://github.com/jacazul-ai/flow/issues/15)) and local display
([#16](https://github.com/jacazul-ai/flow/issues/16)). Sections marked
**Current** describe the code as it is; **Direction** is decided but not yet
implemented; **Open** still needs a decision.

## Taskwarrior model

Taskwarrior is the reference for users coming from `tw-flow`:

| Layer | Behavior |
|---|---|
| Storage | Every date is a UTC Unix epoch integer, accurate to one second. Taskwarrior 3 (TaskChampion) stores "UNIX epoch timestamps, in the form of an integer." |
| JSON export | Fixed-width ISO 8601 basic UTC, `YYYYMMDDTHHMMSSZ` (`20120110T231200Z`); the format specification allows no other. |
| Display | `rc.dateformat` controls the layout; values are shown in local time. |

The last row is inferred rather than quoted: the documentation describes
`dateformat` but does not name the timezone source. Displayed times follow
the host's timezone database, as reports of shifted times after `tzdata`
updates show.

The reference `tw-flow` does not follow that model consistently:

- `ponder.py` prints raw UTC export text, cut mid-field
  (`data["end"][:10]` renders `20260924T0`).
- `ponder.py` compares a UTC `due` with `datetime.now()` formatted as local
  time plus a literal `Z`, so its overdue flag is off by the UTC offset.

Neither is a contract to port.

## Current: storage

All timestamps are text. `timestamp()` in `internal/storage/sqlite/store.go`
writes `time.Now().UTC().Format(time.RFC3339Nano)`. Columns holding
timestamps: `created_at`, `updated_at`, `started_at`, `completed_at`,
`occurred_at`, `recorded_at`, `acknowledged_at`, `expires_at`, `wait_until`.
`due_at` holds a date only (`2006-01-02`).

`RFC3339Nano` trims trailing zeros from the fraction, so the text is not
fixed width and text order can disagree with time order:

```text
2026-09-27T10:00:05.1Z     written first  (.100s)
2026-09-27T10:00:05.123Z   written second (.123s)
time order: first < second    text order: first > second
```

At the character after `.1`, `Z` meets `2` and sorts later. A whole second
(`...05Z`, as the Taskwarrior importer writes) sorts after every fraction of
the same second for the same reason. SQLite compares these columns as text
in:

| Query | Ordering | Effect |
|---|---|---|
| `history.go` | `ORDER BY occurred_at, sequence, id` | Primary key of history order: events written in the same second (`execute`, `outcome`, `done`) can come back out of order. |
| `store.go` `ListTasks` | `ORDER BY t.position, t.created_at, t.id` | Tie-break between tasks at the same position. |
| `focus.go` | `ORDER BY updated_at DESC, session_id` | Session list order. |

Comparisons outside SQL (focus cache expiry, session notes) parse the text
with `time.RFC3339Nano` first and are not affected.

## Current: days and display

- "Today" is the UTC date: `overdue` (`internal/cli/views.go`) and the
  `ponder` pulse (`Done Today`, overdue count in
  `internal/cli/dashboard.go`) compare against
  `time.Now().UTC().Format("2006-01-02")`. In Toronto, a task closed at
  21:00 counts for tomorrow.
- The importer reduces a Taskwarrior `due` timestamp to its UTC date
  (`normalizeDate` in `internal/migration/importer.go`), so a due at 22:00
  in Toronto becomes the next day.
- Text reports show few times today. Where a timestamp appears (the session
  list), it is the stored UTC text.

## Direction

Decided on 2026-09-27; items 1 and 2 belong to #15, items 3 and 4 to #16:

1. **Store UTC, fixed width.** Every timestamp is UTC text with nine
   fractional digits, `2006-01-02T15:04:05.000000000Z`, so text order equals
   time order and rows stay readable. Text is kept over integers on purpose.
   Parsing with `time.RFC3339Nano` accepts this layout, so readers keep
   working.
2. **Migrate existing rows** to that layout, and make the Taskwarrior
   importer write it.
3. **Show local time in text output only.** Human-facing text renders in the
   local timezone; `json`, `jsonl` and `xml` stay in UTC because they are
   read by programs.
4. **No timezone override for now.** On the client, the process runs on the
   user's machine, so `time.Local` is already the right zone.

A future server keeps storing and returning UTC and never renders local
time: its own zone means nothing to users elsewhere. The client that shows
the value converts it.

## How Go resolves the local timezone

`time.Local` is resolved once per process:

| Platform | Source | Honors `TZ` |
|---|---|---|
| Linux | `TZ`, else `/etc/localtime`; `TZ=""` means UTC | Yes |
| macOS | Same code as Linux (`zoneinfo_unix.go`); `/etc/localtime` follows System Settings | Yes |
| Windows | `GetTimeZoneInformation` from the OS settings | No |

`time.Local` needs no bundled zone database on any of them. Loading a zone
by name (`time.LoadLocation("America/Toronto")`) does: Unix reads
`/usr/share/zoneinfo`, while Windows has no system source and needs
`$ZONEINFO` or `import _ "time/tzdata"` (about 450 KB). Windows also applies
the current year's daylight-saving rules to every year, which can shift old
timestamps by an hour when displayed; storage in UTC is unaffected.

Because `time.Local` is fixed for the process, changing `TZ` inside a test
has no effect. Output that renders local time therefore needs the zone
passed in to stay deterministic in tests.

## Open

- **Where the zone enters the engine.** Proposed: a `Location` field on
  `flow.Env`, set to `time.Local` by `EnvFromOS`, UTC when nil, a fixed zone
  in tests. This follows the rule that the engine reads the process
  environment only in `EnvFromOS`.
- **Which "today".** `Done Today` and `overdue` should count days in the same
  zone used for display.
- **Due dates.** Whether `due_at` stays a date or becomes a timestamp, and
  whether that is part of #15 or its own ticket.
- **Which text outputs show times** once rendering is local.

## Sources

- [Taskwarrior: Date & Time](https://taskwarrior.org/docs/dates/)
- [TaskChampion: Tasks](https://gothenburgbitfactory.org/taskchampion/tasks.html)
- [Taskwarrior JSON task format](https://github.com/GothenburgBitFactory/taskwarrior/blob/develop/doc/devel/rfcs/task.md)
- [Arch Linux Forums: Task shows wrong time](https://bbs.archlinux.org/viewtopic.php?id=129427)
- Go 1.27 source: `src/time/zoneinfo_unix.go`, `src/time/zoneinfo_windows.go`
