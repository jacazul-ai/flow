# Taskwarrior to flow migration playbook

This is the operational source of truth for the Taskwarrior-to-`flow`
migration. Read it before changing the importer, writing canonical fixtures, or
running a cutover. The native SQLite database is the destination; Taskwarrior
is a read-only compatibility source. This playbook is only for projects
migrating an existing `tw-flow` workflow; green-field projects should start
with native `flow` commands and do not need Taskwarrior.

## 1. Contract first

The migration contract is recorded in the `taskwarrior-migration-0.0.1`
initiative, beginning at task `c1a0ca77`. Do not infer behavior from an old
agent handoff when this document or the task annotations says otherwise.

### Domain terms

- **`PROJECT_ID`** selects the top-level native silo. It is not an initiative
  name and must never be concatenated into a user-visible initiative name.
- **Taskwarrior `project`** is the source grouping label. After the archive
  rule below is applied, its final name becomes the native initiative/plan
  name.
- **Task UUID** is the durable task identity. Taskwarrior numeric IDs are
  lookup-only values inside one export.
- The destination has one initiative name per target silo:
  `UNIQUE(project_id, name)`. The internal lookup key is therefore the pair
  `(target project, final initiative name)`, not a synthetic name such as
  `PROJECT_ID/foo`.

### Task buckets

Buckets classify **tasks**, not initiatives/plans. A bucket is a task-level
field with one of the initial kinds `active`, `archive`, or `trash`; it is not a
tag, initiative status, or relationship to an initiative bucket table.

The source project label is split into two independent mappings:

| Source project | Destination initiative | Task bucket | Task lifecycle |
|---|---|---|---|
| `foo` | `foo` | `active` | preserve source state |
| `foo:_archive` | `foo` | `archive` | preserve source state |
| `foo:_trash` | `foo` | `trash` | preserve source state |
| only `foo:_archive` exists | create normal `foo` | `archive` | preserve source state |
| only `foo:_trash` exists | create normal `foo` | `trash` | preserve source state |

Do not create `foo:_archive` or `foo:_trash` as native initiatives. Do not put
`archived` on the initiative: archive is a task bucket. Initiative lifecycle
remains limited to its native workflow states such as active, backlog, and
completed. Bucket membership and task lifecycle are independent.

The operational chain and tree are projections of active-bucket tasks. If the
source chain is `1 -> 2 -> 3 -> 4` and task `2` moves to `archive`, native tree
output is `1 -> 3 -> 4`. The archived task remains persisted for history but
must not render as an error, placeholder, ghost, or phantom node.

The current canonical source has 11 exact `foo`/`foo:_archive` pairs. They all
map to one initiative name with different task bucket values, so there is no
initiative status conflict. The snapshot has no `:_trash` labels; add a
synthetic fixture before claiming trash parity.

The native persistence shape is intentionally small:

```text
tasks.bucket TEXT NOT NULL DEFAULT 'active'
valid values: active, archive, trash
```

There is no `initiative_buckets` table and no bucket-to-initiative
relationship. The task keeps its existing initiative reference; bucket is an
independent task classification. `trash` does not set
`disposition=discarded`; that disposition remains reserved for explicit
discard evidence or the native discard operation.

### Cutoff identity and retry

- Preserve every source task UUID when importing.
- Derive or persist a stable native initiative UUID for the final initiative
  name within the selected target project. This is an internal identity rule;
  it does not change the initiative name.
- One captured source snapshot is the migration boundary. Record its checksum
  with the cutoff report.
- A dry-run is read-only and only renders its report.
- Apply is one atomic cutover into a clean target. It may persist a simple
  `migration_issues` audit for that cutoff, but it does not create a lineage
  of parent runs or an incremental synchronization queue.
- A retry is allowed only after rollback or against a clean target. It must
  preserve source UUID identity and must not duplicate records.
- Changed snapshots, stale-source reconciliation, and ongoing synchronization
  are outside this migration.

### Error policy

The migration must distinguish output severity from process failure:

- **WARNING:** optional metadata is normalized, retained elsewhere, or cannot
  be represented exactly. Continue. If there are warnings only, exit `0`.
