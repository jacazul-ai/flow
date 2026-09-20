# flow Architecture

## Boundary

`flow` coordinates operational workflow state: initiatives, tasks,
dependencies, readiness, focus, sessions, annotations, and context. It does
not own project knowledge, client prompt generation, persona rendering, or
launcher bootstrap.

The architecture must preserve the current local feature-parity target while
leaving a clean boundary for future team orchestration. The target distributed
context model is documented in [DISTRIBUTED-CONTEXT.md](DISTRIBUTED-CONTEXT.md).

This document describes what the code implements today. Decided architecture
that is not implemented yet is marked as such where it appears.

## Distribution Model

`flow` is a Go library with thin executables on top. It is not a separately
installed component that another program discovers at runtime.

```text
github.com/jacazul-ai/flow          (this repository)
├── flow.go                         package flow: public boundary
├── cmd/jczl-flow/                  standalone executable
└── internal/...                    implementation packages

github.com/jacazul-ai/jacazul-ai-cli
└── jacazul flow ...                imports package flow and calls flow.Run
```

- **`jacazul flow`** is the user-facing entry point. The jacazul CLI compiles
  the engine in, so one jacazul release carries one engine version and updates
  need no component manager, version handshake, or `PATH` lookup.
- **`jczl-flow`** is built from this repository for tests and independent
  distribution. It runs exactly the same code path as `jacazul flow`.
- The engine version consumed by jacazul is pinned by jacazul's `go.mod`.

The standalone executable installs with:

```bash
go install github.com/jacazul-ai/flow/cmd/jczl-flow@latest
```

## Public API Boundary

The module root is the only public package. Everything else stays under
`internal/`, which the Go toolchain prevents other modules from importing.

```go
package flow

// Run executes one flow command and returns its process exit status.
func Run(ctx context.Context, args []string, env Env, streams Streams) int

// Env is the resolved runtime context for one invocation.
type Env struct {
	ProjectID    string
	SessionID    string
	DatabasePath string
	Home         string
}

// Streams are the invocation's standard streams.
type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// EnvFromOS builds Env from process environment for standalone executables.
func EnvFromOS() Env
```

Rules:

- `Run` receives everything it needs. Inside `Run` the engine never reads
  process environment, never reads or mutates `os.Args`, never writes to the
  process stdout or stderr, and never calls `os.Exit`.
- Thin mains own the process: `cmd/jczl-flow` builds `Env` with `EnvFromOS`;
  jacazul builds `Env` from the project and session it already resolved.
- `ctx` carries cancellation from the caller.
- `go-flags`, `internal/cli`, storage types, and domain types do not cross the
  boundary. Data-returning entry points such as a structured onboard snapshot
  are added as separate, narrow functions only when a real consumer needs them.
- The boundary is a concrete function, not a Go interface. There is one
  implementation. A consumer that needs a test seam defines it in its own
  package, for example a function type whose production value is `flow.Run`.

## Runtime Layers

```text
cmd/jczl-flow/main.go   or   jacazul-ai-cli
            ↓
        flow.Run(ctx, args, Env, Streams)
            ↓
internal/config         option resolution from flags, then the injected Env
internal/cli            command registry and one command type per command,
                        every command writing to the injected Streams
            ↓
internal/storage/sqlite Store opened from the resolved database path
    └── SQLite database (one file per PROJECT_ID)
        ├── initiatives and tasks
        ├── dependencies and annotations
        ├── focus, sessions, and plan interests
        ├── roadmap ledger and workflow history
        └── derived output cache

internal/task           domain types and validation
internal/migration      Taskwarrior snapshot import
internal/testharness    isolated fixtures for contract tests
```

The thin main owns the process: it builds `Env` and `Streams`, calls
`flow.Run`, and turns the returned status into an exit code. Inside `Run`
nothing reads the process environment or `os.Args`, and nothing writes to the
process streams: `internal/config` resolves options from flags and then the
injected `Env`, and every command in `internal/cli` prints through the
injected `Streams` and passes the invocation context to the store.

### CLI

`internal/cli` owns one concrete command type per command. Each command owns
its positional arguments and exposes an `Execute(args []string) error`
boundary, following the `nvimim` pattern. Commands are registered through an
explicit `CommandRegistry` on a `go-flags` parser.

The CLI must not contain persistence policy. Commands call focused `Store`
operations and must not manipulate database files directly.

## Persistence

### SQLite store

`internal/storage/sqlite.Store` is the single local workflow store. It uses
`database/sql` with the pure-Go `modernc.org/sqlite` driver, a single open
connection per store, and parameterized SQL written in the store package.

The project-direct directory is the complete local workflow container. Its
SQLite file stores initiatives, tasks, dependencies, annotations, focus,
sessions, plan interests, roadmap state, workflow history, and the derived
output cache. Session state is not scattered into separate files. The output
cache lives in `cache_entries`, scoped by project and session.

### Schema migrations

Schema evolution uses Pressly Goose as an embedded library, not as a CLI
subprocess. `Store` applies pending migrations when it opens a database. The
provider has ten ordered migration steps:

1. initial schema;
2. task lifecycle columns (a Go migration, so it can add missing columns to
   databases created by the earlier migration runner);
3. roadmap ledger;
4. native session notes;
5. task due dates;
6. task mode catalog;
7. task metadata;
8. focus plan interests;
9. workflow history;
10. task order.

The provider uses its own version table and keeps the application silent by
default.

Before applying anything, the store compares the version already recorded in
the database against the highest migration embedded in the binary. A database
a newer release migrated is refused with both versions and an `ACTION:`,
rather than opened and written through by a binary that cannot know what the
newer schema means. A database that is merely behind still migrates up.

