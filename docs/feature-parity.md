# flow Feature Parity

## Purpose

This document defines the feature-parity target for the current port from the
Python/shell `tw-flow` implementation in `jacazul-ai-cli` to Go.

Parity means preserving observable workflow contracts, safety behavior, state
transitions, isolation boundaries, and actionable errors. It does not mean
copying the Python structure or its internal implementation.

Reference tests:

```text
~/source/jacazul-ai/jacazul-ai-cli/master/tests
```

The local source of truth is a native SQLite database with one physical file
per canonical `PROJECT_ID`. An initiative/plan is a first-class record; a
Taskwarrior `project` value is only a compatibility projection or migration
key. Sessions and cache records are scoped by `session_id` inside the owning
project database. Taskwarrior is not a runtime dependency; it is retained only
as an optional legacy import/export boundary for migration.

The in-repository `internal/testharness` owns sandboxed database fixtures.
Every parity slice must test its behavior locally and must prove that separate
project databases cannot observe one another's state.

## Current Phase Rule

This is a temporary rule for the porting phase only:

```text
Red -> Green -> Refactor -> Mutation Check -> Parity Review
```

1. Write a valid failing contract test.
2. Implement the smallest behavior that makes it pass.
3. Refactor command boundaries and reduce accidental complexity.
4. Break the protected rule intentionally and verify the test fails.
5. Compare observable behavior with the reference test and implementation.

A green build without meaningful parity coverage is not feature completion.

## Reference Test Audit

The reference suite contains candidate tests for the following core/runtime
areas:

| Area | Reference tests | Parity treatment |
|---|---|---|
| Project isolation | `core_test.py`, `test_project_identity.py` | Port with temporary project state and cross-project leak checks |
| Lifecycle | `flow_test.py` | Port plan, dependencies, execute, outcome, done, reopen, discard, and handoff |
| Focus and sessions | `flow_test.py`, `test_session_list.py` | Port global focus, task stack, smart focus, independent sessions, heartbeat, and fallback |
| Context | `flow_test.py` | Port structured annotations, inherited context, notes, and ticket inheritance |
| Cache | `cache_test.py` | Port storage, TTL, cached signals, force refresh, selective invalidation, and session isolation |
| Dashboard | `flow_test.py`, `test_recently_closed.py` | Port status, ponder, plans, filters, history, and output contracts |
| Backlog | `test_backlog.py` | Port hide/show/activate behavior and state markers |
| Roadmap | `test_roadmap_init_guard.py` | Port duplicate-ledger protection for pending and completed phases plus roadmap phase shipping |
| Safety | `security_test.py`, `flow_test.py` | Port workflow vaccination, outcome gates, and actionable errors |
| Operational output | `AGENTS.md` Error as Prompt/Prompt as Ad rules | Keep healthy verification quiet, emit state changes, and provide `ACTION:` guidance on failures |
| Ticket boundary | selected `flow_test.py` tests | Port metadata persistence and inherited ticket awareness; provider brokers remain external |

The initial inventory identified 104 candidate core/runtime tests:

- core: 3
- flow: 41
- cache: 29
- security: 6
- backlog: 5
- recently closed: 4
- roadmap guard: 3
- session list: 9
- project identity: 4

## Complete Reference Command Acceptance Matrix

The parity target is the complete observable `tw-flow` command surface. This
matrix is an inventory and acceptance contract, not a pre-filter for commands
that are convenient to port. Every reference surface below requires a native
implementation and contract coverage unless a deliberate incompatibility is
later documented with evidence.

