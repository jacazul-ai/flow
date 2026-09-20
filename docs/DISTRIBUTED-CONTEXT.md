# flow Distributed Context

## Status

Design direction. This document describes the target distributed architecture;
it is not an implementation guarantee yet.

## Mission

`flow` is the operational context system for agents. It is not only a local
task organizer and it is not only a ticket API. It preserves and coordinates
the context required to continue work across agents, machines, sessions, and
reconnections.

The distributed context includes:

- tickets and work items;
- initiatives, plans, and tasks;
- dependencies and readiness;
- decisions, research, lessons, questions, and outcomes;
- handoffs and ownership;
- agent capabilities and assignments;
- execution leases and heartbeats;
- revisions, audit events, and synchronization cursors.

Taskwarrior is not part of the target architecture or the source of truth. Any
migration or compatibility adapter in the current codebase is transitional.
The flow domain and its native persistence own the operational state.

## Roles

The roles are relative to a connection, not permanent identities of a process.
A local Bastion is a Bastion for its local Peasants and a Peasant when it
connects upstream to the central Bastion.

```text
Worker Peasants
        |
        v
Local Bastion
  local authority, firewall, scheduler, pool
        |
        | authenticated sync stream
        v
Central Bastion / flow-server
  global authority, distributed context, API, Web UI
```

### Worker Peasant

A Peasant executes work assigned by the local Bastion. It should not need to
know the public server address, external ticket providers, or the global
synchronization protocol.

### Local Bastion

The local Bastion is the source of authority for the local runtime. It:

- authenticates and authorizes local Peasants;
- owns the local scheduler and worker pool;
- enforces local project and capability policy;
- keeps local context available during upstream outages;
- maintains an outbox of local events and an inbox of remote events;
- connects upstream instead of exposing Peasants directly to the internet;
- reports execution state and results to the central Bastion.

### Central Bastion / `flow-server`

The central Bastion is the source of truth for shared, cross-machine context.
It:

- owns the global ticket and work-item registry;
- coordinates projects, plans, assignments, and leases across Bastions;
- orders accepted events and assigns a server sequence;
- provides authentication, authorization, audit, API, and Web UI;
- serves synchronization history after reconnect;
- integrates with external systems such as GitHub and Jira.

The central Bastion sends work and intent to a local Bastion. It does not
bypass the local Bastion to command a Worker Peasant directly.

## Authority Boundaries

There must not be two authorities for the same mutable field. The initial
boundary is:

| Concern | Authority |
|---|---|
| Global tickets and work-item identity | Central Bastion |
| Project membership and cross-machine assignment | Central Bastion |
| Local agent admission and capabilities | Local Bastion |
| Local scheduling and execution policy | Local Bastion |
| Worker heartbeat and process state | Local Bastion |
| Accepted global event ordering | Central Bastion |
| Final local application of a command | Local Bastion |
| Execution result | Local Bastion reports; central Bastion records |

The central Bastion owns shared intent and history. The local Bastion owns
whether and how that intent is executed locally.

## Distributed Context Model

`flow` context is an event-backed graph rather than a collection of independent
copies. A context projection is built from the canonical event history and the
local events that have not yet been accepted upstream.

Core entities include:

```text
Project
  Initiative / Plan
    Task / Ticket
      Dependency
      Annotation
      Assignment / Lease

Agent
Bastion
Session
Event
ExternalReference
```

Every task and ticket has a stable flow UUID. A human-readable key such as
`JF-123` is presentation data and must not replace the UUID.

Annotations remain structured operational context. Examples include
`DECISION`, `RESEARCH`, `QUESTION`, `HYPOTHESIS`, `LESSON`, `HANDOFF`, and
`OUTCOME`. They are durable events, not transient chat messages.

## Event and Synchronization Contract

A mutation is represented as an event. The event must be safe to deliver more
than once and safe to resume after a connection failure.

```text
event_id
origin_bastion
project_id
aggregate_type
aggregate_id
operation
base_revision
local_sequence
server_sequence
payload
signature
```

The server assigns `server_sequence` only after validating and persisting the
event. A local Bastion tracks:

- an outbox cursor for events waiting to be accepted;
- an inbox cursor for events applied from the server;
- an idempotency set keyed by `event_id`;
- a revision for each mutable aggregate.

The delivery contract is at-least-once. Exactly-once behavior is achieved at
the domain boundary by idempotency, not by assuming that a network connection
cannot duplicate a message.

### Synchronization lifecycle

```text
CONNECT
  REGISTER
  AUTHENTICATE
  SUBSCRIBE
  CATCH_UP from the last server cursor
  FLUSH local outbox
  LIVE event stream

DISCONNECT
  preserve outbox and cursors

RECONNECT
  authenticate again
  request events after the last cursor
  retry unacknowledged events by event_id
  return to LIVE
```

