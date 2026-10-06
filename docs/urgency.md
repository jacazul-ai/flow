# Workflow Urgency

## Current boundary

Native `flow` stores imported/observed `urgency` metadata, but selection does
not treat that value as authoritative. It calculates a fresh
Taskwarrior-compatible score for modeled attributes, applies dependency
inheritance, filters ready work, and ranks the candidates deterministically.
Tags, initiative weight, and other unsupported Taskwarrior inputs remain
explicit follow-up boundaries.

This document defines the compatibility baseline and the native extension
needed for dependency-driven initiatives. It is an urgency design boundary,
not an engine-cutoff or tombstone protocol.

## Taskwarrior baseline

Taskwarrior calculates urgency as a configurable polynomial: the task's
attributes contribute weighted terms to a derived score. Its default
coefficients are:

| Term | Default |
|---|---:|
| `+next` tag | 15.0 |
| Due or overdue date | 12.0 |
| Blocking another task | 8.0 |
| Priority `H` | 6.0 |
| Scheduled | 5.0 |
| Active | 4.0 |
| Priority `M` | 3.9 |
| Age | 2.0 |
| Priority `L` | 1.8 |
| Has annotations | 1.0 |
| Has tags | 1.0 |
| Has a project | 1.0 |
| Waiting | -3.0 |
| Blocked | -5.0 |

Tags and annotations use a count factor: `0.8` for one, `0.9` for two, and
`1.0` for three or more. Age stops increasing after 365 days by default. The
Taskwarrior due factor is `1.0` at least seven days overdue, `0.2` more than
fourteen days in the future, and linear between those bounds. Age is the
elapsed-day fraction of the 365-day cap. Coefficients can be customized by
tag, project, keyword, or UDA in Taskwarrior configuration.

References:

- <https://taskwarrior.org/docs/urgency/>
- <https://taskwarrior.org/docs/man/taskrc.5/>

## Legacy Jacazul behavior

The launcher did not contain a separate urgency calculator. The legacy path
combined Taskwarrior's own urgency with Jacazul-specific ordering:

- the caged and unhinged Taskwarrior templates define `critical=5.0` and
  `next=10.0` tag coefficients;
- the `ranked` report sorts by `project_weight` descending and then urgency
  descending;
- `ponder` sorts active tasks first and then the stored Taskwarrior urgency;
- `next` delegates readiness to Taskwarrior's `ready` filter;
- `urgent` attempts to write `urgency:15.0` by default and `priority:H`.
  Taskwarrior treats `urgency` as derived/read-only, rejects the combined
  modification, and the legacy command ignores the failure before printing
  success. The effective task can therefore remain at `priority:M` with its
  previous computed urgency.

There is no standalone Bash urgency calculator to port. The important legacy
behavior is the combination of Taskwarrior's derived score, project weight,
and readiness filtering. The `urgent` command is a compatibility bug, not a
working urgency input.

## Diagnostic: legacy `urgent`

The launcher implementation in `jacazul/cli/flow.py:1529` calls
`modify urgency:15.0 priority:H` and reports success without checking the
Taskwarrior result. A disposable `TASKDATA` reproduction showed:

```text
before: priority M, urgency 5.7
urgent: Task <uuid> marked as urgent (urgency: 15.0)
after:  priority M, urgency 5.7
```

This must not become a native design contract. A future replacement must
change real urgency inputs (for example priority and a native `next`/critical
signal), or deliberately remove the command; it must never pretend that a
derived score was written.

## Native chain-aware model

Taskwarrior's `blocking` and `blocked` terms are useful compatibility signals,
but they are not enough for Jacazul initiatives. A chained initiative needs to
prefer ready work that unlocks the largest or deepest remaining path.

The native selection boundary is:

```text
candidates = pending tasks whose dependencies are complete and wait_until has passed
base_score = Taskwarrior-compatible score for the modeled task attributes
inherited_score = highest effective score of recursively blocked downstream tasks
flow_score = max(base_score, inherited_score)
```

Selection rules:

1. Filter to executable candidates before ranking. A blocked task must not win
   merely because its metadata score is high.
2. Rank candidates by `flow_score` descending.
3. Break equal scores with the explicit initiative `position`, then creation
   order, then full UUID for deterministic output.
4. Keep the source/manual urgency value intact; the derived score is a report
   and selection value, not a destructive rewrite of imported metadata.
5. Keep project/session isolation and UUID-first identity unchanged.

The inherited score is graph-aware and recursive. A direct `blocking` bonus
alone cannot distinguish a task that unlocks an urgent downstream task; the
inheritance term carries that downstream urgency to the blocker without an
unbounded count bonus.

## Design review decisions

The design review closed these decisions as part of the urgency contract:

1. **Manual control.** `priority:H` is the first native operator lever. It
   changes a supported input and never writes a derived urgency score.
2. **Initiative weight.** Legacy `project_weight` is not silently folded into
   the first native task score. Cross-initiative weighting remains a separate
   native-model decision.
3. **Chain impact.** The first slice uses recursive maximum downstream
   inheritance, bounded by the downstream scores themselves, and evaluates
   dependencies within the current project.
4. **Read-only derived score.** Native selection ignores writable/imported
   numeric urgency. It uses a fresh derived score from real inputs.
5. **Inheritance baseline.** The model follows Taskwarrior's
   `urgency.inherit` behavior: a blocker inherits the highest urgency of tasks
   it unblocks recursively.

Annotations remain a compatibility term, but they should not be treated as a
manual importance control. In Jacazul, a heavily discussed DESIGN task may have
more annotations without being more urgent.

### First vertical slice

The first end-to-end slice makes these decisions explicit:

- `priority:H` is the manual urgency lever; no writable numeric urgency
  override is introduced.
- Chain impact follows Taskwarrior's `urgency.inherit` model: a blocker
  inherits the highest derived score of the tasks it recursively unblocks.
  Dependencies are evaluated within the current project, and readiness is
  still filtered before a task can be selected.
- Initiative `project_weight` is not silently folded into task urgency in this
  slice. Cross-initiative weighting remains a separate native-model decision.
- The existing stored `urgency` value is treated as observed/imported data;
  reports and selection use a freshly derived score.

This keeps the first implementation small, deterministic, and reversible while
making the omitted native data-model work visible.

## Compatibility scope

The first implementation can calculate terms represented by the native model:
priority, active state, due date, wait state, dependency blocking, project
presence, annotations, age, and the dependency graph. Tags, scheduled/until
fields, project-specific coefficients, initiative weight, and other
Taskwarrior terms require an explicit native data-model decision rather than
silent approximation.

Migration preserves the source fields that determine urgency and must not
silently turn a computed Taskwarrior score into a mutable native override. A
derived native score is calculated when a report or ready-task selection needs
it. If an export includes an observed urgency value, it is diagnostic history,
not an input that replaces the native calculation.

## Non-goals

This initiative does not implement engine selection, `MIGRATED` tombstones,
launcher cutoff, live Taskwarrior synchronization, or remote coordination.
Those remain outside the local workflow engine boundary.
