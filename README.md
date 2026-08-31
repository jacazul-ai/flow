# jaflow

`jaflow` means **Jacaré Azul Flow**.

This project is the Go-native migration of the current `tw-flow` workflow tooling used by [jacazul-ai-cli](https://github.com/jacazul-ai/jacazul-ai-cli).

The goal is not to wrap Taskwarrior forever. The goal is to move the useful workflow semantics into a Go implementation that can serve Jacazul agents with a sharper, more portable, and more controllable flow engine.

## Why this exists

Today, `jacazul-ai-cli` relies on shell-based Taskwarrior helpers such as:

- `taskp`
- `tw-flow`
- `tw-flow ponder`
- session focus/context helpers
- structured task annotations
- per-project Taskwarrior databases

Those tools work, but they are split across shell scripts, Taskwarrior behavior, local conventions, and agent instructions.

`jaflow` exists to consolidate that behavior into a Go project.

## Migration scope

The first migration target is the Taskwarrior workflow skill currently living in:

```text
jacazul-ai-cli/tw-flow-to-go/skills/taskwarrior-expert
```

The migration scope includes:

- one native SQLite database per canonical `PROJECT_ID`
- `sqlok` as the official SQLAlchemy-like SQL/schema layer
- a driver owned by `jaflow`, selected independently from `sqlok`
- per-project database isolation
- plan/initiative creation
- task focus and anchor management
- task execution modes such as `DESIGN`, `INVESTIGATE`, `GUIDE`, `EXECUTE`, `TEST`, `DEBUG`, and `REVIEW`
- structured annotations such as `DECISION`, `RESEARCH`, `BLOCKED`, `LESSON`, `OUTCOME`, and `HANDOFF`
- project dashboard/status views currently handled by `tw-flow ponder` and `tw-flow status`
- session handoff and resume context
- UUID-first task references
- output caching for repeated status/dashboard calls

Long-term, `jaflow` should implement the Taskwarrior-like behavior needed by Jacazul workflows directly in Go. Taskwarrior compatibility is a design constraint, not the final architecture.

The local engine should also be designed so it can connect to a centralized server in the future. That server would orchestrate tasks, context, session state, and agent workflow coordination across machines or agents when needed.

## Documentation

- [Vision](docs/VISION.md): mission, operational memory, and team direction.
- [Architecture](docs/ARCHITECTURE.md): local Coordinator, backends, and
  future team orchestration.
- [Feature parity](docs/feature-parity.md): reference contracts, test audit,
  implementation backlog, and parity completion criteria.
- [Migration](docs/migration.md): Taskwarrior mapping, safety, idempotency,
  verification, cutover, and rollback.
- [CLI navigation](docs/cli.md): intent-first help taxonomy, aliases, and
  workflow navigation examples.
- [Task history](docs/task-history.md): native events, TaskChampion extraction,
  import rules, and verification contracts.

## Consumer project

The primary consumer will be:

<https://github.com/jacazul-ai/jacazul-ai-cli>

`jacazul-ai-cli` is expected to use `jaflow` as the underlying flow/task engine for agent workflow state, project context, and session navigation.

## CLI design direction

The CLI follows a Git-like command model:

```text
jaflow <command> [<args>]
```

The global layer parses global options and dispatches to a command. After dispatch, the command owns its arguments.

Examples of the intended shape:

```text
jaflow help
jaflow help <command>
jaflow plan <name> ...
jaflow focus task <uuid>
jaflow status
jaflow ponder
```

The command registry should become the source of truth for command metadata, routing, and help rendering. The parser is an implementation detail; it should not dictate the user experience.

Agent-facing help is an operational briefing, not a short usage line:

```text
jaflow help
jaflow help plan
```

Help explains the workflow role, prerequisites, dependency effects, state
transitions, output, actionable errors, examples, and the next valid command.
Errors follow the `Error as Prompt` contract and include an `ACTION:` whenever
the agent needs to recover.

## Local storage direction

`jaflow` owns the database driver and opens one SQLite database per project.
All project workflow state lives under:

```text
$JACAZUL_HOME/jaflow/<PROJECT_ID>/jaflow.sqlite3
```

The database contains initiatives, tasks, dependencies, annotations, focus,
sessions, cache, and roadmap state. `sqlok` owns SQL generation, schema
definitions, migrations, and query/session abstractions over the
application-provided `database/sql` connection. The workflow engine must not
embed a second SQL builder or import `sqlok/internal`.

## Task lifecycle

Tasks inside an initiative form a dependency chain. A blocked task cannot be
started until its dependency is complete:

```bash
jaflow plan parity "Define schema" "Implement store"
jaflow execute <first-uuid>
jaflow outcome <first-uuid> "Schema defined"
jaflow done <first-uuid>
```

`done` requires an `OUTCOME` and reports the next task released by the chain.
Use `jaflow help <command>` for prerequisites, side effects, recovery actions,
and the next valid command.

## Focus and session switching

Focus is stored per project and session. The task stack makes switching work
safe without losing the initiative anchor:

```bash
jaflow focus plan parity
jaflow focus task <uuid>
jaflow focus pop
jaflow focus clear
jaflow session list
```

Use `JACAZUL_SESSION_ID` to isolate one agent session from another while they
share the same project database.

## Future server orchestration

The first versions can operate locally, but the design should not block a future centralized coordination layer.

Future server-backed orchestration may include:

- syncing tasks and context across environments
- sharing focused session state between agents
- coordinating task ownership and workflow transitions
- storing long-lived context outside a single machine
- exposing APIs for `jacazul-ai-cli` and other Jacazul tools

The local CLI should therefore keep clean boundaries between command routing, workflow logic, persistence, and context transport.

## Design principles to preserve

### Error as Prompt

Errors are operational signals. When a command fails, the error output should guide the next action instead of being treated as noise.

A good error should answer:

- what failed
- why it failed, when known
- what action should be taken next

### Prompt as Ad

Operational banners, tips, warnings, and instructions are part of the interface contract. If the tool prints guidance, agents should treat it as mandatory context, not decoration.

This applies to output such as:

- focus guidance
- cache messages
- warning banners
- action hints
- mode restrictions

### Context Cache

Repeated status and dashboard commands should be cache-aware.

The existing behavior to preserve:

- unchanged output may return a short cached signal
- the cached signal means the previous full output still applies
- force refresh should exist, but should not be the default
- session-scoped cache should avoid cross-session context leaks

## Current status

This repository is in bootstrap.

Initial work is focused on:

- creating the Go module
- defining the CLI command model
- documenting the migration scope
- preparing the first project files
