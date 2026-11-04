# Focus

Owner of: the session anchor, its stack, and re-anchoring.

The focus is the session's single mission state: an initiative and a task.
Establish it before material work; without it, a restarted session loses the
thread.

## Smart focus: the scope rule

Stay where you entered: a session in the global focus stays global, a session
in an independent focus stays independent.

- A session starts in the project's global focus, shared by every session that
  has no focus of its own. `focus <initiative>`, `focus plan`, `focus task`,
  `focus pop`, `focus clear` and `focus interest` write that shared focus, and
  every global session sees the change.
- `focus ind ...` is the only way out: from then on every focus command of
  that session, with or without `ind`, writes its own focus.
- `focus back` is the only way back. In a session already in the global focus
  it refuses with an `ACTION:`.
- Handoffs follow the same scope: in the global focus, `session dump`,
  `resume`, `ack`, `finalize` and `dumps` act on the shared global handoff,
  which any global session receives; an independent session keeps its own.
- Two agents working in parallel on one project share one focus unless one of
  them uses `focus ind`.

## Anchor

- `focus` or `focus show` reads the current anchor.
- `focus <initiative>` or `focus plan <initiative>` anchors an initiative and
  its next ready task.
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
returns to the global focus and its shared handoff. It drops only the session's independent focus and
cached output: handoff notes and dumps stay readable, and only
`session purge --hard --confirm` deletes session history.

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
