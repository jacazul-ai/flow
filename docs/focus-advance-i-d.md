# I&D: focus advanced to a blocked task

## Conclusion

The focus state can advance to a task that is still blocked by an unfinished
dependency. The failure is in the legacy Python `tw-flow` focus advancement
path, not in dependency readiness itself.

The current `jaflow` Go code already exposes the correct readiness boundary
through `Store.ReadyTasks`. Any future automatic focus advancement must use
that boundary instead of treating `status == pending` as executable.

## Observed symptom

After completing a focused task, `tw-flow done` reported and selected another
pending task from the same initiative even though that task still had an
unfinished dependency. The next focus was therefore not executable.

This violates the workflow contract:

```text
focused task → complete → next ready task
```

It must never become:

```text
focused task → complete → arbitrary pending task
```

## Evidence

### Legacy reference path

`jacazul-ai-cli/master/jacazul/taskwarrior/core.py` contains
`FocusManager.advance`:

- line 402: `advance` loads the focus stack and restricts entries to the same
  plan;
- line 437: it accepts the first entry whose exported status is exactly
  `pending`;
- it does not evaluate the entry's dependencies or call the ready-task query.

The relevant condition is effectively:

```python
if status == "pending":
    select_as_focus(entry)
```

A pending task is not necessarily ready. This is how a blocked task can become
the next focus.

### Native Go path

`jaflow` has the required readiness calculation:

- `internal/storage/sqlite/lifecycle.go:96` — `ReadyTasks` returns only
  pending tasks whose dependencies are completed;
- `internal/cli/focus.go:127` — `focus plan` already consumes `ReadyTasks`;
- `internal/cli/lifecycle.go:114` — `done` reports `ReadyTasks` after
  completion, but currently does not mutate focus automatically.

The Go migration must preserve the readiness rule and must not copy the
legacy `pending`-only selection accident.

## Mechanism

The legacy focus stack is an ordering/navigation structure. It is not a
workflow scheduler and does not contain enough information to decide whether a
task is executable. `FocusManager.advance` asks the stack for the next entry,
then uses only the task status as a gate. Dependency state is therefore bypassed
at the exact point where the focus anchor changes.

The correct ownership is:

```text
storage/workflow readiness → identifies executable tasks
focus manager             → records the selected anchor
CLI done                  → completes, advances only to ready work, reports it
```

## Impact

- The agent can be anchored to a task that `execute` will reject as blocked.
- The focus anchor no longer represents the next actionable step.
- A dependency chain can appear to advance while remaining operationally stuck.
- A plan-level focus command and a completion-driven focus transition can make
  different readiness decisions.

This is a workflow-integrity defect, not merely a display issue.

## Corrective direction

When automatic advancement is implemented in `jaflow`:

1. complete the current task through the lifecycle store;
2. query `ReadyTasks` for the same initiative and project;
3. select only from that result;
4. preserve the initiative/session boundary;
5. never select a task based on `status == pending` alone;
6. add a contract test with a completed task, a blocked pending task, and a
   separate ready task.

If no task is ready, the implementation must define an explicit no-ready-work
state rather than silently selecting blocked work. That policy should be
recorded before the focus stack semantics are finalized.

## Required regression test

The parity test should construct this graph in isolated temporary storage:

```text
A (focused, active)
B (pending, depends on C)
C (pending, not complete)
D (pending, dependencies complete)
```

After completing `A`, focus advancement must select `D` or report no ready
work. It must never select `B`. The test must also verify that a task from a
different initiative cannot become the new focus.

Reference coverage exists in:

```text
jacazul-ai-cli/master/tests/test_execute_loop_control.py
```

That coverage currently proves same-plan filtering, but its second test names
`pending` rather than `ready`; the Go parity test should strengthen the oracle
with an actual dependency graph.

## Status

This document records the diagnosis and migration boundary. It does not change
focus behavior yet. The implementation belongs to the `jaflow` lifecycle/focus
parity slice, with isolated contract coverage before the fix is declared
complete.
