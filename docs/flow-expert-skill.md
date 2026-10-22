# flow-expert Skill

## Purpose

`flow-expert` is the canonical agent skill for the native workflow engine. It
replaces the legacy `taskwarrior-expert` skill from `jacazul/launcher`, which
remains behavioral reference material only. This document records how the
skill is generated, the guarantees its tests provide, the decisions behind
it, and the known gaps. It is tracked by
[GitHub #26](https://github.com/jacazul-ai/flow/issues/26).

## Ownership

`jczl-flow skill` is flow's own hatch: it forges the `flow-expert` tree from
the engine's code. The tree is the same for every client.

- `flow` owns the skill content: workflow-domain agent rules such as
  onboarding, the work loop, task authoring, annotations, focus, sessions,
  reports, cache, and errors.
- The launcher installs the tree and keeps personas, client prompts,
  environment approval modes, Git, and routing. It never rewrites the skill.

## Layout

```text
SKILL.md                  hub: onboard, orient (focus, status, ponder),
                          the work loop, rules, help, routing table
references/
|-- initiatives.md        plan/ini, create, contract, backlog, organize
|-- tasks.md              task spec, modes, readiness, lifecycle
|-- focus.md              anchor, stack, independent focus, re-anchor
|-- sessions.md           onboard, handoffs, session state
|-- context.md            annotation kinds, inherited context, tickets
|-- reports.md            status, ponder, other reports, cache, errors
|-- runtime.md            project, home, session identity
\-- commands.md           index of every command by topic
```

The hub stays short: it is what every session needs. Each reference starts
with an `Owner of:` line and ends with its commands table and a pointer to
`jacazul flow help <command>`, which remains the live authority for syntax.

## Usage

```bash
jczl-flow skill                  # print the SKILL.md hub
jczl-flow skill --out <dir>      # write the whole tree below <dir>
```

`--out` replaces the generated files, removes reference files the skill no
longer produces, and leaves every other file alone, such as hand-written
evals. The command never opens workflow storage. Rendered text uses the
`jacazul flow` invocation; `jczl-flow` appears only in standalone use.

## Sources

Templates live in `internal/cli/templates/flow-expert/` and render through
Pongo2. Prose explains concepts; facts come from code:

| Fact | Source |
|---|---|
| commands, summaries, aliases | the command registry |
| which reference explains a command | the `topic` of its help entry |
| task modes | `task.TaskModes` |
| annotation kinds and aliases | `task.AnnotationKinds` |
| title limits | `task.TitleWarningLength`, `task.TitleMaxLength` |
| due date forms | `dueDateKeywords` and `dueDateLayout` in `internal/cli/plan.go` |
| runtime flags, variables, defaults | `config.AppOptions` tags and the `config` constants |

Guidance text for each mode and each annotation kind lives in
`internal/cli/skill.go`, keyed by the catalog entry.

## Guarantees

Rendering fails, with an `ACTION:`, when:

- a visible command has no topic, or its topic has no reference;
- a task mode or an annotation kind has no guidance.

The tests verify that:

- the hub starts with the skill frontmatter and covers onboard, focus,
  status, ponder, and help;
- every hub link resolves and every reference is routed from the hub;
- every visible command appears in its topic's reference and in the index;
- every mode, annotation kind and alias, title limit, due form, and runtime
  name appears in the reference that owns it;
- no file exposes `jczl-flow`, `TODO`, or template syntax;
- schema upgrades are silent when unnecessary, announced before automatic
  application when pending, and rejected when the database is newer;
- `--out` writes a deterministic tree, drops stale references, and keeps
  unrelated files;
- **every `bash` block runs** on a fresh isolated project through `flow.Run`.
  `<first-uuid>` and `<second-uuid>` bind to the tasks the block's `plan`
  creates. An example the engine rejects fails the build.

Each rule was mutation-checked: an argument-less `focus ind`, an invalid due
keyword, a broken hub link, a mode or kind without guidance, a command
without a topic, disabled stale cleanup, and dropped aliases all fail the
suite.

There is no golden file:
[characterization-tests.md](characterization-tests.md) reserves goldens for
refactors of existing behavior. Prose changes are reviewed in the template
diff.

## Decisions

1. The skill is a complete operating contract with a lean hub. Syntax is
   delegated to `help`; the skill never duplicates the registry's
   prerequisites, effects, and examples.
2. Onboarding is deterministic: `jacazul flow onboard` renders the pending
   session handoff, focus, context, and status or ponder, records native dump
   delivery without acknowledging it, and never starts work. Native dumps use
   fixed Focus, References, Actions, and Notes sections; explicit `session ack`
   releases the next dump.
3. Schema lifecycle is deterministic: commands opening SQLite check the
   embedded Goose version, announce and apply pending upgrades for existing
   databases, stay silent for fresh/current databases, and reject newer schemas
   with actionable guidance.
4. After `done`, agents re-anchor only to a ready task in the same initiative,
   or clear the focus (see [focus-advance-i-d.md](focus-advance-i-d.md)).
5. The skill is honest about unimplemented behavior. Gaps are tracked as
   non-rendered Pongo2 comment blocks marked `TODO(#26)`.
6. The command stays named `skill` so it is not confused with the launcher's
   `jacazul-hatch`.
7. Taskwarrior is a migration-time compatibility source only for existing
   `tw-flow` projects: the default migration preflights and invokes `task`
   directly, accepts an explicit binary and optional legacy prefix map from the
   launcher, and keeps green-field flow commands native.

## Known gaps

### Metadata is modeled but not agent-writable

Initiative and task metadata (`internal/task/task.go`) is persisted and shown
in structured reports, but the CLI only writes the initiative goal through
`goal`; `amend` corrects only the description and ticket. Text `status` and
`context` show no task metadata except fixmes. Until the metadata commands
exist, the skill tells agents to record acceptance criteria as `AC`
annotations.

The write and read boundary belongs to the `[EXECUTE] Implement native flow
boundary required by skill and hatch` task:

- writable initiative fields: description, goal, scope, acceptance criteria,
  out of scope, risks, references, and fixmes;
- writable task fields: expected result, acceptance criteria, references, and
  fixmes;
- `context` and `status` render the task contract and an initiative contract
  summary before annotations;
- structured fields (contract approvals, relations, rejected paths) may follow
  in a second slice.

### Differences from taskwarrior-expert

| Legacy guidance | Native state | Direction |
|---|---|---|
| Plan spec tags such as `+research` | `parseTaskSpec` accepts the tag field and does not create stored task tags | The plan-spec tag field remains legacy-only; Taskwarrior stored tags are managed by migration and `amend +tag` |
| Modes `[PLAN]` and `[PR-REVIEW]` | Not in the native mode catalog; an unknown prefix stays in the description | Compatibility decision required: map `PLAN` to `DESIGN`, and add or map `PR-REVIEW` |
| `_archive` sub-projects | Native `backlog` and `activate` | Covered in `initiatives.md` |
| Priority H, M, L | `urgent` raises priority | Covered in `tasks.md` under the cool-down rule |
| `PROJECT_ID`, `TASKDATA`, `JACAZUL_SESSION_ID` | `JACAZUL_PROJECT`, `JACAZUL_HOME`, `JACAZUL_SESSION` | Covered in `runtime.md` and [runtime-defaults.md](runtime-defaults.md) |
| Focus pre-seed through `JACAZUL_FOCUS_PLAN` and `JACAZUL_FOCUS_TASK` | Not a flow concern | Remains in the launcher bootstrap |
| Hierarchical project names with `:` | Initiatives are first-class objects | Dropped |

### Engine findings from writing the scenarios

- Adding a dependency that already exists (`block <uuid> <uuid>`) fails with
  a raw SQLite `UNIQUE constraint failed` error and no `ACTION:`.
- The invalid due date error lists `YYYY-MM-DD, today, or tomorrow` but
  omits the accepted `yesterday`.

## Next steps

- Skill behavior evals beside the tree, outside the generated paths, so
  `--out` keeps them. Initial scenarios: closing without an outcome, a
  blocked next task after `done`, onboard then stop, `jacazul flow` instead
  of `jczl-flow`, and a plan spec with a tag.
- The `PLAN` and `PR-REVIEW` mode decision.
