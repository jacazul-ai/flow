# flow Agent Guide

## Mission

`flow` is the Go-native migration of the workflow engine currently shipped
by `jacazul-ai-cli`. Its purpose is to replace the Python/shell `tw-flow`
stack with a portable local engine that can later support centralized
coordination.

The primary consumer is:

- `~/source/jacazul-ai/jacazul-ai-cli/master`
- <https://github.com/jacazul-ai/jacazul-ai-cli>

The reference implementation is the `jacazul/` package in that repository.
Use it to recover behavior and compatibility requirements, but do not copy
implementation accidents into Go without a reason.

## Runtime Context

The Jacazul distribution reaches agents through several client launchers:

- Pi
- Claude
- Copilot
- Gemini
- OpenCode

Each client has its own launcher/bootstrap path, but the runtime contract is
shared:

1. The environment bootstrap resolves the canonical `PROJECT_ID`, `TASKDATA`,
   session ID, mode, language, and Taskwarrior setup.
2. The client bootstrap makes the relevant Jacazul skills, agents, or
   extensions available to that client.
3. The client launcher starts the agent with the Jacazul runtime protocol.
4. The agent uses the workflow engine to preserve focus, plans, task context,
   annotations, session handoffs, and cached status output.

`flow` is the shared workflow engine in this chain. It must remain
independent of any specific client, persona, launcher, prompt format, or
agent bootstrap implementation.

## Explicit Non-Goals

Prompt and agent artifact generation is outside this repository's
responsibility. The `jacazul-ai-cli` `hatch` subsystem owns JIT prompt forging,
persona anchoring, and generated client-specific agent files. Do not move hatch
logic into `flow` or make the workflow engine depend on it. `flow` should
consume stable workflow inputs and expose workflow behavior to any client.

## Current Phase: Feature Parity

This repository is currently implementing feature parity with the existing
`tw-flow` behavior. The tests under
`~/source/jacazul-ai/jacazul-ai-cli/master/tests` are the primary behavioral
reference for this phase, especially:

- `tests/core_test.py`: executable entry points and strict project isolation.
- `tests/flow_test.py`: lifecycle, focus, context inheritance, tickets,
  dashboards, modes, handoffs, and completion safety.
- `tests/cache_test.py`: cache storage, TTL signals, force refresh, selective
  invalidation, filter-specific keys, and session isolation.
- `tests/security_test.py`: workflow vaccination, archive auditing, quoting,
  and raw Taskwarrior restrictions.
- `tests/test_session_list.py`, `tests/test_backlog.py`,
  `tests/test_recently_closed.py`, and `tests/test_roadmap_init_guard.py`:
  focused regressions for session, backlog, history, and roadmap behavior.

Parity rules for this phase:

- Port the existing behavioral tests or equivalent Go contract tests before
  declaring a feature complete.
- Use isolated temporary state in tests; never use the developer's real
  `TASKDATA`, home directory, cache, or network.
- Treat `internal/testharness` as fixture infrastructure, not feature proof.
  A harness test may verify that fixtures are isolated, but parity tests must
  exercise the real command/API and assert both sides of the boundary: project
  A must not observe project B's state, and project B must not observe project
  A's state.
- Preserve observable command behavior, safety restrictions, state transitions,
  cache boundaries, and actionable errors unless a deliberate compatibility
  decision is recorded.
- Do not add new workflow semantics merely because the Go implementation can
  support them. First match the reference behavior, then evolve it in a
  separately identified phase.
- A passing build without parity coverage is not feature completion.

### Phase-Scoped Development Loop

Only for the current porting and feature-parity phase, use a strict
Red-Green-Refactor loop:

1. **Red:** write a valid failing contract test for the reference behavior.
2. **Green:** implement the smallest change that makes the test pass.
3. **Refactor:** split command responsibilities and reduce accidental
   complexity without changing the contract.
4. **Mutation check:** intentionally break the protected rule, when practical,
   and verify the test fails rather than passing vacuously.
5. **Parity review:** compare the resulting observable behavior with the
   `jacazul-ai-cli` reference test and implementation.

This loop is a temporary rule for the migration phase. It must not be treated
as a permanent project-wide process requirement after parity is established.

## Migration Scope

The first compatibility target is the behavior exposed by:

- `tw-flow`: plan/task lifecycle, focus, status, ponder, context, notes,
  outcomes, handoffs, backlog, roadmap, sessions, and commit drafts.
- `taskp`: project-aware Taskwarrior access with workflow safety restrictions.
- `jacazul.taskwarrior.core`: project isolation, focus persistence, cache
  management, and task/broker boundaries.

Preserve these domain contracts unless a design decision explicitly changes
 them:

