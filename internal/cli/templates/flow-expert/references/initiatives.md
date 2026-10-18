# Initiatives

Owner of: creating, organizing, and describing initiatives.

An initiative is the workflow aggregate for one goal: it owns an ordered chain
of tasks. `plan`, `ini`, and `initiative` name the same thing; `plans`,
`inis`, and `initiatives` list them.

## Create

`plan` creates the initiative when it is absent and appends one task per
spec. Each task after the first depends on the previous one, so the chain
runs in order. Task specs are explained in [tasks](tasks.md).

```bash
jacazul flow plan parity 'DESIGN|Define schema' 'EXECUTE|Implement store'
jacazul flow goal parity 'Persist workflow state natively'
jacazul flow plans
```

Name an initiative after its outcome, in a short kebab-case word group. Before
creating it, extract from the conversation what a cold-start agent needs:
the goal, the first tasks, and every agreed but ambiguous point, recorded as a
`DECISION` right after creation.

## Contract

An initiative carries a durable contract separate from its tasks'
annotations: description, goal, scope, acceptance criteria, out of scope,
risks, approvals, tickets, rejected paths, relations, references, and fixmes.
Read it before acting.

Today only the goal is writable, through `goal`; `plans` and `ponder` show it.
The other contract fields are not yet writable from the CLI and appear only
in structured reports. Until they are, record agreed acceptance criteria on
the first task as `AC` annotations.{% comment %}
TODO(#26): replace the paragraph above once agent-writable initiative
metadata ships and context/status render the contract.
{% endcomment %}

## Organize

- An initiative that is paused but not finished goes to `backlog`, which hides
  it from default dashboards; `activate` restores it. Do not discard or rename
  work to hide it.
- `organize` reorders pending tasks inside one initiative.
- `rename` changes an initiative's name and keeps its tasks, focus, and
  tickets.
- `roadmap` keeps the project roadmap ledger across initiatives.

```bash
jacazul flow plan parity 'Define schema'
jacazul flow backlog parity
jacazul flow plans --with-backlog
jacazul flow activate parity
```

## Commands

| Command | Purpose |
|---|---|
{% for command in topics.initiatives.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
