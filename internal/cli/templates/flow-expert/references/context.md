# Context

Owner of: annotations, inherited context, and tickets.

Annotations are the task's operational memory. Record what changes the
direction of the work, not the conversation: a cold-start agent must be able
to continue from the record alone.

## Kinds

`note <uuid> <kind> <message>` accepts each kind or any of its aliases.

| Kind | Aliases | Record when |
|---|---|---|
{% for kind in kinds %}| `{{ kind.Kind|safe }}` | {% for alias in kind.Aliases %}`{{ alias|safe }}`{% if not forloop.Last %}, {% endif %}{% endfor %} | {{ kind.Guidance|safe }} |
{% endfor %}
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
{% for command in topics.context.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