- Per-project state isolation through `PROJECT_ID`.
- Session-specific focus and cache state; never leak context across sessions.
- UUID-first task references; short UUIDs are presentation-only identifiers.
- Structured annotations such as `DECISION`, `RESEARCH`, `BLOCKED`, `LESSON`,
  `OUTCOME`, and `HANDOFF`.
- `OUTCOME` is required before a task can be closed.
- Errors, warnings, tips, and cache signals are operational interface output.
  Errors should explain the next corrective action.
- Normal output should remain quiet unless state changes, an error occurs, or
  debug mode is enabled.
- Taskwarrior compatibility is a migration constraint, not the final
  architecture. Keep persistence and workflow logic behind clean boundaries
  so a native store or future server can be introduced later.

## Repository Layout

```text
cmd/jczl-flow/    CLI executable and global option parsing
internal/cli/      command implementations and command routing
internal/config/   application-wide options and dispatch integration
internal/testharness/ isolated test fixtures and fake external commands
README.md         migration context and public design direction
go.mod            module and dependency declarations
```

The command model is intentionally Git-like:

```text
jczl-flow <command> [<args>]
```

The command registry should become the source of truth for command metadata,
routing, and help rendering. Keep parser details subordinate to the CLI
experience.

## CLI Reference Pattern

Use `~/source/candango/nvimim` and
`~/source/idzoid/cryptozoid/master` as references for Go CLI composition and
user experience. Their implementation is a style reference, not a dependency
or a behavioral source for workflow semantics. The parent
`~/source/jacazul-ai/jacazul-ai-cli/master` remains the behavioral source for
`tw-flow` compatibility; do not copy its Python `FlowManager` shape into Go.

### House CLI Style

Adopt the following composition style for `flow`:

- Keep `cmd/jczl-flow/main.go` thin: construct global options, configure the
  `go-flags` parser, register commands, handle help/version, and map final
  errors to stderr and exit status.
- Use an explicit command registry as the source of truth for command names,
  summaries, detailed help, routing, and the visible command tree. Parser tags
  support the registry; they must not define the domain model.
- Put command behavior in `internal/cli`. Each logical command owns a small
  concrete command type with an `Execute(args []string) error` method. Small
  related commands may share a file, but they must not share an undifferentiated
  command object or a monolithic manager.
- Model command groups explicitly when the user experience is hierarchical,
  following the `cryptozoid ec generate`, `ec encrypt`, and `ec decrypt`
  pattern. Use nested command structs and `command` metadata for the tree;
  keep group-level behavior separate from leaf-command behavior.
- Declare command-local flags on the command that consumes them. Declare
  positional argument rules and validation in that command, not in the root
  parser or a shared options package.
- Resolve process-wide and project-scoped options once at dispatch, then pass
  them through a small consumer-owned handoff such as `SetAppOptions`. Never
  use package globals or hidden mutable configuration.
- Keep the process boundary responsible for `os.Exit`, help rendering, version
  output, stderr formatting, and exit status. Command packages return errors
  and never terminate the process themselves.
- Use the shared output contract: successful state changes go to stdout;
  failures go to stderr and explain the next valid action with an `ACTION:`
  prompt; healthy no-op checks stay quiet unless the command is a report.
- Keep CLI responsibilities narrow: parse and validate input, call focused
  workflow/domain components, and render user-facing results. Persistence,
  Taskwarrior compatibility, cache policy, session state, and future transport
  belong behind separate packages with explicit boundaries.
- Keep command complexity local and observable. A command such as `focus`,
  `status`, `done`, `session`, or `cache` gets its own command boundary and
  focused contract tests rather than expanding a central `FlowManager`.

### CLI Documentation and Verification

- Document the command tree, options, defaults, positional inputs, examples,
  stdin behavior, output shape, and failure conditions in `README.md` and a
  focused `docs/cli.md` when the command surface is large enough to need it.
- Test command behavior at the narrowest reliable boundary: direct command
  tests for validation and domain calls, plus subprocess contract tests for
  routing, help, exit status, stdout/stderr, project isolation, and actionable
  errors.
- Use `t.TempDir()`, controlled environment variables, fake executables, local
  HTTP test servers, and controlled stdin. Tests must not use the developer's
  home directory, real Taskwarrior data, live network services, or production
  tickets.
- Before finalizing Go CLI changes, run `gofmt`, `goimports` when installed,
  `go test ./...`, `go vet ./...`, and `go test -race ./...` for behavioral
  changes. If `goimports` is unavailable, use `gofmt` and report the skipped
  import-organization pass.

## Error as Prompt and Prompt as Ad

Import the workflow contract from `jacazul-ai-cli`:

- Errors are operational signals, not dead ends. Emit them on `stderr` with
  enough context to explain what failed and a concrete `ACTION:` describing
  the next valid move.
- The process boundary owns error formatting and exit status; command code
  returns actionable errors without calling `os.Exit`.
