# Taskwarrior to flow Migration

## Purpose

This document defines the first local migration from the legacy
Taskwarrior-backed `jacazul-ai-cli` workflow to native flow. The migration
moves workflow state into the project-scoped flow SQLite database while
keeping `jacazul-ai-cli` available as the behavioral oracle and temporary
compatibility client.

The migration is not a runtime dependency of flow. Normal flow commands
must not invoke Taskwarrior, read legacy `TASKDATA`, call a ticket broker, or
load output caches. Legacy access belongs to an explicit importer boundary.

## Source and target

### Source

The importer consumes an isolated Taskwarrior export snapshot and optional
legacy session files:

- `task export` JSON from the selected legacy `TASKDATA`;
- the resolved `PROJECT_ID` and source project names;
- `focus.json` and `focus-<SESSION_ID>.json` files when session migration is
  requested;
- session handoff notes associated with those focus files;
- no cache files, credentials, broker tokens, source code, or client prompt
  artifacts.

The source snapshot must be captured before applying changes. The importer
must receive explicit paths and environment values; it must not discover or
modify the operator's real home, `TASKDATA`, or production ticket state by
accident.

### Target

The target is one native database selected by the canonical `PROJECT_ID`:

```text
$JACAZUL_HOME/flow/<PROJECT_ID>/flow.sqlite3
```

The native database owns initiatives, tasks, dependencies, annotations,
focus, sessions, session notes, roadmap entries, and derived cache policy.
Imported caches are discarded because they are derived and may contain stale
or private output.

## Mapping contract

| Legacy value | Native value | Migration rule |
|---|---|---|
| Taskwarrior `project` | `initiatives.project_id` + `initiatives.name` | Derive one first-class initiative per project name within the selected project. Never use a grouping string as the runtime source of truth. |
| Taskwarrior `uuid` | `tasks.id` | Preserve the full UUID. Numeric Taskwarrior IDs are lookup-only and are never persisted as identity. |
| `description` with a recognized `[MODE]` prefix | `tasks.description` + `task_mode_code` | Extract the mode into the native catalog and store the clean description. Unknown prefixes remain in the description and are reported. |
| `status:pending` without a start | `tasks.status = pending` | Preserve pending state. Dependency readiness is recalculated in the native store. |
| `status:pending` with `start` | `tasks.status = active` | Preserve the active execution state and `started_at`. |
| `status:waiting` or future `wait` | `tasks.status = pending` + `wait_until` | Preserve the wait date; the task is not ready before that date. |
| `status:completed` | `tasks.status = completed` | Preserve `completed_at`, outcome, priority, urgency, due date, and metadata. Historical completed tasks may lack an outcome; report that condition instead of inventing one. |
| archived/discarded task markers | completed task + `disposition = discarded` | Preserve the task and add the discard audit when the source proves it was discarded. Do not silently delete archive history. |
| `depends` | `task_dependencies` | Resolve UUID and numeric references through the snapshot map before inserting edges. Unresolved or self-referential edges fail the apply with an actionable report. |
| `externalid` | `tasks.external_ticket` | Preserve direct ticket metadata without contacting a broker. Inherited ticket lookup remains a native behavior. |
| `priority`, `urgency` | task priority and urgency metadata | Preserve values; default only when the source field is absent. Do not recalculate historical urgency during import. |
| `due` | `tasks.due_at` | Normalize supported Taskwarrior date forms to the native date representation and report invalid values. |
| annotations | `annotations` | Preserve body and source timestamp. Map recognized semantic prefixes to canonical kinds; preserve unknown kinds in a lossless migration representation and report them. |
| `focus.json` | the global native session | Preserve focused initiative, focused task, task stack, and plans of interest after UUID resolution. A blocked imported anchor may be preserved as navigation history but must remain non-executable. |
| `focus-<SESSION_ID>.json` | a native session keyed by `project_id` and `session_id` | Import each independent focus file into its matching native session without crossing project boundaries. |
| session handoff note | `session_notes` | Preserve content, session identity, timestamps, and acknowledged state. |
| roadmap task metadata | `roadmap_entries` | Import recognized roadmap phases as ledger entries; do not turn roadmap records into operational task dependencies. |
| output cache files | nothing | Never import caches. The first native report must be fresh and session-scoped. |

Tags and other Taskwarrior fields not represented by the current native model
must be included in the dry-run report as retained-data gaps. The importer may
not silently drop them. Adding a native field or a lossless extension is a
design decision for the implementation slice that owns that mapping.

## Import modes

The first importer interface should expose the following modes:

