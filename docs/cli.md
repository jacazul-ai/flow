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
- `--project-id`: select the project identity;
- `--taskdata`: select the legacy Taskwarrior data directory;
- `--database-path`: select the project SQLite database;
- `--session-id`: select the workflow session identity;
- `-h`, `--help`: show the root briefing.

Compatibility aliases remain routable and are documented with
`jczl-flow help <alias>` without appearing as duplicate canonical commands in the
root taxonomy.

## Deterministic agent onboarding

`jczl-flow onboard` is the one-shot bootstrap briefing for an agent. It composes
existing workflow primitives without creating a second persistence model or
replacing the individual commands:

1. render a pending session handoff first;
2. render the current session focus and focused task context;
3. render focused `status` when an anchor exists;
4. render project-wide `ponder` when no anchor exists;
5. acknowledge the handoff only after the complete briefing renders
   successfully.

`jczl-flow session dump` remains the producer of a resumable handoff,
`jczl-flow session resume` remains the low-level reader, and `jczl-flow session ack`
remains available for explicit acknowledgement and diagnostics. A failed
onboard briefing does not acknowledge a pending handoff.

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
- `status`: inspect project task state;
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
- `urgent`: raise priority and urgency;
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

- `migrate`: import an explicit Taskwarrior snapshot into native flow.

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