### Database location

| | Path |
|---|---|
| Default | `$JACAZUL_HOME/flow/<PROJECT_ID>/flow.sqlite3` |
| Override | `--database-path`, `JACAZUL_FLOW_DATABASE_PATH` |

A database left at the pre-rename location,
`$JACAZUL_HOME/jaflow/<PROJECT_ID>/jaflow.sqlite3`, is moved to the current
path the first time the store opens. The move carries the `-wal` and `-shm`
sidecars and fails closed with `ACTION:` guidance when both paths exist, the
legacy database is locked by another writer, or the rename would cross
filesystems. The legacy path is derived only when the database path itself was
derived, so an explicit `--database-path` migrates nothing.

`flow.Run` requires `Home` from its caller to derive default paths and fails
with `ACTION:` guidance without it. Only `EnvFromOS`, for standalone
executables, falls back to the user home directory when `JACAZUL_HOME` is
unset.

### SQL layer: sqlok (deferred)

[`sqlok`](https://github.com/candango/sqlok) remains the intended
SQLAlchemy-like query, schema, and migration layer. The ownership split is:

- `flow` owns the concrete driver (`modernc.org/sqlite`) and the
  `database/sql` connection lifecycle per project;
- `sqlok` receives the application-provided connection and SQLite dialect to
  generate DDL and parameterized SQL, apply migrations, and run
  transaction-aware queries.

The integration is deferred, not abandoned. `sqlok` does not yet expose the
public surface `flow` needs: a SQLite dialect, schema and migration APIs, and
transaction-aware execution over an application-provided `*sql.DB`. Its
current integration target is PostgreSQL. Because the concrete workflow
behavior is the source material for that API, `flow` first built a provisional
store with explicit SQL and embedded Goose migrations, and the parity review
kept that store for the Taskwarrior cutover.

Until the refactor:

- keep all direct SQL isolated inside `internal/storage/sqlite`;
- do not add a second SQL builder or import `sqlok/internal`;
- treat the current SQL and migrations as provisional evidence for the `sqlok`
  API, to be moved onto it in a separate architecture task.

## Storage Contracts

The store is a concrete type, not an interface. Other storage directions are
real boundaries, but they are not store implementations today:

```text
internal/storage/sqlite.Store   local implementation (current)
internal/migration              Taskwarrior snapshot import (current)
server backend                  shared team coordination (future)
```

`initiative` is a first-class domain entity. A Taskwarrior `project` value is
only a compatibility projection or migration key. Tasks reference an
`initiative_id`; initiative lifecycle, metadata, and external tickets do not
come from grouping strings.

Introduce a storage interface only when a second real implementation exists,
such as the future server backend, and define it next to the consumer that
switches between implementations. Behavior-focused contracts may then be split
when their consumers differ:

- initiative and task state;
- dependencies and readiness;
- annotations and ticket metadata;
- focus and session state;
- output cache and invalidation.

Required cross-backend invariants include:

- stable UUIDs;
- explicit `PROJECT_ID` scope;
- explicit `SESSION_ID` scope where applicable;
- idempotent transitions where possible;
- revision/version metadata for future coordination;
- context cancellation for operations that can become remote.

## Taskwarrior Compatibility

Taskwarrior compatibility is limited to the import boundary in
`internal/migration`. It reads an explicit Taskwarrior export snapshot and
optional legacy focus and session files, and writes native records through the
store. It must never become the normal local backend, a server dependency, or
the source of truth for workflow state, and normal operation never invokes the
Taskwarrior binary. The procedure is documented in [migration.md](migration.md).

## Team Orchestration

Team collaboration is a future boundary above the local agent workflow:

```text
Local Agent A ─┐
Local Agent B ─┼── Team Coordinator ── Shared backend
Local Agent C ─┘
```

The future Team Coordinator owns shared concerns:

- initiative and plan registry;
- task assignment and ownership;
- execution leases;
- handoff acknowledgement;
- revisions and conflict resolution;
- audit events;
- authentication and authorization.

Local execution and presentation stay in the local engine. It must not pretend
that a copied snapshot is a shared workflow.

## Initiative Exchange

An initiative is a portable operational-work aggregate. Its envelope should
include:

```text
schema_version
initiative_id
source_project
source_agent
revision
external_ticket
tasks[]
  task_id
  description
  mode
  status
  dependencies[]
  annotations[]
ownership
```

By default it excludes:

- source code;
- project documentation;
- output caches;
- credentials;
- private session details unrelated to the handoff.

The intended progression is:

```text
send <ini>       one-way workflow handoff
receive <ini>    materialize a local workflow copy
sync <ini>       reconcile later changes
```

A repository target can transport an envelope during an early local phase,
but a repository snapshot is not a live shared coordinator. Bidirectional sync
requires explicit revision, ownership, lease, and conflict semantics.

## Current Phase Boundary

During feature parity:

- keep the local per-project SQLite store and its embedded migrations;
- model initiatives as first-class records and tasks through `initiative_id`;
- keep Taskwarrior as an import compatibility boundary only;
- do not shell out to the Taskwarrior binary for normal workflow operations;
- rename the module, executable, runtime paths, and environment variables to
  `flow`, `jczl-flow`, and `JACAZUL_FLOW_*`;
- move the process boundary into `flow.Run` so jacazul can embed the engine;
- port and validate observable `tw-flow` behavior;
- design, but do not implement, server coordination and live sync;
- keep send/receive/sync as future protocol boundaries unless a parity contract
  requires a local primitive.

The architecture must not add team-server complexity before the local workflow
contracts are proven by meaningful tests and mutation checks.