```text
jczl-flow migrate taskwarrior --source <export.json> --project-id <id> --dry-run
jczl-flow migrate taskwarrior --source <export.json> --project-id <id> --apply
```

- **Dry-run is the default.** It validates the snapshot, resolves references,
  computes changes, and writes a report without opening the target for mutation.
- **Apply is explicit.** It requires a source snapshot and a target project
  identity. It must not infer an untrusted target from the current directory.
- **Repeat imports are idempotent.** Reapplying the same snapshot does not
  duplicate initiatives, tasks, dependencies, annotations, focus entries, or
  session notes.
- **Changed snapshots are reported.** The importer compares source identity
  and source fingerprints with prior imports. Mutable source fields may be
  updated before cutover; native-only changes and conflicts require an
  explicit force policy rather than silent overwrite.
- **One project is one transaction boundary.** A failed apply rolls back the
  project import rather than leaving a half-migrated dependency graph.

The import report must include created, updated, unchanged, skipped, and
conflicted records, unresolved dependency references, invalid dates, unknown
modes or annotation kinds, retained-data gaps, and the exact target database
path. Reports must not include secrets or ticket credentials.

## Identity and idempotency

The source UUID is the primary import key. Numeric Taskwarrior IDs are mapped
only while reading one snapshot because they can change across exports. The
importer must build all UUID and numeric lookup maps before writing edges.

An import ledger or equivalent source metadata must record at least:

- source snapshot identity and fingerprint;
- target project identity;
- source task UUID and imported native UUID;
- import timestamp and importer version;
- field-level conflict or warning state.

The importer must reject ambiguous UUID prefixes and duplicate source UUIDs.
A duplicate source UUID is a malformed snapshot, not two tasks to merge.

## Safety boundary

The migration path is privileged over ordinary reporting because it writes the
workflow database. It therefore follows these rules:

- no real migration during tests or investigations;
- fixtures use `t.TempDir()` or the persistent sandbox root;
- source export is treated as untrusted input;
- SQL remains parameterized and identifiers are validated before use;
- external command execution uses an explicit command path, controlled
  environment, context cancellation, and captured diagnostics;
- no shell interpolation of descriptions, projects, annotations, UUIDs, or
  ticket values;
- no broker calls, network access, credential reads, or production ticket
  synchronization;
- apply creates a native backup/checkpoint before mutation and reports its
  location without embedding secrets;
- rollback restores the pre-apply target or removes the new target database;
- dry-run and apply expose the same validation report so the operator can
  inspect the exact planned changes first.

The existing `scripts/migrate-project-prefix.sh` is not the migration engine.
It mutates legacy state in place, uses shell parsing, and cannot provide native
transactional import or rollback. It may remain a historical compatibility
script but must not be called by the native importer.

## Verification and cutover

Migration verification has two separate oracles:

1. **State verification:** compare normalized source and native records for
   UUIDs, initiatives, statuses, dependency edges, annotations, tickets,
   focus, sessions, and dates.
2. **Behavior verification:** run the same isolated scenarios through the
   legacy `tw-flow` adapter and the native `flow` adapter. Normalize UUIDs,
   timestamps, paths, and known formatting differences before comparison.

Known legacy defects are not migration targets. For example, an imported
blocked focus anchor may be preserved as historical state, but native
`next`, `focus plan`, and automatic advancement must use readiness and never
select blocked work merely because it is pending.

The cutover sequence is:

1. capture and checksum a source export snapshot;
2. run dry-run and review the report;
3. create a native backup/checkpoint;
4. apply into an isolated target and run state verification;
5. run differential behavior scenarios;
6. switch `jacazul-ai-cli` to the native flow client path;
7. keep the legacy path read-only and available for rollback;
8. monitor native errors and retain the source snapshot until acceptance;
9. remove legacy runtime writes only after an explicit rollback window.

The migration is complete only when a second apply is unchanged, no required
fields are silently lost, state and behavior verification pass, and rollback
has been exercised on an isolated target.

## Operator runbook

The following procedure is for an explicitly selected project and an isolated
sandbox first. Replace every path with a deliberate target; do not run it
against the operator's real `TASKDATA` or flow database until the dry-run and
acceptance review are complete.

### 1. Capture the source snapshot

Use the controlled legacy boundary to create a JSON snapshot and retain its
checksum. Do not build the snapshot with shell interpolation of task fields.

```bash
mkdir -p /tmp/flow-migration
TASK_EXPORT=/tmp/flow-migration/tasks.json
taskp export > "$TASK_EXPORT"
sha256sum "$TASK_EXPORT" > "$TASK_EXPORT.sha256"
```