| Surface | Reference contract | Native acceptance boundary | Reference evidence |
|---|---|---|---|
| `plan`, `initiative`, `ini` | Create an initiative and an ordered task chain; accept mode, tag, and optional due metadata; default to priority `M` without inventing a due date; support hyphenated names and short UUID output. | Persist the initiative and dependency chain in the native store, preserve task mode semantics, invalidate derived views, and keep full UUIDs as identity. | `flow_test.py` plan and cool-down coverage |
| `plans`, `inis`, `initiatives` | List active initiatives by default; support `--all`, `--closed`, `--with-backlog`, and `--force`; aliases share the same behavior; cache keys are filter-specific. | Render the same lifecycle markers and counts, hide backlog by default, and keep each filter isolated in cache. | `flow_test.py`, `cache_test.py`, `test_backlog.py` |
| `status` | Resolve the focused initiative when no filter is supplied; support pending-only and table views; show pending/completed tasks, focus, inherited context, tickets, mode alerts, and actionable restrictions. | Match output, ordering, exit behavior, inherited context, and cache signals without leaking another project or session. | `flow_test.py`, `cache_test.py` |
| `ponder` | Render the project dashboard with tactical readout, interests, blocked/active counts, filters, backlog handling, recently closed history, and `--force`. | Produce the complete dashboard in one call, preserve project/session scope, and invalidate or bypass cache exactly as required. | `flow_test.py`, `cache_test.py`, `test_recently_closed.py` |
| `next` | Select ready work, optionally constrained to an initiative or filter; never treat every `pending` task as executable; report when no task is ready. | Query native readiness, preserve initiative boundaries, display short UUIDs, and reject blocked candidates. | `flow_test.py`, `docs/focus-advance-i-d.md` |
| `execute`, `outcome`, `done` | Start ready work, require an `OUTCOME` before completion, expose newly unblocked tasks, and preserve handoff/focus behavior. | Enforce lifecycle transitions atomically, keep blocked tasks unexecutable, emit actionable errors, and invalidate affected views. | `flow_test.py`, `test_execute_loop_control.py` |
| `reopen`, `discard` | Reopen completed work; discard through the workflow boundary with archive movement and an auditable outcome. | Preserve historical annotations, enforce safe state transitions, and prevent direct unsafe deletion paths. | `flow_test.py`, `security_test.py` |
| `handoff` | Start the target task when needed and add a signed `HANDOFF` annotation. | Preserve dependency readiness, task signatures, focus/session state, and cache invalidation. | `flow_test.py`, `test_task_signature.py` |
| `amend`, `rename` | Amend description/ticket metadata; rename an initiative and synchronize task grouping, focus, interest state, and inherited references. | Keep completed-task safety rules, update all native references consistently, and provide an actionable validation error. | `flow_test.py` |
| `note`, `notes`, `context`, `ticket` | Persist semantic annotations, list/delete timestamped notes, collect dependency-first context, and resolve direct or inherited tickets. | Keep annotation kinds, timestamps, signatures, inheritance order, UUID resolution, and ticket cache invalidation compatible. | `flow_test.py`, `test_task_signature.py` |
| `focus` | Support global and independent plan/task focus, task stack/pop, interest add/remove/list, clear, back, aliases, and smart focus. | Scope anchors by project and session, select only valid ready work where execution is implied, and never cross initiative boundaries. | `flow_test.py`, `test_execute_loop_control.py` |
| `session` | Support `list`, `show`, `resume`, `ack`, `dump`, and `purge`; track heartbeat age and current-session markers; preserve handoff lifecycle. | Persist native sessions separately per project/session, classify active/idle/orphan consistently, and require confirmation for purge. | `flow_test.py`, `test_session_list.py` |
| `active`, `blocked`, `overdue`, `urgent`, `tree` | Report derived task views; mark urgency; show dependency tree; preserve status and dependency semantics. | Match filters, output, metadata mutation, and actionable failures while keeping blocked and ready states distinct. | `flow_test.py`, `flow.py` dispatch; add focused tests where absent |
| `block`, `unblock`, `wait` | Add/remove dependency edges and set waiting dates through the workflow command boundary. | Validate UUIDs, prevent invalid or cross-project edges, preserve dependency readiness, and invalidate affected views. | `flow.py` dispatch; add focused contract tests |
| `backlog`, `activate` | Hide or restore an initiative in dashboard views while preserving its tasks and recoverability. | Persist initiative backlog state, render markers, and invalidate only affected derived views. | `test_backlog.py` |
| `cache` | Support `info`, `clear`, `clear status`, and `clear ponder`; maintain TTL signals, force refresh, filter-specific keys, selective invalidation, and session isolation. | Keep cache storage project/session scoped, remove only requested entries, and never expose stale cross-scope output. | `cache_test.py`, `test_execute_loop_control.py` |
| `roadmap`, `ship` | Initialize a roadmap once, add/show phases, ship phase entries, and reject duplicate ledgers without creating ghost tasks. | Preserve roadmap history and initiative projection while enforcing the initialization guard and phase validation. | `test_roadmap_init_guard.py`, `flow_test.py` |
| `commit` | Draft a conventional commit from focus, select `Refs` versus `Fixes`, and never stage or commit implicitly. | Render the repository-aware draft, use inherited tickets, preserve user confirmation gates, and exclude internal IDs. | `flow_test.py`, `test_execute_loop_control.py` |
| `help` and aliases | Render usage, command guidance, prerequisites, effects, examples, and next actions for the complete command tree. | Keep registry metadata authoritative and ensure every exposed command is documented and routable. | `core_test.py`, native command registry |

