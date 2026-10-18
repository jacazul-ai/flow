---
name: flow-expert
description: Native Jacazul workflow engine for initiatives, tasks, focus, sessions, structured context, handoffs, and outcomes through jacazul flow. Use this when orienting a session, planning work, executing or closing tasks, recording decisions, or handing work to another session.
---

# flow-expert

`flow` is the workflow engine. It keeps initiatives, tasks, focus, sessions,
annotations, handoffs, and outcomes for one project. Always call it as:

```text
jacazul flow <command>
```

When in doubt, ask the engine instead of guessing:
`jacazul flow help` lists every command and `jacazul flow help <command>`
gives its syntax, prerequisites, effects, examples, and next action.

## Onboard

At bootstrap, and whenever the operator asks for onboarding, run exactly
`jacazul flow onboard`. It is deterministic and observational: it shows the
pending session handoff, the focus and its context, then the focused status
or the project ponder view. It never starts a task. Present a short summary
and wait for the operator's direction.

## Orient

| Command | Answers |
|---|---|
| `jacazul flow focus` | Where am I anchored? The focus is the session's single mission state. |
| `jacazul flow status` | What is pending in the focused initiative? |
| `jacazul flow ponder` | What does the whole project look like? |

Use `status` while working and `ponder` for the project landscape. A
`[cached]` signal means nothing changed: reason from the last full report.

## Work

```text
focus → execute → note → outcome → done → re-anchor
```

- Execute only a ready task: a pending task can still be blocked.
- Record decisions, research, blockers, and lessons as they happen.
- Record an `OUTCOME` before `done`; the engine refuses to close without it.
- Ask the operator before completing a task.
- After `done`, focus a ready task of the same initiative, or clear the focus.

## Rules

- Never call raw `task` or `taskp`; flow is the only workflow interface.
- Use short UUIDs to refer to tasks, never positions.
- Treat an `ACTION:` line in an error as the next valid move.
- Project and session are isolation boundaries; never cross them.
- Store task data in English.

## Learn more

| Reference | Read it to |
|---|---|
| [initiatives](references/initiatives.md) | create and organize initiatives (plans, inis) and their contract |
| [tasks](references/tasks.md) | write task specs, pick modes, run the task lifecycle |
| [focus](references/focus.md) | anchor, stack, and re-anchor the session focus |
| [sessions](references/sessions.md) | onboard, hand off, and resume sessions |
| [context](references/context.md) | record annotations and read inherited context |
| [reports](references/reports.md) | read status, ponder, and the other reports, cache, and errors |
| [runtime](references/runtime.md) | understand project, home, and session identity |
| [commands](references/commands.md) | find any command by name |
