# jaflow Architecture

## Boundary

`jaflow` coordinates operational workflow state. It does not own project
knowledge, client prompt generation, persona rendering, or launcher bootstrap.

The architecture must preserve the current local feature-parity target while
leaving a clean boundary for future team orchestration. The target distributed
context model is documented in [DISTRIBUTED-CONTEXT.md](DISTRIBUTED-CONTEXT.md).

## Runtime Layers

```text
cmd/jaflow
    ↓
internal/cli
    ↓
Coordinator
    └── sqlok ProjectStore
        └── SQLite database (one file per PROJECT_ID)
            ├── initiatives and tasks
            ├── dependencies and annotations
            ├── focus and sessions
            └── derived output cache
```

### CLI

`cmd/jaflow/main.go` owns the process boundary:

- global option parsing;
- command registration;
- help and version handling;
- stderr and exit status.

`internal/cli` owns one concrete command type per command. Each command owns
its positional arguments and exposes an `Execute(args []string) error`
boundary, following the `nvimim` pattern.

The CLI must not contain persistence or orchestration policy.

### Coordinator

The local `Coordinator` is the agent-side orchestration boundary. It combines
workflow operations without becoming a monolithic command manager.

It coordinates:

- initiative and task transitions;
- dependencies and readiness;
- focus and handoffs;
- outcomes and structured annotations;
- ownership of the local operation;
- cache invalidation after state changes.

Commands should call focused Coordinator operations. They must not manipulate
storage files directly.

### TaskBackend

`TaskBackend` is the behavior contract for task management. It represents
workflow operations rather than raw Taskwarrior binary commands.

The current implementation target is the official `sqlok`-backed local
store and native Jaflow source of truth:

- one project-direct directory at
  `$JACAZUL_HOME/jaflow/<PROJECT_ID>/`;
- one physical `jaflow.sqlite3` file inside that directory;
- database location resolved from the project-scoped runtime root;
- schema migrations, SQL generation, and transaction boundaries delegated to
  `sqlok`;
- no dependency on the Taskwarrior binary for normal operation;
- independent from process-global state through injected paths and
  configuration;
- independent from the Taskwarrior binary and its runtime data.

Taskwarrior compatibility is limited to an optional legacy import/export
adapter. It belongs to the separate migration boundary and must never become
the normal local backend, a server dependency, or the source of truth for
Jaflow state.

A future `ServerTaskBackend` may implement the same behavior contract for
shared team coordination. That backend is not part of the current parity
implementation.

### SQLite Persistence

The project-direct directory is the complete local workflow container. Its
SQLite file stores initiatives, tasks, dependencies, annotations, focus,
sessions, cache, and roadmap state; session state is not scattered into
separate files. The `sqlok`-backed project store owns the local database
boundary. `sqlok` is responsible for:

- resolving or receiving one database path for the canonical `PROJECT_ID`;
- SQLite dialect behavior and parameterized SQL generation;
- schema migrations, constraints, and indexes;
- connection, transaction, locking, and atomic durability boundaries.

`jaflow` is responsible for workflow/domain behavior and asks `sqlok` to
persist it. Neither layer knows CLI prompt rendering, agent personas, or
external GitHub credentials.

Schema evolution uses Pressly Goose as an embedded library, not as a CLI
subprocess. The SQLite store supplies an `embed.FS` migration provider with
nine ordered migration steps: the initial schema, task lifecycle columns,
the roadmap ledger, native session notes, task due dates, the task mode
catalog, task metadata, session plan interests, and workflow history. The provider uses its own version
table and keeps the application silent by default. The task lifecycle
migration is a Go migration so it can safely add missing columns to databases
created by the earlier migration runner.

Focus, sessions, and cache remain separate behavioral concerns, but their
records are scoped inside the owning project's database. Do not create a
generic grab-bag interface just to hide SQLite.

### SQL Backend Decision

`sqlok` is the official SQL backend for `jaflow`. The current `sqlok` public
surface is still under development, so the parity work must collect and close
its required backend capabilities instead of duplicating SQL generation inside
`jaflow`.

The required public `sqlok` surface is:

- SQLite dialect and parameterized SQL generation;
- public schema definitions for tables, indexes, foreign keys, and constraints;
- migration creation and application with version tracking;
- connection and transaction lifecycle boundaries;
- SQLite in-memory and temporary-file test support;
- low-complexity, wrapped errors suitable for the `Error as Prompt` boundary.

`jaflow` owns the concrete `database/sql` driver and connection lifecycle.
Driver choices such as `modernc.org/sqlite`, `go-libsql`, or a future Turso
adapter are selected by `jaflow` and remain behind `ProjectStore`. `sqlok`
receives the application-provided connection and SQLite dialect to generate and
execute SQL. `jaflow` must not import `sqlok/internal` or embed a parallel SQL
builder.

Remote replication, primary URLs, credentials, and sync failure semantics are
future backend concerns. Local feature parity must remain usable without them.

## Storage Contracts

The project has two real storage directions:

```text
ProjectStore
├── sqlokStore               # official local implementation
├── LegacyTaskwarriorAdapter # optional migration/import boundary
└── ServerTaskBackend        # future shared implementation
```

`initiative` is a first-class domain entity. A Taskwarrior `project` value is
only a compatibility projection or migration key. Tasks reference an
`initiative_id`; initiative lifecycle, metadata, and external tickets do not
come from grouping strings.

Behavior-focused contracts may be split when their consumers differ:

- initiative and task state;
- dependencies and readiness;
- annotations and ticket metadata;
- focus and session state;
- output cache and invalidation.

Do not create interfaces solely for hypothetical implementations. Keep the
local store independent from the CLI so the optional legacy adapter and future
server backend can be introduced without changing native command behavior.

Required cross-backend invariants include:

- stable UUIDs;
- explicit `PROJECT_ID` scope;
- explicit `SESSION_ID` scope where applicable;
- idempotent transitions where possible;
- revision/version metadata for future coordination;
- context cancellation for operations that can become remote.

## Team Orchestration

Team collaboration is a future boundary above the local agent workflow:

```text
Local Agent A ─┐
Local Agent B ─┼── Team Coordinator ── Shared TaskBackend
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

The local Coordinator owns local execution and presentation. It must not
pretend that a copied snapshot is a shared workflow.

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
but a repository snapshot is not a live shared Coordinator. Bidirectional sync
requires explicit revision, ownership, lease, and conflict semantics.

## Current Phase Boundary

During feature parity:

- implement the local per-project SQLite store and schema migrations;
- model initiatives as first-class records and tasks through `initiative_id`;
- keep Taskwarrior as an import/export compatibility boundary only;
- do not shell out to the Taskwarrior binary for normal workflow operations;
- keep the Coordinator local;
- port and validate observable `tw-flow` behavior;
- design, but do not implement, server coordination and live sync;
- keep send/receive/sync as future protocol boundaries unless a parity contract
  requires a local primitive.

The architecture must not add team-server complexity before the local workflow
contracts are proven by meaningful tests and mutation checks.
