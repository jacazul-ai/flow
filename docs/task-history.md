# Native Task History

## Purpose

`flow` needs a durable task history, not only the current task snapshot and
structured annotations. The history contract preserves what changed, when it
changed, and which source or session produced the event. It supports both the
explicit history-scope commands and migration from the legacy TaskChampion
operation log.

History is task workflow state. Persona files, prompt artifacts, output cache,
and unrelated session presentation are not history records.

## Native event model

The native store should add an append-only `task_history` table:

| Field | Meaning |
|---|---|
| `id` | Native event identity |
| `task_id` | Full task UUID |
| `source` | `native` or a named migration source such as `taskchampion` |
| `source_event_id` | Source event identity for idempotent import |
| `sequence` | Source ordering when timestamps collide |
| `event_type` | `create`, `update`, `delete`, or workflow transition |
| `property` | Changed property, when applicable |
| `old_value` | Previous serialized value |
| `new_value` | New serialized value |
| `occurred_at` | Source or native event time |
| `actor` | Optional agent/persona-independent actor identity |
| `session_id` | Optional session that caused a native change |
| `recorded_at` | Time the native store recorded the event |

The idempotency key is `(source, source_event_id)`. Native events use a
stable generated event ID. Imported TaskChampion events use the source
operation ID, not a Taskwarrior numeric task ID.

History is immutable. A correction creates a new event; it does not rewrite
an earlier event. Current task tables remain the materialized state used by
normal workflow commands.

## Events to record natively

The native store appends events for state-changing operations:

- task creation and metadata amendment;
- task start, outcome, completion, reopen, and discard;
- dependency add/remove;
- urgency, priority, due, and wait changes;
- initiative creation, rename, backlog, and activation when the event is
  associated with a task or initiative history boundary.

Focus navigation, cache refreshes, and persona/bootstrap changes are not task
history. Session handoff content remains in annotations and session notes; the
history event may record that a handoff transition occurred without copying
private prompt data.

## CLI contract

The user-facing commands use an explicit history scope:

```text
jczl-flow history task <task-reference>
jczl-flow history initiative <initiative-reference>
jczl-flow history ini <initiative-reference>
jczl-flow history plan <initiative-reference>
```

Task references resolve full or unambiguous short UUIDs. Initiative references
resolve an exact project-scoped name, a full ID, or an unambiguous ID prefix of
at least eight characters. The `initiative`, `ini`, and `plan` forms are
aliases with identical behavior. A bare `jczl-flow history <reference>` is
invalid so the subject cannot be guessed.

History must:

- read only the selected project database;
- order events by `occurred_at`, `sequence`, and event ID;
- render short UUIDs while retaining full UUID identity internally;
- include the resolved initiative short ID in initiative history subjects;
- show event type, property, old/new values where available, and timestamp;
- return a quiet, successful no-history result when the subject exists but has
  no events;
- return an actionable error when the subject does not exist or a short ID is
  ambiguous;
- never mutate the task, cache, focus, or session state.

Example shape:

```text
$ jczl-flow history task 57c3fc80
HISTORY: task 57c3fc80
[2026-08-30T12:00:00Z] create
[2026-08-30T12:01:00Z] update description: "Draft" -> "Validated draft"
[2026-08-30T12:02:00Z] transition status: pending -> active

$ jczl-flow history initiative parity
HISTORY: initiative parity [id:91b2c3d4]
[2026-08-30T12:00:00Z] create name: <empty> -> "parity"
```

Initiative list commands expose the same short reference for follow-up
commands:

```text
$ jczl-flow plans --force
PROJECT: example
INITIATIVES:
- [ACTIVE] parity [id:91b2c3d4] pending:2 active:0 completed:0 blocked:1
```

History output is intentionally a presentation contract, not a promise to
reproduce Taskwarrior's table formatting byte-for-byte. Differential tests
compare normalized event facts, timestamps, properties, and transitions.

## TaskChampion migration source

The copied TaskChampion database established the following source boundary:

- current state is in `tasks(uuid, data)`;
- historical operations are in `operations(id, data, synced)`;
- observed operation variants are `Create`, `Update`, and `Delete`;
- `Update` payloads carry `uuid`, `property`, `old_value`, `value`, and
  `timestamp`;
- `Delete` payloads carry `uuid` and an `old_task` snapshot;
- operation IDs provide a stable source ordering;
- the observed database contains 33,421 operations and all observed operations
  are unsynced.

These tables are a forensic source format, not a flow runtime dependency.
The migration boundary should be a read-only, schema-gated exporter owned by
`jacazul-ai-cli` or its migration adapter:

```text
taskchampion.sqlite3
        ↓ read-only adapter
history snapshot JSON
        ↓ explicit source input
flow migration importer
        ↓ idempotent native events
task_history
```

The exporter must verify the TaskChampion schema/version before reading it and
fail closed when the format is unknown. It must not write to the source DB,
call a broker, or expose raw task values in logs. `taskp <uuid> history`
remains the behavior oracle for selected tasks; direct operation extraction is
used only when the structured history snapshot is required.

A portable history snapshot should contain only structured event fields:

```json
{
  "schema_version": 1,
  "source": "taskchampion",
  "operations": [
    {
      "source_event_id": "42",
      "task_uuid": "<full-uuid>",
      "event_type": "update",
      "property": "status",
      "old_value": "pending",
      "new_value": "completed",
      "sequence": 42,
      "occurred_at": "<normalized-timestamp>"
    }
  ]
}
```

The actual exporter must preserve values in the protected snapshot but must
redact them from diagnostics and reports unless the operator explicitly asks
for event details.

## Import rules

- Current exported task state remains authoritative for the materialized task.
- History events are imported independently and linked by full task UUID.
- A missing event target is a validation error, not a silently orphaned row.
- Reapplying the same snapshot inserts no duplicate history events.
- Source operation order is preserved even when timestamps are equal.
- Unknown operation variants are reported and block a production apply until
  the adapter has a deliberate mapping.
- Delete snapshots are retained as event data; they do not delete native task
  records during migration.
- Annotation changes may appear in history, but the annotations table remains
  the queryable structured-context source.
- History migration excludes persona, caches, credentials, and client files.

## Verification contract

The history slice is complete only when isolated tests prove:

1. native lifecycle writes append ordered events;
2. `jczl-flow history <uuid>` returns only the selected task's events;
3. a second import is idempotent by source event identity;
4. separate project databases cannot observe one another's events;
5. malformed, unknown, or orphaned source events fail with an actionable
   report;
6. normalized native events agree with selected `taskp <uuid> history` facts;
7. a failed import leaves the target history unchanged through transaction
   rollback.

The history feature must not claim complete legacy parity until the exporter
has covered the full TaskChampion operation vocabulary and the remaining
current v1 migration limitations are resolved.
