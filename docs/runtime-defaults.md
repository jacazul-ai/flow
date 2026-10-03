# Runtime Defaults

This document is the canonical runtime contract implemented by the
`flow-strong-defaults` initiative ([GitHub #17](https://github.com/jacazul-ai/flow/issues/17)).
It describes the values used by standalone `flow` invocations and by launchers
that inject a resolved `flow.Env`.

## Scope and status

Implemented in #17:

- canonical project identity resolution;
- canonical home and session defaults;
- canonical CLI flags and environment variables;
- launcher-owned session continuity;
- project and session isolation through the resolved runtime context.

Not implemented in #17:

- a persisted configuration file, its location, or its schema.

The persisted configuration layer is tracked separately in
[`flow-config-layer` / GitHub #18](https://github.com/jacazul-ai/flow/issues/18).
No launcher or engine component should invent a configuration-file path before
that feature is designed and implemented.

## Canonical inputs

The canonical names are:

| Concern | CLI parameter | Environment variable | Default |
|---|---|---|---|
| Project | `--project` | `JACAZUL_PROJECT` | Canonical project resolution |
| Home | `--home` | `JACAZUL_HOME` | `$HOME/.jacazul-ai` |
| Session | `--session` | `JACAZUL_SESSION` | `global` |

The source precedence contract is:

```text
explicit CLI parameter > environment variable > configuration file > default
```

Because the configuration-file source is not implemented yet, the effective
precedence in #17 is:

```text
explicit CLI parameter > environment variable > computed default
```

For embedded callers, `flow.Run` never reads process environment. The caller
supplies the resolved values through `flow.Env`; CLI parameters still override
that injected runtime context. `flow.EnvFromOS` is the standalone boundary
that reads the canonical environment variables and computes defaults.

## Project resolution

When `JACAZUL_PROJECT` or `--project` is absent, `flow` derives the project
identity from the canonical current directory:

1. **Non-Git directory:** use the canonical current directory as the anchor.
2. **Normal Git repository:** use `git rev-parse --show-toplevel` as the
   anchor.
3. **Linked worktree:** when `git-common-dir` ends in `.git` or `.bare`, use
   the parent of that shared Git directory as the anchor. This prevents the
   worktree directory name from becoming part of the project identity.

The identity is always:

```text
basename(parent(anchor)) + "_" + basename(anchor)
```

For this repository:

```text
worktree: /home/fpiraz/source/jacazul-ai/flow/master
anchor:   /home/fpiraz/source/jacazul-ai/flow
project:  jacazul-ai_flow
```

The result is `jacazul-ai_flow`, never `jacazul-ai_master`, whether the
resolver starts from the shared root or from the linked worktree.

## Home resolution

The runtime home is resolved as:

```text
--home > JACAZUL_HOME > $HOME/.jacazul-ai
```

Default project-scoped storage is derived below that home:

```text
<home>/flow/<project>/flow.sqlite3
<home>/.task/<project>
```

An explicit database path remains a separate override through
`--database-path` or `JACAZUL_FLOW_DATABASE_PATH`.

## Session resolution

The session is resolved as:

```text
--session > JACAZUL_SESSION > global
```

A launcher owns and preserves a named `JACAZUL_SESSION` across commands. The
flow engine never generates a new session ID per process. A named session
selects independent focus and cache state; the absent-session `global` scope is
stable.

## Legacy runtime names

The following names are historical runtime inputs from the previous launcher
contract. They are documented for migration only; they are not aliases in the
new `flow` CLI or standalone resolver:

| Legacy name | Canonical name | Handling in `flow` |
|---|---|---|
| `PROJECT_ID` | `JACAZUL_PROJECT` | Not read as standalone project input |
| `JACAZUL_SESSION_ID` | `JACAZUL_SESSION` | Not read as standalone session input |
| `--project-id` | `--project` | Rejected by the new CLI |
| `--session-id` | `--session` | Rejected by the new CLI |

Any compatibility translation belongs in `jacazul-ai-cli/upstream`, not in
this new engine.
