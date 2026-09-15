# Jaflow CLI Navigation

Jaflow's root help is organized by operator intent rather than alphabetic
command name. Use it to choose the workflow family first, then use
`jaflow help <command>` for the exact contract.

```bash
jaflow help
jaflow --help
jaflow help status
jaflow help migrate
```

`jaflow help` and `jaflow --help` render the same agent-facing root briefing.
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
`jaflow help <alias>` without appearing as duplicate canonical commands in the
root taxonomy.

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

- `migrate`: import an explicit Taskwarrior snapshot into native Jaflow.

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
jaflow help ini
jaflow help initiatives
jaflow help ship
```

## History references

History requires an explicit subject scope:

```bash
jaflow history task <task-reference>
jaflow history initiative <initiative-reference>
jaflow history ini <initiative-reference>
jaflow history plan <initiative-reference>
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

A bare `jaflow history <reference>` is rejected with an `ACTION:` prompt; the
CLI never guesses whether the reference identifies a task or initiative.
Unknown and ambiguous references also return actionable errors. History reads
only the selected project database and does not mutate workflow state.

## Organize pending tasks

Organization is explicit and scoped to one initiative:

```bash
jaflow organize order <initiative-reference> <task-reference> <task-reference> [...]
jaflow organize first <initiative-reference> <task-reference>
jaflow organize after <initiative-reference> <task-reference> <anchor-reference>
jaflow organize block <initiative-reference> <task-reference> <task-reference> [...] --first
jaflow organize block <initiative-reference> <task-reference> <task-reference> [...] --after <anchor-reference>
```

Initiative references accept an exact name, full ID, or unambiguous short ID.
Task references accept a full or unambiguous short UUID. Every task reference
must resolve inside the selected initiative and project.

`order` permutes only the slots occupied by the named pending tasks, so omitted
tasks do not move. `first`, `after`, and `block` explicitly move one task or an
ordered block while preserving the relative order of all omitted pending tasks.
For example, after tasks `One` and `Two` are completed:

```bash
jaflow organize order parity 6f6f6f6f 4f4f4f4f 5f5f5f5f 3f3f3f3f
jaflow organize first parity 5f5f5f5f
jaflow organize after parity 3f3f3f3f 5f5f5f5f
jaflow organize block parity 6f6f6f6f 4f4f4f4f --after 5f5f5f5f
```

Only pending tasks can be moved or used as anchors. Completed and active tasks,
duplicate references, ambiguous UUIDs, and cross-project or cross-initiative
references fail with `ACTION:` guidance. Blocked pending tasks may be reordered,
but ordering never changes dependencies, completion state, wait dates, or
readiness. Run `jaflow next <initiative>` after organizing to identify the
actual executable task.

## Recommended navigation loop

Use the smallest command that answers the current workflow question:

```text
orient → inspect → focus → execute → outcome → done → next focus
```

Typical sequence:

```bash
jaflow status
jaflow next <initiative>
jaflow focus task <uuid>
jaflow execute <uuid>
jaflow outcome <uuid> "Describe the result"
jaflow done <uuid>
jaflow focus plan <initiative>
```

Do not execute a blocked task. `pending` is not equivalent to `ready`; native
readiness is determined from dependency state and wait dates.

## Output and error contract

Successful state changes go to stdout. Errors include an `ACTION:` prompt that
explains the next valid command. Healthy report commands may be quiet when no
state changed, while explicit report commands such as `status`, `ponder`, and
`cache info` render their state.

Use full UUIDs as identity and short UUIDs for display. The root help is a
navigation map; command-specific help is the detailed operational contract.
