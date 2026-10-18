# Reports

Owner of: reading workflow state, output formats, the cache, and errors.

## Which report

- `status` is the hands-on view of the focused initiative: use it while
  working.
- `ponder` is the project landscape across initiatives: use it to orient
  when nothing is anchored or the operator asks for the whole picture.
- `tree` shows an initiative's dependency chain; `active`, `blocked`, and
  `overdue` filter tasks by state; `history` shows what changed.

```bash
jacazul flow plan parity 'Define schema' 'Implement store'
jacazul flow status
jacazul flow ponder
jacazul flow tree parity
jacazul flow blocked
```

## Formats

Text is the default conversation format. Use a structured format only when a
program must parse a complete report; `jacazul flow help <command>` lists the
formats a report accepts. Full UUIDs are the identity in structured output.

## Cache

Reports cache their output per session. A `[cached]` signal means nothing
changed since the last full report: reason from that report and do not
reproduce it. Force a refresh only on an explicit request or a concrete
reason to distrust the cache; `cache` inspects or clears it.

## Errors

Successful state changes print to stdout. Failures print to stderr with an
`ACTION:` line: treat it as the next valid move and follow it instead of
retrying blindly. Healthy no-op checks stay quiet unless the command is a
report.

## Commands

| Command | Purpose |
|---|---|
{% for command in topics.reports.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
