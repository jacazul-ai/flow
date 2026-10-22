# Tasks

Owner of: task specs, modes, readiness, and the task lifecycle.

A task belongs to one initiative and keeps a full UUID as its identity. Short
UUIDs are presentation references and must be unambiguous.

## Spec

`plan` takes one spec per task:

```text
MODE|title|tag|due
```

- `MODE` is optional and sets what the agent may do (below).
- The title is concise and action-oriented: ideally within 79
  characters, never more than 120. Move extra context out of the
  title instead of truncating it.
- The plan-spec tag field is accepted for legacy compatibility but is not the
  same as a stored Taskwarrior task tag; use `amend +tag` to manage stored tags.
- The due field accepts `YYYY-MM-DD`, `yesterday`, `today`, `tomorrow`.

Keep urgency cool: no due date unless the operator asks or it is logically
mandatory, default priority, and raise priority with `urgent` only for the
immediate focus or a blocker, never while planning.

```bash
jacazul flow plan parity 'DESIGN|Define schema' 'EXECUTE|Implement store||tomorrow'
```

## Stored tags

Taskwarrior stored tags are one-word labels. A task may have any number of
stored tags, which flow preserves, displays, and filters with `+tag` and `-tag`.
Use `amend <uuid> +tag -tag` to add and remove stored tags. Taskwarrior
virtual tags such as `BLOCKED`, `READY`, `ACTIVE`, and `TAGGED` are derived
predicates and are not persisted as stored tags. The legacy `DISCARDED` marker
maps to native `disposition=discarded` instead of becoming a native tag.

## Modes

| Mode | The agent |
|---|---|
| `DESIGN` | architecture, trade-offs, contracts, and task breakdown; no direct edits |
| `SPIKE` | time-boxed research or a disposable proof of concept |
| `INVESTIGATE` | read-only code exploration and de-risking |
| `GUIDE` | the operator codes; the agent gives steps and suggested diffs |
| `EXECUTE` | the agent edits code, tests, and required documentation |
| `REFINE` | polish and incremental improvement with direct edits |
| `TEST` | verification; add or update tests in scope |
| `DEBUG` | root-cause analysis; propose the fix before applying it |
| `REVIEW` | audit and critique; edits only on explicit escalation |

Without a mode, do not assume `EXECUTE`: infer from the request and ask
before direct edits.

## Readiness

A pending task is not necessarily ready: an unfinished dependency or a wait
date blocks it, and `execute` refuses blocked work. Dependencies express
order; do not hide ordering in prose.

- `next` lists the ready tasks of an initiative.
- `block` adds a dependency and `unblock` removes it.
- `wait` postpones a task until a date.

## Lifecycle

1. `execute` starts a ready task.
2. Work, recording context as it happens ([context](context.md)).
3. Validate against the acceptance criteria and the project's gates.
4. `outcome` records the result; `done` refuses a task without one.
5. Ask the operator, then `done`. It lists the tasks that became ready.
6. Re-anchor to a ready task of the same initiative ([focus](focus.md)).

```bash
jacazul flow plan parity 'DESIGN|Define schema' 'EXECUTE|Implement store'
jacazul flow next parity
jacazul flow execute <first-uuid>
jacazul flow outcome <first-uuid> 'Schema defined and covered by contract tests'
jacazul flow done <first-uuid>
jacazul flow execute <second-uuid>
```

- `reopen` returns a completed task to pending; `discard` drops a task and
  records why. Never close, discard, or mutate completed work silently.
- `amend` corrects a task's description or ticket.
- `commit` drafts a conventional commit message from the focus; it never
  stages or commits files.

## Commands

| Command | Purpose |
|---|---|
| `execute` | Start a ready task |
| `outcome` | Record the result required for completion |
| `done` | Complete a task and expose ready work |
| `reopen` | Return a completed task to pending |
| `discard` | Discard a task with an audit outcome |
| `commit` | Draft a conventional commit from focus |
| `next` | List ready tasks |
| `amend` | Correct a task description or ticket |
| `urgent` | Raise task urgency |
| `block` | Add a task dependency |
| `unblock` | Remove a task dependency |
| `wait` | Postpone task readiness |

Run `jacazul flow help <command>` for the syntax of each one.
