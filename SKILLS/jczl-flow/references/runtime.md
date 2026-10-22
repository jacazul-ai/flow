# Runtime

Owner of: project, home, and session identity.

Every command runs inside one project and one session:

| Concern | Flag | Environment | Default |
|---|---|---|---|
| Project | `--project` | `JACAZUL_PROJECT` | resolved from the working directory, else global |
| Home | `--home` | `JACAZUL_HOME` | $HOME/.jacazul |
| Session | `--session` | `JACAZUL_SESSION` | global |

Precedence is flag, then environment, then default. A linked Git worktree
resolves to its shared repository root, so every worktree of one repository
is the same project.

The launcher owns these values: never export or override them by hand, and
never point flow at another project's state. Legacy Taskwarrior variables are
not read; `migrate` imports legacy state explicitly. It is only for existing
`tw-flow`/Taskwarrior projects; green-field projects start with native flow
commands. The default migration path checks for `task` and invokes it directly
with the resolved Taskwarrior data directory. Launchers may provide the resolved executable through
`--task-binary` or the embedding `flow.Env.TaskBinary`. An explicit `--source`
snapshot is the offline alternative. `--legacy-map` applies only explicit,
validated historical annotation and mode mappings; normal flow commands never
invoke Taskwarrior.

`info` shows the resolved values and where each one came from, without
opening storage.

```bash
jacazul flow info
```

## Schema upgrades

Commands that open SQLite check the embedded Goose schema before running.
Fresh and current databases stay silent. An existing older database announces
its version transition and applies pending migrations automatically. A database
newer than the binary fails with an `ACTION:` to upgrade the engine. Schema
upgrades do not require a separate `migrate up` command.

## Commands

| Command | Purpose |
|---|---|
| `help` | Show the agent workflow briefing |
| `skill` | Render the canonical flow-expert agent skill |
| `info` | Show resolved runtime defaults |
| `migrate` | Migrate legacy tw-flow workflow state |

Run `jacazul flow help <command>` for the syntax of each one.
