# flow Vision

## Mission

`flow` is the operational workflow engine for Jacazul agents.

Its job is to preserve and coordinate the work being done: initiatives,
plans, tasks, dependencies, focus, handoffs, decisions, outcomes, and session
state. It exists so multiple agents can continue the same operational thread
without losing ownership or context.

## What flow Is Not

`flow` is not the project's knowledge base.

Knowledge remains in the project repository and its documentation systems:

- source code;
- technical documentation;
- specifications;
- research artifacts;
- design records;
- project files.

`flow` manages operational memory about that work. It does not copy or
synchronize project knowledge by default.

## Local-First Direction

The current phase is a Go-native feature-parity port of the existing
`tw-flow` behavior. The local engine must become useful and reliable before
team orchestration is implemented.

The local workflow is coordinated by a `Coordinator` over one native SQLite
database per canonical `PROJECT_ID`. Initiatives/plans are first-class domain
entities with their own identity, lifecycle, metadata, and ticket relationship;
tasks reference them explicitly. Taskwarrior semantics remain a compatibility
target, but the Taskwarrior binary and its grouping model are not the local
source of truth.

Existing Taskwarrior project groups may be imported as initiative records. A
Taskwarrior adapter can preserve interoperability without forcing the workflow
engine to depend on Taskwarrior at runtime.

## Team Collaboration

The longer-term goal is collaboration between multiple agents working on the
same workflow:

```text
Agent A ─┐
Agent B ─┼── Team Coordinator ── Shared TaskBackend
Agent C ─┘
```

A team Coordinator will eventually provide:

- shared initiative and plan state;
- task ownership and assignment;
- leases to prevent conflicting execution;
- handoffs between agents;
- revision and conflict management;
- audit history;
- authentication and permissions.

The local Coordinator remains the agent-side workflow boundary. The team
Coordinator becomes the shared orchestration boundary. Both coordinate
operational work; neither replaces the project's knowledge repository.

## Initiative Transport

The first collaboration primitive is explicit initiative transport:

```text
jczl-flow send <ini> --to <target>
jczl-flow receive <ini>
jczl-flow sync <ini>
```

`send` and `receive` establish a portable workflow handoff. `sync` reconciles
subsequent changes and therefore requires stable identity, revisions, ownership
and conflict policy.

An initiative exchange carries workflow state:

- initiative identity;
- tasks and dependencies;
- status and ownership;
- decisions, outcomes, and handoffs;
- external ticket references;
- source and revision metadata.

It does not carry source code, documentation, cache data, or private session
state unless a later protocol explicitly opts into those fields.

## Evolution Path

1. Establish verified feature parity with `tw-flow`.
2. Implement a clean local Coordinator over the per-project SQLite store.
3. Define a portable initiative envelope and explicit send/receive flow.
4. Add a shared Team Coordinator and server-backed TaskBackend.
5. Add bidirectional synchronization, leases, conflict resolution, audit, and
   team permissions.