- **ERROR:** a record-level conversion problem or incompatible metadata. Record
  the error, use the documented safe fallback or skip only the affected
  record, and continue processing the rest of the snapshot. If any ERROR was
  recorded, complete the run and exit `1`.
- **FATAL:** the source cannot be read or decoded, the target cannot be opened,
  the transaction cannot commit, or the database is otherwise unsafe to use.
  Abort rather than claiming a completed migration and exit non-zero.

No field may be silently dropped. Reports must identify the task, initiative,
field, severity, fallback, and whether the source value was retained in
metadata or annotations.

## 2. Current canonical source baseline

The launcher source is the explicit Taskwarrior data directory:

```text
$HOME/.jacazul-ai/.task/jacazul-ai_launcher
```

The latest read-only verification used during this design work found:

| Measure | Count |
|---|---:|
| Tasks | 1,961 |
| Distinct project labels | 236 |
| Non-archive labels | 212 |
| `:_archive` labels | 24 |
| Exact `foo` + `foo:_archive` pairs | 11 |
| Archive-only buckets | 13 |
| Labels after archive folding | 225 |

These are a snapshot baseline, not permanent constants. Always recapture the
export before creating or updating canonical fixtures. The current exact pairs
were:

```text
enforce-outcome
flow-cached-signal
jacazul-home
jacazul-pi
onboard
skill-agent-modularization
skill-system-evolution
smoke
test-feature
tw-flow-visibility
workflow_stabilization
```

The archive-only buckets observed in that snapshot were:

```text
"urgency-test-v4":_archive
i-d-clarity:_archive
jacazul-go-launcher:_archive
mode-test:_archive
pending:_archive
personas:_archive
smoke-debug2:_archive
smoke-debug3:_archive
test-backlog-sandbox:_archive
test_del:_archive
test_rename_new:_archive
unscoped:_archive
worktree-support-chase:_archive
```

Archive-bucket task statuses in the same snapshot were:

```text
completed=404
deleted=1
pending=22
```

Those pending tasks are why archive projects must not force every task into an
archived lifecycle state.

## 3. Capture and inspect the source safely

The normal migration path invokes the `task` executable directly as a
subprocess. It preflights the executable, passes the explicit data directory
with `rc.data.location`, captures JSON from stdout, and reports stderr as an
actionable migration error. A launcher that already resolved the real binary
must pass `--task-binary` (or set `flow.Env.TaskBinary` when embedding the
engine); standalone invocations without it use `task` from `PATH`. The source
is read-only input; the native flow database is the only write target.

```bash
set -eu

PROJECT_ID="<target-project>"
TASK_DATA="$HOME/.jacazul-ai/.task/$PROJECT_ID"
TARGET_HOME="$(mktemp -d "${TMPDIR:-/tmp}/flow-home.XXXXXX")"
TARGET_DB="$TARGET_HOME/flow/$PROJECT_ID/flow.sqlite3"
LEGACY_STATE="$(mktemp -d "${TMPDIR:-/tmp}/flow-legacy.XXXXXX")"
TASK_BINARY="/path/to/task"

jczl-flow \
  --home "$TARGET_HOME" \
  --project "$PROJECT_ID" \
  --database-path "$TARGET_DB" \
  --taskdata "$TASK_DATA" \
  --task-binary "$TASK_BINARY" \
  migrate taskwarrior \
  --legacy-data-dir "$LEGACY_STATE" \
  --dry-run
```

If `task` is absent, the command fails before opening the native database and
prints an `ACTION:` explaining how to make the executable available. An
already captured JSON snapshot remains available as an explicit offline
alternative with `--source`; that mode does not invoke Taskwarrior.

An optional compatibility map can recover reviewed legacy semantics without
changing the default fallback:

```json
{
  "annotations": {
    "SPEC": "DECISION",
    "DONE": "OUTCOME",
    "TODO": null
  },
  "modes": {
    "PLAN": "DESIGN",
    "DOCS": "REFINE"
  }
}
```

See `docs/legacy-map.example.json` for a conservative starting point. Pass it
with `--legacy-map /path/to/legacy-map.json`. Missing entries and
`null` values keep the current behavior; an invalid or missing explicit map is
a fatal error before the target database is opened. Mapped annotation bodies
retain their original legacy prefix.

