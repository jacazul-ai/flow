# Sessions

Owner of: onboarding, handoffs, and session state.

A session is one agent conversation inside a project. It reads the project's
shared global focus by default, or its own after `focus ind` (see the focus
reference). It owns its cache and its handoffs, one per scope: one session
never reads, acknowledges, or changes another session's handoff.

## Onboard

`onboard` is the one-shot briefing, in a fixed order:

1. the pending session handoff, if any;
2. the focus and the focused task's context;
3. the focused status, or the project ponder view when nothing is anchored.

Native dump delivery is recorded after the whole briefing renders, but the
dump remains pending until `session ack`; a failed briefing leaves it for the
next try. Legacy session notes retain their compatibility acknowledgement
path. It never starts a task: present the summary and wait for the operator.

## Handoffs

Handoffs live at two scopes:

- `handoff <uuid> <message>` records a durable `HANDOFF` annotation on the
  target task and starts it when needed. Use it when another agent continues
  that task.
- `session dump` creates an append-only draft for this session. It contains
  fixed `Focus`, `References`, `Actions`, and `Notes` sections.
- `session finalize <dump-id> --actions <steps>` seals the draft as pending;
  `--references` and `--notes` carry agent-owned context, while `Actions` is
  mandatory.
- `session resume` and `onboard` deliver the latest pending dump without
  acknowledging it. `session ack` acknowledges it and releases the next dump.
- `session dumps` lists the current session's sequence; `session dumps --all`
  includes project history, including logically purged sessions.
- `session list` shows every session's scope, focus, and handoff state;
  `session list --all` adds purged sessions.
- `session purge` is logical by default and changes only the session state:
  a purged session is hidden and never acts again, but stays visible with
  `--all`. Only `session purge --hard --confirm` physically removes an
  already-purged orphan session and its history.

Write a handoff for a cold-start reader: what was done, what is next, and
what was decided but is not obvious from the code.

```bash
jacazul flow plan parity 'Define schema' 'Implement store'
jacazul flow handoff <first-uuid> 'Schema draft is in docs; validate field names next'
jacazul flow session dump
jacazul flow session dumps
jacazul flow session list
```

## Commands

| Command | Purpose |
|---|---|
{% for command in topics.sessions.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
