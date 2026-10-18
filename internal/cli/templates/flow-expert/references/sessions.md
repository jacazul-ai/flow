# Sessions

Owner of: onboarding, handoffs, and session state.

A session is one agent conversation inside a project. It owns its focus, its
cache, and its handoff. One session never reads, acknowledges, or changes
another session's state.

## Onboard

`onboard` is the one-shot briefing, in a fixed order:

1. the pending session handoff, if any;
2. the focus and the focused task's context;
3. the focused status, or the project ponder view when nothing is anchored.

It acknowledges the handoff only after the whole briefing renders, so a
failed briefing leaves it for the next try. It never starts a task: present
the summary and wait for the operator.

## Handoffs

Handoffs live at two scopes:

- `handoff <uuid> <message>` records a durable `HANDOFF` annotation on the
  target task and starts it when needed. Use it when another agent continues
  that task.
- `session dump` writes a resumable note for this session. The next onboard
  replays it once. `session resume` and `session ack` expose the same
  lifecycle by hand.

Write a handoff for a cold-start reader: what was done, what is next, and
what was decided but is not obvious from the code.

```bash
jacazul flow plan parity 'Define schema' 'Implement store'
jacazul flow handoff <first-uuid> 'Schema draft is in docs; validate field names next'
jacazul flow session dump
jacazul flow session list
```

## Commands

| Command | Purpose |
|---|---|
{% for command in topics.sessions.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
