# Focus

Owner of: the session anchor, its stack, and re-anchoring.

The focus is the session's single mission state: an initiative and a task.
Establish it before material work; without it, a restarted session loses the
thread. Each session has its own focus.

## Anchor

- `focus` or `focus show` reads the current anchor.
- `focus plan <initiative>` anchors an initiative and its next ready task.
- `focus task <uuid>` pushes a task onto the session stack; `focus pop`
  returns to the previous one.
- `focus clear` removes the anchor but keeps the session record.

```bash
jacazul flow plan parity 'Define schema' 'Implement store'
jacazul flow focus plan parity
jacazul flow focus task <first-uuid>
jacazul flow focus pop
jacazul flow focus show
```

## Independent focus

`focus ind plan <initiative>` or `focus ind task <uuid>` anchors an
independent focus for this session, apart from the global one; `focus back`
returns to the global focus.

```bash
jacazul flow plan parity 'Define schema'
jacazul flow focus ind task <first-uuid>
jacazul flow focus back
```

## Re-anchor after done

`done` lists the tasks that became ready. Focus one of them in the same
initiative; when none is ready, clear the focus. Never anchor to a pending
task that is still blocked, and never jump to another initiative because of
the project-wide urgency order.

```bash
jacazul flow plan parity 'Define schema' 'Implement store'
jacazul flow execute <first-uuid>
jacazul flow outcome <first-uuid> 'Schema defined'
jacazul flow done <first-uuid>
jacazul flow focus task <second-uuid>
```

## Commands

| Command | Purpose |
|---|---|
{% for command in topics.focus.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
