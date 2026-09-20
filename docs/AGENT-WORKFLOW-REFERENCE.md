# Agent Workflow Reference for Parity Tests

## Purpose

This document defines the evidence boundary for verifying that the native Go
`flow` engine preserves the observable behavior of the reference `tw-flow`
workflow. It is a test reference, not a generated agent prompt and not a
replacement for the reference implementation.

The parity target is behavior. `flow` may use different packages, storage, and
process boundaries as long as the supported behavior, safety rules, state
transitions, and actionable errors remain compatible.

## Reference Repository

The behavioral reference is the `jacazul-ai-cli` repository:

```text
~/source/jacazul-ai/jacazul-ai-cli/master/
```

Use the reference checkout in an isolated test environment. Never use the
operator's real Taskwarrior data, home configuration, cache, session files, or
external services while running parity scenarios.

## Authority Order

When investigating a behavior, use the following order:

1. Reference tests under `tests/` define executable observable contracts.
2. The `jacazul/` package explains the reference implementation mechanism.
3. Reference documentation explains user-facing intent and terminology.
4. `AGENTS.md`, skills, and templates define agent and workflow operating
   rules, but do not by themselves prove a command's domain behavior.

If a test and an implementation detail disagree, preserve the tested behavior
and record a compatibility decision when the difference is material.

## Agent and Workflow Resolution Chain

The reference agent behavior is assembled through several layers:

```text
scripts/configure
  setup: directories, links, and initial templates
        |
        v
scripts/bootstrap/
  runtime: environment, session, settings, and resource detection
        |
        v
jacazul/hatch/templates/
  source of generated skills and agent prompts
        |
        v
agents/ and skills/
  generated client artifacts
        |
        v
tw-flow and taskp
  workflow state, safety restrictions, and task operations
```

Generated agent files must not be edited directly. Changes to prompt or skill
behavior belong in `jacazul/hatch/templates/` and require regeneration. Agent
prompt generation is outside the flow domain; flow should implement the
stable workflow contracts consumed by any client.

The current reference repository still describes Taskwarrior wrappers and
`tw-flow`. The flow target uses native persistence and domain APIs. Any
Taskwarrior compatibility code in the migration is a transitional boundary,
not a source-of-truth requirement for the target engine.

## Test Coverage Map

| Behavior area | Primary reference evidence | Required flow proof |
|---|---|---|
| Entry points and project isolation | `tests/core_test.py` | Subprocess routing, exit status, project-scoped state, and cross-project non-observation |
| Lifecycle, focus, context, tickets, dashboards, modes, handoffs, and completion safety | `tests/flow_test.py` | Command/API contracts plus persisted state and actionable errors |
| Cache storage, TTL, refresh, invalidation, filters, and sessions | `tests/cache_test.py` | Key boundaries, expiry signals, force refresh, and session isolation |
| Vaccination, archive auditing, quoting, and raw command restrictions | `tests/security_test.py` | Rejection behavior, guidance, and no unsafe bypass |
| Session listing | `tests/test_session_list.py` | Correct current and historical session views without cross-session leakage |
| Backlog behavior | `tests/test_backlog.py` | Backlog transitions, visibility, and plan isolation |
| Recently closed history | `tests/test_recently_closed.py` | Correct completion history and output ordering |
| Roadmap initialization guard | `tests/test_roadmap_init_guard.py` | Refuse invalid initialization and preserve existing roadmap state |
| Agent artifact generation | `tests/test_hatch.py`, agent-directory tests | Only when client/bootstrap behavior is explicitly in scope; test templates and generated output at their boundary |

The reference tests named above are the minimum map for the current parity
phase. Add a focused regression test when a previously unprotected behavior is
discovered.

## Contract Comparison

A parity test must compare both sides of the boundary:

```text
same isolated inputs
        |
        +--> reference flow
        |       output + error + state
        |
        +--> native flow flow
                output + error + state
```

Capture and compare:

- command or API result;
- process exit status;
- stdout and stderr ownership;
- actionable `ACTION:` guidance on failures;
- state transitions and persisted records;
- task, initiative, dependency, annotation, focus, session, and cache scope;
- ordering where the user can observe it;
- security restrictions and rejection behavior.

Do not compare implementation accidents such as Python stack traces, SQLite
layout details, internal class names, or numeric Taskwarrior IDs. Compare the
contract visible to a client or operator.

## Isolation Requirements

Each test scenario must use controlled temporary state:

- a temporary home and flow runtime root;
- an isolated project identity;
- isolated session identity when session behavior is under test;
- temporary databases and cache directories;
- fake external commands or local test servers where required;
- no live network, production ticket, user credentials, or real workflow data.

A fixture that merely proves its own files are separate is not enough. The
parity test must assert both directions:

```text
project A cannot observe project B
project B cannot observe project A
session A cannot observe session B
```

The real command or public API must cross the boundary under test. Test
harness helpers provide isolation; they are not proof of feature behavior.

## Red-Green-Refactor-Mutation Loop

For each missing or suspect behavior:

1. **Red:** add a valid failing contract test based on the reference behavior.
2. **Green:** implement the smallest native flow change that passes it.
3. **Refactor:** simplify ownership and boundaries without changing the
   contract.
4. **Mutation check:** intentionally break the protected rule and verify that
   the test fails rather than passing vacuously.
5. **Parity review:** compare the result with the reference test and mechanism.

A passing test without a meaningful failure mutation does not demonstrate that
the assertion protects the behavior.

## Context and Annotation Contracts

The distributed and local context relevant to parity includes structured
annotations and their inheritance behavior:

- `DECISION`;
- `RESEARCH`;
- `OUTCOME`;
- `LESSON`;
- `HANDOFF`;
- `QUESTION`;
- `HYPOTHESIS`.

The current compatibility contract requires:

- semantic note aliases normalize to uppercase annotation kinds;
- notes can be listed and deleted by timestamp;
- context and status inherit supported annotations through dependency edges;
- inheritance is dependency-first and recursive;
- direct external ticket references take precedence over inherited references;
- handoff starts a target task when necessary and records `HANDOFF`;
- notes remain allowed on completed tasks while lifecycle mutations stay
  protected;
- UUIDs are the identity boundary; presentation identifiers are not identity.

These rules must be asserted through the flow context/status behavior, not
only through direct database queries.

## Operational Rules From `AGENTS.md`

The reference agent protocol also establishes operational contracts that must
remain visible where applicable:

- answer the user's actual request before workflow exposition;
- orient from focus and context before material workflow actions;
- preserve project and session isolation;
- keep normal successful no-op output quiet;
- emit state changes on stdout;
- emit failures on stderr with corrective guidance;
- require an `OUTCOME` before completion;
- do not close a task silently;
- use generated agent artifacts through their templates;
- treat banners, warnings, tips, and errors as operational signals.

These rules are part of the agent-facing integration contract. They do not
require the native engine to reproduce the reference implementation's Python
or shell structure.

## Completion Gate

The parity initiative is complete only when:

- every behavior in the coverage map has a real contract test or an explicit
  documented compatibility decision;
- project and session isolation are tested at the command/API boundary;
- security restrictions have regression and mutation coverage;
- cache and context inheritance behavior are covered;
- documentation describes user-facing changes;
- `go test ./...`, `go vet ./...`, and the project's race validation pass;
- the final task contains an English `OUTCOME` describing evidence and known
  residual risks.

Until these conditions are met, a green build is not feature completion.

## Related Design Documents

- [Architecture](ARCHITECTURE.md)
- [Distributed Context](DISTRIBUTED-CONTEXT.md)
- [Feature Parity](feature-parity.md)