### Matrix-wide acceptance rules

Every row is also subject to these rules:

- Full UUIDs are the persisted identity; short UUIDs are presentation and
  must be unambiguous at the command boundary.
- Project, initiative, and session boundaries are hard isolation rules. A
  command must not read, mutate, focus, or cache another scope's state.
- `pending` is not the same as `ready`; dependency queries own readiness.
- Errors go to the failure channel with a concrete `ACTION:`. Healthy checks
  stay quiet unless the command is a report.
- Cache behavior includes storage location, TTL, cache-hit signal, force
  refresh, filter-specific keys, selective invalidation, and session isolation.
- Completed-task mutation rules and `OUTCOME` completion gates remain
  observable contracts. Notes may remain valid on completed tasks only where
  the reference contract allows them.
- The native SQLite store remains the workflow source of truth. Taskwarrior
  compatibility belongs at an explicit migration boundary, not in normal
  command execution.
- Known reference defects are negative acceptance cases, not behavior to copy.
  In particular, automatic focus advancement must never select a blocked task
  merely because its status is `pending`.

### Inventory completion rule

The design slice is complete when every command and alias in this matrix has
one of the following concrete outcomes:

1. a native contract test and implementation;
2. a recorded failing parity test with an assigned implementation slice; or
3. an explicitly documented incompatibility backed by evidence and user
   confirmation.

A command is not considered out of scope merely because the initial native
registry does not expose it. Low-hanging structural improvements may be made
while implementing the matrix when they preserve the observable contract,
carry appropriate tests, and are reported to the user before they expand the
change boundary.

## Residual Context Contract

The remaining context slice follows the reference `tw-flow` behavior without
reintroducing Taskwarrior as the native source of truth:

- Annotations persist an uppercase semantic kind, body, and creation timestamp.
  Supported note aliases include `RESEARCH`, `DECISION`, `OUTCOME`, `HANDOFF`,
  `BLOCKED`, `LESSON`, `QUESTION`, `HYPOTHESIS`, `AC`, `NOTE`, and `LINK`.
- `notes` lists annotations with timestamps, and `note delete` removes one
  annotation by its timestamp. Notes remain valid on completed tasks.
- `context` and focused status output collect relevant annotations from the
  dependency graph in dependency-first order. The focused task is excluded
  from inherited context; direct task context remains available separately.
- A direct task ticket takes precedence. If absent, ticket lookup recursively
  walks dependencies and reports the first available inherited ticket.
- `handoff` starts the target task when necessary and records a `HANDOFF`
  annotation on that task. It must preserve the existing lifecycle safety
  rules and actionable errors.

The native context command surface is:

```text
jczl-flow note <uuid> <type> <message...>
jczl-flow notes <uuid>
jczl-flow context <uuid>
jczl-flow ticket <uuid> <ticket>
jczl-flow handoff <uuid> <message...>
jczl-flow focus ind task <uuid>
jczl-flow focus back
jczl-flow session <list|show|resume|ack|dump|purge>
jczl-flow active [initiative]
jczl-flow blocked [initiative]
jczl-flow overdue [initiative]
```

`note` normalizes semantic aliases to uppercase kinds, `notes` exposes
creation timestamps for deletion, and `context` renders direct plus inherited
annotations. These commands operate on the native SQLite store; they do not
require the Taskwarrior binary.

Task interaction modes are catalog values in `task_modes`. Tasks persist a
stable `task_mode_code` foreign key; the CLI and API translate it to semantic
names such as `DESIGN` and `EXECUTE` at the boundary. The legacy text mode
column remains only for migration compatibility.

This contract is intentionally expressed at the workflow/store boundary so
future Taskwarrior import or broker adapters can project the same behavior
without owning native context semantics.

