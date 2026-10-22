# flow CLI Navigation

`flow`'s root help is organized by operator intent rather than alphabetic
command name. Use it to choose the workflow family first, then use
`jczl-flow help <command>` for the exact contract.

```bash
jczl-flow help
jczl-flow --help
jczl-flow help status
jczl-flow help migrate
```

`jczl-flow help` and `jczl-flow --help` render the same agent-facing root briefing.
The parser remains responsible for recognizing options, but it does not own
primary help presentation. Root help includes these global options:

- `-v`, `--verbose`: enable verbose mode;
- `-V`, `--version`: show the version;
- `--project`: select the project identity;
- `--taskdata`: select the legacy Taskwarrior data directory;
- `--database-path`: select the project SQLite database;
- `--session`: select the workflow session identity;
- `--home`: select the runtime home directory;
- `-h`, `--help`: show the root briefing.

Compatibility aliases remain routable and are documented with
`jczl-flow help <alias>` without appearing as duplicate canonical commands in the
root taxonomy. Legacy runtime flags are different: `--project-id` and
`--session-id` are rejected by the new CLI.

### Runtime defaults

The implemented standalone runtime contract is maintained in
[Runtime Defaults](runtime-defaults.md). It covers canonical names,
precedence, project/worktree resolution, launcher-owned sessions, and the
legacy-name migration table. The persisted configuration layer is explicitly
deferred to [flow-config-layer / GitHub #18](https://github.com/jacazul-ai/flow/issues/18).

## Canonical embedded skill

`jczl-flow skill` prints the `SKILL.md` hub of the canonical `flow-expert`
agent skill; `jczl-flow skill --out <dir>` writes the hub and its references
below `<dir>`. The skill is generated from the command registry, the task and
annotation catalogs, and the runtime names, and every example it contains is
executed by the test suite. Rendered text uses `jacazul flow ...` for launcher
consumers, while `jczl-flow ...` remains the standalone form. The command
never opens workflow storage. Layout, sources, guarantees, and known gaps are
recorded in [flow-expert-skill.md](flow-expert-skill.md).

## Deterministic agent onboarding

`jczl-flow onboard` is the one-shot bootstrap briefing for an agent. It composes
existing workflow primitives without creating a second persistence model or
replacing the individual commands:

1. render a pending session handoff first;
2. render the current session focus and focused task context;
3. render focused `status` when an anchor exists;
4. render project-wide `ponder` when no anchor exists;
5. deliver the handoff after the complete briefing renders successfully;
   native dumps remain pending until `session ack` explicitly acknowledges them.

`jczl-flow session dump` creates a native draft. Finalize it before delivery:

```bash
jczl-flow session dump
jczl-flow session finalize <dump-id> \
  --references 'Refs: #27; internal/cli/session.go:223' \
  --actions 'Run the next command' \
  --notes 'Record gotchas and decisions'
```

The finalized dump contains fixed `Focus`, `References`, `Actions`, and `Notes`
sections. `Actions` is mandatory; the other agent-owned fields may use their
explicit empty defaults. `session resume` and `onboard` deliver the latest
pending dump and record delivery without acknowledging it. Use
`jczl-flow session ack` to release the next dump. Legacy session notes retain
their compatibility behavior. Onboard is observational: it does not execute or
start the focused task, and output failure leaves the native handoff pending for
retry.

### Runtime info report

`jczl-flow info` resolves and reports the invocation context without opening
SQLite or mutating runtime state. It shows project and session identity, home,
Taskwarrior data, native and legacy database paths, output format, working
directory, version, path existence, and the source of each resolved value.

```bash
jczl-flow info
jczl-flow info --format json
```

Sources are reported as `flag`, `environment`, `runtime`, `default`, or
`derived`. This command does not run Taskwarrior export, migrate legacy state,
read focus files, or create missing directories.

### Schema upgrades

Commands that open the native SQLite database check the embedded Goose schema
before running. Fresh databases and databases already at the supported version
stay silent. An existing older database reports the transition, for example
`Schema upgrade required: v14 -> v15`, then applies the pending migrations
automatically. A database newer than the binary is rejected with an `ACTION:`
to upgrade the engine. There is no separate schema `migrate up` command.

### Session list report

`jczl-flow session list` renders the current project's persisted sessions as a
compact text table:

```text
SESSION    PLAN                TASK      AGE  STATUS  HANDOFF
* current  session-list-table  373e0c61  0s   active  pending
```

The current session is prefixed with `*`. `PLAN` is the initiative name,
`TASK` is the focused task's short UUID, and missing anchors are shown as `-`.
`STATUS` is `active` below two hours, `idle` below eight hours, and `orphan`
after eight hours. `HANDOFF` is `pending` until `session ack` records the
acknowledgement, then becomes `acknowledged`; sessions without a note show `-`.
The report is project-scoped and never lists sessions from another project.
Logically purged sessions are hidden from the default list.

Structured formats preserve the existing session identity fields and add
`plan`, `task`, and `handoff` fields:

```bash
jczl-flow session list --format json
```

### Session dump history

`session dumps` lists the current session's dump sequence without loading
payloads. Each row includes the creation time, lifecycle status, short dump ID,
and focus summary. Statuses are `draft`, `pending`, and `acknowledged`:

```bash
jczl-flow session dumps
jczl-flow session dumps --all
```

The default view is session-scoped. `--all` adds the session ID and exposes
history, including logically purged sessions, until an explicit hard purge
removes their records. Logical purge preserves session dumps and notes:

```bash
jczl-flow session purge
jczl-flow session purge --hard
jczl-flow session purge --hard --confirm
```

The default purge marks orphan sessions as purged and hides them from normal
session views. Only `--hard --confirm` physically deletes an already-purged
orphan session and its dumps, notes, and cache. It never targets the current or
global session.

## Initiative and task metadata

Initiatives keep contract metadata separate from task lifecycle and annotations.
Set the first-class goal with:

```bash
jczl-flow goal parity "Ship deterministic workflow parity"
```

`plans`, `ponder`, and structured reports render the goal and the persisted
metadata collections. Native initiative and task titles are required and may
contain at most 120 Unicode characters. Titles longer than 79 characters are
accepted with a warning; titles over 120 characters fail with an `ACTION:`
prompt telling the operator to move context into the open description.

Legacy migration extracts a leading `[MODE]` before validation. A migrated title
longer than 120 characters keeps its complete cleaned source in metadata,
uses the first 50 characters plus `FIXME: <truncated:50>` as its title, and
emits a retention warning. Native create and amend operations never truncate a
title silently.

### Task tags

Task tags follow Taskwarrior's stored-tag model. A task may have any number of
one-word stored tags; tags are preserved in native storage, shown in text and
structured task reports, and can be filtered with `+tag` and `-tag` predicates:

```bash
jczl-flow status +research -legacy
jczl-flow next parity +urgent
jczl-flow amend 57c3fc80 +urgent -legacy
```

In filters, `+tag` requires a tag and `-tag` excludes it. In `amend`, `+tag`
adds a tag and `-tag` removes one. Multiple predicates combine with `AND`.
The native store keeps tags in a relational `task_tags` table, so repeated
imports and repeated additions do not create duplicates.

Taskwarrior virtual tags such as `BLOCKED`, `READY`, `ACTIVE`, and `TAGGED` are
not stored tags and are never persisted as rows. When a virtual predicate has a
native equivalent, flow evaluates it from workflow state instead. The legacy
`DISCARDED` marker is a migration compatibility signal for the native
`disposition=discarded` state, not a native task tag.

## Root help taxonomy

The root help exposes canonical commands once in this fixed order:

### Start and organize work

- `plan`: create an initiative and chained tasks;
- `organize`: reorder pending tasks within an initiative;
- `roadmap`: manage strategic phases;
- `rename`: rename an initiative;
- `backlog`: pause an initiative;
- `activate`: restore a backlog initiative.

### Examine workflow state

- `help`: show the agent workflow briefing;
- `skill`: render the canonical flow-expert agent skill;
- `status`: inspect project task state;
- `info`: inspect resolved runtime defaults without opening storage;
- `ponder`: render the project dashboard;
- `plans`: list initiative summaries with short UUID references;
- `next`: list ready tasks;
- `tree`: inspect dependency markers;
- `history`: inspect task or initiative history by explicit scope;
- `active`, `blocked`, `overdue`: inspect derived task views.

### Work on the current task

- `focus`: inspect or switch the current anchor;
- `execute`: start ready work;
- `outcome`: record the completion result;
- `done`: complete a task after its outcome;
- `handoff`: transfer execution context;
- `reopen`: return completed work to pending;
- `discard`: archive work with an audit outcome.

### Change and reprioritize work

- `amend`: update task description or ticket metadata;
- `urgent`: raise priority; urgency is derived from task state;
- `block`: add a dependency;
- `unblock`: remove a dependency;
- `wait`: postpone readiness until a date.

### Preserve and maintain context

- `session`: inspect or resume session state;
- `note`: add or delete structured context;
- `notes`: list annotations;
- `context`: inspect direct and inherited context;
- `ticket`: link external ticket metadata.

### Maintain derived workflow state

- `cache`: inspect or clear derived output cache.

### Prepare and integrate changes

- `commit`: draft a conventional commit without staging or committing files.

### Migrate legacy state

- `migrate`: import Taskwarrior state into native flow; by default it runs `task` directly, while `--source` accepts an explicit offline snapshot.

## Compatibility aliases

Aliases remain routable for existing agents but are hidden from the root
canonical list:

| Alias | Canonical command |
|---|---|
| `initiative`, `ini` | `plan` |
| `inis`, `initiatives` | `plans` |
| `ship` | `roadmap ship` |

Detailed help remains available for aliases:

```bash
jczl-flow help ini
jczl-flow help initiatives
jczl-flow help ship
```

## History references

History requires an explicit subject scope:

```bash
jczl-flow history task <task-reference>
jczl-flow history initiative <initiative-reference>
jczl-flow history ini <initiative-reference>
jczl-flow history plan <initiative-reference>
```

Task references accept a full or unambiguous short UUID. Initiative references
accept an exact name, a full ID, or an unambiguous ID prefix of at least eight
characters. The `initiative`, `ini`, and `plan` forms are equivalent. Initiative
listings expose the short reference for follow-up commands:

```text
PROJECT: example
INITIATIVES:
- [ACTIVE] parity [id:91b2c3d4] pending:2 active:0 completed:0 blocked:1
```

A bare `jczl-flow history <reference>` is rejected with an `ACTION:` prompt; the
CLI never guesses whether the reference identifies a task or initiative.
Unknown and ambiguous references also return actionable errors. History reads
only the selected project database and does not mutate workflow state.

## Organize pending tasks

Organization is explicit and scoped to one initiative:

```bash
jczl-flow organize order <initiative-reference> <task-reference> <task-reference> [...]
jczl-flow organize first <initiative-reference> <task-reference>
jczl-flow organize after <initiative-reference> <task-reference> <anchor-reference>
jczl-flow organize block <initiative-reference> <task-reference> <task-reference> [...] --first
jczl-flow organize block <initiative-reference> <task-reference> <task-reference> [...] --after <anchor-reference>
```

Initiative references accept an exact name, full ID, or unambiguous short ID.
Task references accept a full or unambiguous short UUID. Every task reference
must resolve inside the selected initiative and project.

`order` permutes only the slots occupied by the named pending tasks, so omitted
tasks do not move. `first`, `after`, and `block` explicitly move one task or an
ordered block while preserving the relative order of all omitted pending tasks.
For example, after tasks `One` and `Two` are completed:

```bash
jczl-flow organize order parity 6f6f6f6f 4f4f4f4f 5f5f5f5f 3f3f3f3f
jczl-flow organize first parity 5f5f5f5f
jczl-flow organize after parity 3f3f3f3f 5f5f5f5f
jczl-flow organize block parity 6f6f6f6f 4f4f4f4f --after 5f5f5f5f
```

Only pending tasks can be moved or used as anchors. Completed and active tasks,
duplicate references, ambiguous UUIDs, and cross-project or cross-initiative
references fail with `ACTION:` guidance. Blocked pending tasks may be reordered,
but ordering never changes dependencies, completion state, wait dates, or
readiness. Run `jczl-flow next <initiative>` after organizing to identify the
actual executable task.

## Recommended navigation loop

Use the smallest command that answers the current workflow question:

```text
orient → inspect → focus → execute → outcome → done → next focus
```

Typical sequence:

```bash
jczl-flow status
jczl-flow next <initiative>
jczl-flow focus task <uuid>
jczl-flow execute <uuid>
jczl-flow outcome <uuid> "Describe the result"
jczl-flow done <uuid>
jczl-flow focus plan <initiative>
```

Do not execute a blocked task. `pending` is not equivalent to `ready`; native
readiness is determined from dependency state and wait dates.

## Output and error contract

Successful state changes go to stdout. Errors include an `ACTION:` prompt that
explains the next valid command. Healthy report commands may be quiet when no
state changed, while explicit report commands such as `status`, `ponder`, and
`cache info` render their state.

Report commands (`status`, `ponder`, `plans`, `tree`, `next`, `active`,
`blocked`, `overdue`, `history`, `context`, `notes`, `focus`, `session list`,
`roadmap show`, `cache info` and `onboard`) take `--format text|json|jsonl|xml`.
`text` is the default; `JACAZUL_FLOW_FORMAT` sets another default for
standalone `jczl-flow`. Commands that change state take no `--format`. The
envelope, the record fields and the cache signal are specified in
[Output formats](output-formats.md).

```bash
jczl-flow status --format json
jczl-flow next --format jsonl
JACAZUL_FLOW_FORMAT=xml jczl-flow plans
```

Use full UUIDs as identity and short UUIDs for display. The root help is a
navigation map; command-specific help is the detailed operational contract.
