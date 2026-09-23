# Agent Output Formats

## Why this exists

The expensive thing is not a verbose response. It is a second call.

Verbosity costs tokens linearly and modestly. An agent that calls `status`
twice because the first output was not enough to decide, or re-runs `ponder`
because it lost the thread, costs a full round trip: a tool call, a response,
and the context to hold both. The engine already spends its design budget on
killing round trips rather than on saving characters, and this contract is
shaped by the same rule:

- the `[cached]` signal says "do not spend context, you already have this";
- `onboard` is one call in place of focus, session resume, context and status;
- `context <uuid>` hands over inherited decisions so nothing is reinvestigated.

**The test of a report format is whether it is complete enough to decide on
first read.** Which serialization parses best is the second question.

## The formats

| Format | Role |
|---|---|
| `text` | Default. Concise, readable by a person and by an agent holding it in context. |
| `json` | One complete, versioned document. For a program that parses a whole result. |
| `jsonl` | One meta line, then one line per record, each independently valid. For streaming and for line-oriented tooling. |
| `xml` | Tagged structure, for agents that read delimited sections more reliably. |

`text` stays the default for every command. The other three are opt-in and
exist only on the report commands listed below.

Whether `xml` measurably improves an agent's reading fidelity over `text` is
**not established**. The recommendation to use XML tags is well founded for
*structuring a prompt you write*; carrying it over to *serializing records an
agent reads* is an extrapolation, not a measured result. `xml` is in this
contract because the operator chose to offer it, and because under the
selection model below a fourth format costs one renderer rather than a change
to any command. Settling the question needs a measurement, not an argument.

## Selecting a format

Resolution order, matching how `config.Resolve` already handles `ProjectID`,
`SessionID` and `DatabasePath`:

1. the `--format` flag on the command;
2. the value injected in `flow.Env`;
3. `text`.

### The engine never reads the environment for this

`flow.Run` does not read the process environment. That boundary is enforced by
contract test, and a format preference does not get an exception:

- `flow.Env` carries a `Format` field;
- `EnvFromOS` reads the environment variable, because it is the one function
  that exists to do so, and only standalone `jczl-flow` uses it;
- `jacazul` passes the value it already resolved from its own configuration.

A launcher-owned configuration file is likewise not parsed here. Bootstrap and
configuration resolution belong to `jacazul-ai-cli` under the Explicit
Non-Goals in `AGENTS.md`; the launcher resolves the file and injects the
result.

### Per-model and per-harness policy belongs to the launcher

The engine does not know which model or harness is running, and must not learn.
A branch on model identity inside `internal/cli` is launcher policy leaking
into the workflow core.

> The launcher decides which format. The engine implements formats.

Per-model, and later per-harness-and-model, selection tables grow in
`jacazul-ai-cli`. The engine gains only another valid value in `flow.Env`, so
that growth costs nothing here.

**Open question:** a per-project default format is arguably workflow state
rather than launcher configuration, and the project database already holds
project state. Whether the engine should store one is unresolved.

## The envelope

Every structured format carries the same three parts: a version, a `meta`
block, and the records.

```json
{
  "format_version": 1,
  "command": "status",
  "meta": {
    "project_id": "jacazul-ai_flow",
    "session_id": "6f4068ec",
    "generated_at": "2026-09-23T14:02:11Z",
    "cached": false
  },
  "records": []
}
```

`jsonl` emits the same content as `{"format_version":1,"command":...,"meta":{...}}`
on the first line and one record object per line after it. Every line is valid
JSON on its own. `xml` carries the same fields under a root element, with
values as child elements rather than attributes.

### The cache fact is a field, not prose

`🐊 [cached] Status unchanged since 12s ago. Use --force to refresh.` is
terminal prose, and structured stdout carries no prose. The *fact* it reports
is the opposite of noise: it is the instruction that saves the round trip.

A cached structured response therefore sets `meta.cached` to `true` and
`meta.unchanged_since` to the age of the entry, and carries the cached records.
The consumer reads `cached` and decides whether to reuse what it already holds
rather than re-reading the payload.

Dropping the signal in structured mode would discard the saving this whole
contract is built around.

## Report commands

`--format` exists on these and nowhere else:

| Command | Notes |
|---|---|
| `status` | |
| `ponder` | |
| `plans` | with the `inis`, `initiatives` and `ini` aliases |
| `tree` | |
| `next` | |
| `active`, `blocked`, `overdue` | |
| `history` | |
| `context` | |
| `notes` | |
| `focus` | the show form only, not the anchoring forms |
| `session list` | not `dump`, `ack` or `purge` |
| `roadmap show` | not `init`, `add` or `ship` |
| `cache info` | not `cache clear` |
| `onboard` | the composite briefing |

Everything else is a state change or a human-facing briefing. A command that
creates, transitions, annotates or closes work reports what changed in `text`
and takes no `--format`. `help` renders guidance, not workflow data, and
`commit` renders a draft message that is itself the artifact.

## Rules

- Structured stdout contains no banner, no cache prose, no tip, no terminal
  decoration, and no colour.
- Workflow errors stay on stderr in `text` with their `ACTION:` guidance,
  whatever `--format` is set to. A structured error contract is a separate
  decision and is not adopted here.
- Exit status is unchanged by format.
- `format_version` increments when a field changes meaning or is removed;
  adding a field does not increment it.
- A record's field set is the same across `json`, `jsonl` and `xml`. The
  formats differ in framing, never in content.

## Open questions

- Does `xml` measurably beat `text` for agent reading fidelity? Unmeasured.
- Should a per-project default format live in the project database?
- Should structured output ever be emitted for state-changing commands, for a
  program driving the engine rather than an agent reading it?
