# Runtime

Owner of: project, home, and session identity.

Every command runs inside one project and one session:

| Concern | Flag | Environment | Default |
|---|---|---|---|
{% for row in runtime %}| {{ row.Concern|safe }} | `{{ row.Flag|safe }}` | `{{ row.Env|safe }}` | {{ row.Default|safe }} |
{% endfor %}
Precedence is flag, then environment, then default. A linked Git worktree
resolves to its shared repository root, so every worktree of one repository
is the same project.

The launcher owns these values: never export or override them by hand, and
never point flow at another project's state. Legacy Taskwarrior variables are
not read; `migrate` imports legacy state explicitly.

`info` shows the resolved values and where each one came from, without
opening storage.

```bash
jacazul flow info
```

## Commands

| Command | Purpose |
|---|---|
{% for command in topics.runtime.Commands %}| `{{ command.Name|safe }}` | {{ command.Summary|safe }}{% if command.Aliases %} (alias: {{ command.Aliases|join:", "|safe }}){% endif %} |
{% endfor %}
Run `jacazul flow help <command>` for the syntax of each one.