A long-lived bidirectional gRPC stream is the preferred central transport:

```text
Local Bastion (protocol Peasant)
        <── gRPC stream ──>
Central Bastion (protocol Bastion)
```

The stream carries typed messages such as:

```text
REGISTER
HEARTBEAT
SYNC_REQUEST
EVENT
ACK
ASSIGNMENT
LEASE_RENEWAL
RESULT
CONFLICT
HANDOFF
```

The transport does not define domain authority. It carries the synchronization
contract; the flow server and local Bastion enforce their respective
boundaries.

## Conflict and Lease Rules

Annotations and handoffs are append-oriented events. They should not be merged
by silently overwriting an earlier event.

Mutable fields use optimistic concurrency:

```text
client sends base_revision = 41
server aggregate revision = 42
server returns CONFLICT
```

Work execution uses leases:

```text
central assigns work
local Bastion accepts lease
scheduler gives work to a Peasant
local Bastion renews lease while active
result or failure closes the lease
expired lease becomes eligible for reassignment
```

A stale result must not overwrite a newer assignment. Every assignment and
result carries an `event_id` and `lease_id`.

## Tickets and External Systems

`flow` is the canonical ticket system. GitHub, Jira, and other providers are
external projections or integrations, not authorities for the agent context.

```text
flow event log
      |
      +--> GitHub connector
      +--> Jira connector
      +--> other connector
```

A ticket may retain external references:

```text
flow_ticket_uuid:  <stable UUID>
external_references:
  - provider: github
    project: org/repository
    key: "#52"
  - provider: jira
    project: PROJ
    key: "PROJ-847"
```

Connectors use an outbox and webhook ingestion:

```text
flow mutation
  -> durable outbox
  -> connector retry
  -> external provider

External webhook
  -> signature validation
  -> normalized event
  -> flow event log
```

Agent operation must not depend on GitHub or Jira availability. External
synchronization may be delayed without blocking local execution.

## Transport and Discovery

Discovery and communication are separate concerns:

```text
mDNS/UDP or configured server address
  -> locate a Bastion endpoint

HTTP/gopeasant or gRPC
  -> directory, nonce, and application requests

Cryptozoid
  -> identity, ECDH, authenticated envelopes, and key handling
```

The open `gopeasant` contract remains useful for directory and nonce-based
client-to-Bastion communication. flow-specific synchronization may use a
Protobuf/gRPC binding without changing the domain model.

Recommended defaults:

- Peasant to local Bastion: gRPC over a Unix socket when processes are local;
- local Bastion to central Bastion: gRPC over TCP with an authenticated secure
  channel;
- LAN discovery: mDNS or UDP multicast only for endpoint discovery;
- application data: never rely on unauthenticated UDP delivery.

The protocol nonce issued by a Bastion is distinct from the AES-GCM nonce
inside a Cryptozoid envelope. The protocol nonce prevents request replay; the
AEAD nonce provides cryptographic uniqueness for encryption.

## Security Boundary

A local network or external ticket provider is not automatically trusted.

- Discovery data is only a candidate endpoint and may be spoofed.
- The Cryptozoid handshake must authenticate the peer key or verify a pinned
  trust record.
- Authentication proves peer identity; authorization decides project and
  operation access.
- Private keys stay with the identity owner and never enter tickets, events,
  logs, or discovery packets.
- Remote requests cannot invoke arbitrary local commands.
- Nonces have bounded lifetime and one-use consumption.
- Event IDs, leases, and revisions protect against duplicate or stale work.
- External webhook signatures and provider credentials are validated and kept
  in the server-side secret boundary.

The local Bastion is a security boundary. Compromising it affects the Peasants
under its control, so its permissions and exposed API must remain minimal.

## Deployment Boundaries

The product can be split into independently deployable components while
sharing stable domain and protocol packages:

```text
flow
  local engine and CLI

flow-bastion
  local daemon, scheduler, firewall boundary, and sync client

flow-server
  central Bastion, API, event store, connectors, and Web UI

cryptozoid
  cryptographic primitives and authenticated envelopes

gopeasant
  open client-to-Bastion protocol
```

The current `flow` repository should keep the local engine independent from
server deployment details. `flow-bastion` may begin as a separate command or
package while its contracts stabilize. `flow-server` and its Web UI are a
separate server product that consumes stable flow domain and synchronization
contracts.

## Evolution Path

1. Finish and verify the native local flow context model.
2. Define event identities, revisions, cursors, leases, and conflict behavior.
3. Implement a local Bastion with a local Peasant pool.
4. Implement the central Bastion and durable event log.
5. Add gRPC synchronization with reconnect and catch-up.
6. Add the Web UI and server API over the same canonical context.
7. Add GitHub, Jira, and other external connectors.
8. Add optional direct peer communication without bypassing Bastion authority.
