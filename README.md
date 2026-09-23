# flow

`flow` is the Jacazul workflow engine.

This project is the Go-native migration of the current `tw-flow` workflow tooling used by [jacazul-ai-cli](https://github.com/jacazul-ai/jacazul-ai-cli).

The goal is not to wrap Taskwarrior forever. The goal is to move the useful workflow semantics into a Go implementation that can serve Jacazul agents with a sharper, more portable, and more controllable flow engine.

## Names

The engine answers to four names, one per layer:

| Name | Layer |
|---|---|
| `github.com/jacazul-ai/flow` | Go module |
| `flow` | public Go package, whose only entry point is `flow.Run` |
| `jczl-flow` | standalone binary, built from `cmd/jczl-flow` |
| `jacazul flow` | what an operator types |

`jacazul flow` is the user-facing form. The engine is imported into
`jacazul-ai-cli` as a Go library and compiled into the single `jacazul` binary,
so engine updates ship with `jacazul` itself rather than as a separate
component. `jczl-flow` builds the same engine as its own executable for tests,
development, and anyone who wants the engine without the rest of Jacazul.

### Naming history

The engine was almost called something else. The candidates and why they lost:

- **rastro** (pt-br for *trace*) was chosen first, then dropped. Registries
  were clear, but Rastro AI (`rastro.ai`, `rastro-mcp` for Claude and Codex) is
  an established brand in the same AI-agent niche, and two small GitHub CLIs
  already ship a `rastro` binary.
- **lastro** (pt-br for *ballast*) was rejected twice over: `loonix/lastro` is
  a Go CLI for signed AI-agent and CI receipts built on the same Portuguese pun
  in the same niche, and Lastro is a funded Brazilian AI startup.
- **sulco** (pt-br for *furrow*) screened clean and was superseded when the
  engine stopped being distributed under a name of its own.
- **plan** was rejected as the subcommand: it collides with the engine's own
  `plan` verb and is too narrow for focus, sessions, handoff, and context.

No trademark search was performed at INPI or CIPO; the naming decision rests on
registry and brand-collision research only.

The RASTRO acronyms survive as livestream lore, not as product names:

- Record of AI State, Tasks, Research & Outcomes
- Relentless AI Stalker of Tasks, Regrets & Outcomes

## Why this exists

Today, `jacazul-ai-cli` relies on shell-based Taskwarrior helpers such as:

- `taskp`
- `tw-flow`
- `tw-flow ponder`
- session focus/context helpers
- structured task annotations
- per-project Taskwarrior databases

Those tools work, but they are split across shell scripts, Taskwarrior behavior, local conventions, and agent instructions.

`flow` exists to consolidate that behavior into a Go project.

## Migration scope

The first migration target is the Taskwarrior workflow skill currently living in:

```text
jacazul-ai-cli/tw-flow-to-go/skills/taskwarrior-expert
```

The migration scope includes:

- one native SQLite database per canonical `PROJECT_ID`
- a driver owned by `flow`, with SQL and migrations kept behind one store
- per-project database isolation
- plan/initiative creation
- task focus and anchor management
- task execution modes such as `DESIGN`, `INVESTIGATE`, `GUIDE`, `EXECUTE`, `TEST`, `DEBUG`, and `REVIEW`
- structured annotations such as `DECISION`, `RESEARCH`, `BLOCKED`, `LESSON`, `OUTCOME`, and `HANDOFF`
- project dashboard/status views currently handled by `tw-flow ponder` and `tw-flow status`
- session handoff and resume context
- UUID-first task references
- output caching for repeated status/dashboard calls

Long-term, `flow` should implement the Taskwarrior-like behavior needed by Jacazul workflows directly in Go. Taskwarrior compatibility is a design constraint, not the final architecture.

The local engine should also be designed so it can connect to a centralized server in the future. That server would orchestrate tasks, context, session state, and agent workflow coordination across machines or agents when needed.

## Documentation

- [Vision](docs/VISION.md): mission, operational memory, and team direction.
- [Architecture](docs/ARCHITECTURE.md): distribution model, the `flow.Run`
  boundary, runtime layers, persistence, and future team orchestration.
- [Feature parity](docs/feature-parity.md): reference contracts, test audit,
  implementation backlog, and parity completion criteria.
- [Migration](docs/migration.md): Taskwarrior mapping, safety, idempotency,
  verification, cutover, and rollback.
- [CLI navigation](docs/cli.md): intent-first help taxonomy, aliases, and
  workflow navigation examples.
- [Output formats](docs/output-formats.md): the text, JSON, JSONL and XML
  contracts, how a format is selected, and which commands offer one.
- [Task history](docs/task-history.md): native events, TaskChampion extraction,
  import rules, and verification contracts.
- [Distributed context](docs/DISTRIBUTED-CONTEXT.md): the target multi-agent
  context model, event log, and connector boundaries.
- [Agent workflow reference](docs/AGENT-WORKFLOW-REFERENCE.md): the evidence
  boundary for parity tests against `tw-flow`.

## Consumer project

The primary consumer will be:

<https://github.com/jacazul-ai/jacazul-ai-cli>

`jacazul-ai-cli` is expected to use `flow` as the underlying flow/task engine for agent workflow state, project context, and session navigation.

## CLI design direction

The CLI follows a Git-like command model:

```text
jczl-flow <command> [<args>]
```

The global layer parses global options and dispatches to a command. After dispatch, the command owns its arguments.

Examples of the intended shape:

```text
jczl-flow help
jczl-flow help <command>
jczl-flow plan <name> ...
jczl-flow focus task <uuid>
jczl-flow status
jczl-flow ponder
```

The command registry should become the source of truth for command metadata, routing, and help rendering. The parser is an implementation detail; it should not dictate the user experience.

Agent-facing help is an operational briefing, not a short usage line:

```text
jczl-flow help
jczl-flow help plan
```

Help explains the workflow role, prerequisites, dependency effects, state
transitions, output, actionable errors, examples, and the next valid command.
Errors follow the `Error as Prompt` contract and include an `ACTION:` whenever
the agent needs to recover.

## Local storage direction

`flow` owns the database driver and opens one SQLite database per project.
All project workflow state lives under:

```text
$JACAZUL_HOME/flow/<PROJECT_ID>/flow.sqlite3
```

The database contains initiatives, tasks, dependencies, annotations, focus,
sessions, cache, and roadmap state. All SQL and the embedded Goose migrations
stay inside `internal/storage/sqlite`; no command touches database files
directly. Moving that SQL onto [`sqlok`](https://github.com/candango/sqlok) is
still the intended direction and is deferred, not abandoned; see
[Architecture](docs/ARCHITECTURE.md) for the ownership split and what `sqlok`
has to expose first.

## Task lifecycle

Tasks inside an initiative form a dependency chain. A blocked task cannot be
started until its dependency is complete:

```bash
jczl-flow plan parity "Define schema" "Implement store"
jczl-flow execute <first-uuid>
jczl-flow outcome <first-uuid> "Schema defined"
jczl-flow done <first-uuid>
```

`done` requires an `OUTCOME` and reports the next task released by the chain.
Use `jczl-flow help <command>` for prerequisites, side effects, recovery actions,
and the next valid command.

## Focus and session switching

Focus is stored per project and session. The task stack makes switching work
safe without losing the initiative anchor:

```bash
jczl-flow focus plan parity
jczl-flow focus task <uuid>
jczl-flow focus pop
jczl-flow focus clear
jczl-flow session list
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

## Building and testing

```bash
make          # list the available targets
make test     # go clean -testcache && go test -race -v ./...
make vet      # go vet ./...
make build    # CGO_ENABLED=0 go build -o bin/jczl-flow ./cmd/jczl-flow
```

`make test` always runs with the race detector and without the test cache.

Tests run against temporary directories and fake executables only. They never
touch a real `JACAZUL_HOME`, a real workflow database, or the network.

To install the standalone executable without cloning:

```bash
go install github.com/jacazul-ai/flow/cmd/jczl-flow@latest
```

## Current status

The engine runs. The command model, the per-project SQLite store with its
embedded migrations, the Taskwarrior snapshot importer, and the `flow.Run`
boundary that lets jacazul embed the engine are all implemented and covered by
contract tests.

The current phase is feature parity with `tw-flow`. Work is focused on:

- porting the remaining reference behavior and its contract tests
- plugging `flow.Run` into `jacazul-ai-cli`
- preparing the Taskwarrior cutover

Server coordination and live sync are designed but deliberately not
implemented; see [Architecture](docs/ARCHITECTURE.md) and
[Distributed context](docs/DISTRIBUTED-CONTEXT.md).

## License

MIT. See [LICENSE](LICENSE).
