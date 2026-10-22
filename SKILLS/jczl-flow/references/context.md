# Context

Owner of: annotations, inherited context, and tickets.

Annotations are the task's operational memory. Record what changes the
direction of the work, not the conversation: a cold-start agent must be able
to continue from the record alone.

## Kinds

`note <uuid> <kind> <message>` accepts each kind or any of its aliases.

| Kind | Aliases | Record when |
|---|---|---|
| `AC` | `ac`, `a` | an acceptance criterion was agreed |
| `BLOCKED` | `blocked`, `b` | progress depends on something outside the task |
| `DECISION` | `decision`, `d` | a choice was made, including its trade-offs |
| `HANDOFF` | `handoff`, `h` | another agent or session continues the task |
| `HYPOTHESIS` | `hypothesis`, `y` | an unproven explanation must survive the session |
| `LESSON` | `lesson`, `l` | something failed and the fix is known |
| `LINK` | `link` | a reference a cold-start agent needs |
| `NOTE` | `note`, `n` | a fact a cold-start agent needs that fits no other kind |
| `OUTCOME` | `outcome`, `o` | the final result; required before completion |
| `QUESTION` | `question`, `q` | an open point must survive the session |
| `RESEARCH` | `research`, `r` | a finding opens or closes an option |

Do not record thinking out loud, intent without a decision, or anything the
code or the task title already says. A recorded `OUTCOME` is trusted: do not
reinvestigate resolved work.

```bash
jacazul flow plan parity 'Define schema'
jacazul flow note <first-uuid> decision 'Use the native store'
jacazul flow note <first-uuid> ac 'Every command keeps project isolation'
jacazul flow notes <first-uuid>
```

## Inherited context

`context <uuid>` shows the task's own annotations and those inherited through
its dependencies, in dependency order. Read it before executing a task.

```bash
jacazul flow plan parity 'Define schema' 'Implement store'
jacazul flow note <first-uuid> decision 'Fields use snake_case'
jacazul flow context <second-uuid>
```

## Tickets

`ticket <uuid> <ticket>` links an external ticket. A direct ticket takes
precedence over one inherited through dependencies.

## Commands

| Command | Purpose |
|---|---|
| `note` | Add or delete structured task context |
| `notes` | List task annotations |
| `context` | Show direct and inherited task context |
| `ticket` | Link a task to an external ticket |

Run `jacazul flow help <command>` for the syntax of each one.