## Test Quality Findings

These reference tests must not be copied blindly:

- `test_backlog.py` and `test_recently_closed.py` do not inject isolated
  `TASKDATA`/`JACAZUL_HOME`; rewrite them with temporary state before porting.
- `flow_test.py::test_vaccinated_done_enforces_python_quality` writes and stages
  a source file; replace it with a sandboxed fake quality-gate contract.
- `test_recently_closed_no_extra_requests` cannot prove internal request count
  from one subprocess invocation; replace it with an observable request seam.
- Several tests ignore setup command exit codes and rely only on substring
  assertions; strengthen them with state assertions and mutation checks.
- Tests that only assert a directory exists, a substring appears, or a command
  returns zero need a deliberate mutation to prove their oracle is meaningful.

A test is valid for parity only when it has an explicit contract, isolated
fixtures, a meaningful oracle, and fails under a relevant mutation.

Error and output contracts are part of parity: failures must guide the next
valid action, while healthy verification must not become noisy protocol dump.

## Legacy Backend Resolution

`flow` commands always use the native project SQLite database. The legacy
`TASKDATA`/`--taskdata` value may remain available as migration context, but it
must not select the runtime backend or invoke the Taskwarrior binary. The
legacy adapter belongs to the separate Taskwarrior-to-`flow` migration
initiative and is not part of normal task creation, lifecycle, context, cache,
or dashboard execution.

## Implementation Backlog

Implement in this order, keeping one command boundary per feature slice and
keeping tests inside the slice that they protect:

1. Define storage boundaries, the first-class initiative model, and the
   per-project SQLite schema contract.
2. Build the in-repository sandboxed SQLite contract harness with separate
   database files per project and baseline parity tests.
3. Implement SQLite migrations, command registry, global options, process
   boundary errors, and project-scoped state.
4. Implement plan/task lifecycle: creation, dependency readiness, execute,
   outcome, done, reopen, discard, and handoff.
5. Implement focus and context: task stack, smart focus, independent sessions,
   annotations, inherited context, ticket metadata, and session-scoped cache.
6. Implement dashboards and derived views: status, ponder, plans, cache TTL,
   cached signals, force refresh, invalidation, backlog, roadmap shipping, recently
   closed history, tree, and commit drafts.
7. Perform final safety, parity, mutation, documentation, race, and vet review.

Tasks 2 through 6 must each follow Red-Green-Refactor with focused contract
coverage and a relevant mutation check. Task 7 is a final review gate, not the
first place tests are defined.

## Command Architecture

Follow the `nvimim` CLI pattern:

```text
cmd/jczl-flow/main.go       parser, global options, command registry, process exit
internal/cli/             one command type and Execute method per command
internal/workflow/        small shared workflow behavior
internal/storage/         project database and persistence boundaries
internal/storage/sqlok/    sqlok-backed schema, migrations, and queries
internal/cache/            cache policy and invalidation behavior
internal/testharness/     isolated project databases and fake externals
```

The CLI must not infer initiative lifecycle from a Taskwarrior project string.
Commands call the sqlok-backed local store through focused behavior boundaries
and render results; they do not manipulate SQL or SQLite tables directly.

Do not translate the Python `FlowManager` into a Go monolith. Feature parity is
measured per command and per contract, not by matching the old class layout.

## Out of Scope for flow Core

Do not port these as flow workflow features:

- JIT prompt generation and the `hatch` subsystem.
- Persona rendering and client-specific agent artifacts.
- Pi, Claude, Copilot, Gemini, or OpenCode launchers and bootstraps.
- GitHub/Bitbucket broker implementation internals.

The workflow engine may expose stable boundaries for clients, tickets, and
brokers, but those integrations remain owned by their respective systems.

## Completion Criteria

A parity slice is complete only when:

- its reference contract has a Go test;
- the test starts red against the missing behavior;
- the implementation passes the test;
- relevant mutations make the test fail;
- fixtures cannot touch real project or user state;
- each project uses a separate SQLite database selected from canonical
  `PROJECT_ID`;
- initiative lifecycle is persisted independently from task grouping;
- command complexity remains local and reviewable;
- affected documentation is updated in the same atomic change;
- `gofmt`, `go test`, `go test -race`, and `go vet` pass;
- behavior and residual differences are documented.