When focus or handoff continuity is required, copy the selected legacy focus
files into an isolated state directory and pass it explicitly:

```bash
LEGACY_STATE=/tmp/flow-migration/legacy-state
mkdir -p "$LEGACY_STATE"
# Copy only the selected focus*.json and session-note-*.md files here.
```

The source directory is input-only. The importer must never modify it.

### 2. Dry-run and review

Run the importer with an explicit project and database target. Omit `--apply`
to keep the operation read-only; `--dry-run` may be supplied for clarity.

```bash
jczl-flow \
  --project-id "$PROJECT_ID" \
  --database-path "$TARGET_DB" \
  migrate taskwarrior \
  --source "$TASK_EXPORT" \
  --legacy-data-dir "$LEGACY_STATE" \
  --dry-run
```

Review the report for:

- task, initiative, dependency, annotation, focus, and session counts;
- unresolved or ambiguous UUID/numeric references;
- unknown statuses, modes, annotation kinds, and invalid dates;
- retained-data warnings such as unsupported tags;
- the exact target project and database path.

Do not apply while required data has a warning that has not been accepted or
mapped. A dry-run must not create the target database.

### 3. Apply with a checkpoint

After reviewing the report, apply the same snapshot to the same explicit
project and target. The command creates a timestamped backup of an existing
SQLite database and available `-wal`/`-shm` sidecars before applying.

```bash
jczl-flow \
  --project-id "$PROJECT_ID" \
  --database-path "$TARGET_DB" \
  migrate taskwarrior \
  --source "$TASK_EXPORT" \
  --legacy-data-dir "$LEGACY_STATE" \
  --apply
```

Record the printed backup path, migration counts, source checksum, target
path, and command version together in the change record. Keep the source
snapshot and backup through the rollback window.

### 4. Verify the native result

Run the state and behavior checks in a new process using the same project and
session identity:

```bash
jczl-flow --project-id "$PROJECT_ID" --database-path "$TARGET_DB" status --force
jczl-flow --project-id "$PROJECT_ID" --database-path "$TARGET_DB" next <initiative>
jczl-flow --project-id "$PROJECT_ID" --database-path "$TARGET_DB" focus
jczl-flow --project-id "$PROJECT_ID" --database-path "$TARGET_DB" session list
jczl-flow --project-id "$PROJECT_ID" --database-path "$TARGET_DB" session resume
```

Confirm that:

- every required source UUID appears exactly once;
- initiative and dependency counts match the normalized source snapshot;
- annotations and direct/inherited tickets are available;
- active, completed, waiting, and blocked states behave correctly;
- focus and session notes point to the migrated native UUIDs;
- a second `--apply` produces no duplicate tasks, edges, annotations, or
  session records;
- a separate project database cannot observe the migrated records;
- the first native report is fresh and does not reuse legacy cache output.

For differential verification, run the same isolated scenario through the
legacy adapter and the native binary. Compare normalized state and behavior,
not volatile UUID formatting, timestamps, paths, or known intentional fixes.

### 5. Rollback

Rollback is an operator-controlled file restoration, not a hidden importer
operation. Stop all flow writers first and preserve the failed target for
forensics. Then restore the backup printed by the apply command, including
sidecars when they exist:

```bash
mv "$TARGET_DB" "$TARGET_DB.failed-$(date -u +%Y%m%dT%H%M%SZ)"
mv "$BACKUP_DB" "$TARGET_DB"
# Restore "$BACKUP_DB-wal" and "$BACKUP_DB-shm" to the matching target names
# when the backup command reported those sidecars.
```

Reopen the restored target only after the file moves finish, rerun the native
status/session checks, and compare against the pre-apply checksum or snapshot.
Do not delete the failed target until acceptance or incident review is
complete.

### Current v1 limitations

The current importer is the first vertical slice, not the final cutover gate:

- source fingerprints and a persistent import ledger are not yet exposed;
- changed-snapshot conflict policy and a `--force` overwrite policy are not
  yet exposed;
- Taskwarrior tags are reported as retained-data warnings but are not yet
  persisted natively;
- roadmap phase import is documented but not yet implemented by the snapshot
  mapper;
- rollback is documented as an operator procedure rather than a CLI command.

These limitations must remain visible in migration reports and must be closed
before production cutover.

## Scope boundary

This initiative covers local import and cutover preparation. It does not
implement live server synchronization, multi-agent conflict resolution,
remote ownership, prompt generation, persona artifacts, or GitHub broker
internals. Those remain separate boundaries above the native local engine.