- Healthy verification should stay quiet unless the command is explicitly a
  status/report operation or debug output was requested.
- Emit normal output when state changes, such as creating or transitioning a
  task. Do not narrate unchanged state as noise.
- Use short, high-value context alerts for task attributes, tickets, focus, or
  mode restrictions. Alerts must help the agent act without dumping the whole
  protocol or interrupting the workflow.
- Contract tests must assert both the error condition and its guidance so
  actionable errors do not regress into generic failures.

## Engineering Rules

- Write idiomatic Go: explicit control flow, small concrete types, and
  behavior-based interfaces only when a real consumer or test seam requires
  one.
- Prefer the standard library. Avoid framework or Java-style layering.
- Preserve line-of-sight readability: handle errors early and keep the happy
  path left-aligned.
- Cyclomatic complexity is prohibited as a design strategy: keep functions
  small and linear, split behavior by responsibility, and reject monolithic
  dispatchers, managers, and deeply nested branches.
- Wrap actionable underlying errors with `%w`; use `errors.Is` and
  `errors.As` for classification.
- Keep client launchers (Pi, Claude, Copilot, Gemini, and OpenCode), shell
  bootstrap, hatch/prompt generation, persona rendering, and GitHub broker
  concerns out of the core workflow domain.
- Do not introduce concurrency until the behavior needs it and ownership,
  cancellation, and exit paths are explicit.
- Never embed credentials, tokens, or secrets in source, fixtures, logs, or
  generated output. Treat external tickets and future server inputs as
  untrusted data.

## Validation

Before considering a Go change complete:

```bash
gofmt -w <touched-go-files>
goimports -w <touched-go-files>  # when installed
go test ./...
go vet ./...
```

The `Makefile` wraps the same gates: `make fmt`, `make vet`, `make test`,
`make test-race`, `make build`, and `make all` for the first three together.
`make fmt` runs `gofmt` then `goimports`, reporting the skip when `goimports`
is unavailable.

The race-enabled command for this project is:

```bash
make test-race     # go test -race -v ./...
```

Run it for behavioral changes; use `make test` and `make vet` as focused local
checks when appropriate. If `goimports` is unavailable, use
`gofmt` and report that import organization was skipped. Add focused tests for
new command behavior and regression tests for compatibility-sensitive workflow
semantics.

The supported Go versions are 1.25 and 1.26. Go versions older than 1.25
are EOL for this project. Do not claim a change is validated across both
supported versions unless the relevant runs were actually performed.

## Runtime Data and Sandbox Safety

The project checkout is the code workspace. User/project runtime data is the
protected original and must remain untouched by tests and development tools
unless explicitly requested.

Protect these sources by default:

- the user's real `TASKDATA` and Taskwarrior database;
- the user's real `JACAZUL_HOME`, cache, session, and focus files;
- the user's home configuration and credentials;
- live external services and production tickets.

Use this operating model:

- Code, tests, and documentation for the current feature belong in this
  checkout, under the task's scope.
- `[INVESTIGATE]` and `[REVIEW]` must not mutate user/project runtime data.
- `[EXECUTE]`, `[REFINE]`, and `[TEST]` use isolated temporary or persistent
  runtime state, never the real home, database, cache, or network.
- The persistent runtime sandbox root is:

  ```text
  ~/.jacazul-ai/sandboxes/<project-id>/<session-id>/
  ```

- The absolute sandbox path must be exposed in task/session context so agents
  can inspect it directly.
- Use `git worktree` or a clone only when the code itself needs a separate
  experimental checkout; it is not a replacement for isolated test data.
- Do not add a `.gitkeep` or ignored runtime-data directory to the project
  tree. Test fixtures and harness code belong in the repository; their data
  belongs in the sandbox or `t.TempDir()`.
- Before any operation against external services or real user data, require an
  explicit user request and verify the exact target.

A user request to implement a feature authorizes code and test changes in this
checkout; it does not authorize touching real runtime data, production
services, commits, merges, or pushes.

## Git and Workflow Safety

- Work from the current task/plan context before material repository changes.
- Keep persistent task and commit data in English; communicate with the user
  in the session language.
- Do not use raw Taskwarrior commands in Jacazul workflow operations. The
  compatibility layer belongs in the controlled workflow boundary.
- Stage only task-relevant files. Never use `git add .` or `git add -A`.
- Do not commit or push without explicit user confirmation.
- Use conventional commit messages, keep titles under 50 characters, and do
  not include internal task UUIDs or Copilot trailers.

## Source of Truth and Decisions

When behavior is unclear, inspect the corresponding workflow implementation
and tests in `jacazul-ai-cli` first, then record the compatibility decision in
this repository's documentation or design notes. Ignore hatch and client
prompt details unless they expose a workflow contract. Prefer a small
executable vertical slice over speculative abstractions.