To verify archive pairs in an already captured offline snapshot before
changing fixtures:

```bash
TASK_EXPORT="/path/to/tasks.json"
python3 - "$TASK_EXPORT" <<'PY
import json
import sys

suffix = ":_archive"
with open(sys.argv[1], encoding="utf-8") as handle:
    tasks = json.load(handle)
labels = {task.get("project", "") for task in tasks}
archives = {label for label in labels if label.endswith(suffix)}
pairs = {label for label in archives if label[:-len(suffix)] in labels}
archive_only = archives - pairs
folded = {
    label[:-len(suffix)] if label.endswith(suffix) else label
    for label in labels
}
print(f"tasks={len(tasks)}")
print(f"labels={len(labels)}")
print(f"archive_labels={len(archives)}")
print(f"pairs={len(pairs)}")
print(f"archive_only={len(archive_only)}")
print(f"folded_labels={len(folded)}")
for label in sorted(pairs):
    print(f"pair={label[:-len(suffix)]} <-> {label}")
PY
```

If focus or handoff continuity is part of the migration, copy only the
selected `focus.json`, `focus-<SESSION_ID>.json`, and
`session-note-<SESSION_ID>.md` files into a separate working directory. Never
let the importer discover or mutate the real legacy state implicitly.

## 4. Run the importer

The destination must be explicit and isolated during development:

```bash
PROJECT_ID="<target-project>"
TARGET_HOME="$(mktemp -d "${TMPDIR:-/tmp}/flow-home.XXXXXX")"
TARGET_DB="$TARGET_HOME/flow/$PROJECT_ID/flow.sqlite3"
LEGACY_STATE="$WORK_DIR/legacy-state"

mkdir -p "$LEGACY_STATE"

jczl-flow \
  --home "$TARGET_HOME" \
  --project "$PROJECT_ID" \
  --database-path "$TARGET_DB" \
  --taskdata "$TASK_DATA" \
  --task-binary "$TASK_BINARY" \
  migrate taskwarrior \
  --legacy-data-dir "$LEGACY_STATE" \
  --dry-run
```

Dry-run is the default and must not create or mutate the target database. It
only renders the report and exit status. It must validate the same bundle and
produce the same warnings/errors that apply would produce.

After reviewing the report, apply the exact same snapshot and target:

```bash
jczl-flow \
  --home "$TARGET_HOME" \
  --project "$PROJECT_ID" \
  --database-path "$TARGET_DB" \
  --taskdata "$TASK_DATA" \
  migrate taskwarrior \
  --legacy-data-dir "$LEGACY_STATE" \
  --apply
```

Apply is one project-scoped transaction. Create a backup/checkpoint before
mutating an existing database and retain the source checksum, target path,
backup path, importer version, and final report together. Apply-time warnings
and errors are persisted as a simple cutoff audit and can be inspected without
re-running the migration:

```bash
jczl-flow \
  --project "$PROJECT_ID" \
  --database-path "$TARGET_DB" \
  migrate taskwarrior \
  --issues
```

`--issues` is observational; it does not run a new migration or create a
parent/child run chain.

## 5. Mapping checklist

The importer must cover these source values without losing identity:

| Taskwarrior source | Native flow destination |
|---|---|
| `uuid` | `tasks.id`, unchanged |
| `id` | temporary lookup only |
| final project name | `initiatives.name`, scoped by target project |
| terminal `:_archive` / `:_trash` | fold into the base initiative and set the task bucket to `archive` / `trash` |
| `description` and `[MODE]` prefix | cleaned task title plus native mode |
| `pending` | pending, or active when the source has `start` |
| `waiting` / `wait` | pending plus `wait_until` |
| `completed` | completed plus source completion metadata |
| `deleted` / discard evidence | completed plus `disposition=discarded` when proven |
| `depends` | native dependency edges after UUID resolution |
| `externalid` | task external ticket metadata |
| priority and urgency | native values; do not recalculate historical urgency |
| dates | normalized native dates, with warnings/errors for invalid values |
| annotations | structured annotations with source timestamps |
| stored tags | relational native tags; virtual tags are not persisted |
| focus/session files | native focus, sessions, and session notes |
| caches | nothing; caches are derived and discarded |

The source UUID map and numeric-ID map must be built before dependency edges are
written. Duplicate UUIDs, ambiguous references, self-dependencies, and
cross-project records must be reported with enough context to correct the
source or fixture.

## 6. E2E development loop

This migration is implemented through `flow.Run` contracts, not by testing
private helpers alone. Every test uses `t.TempDir()` or the persistent sandbox
root and an explicit `Env`; it never reads the real launcher data.

For each migration slice:

1. **Red:** add a failing contract test with a small export fixture.
2. **Green:** implement the smallest importer change that satisfies it.
3. **Refactor:** keep source loading, archive normalization, bundle building,
   persistence, and report/exit-code handling in separate responsibilities.
4. **Mutation check:** intentionally break the protected rule and prove the
   contract fails.
5. **Parity check:** compare the relevant legacy behavior and the documented
   migration contract.
6. **Quality gate:** run `gofmt`, `goimports` when installed, `go test ./...`,
   `go vet ./...`, and `go test -race ./...` (or `make test`).

The canonical contract must prove at least:

- all source tasks are preserved or explicitly reported as record-level
  errors;
- archive and trash buckets fold into base initiatives without creating bucket initiatives;
- archive-only and trash-only buckets create normal base initiatives;
- bucket membership and task lifecycle remain independent;
- tree and chain projections omit non-active bucket tasks without phantom nodes;
- warnings do not stop migration and return exit `0`;
- errors do not stop record processing and return exit `1`;
- dry-run is read-only and renders the complete report;
- apply is atomic and a rollback retry does not duplicate records;
- apply issues are persisted as one cutoff audit, without parent-run lineage;
- project isolation is preserved;
- unsupported metadata is visible in the report;
- mixed `foo`/`foo:_archive` bucket coexistence is covered before release.

## 7. Verification and rollback

After apply, verify through the public command boundary:

```bash
jczl-flow --project "$PROJECT_ID" --database-path "$TARGET_DB" ponder
jczl-flow --project "$PROJECT_ID" --database-path "$TARGET_DB" plans --all
jczl-flow --project "$PROJECT_ID" --database-path "$TARGET_DB" status
```

Verify the committed target once and compare normalized source and native
state for task UUIDs, final initiative names, initiative status, task
lifecycle, dependencies, annotations, tags, focus, sessions, and dates. If
apply fails, restore the target or use a clean target before retrying; do not
layer a new migration onto a partially migrated target.

Rollback is explicit and operator-controlled:

1. stop native writers;
2. preserve the failed database for investigation;
3. restore the backup and any reported WAL/SHM sidecars;
4. rerun native reports;
5. keep the source snapshot and failed target until acceptance is complete.

Do not delete the legacy source or switch launcher consumers until dry-run,
apply, idempotency, differential behavior, and rollback checks pass.

## 8. Do not repeat these mistakes

- Do not preserve `foo:_archive` or `foo:_trash` as native initiatives.
- Do not create a synthetic initiative named `PROJECT_ID/foo`.
- Do not treat `PROJECT_ID` as initiative identity.
- Do not put `archived` on an initiative to represent a task bucket.
- Do not force archive-bucket tasks into an archived task state.
- Do not silently drop incompatible metadata.
- Do not abort the whole migration for a record-level metadata ERROR.
- Do not run raw `task` manually for the cutover; invoke `jczl-flow migrate taskwarrior`, which owns executable discovery, explicit data-directory selection, and error handling.
- Do not run migration tests against the operator's real database.
- Do not use stale handoff counts as canonical fixtures; recapture and checksum
  the launcher export first.
- Do not layer a new migration onto an existing target; cutoff retries require
  rollback or a clean target.
- Do not close the design task until mixed `foo`/`foo:_archive` bucket
  coexistence is recorded and tested.

## Scope boundary

This playbook covers local snapshot import, archive normalization, validation,
idempotency, and cutover preparation. It does not cover live Taskwarrior
synchronization, remote ownership, multi-agent conflict resolution, prompt or
persona generation, broker internals, or production deployment automation.
