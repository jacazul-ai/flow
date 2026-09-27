# Silos

A silo is a project database together with the workflow data inside it:
initiatives, chained tasks, tickets, and task states. The database alone is
empty; the data is what makes a silo useful for a test, a benchmark, or a
manual inspection. See [#14](https://github.com/jacazul-ai/flow/issues/14).

Silos are generated, never hand-written. Every row comes from `flow.Run`,
the same boundary an agent uses, so a silo holds only states the engine
itself can produce.

## `silo.Generate`

`internal/silo` describes a silo declaratively:

```go
ids, err := silo.Generate(ctx, env, []silo.Chain{
	{Name: "alpha", Tasks: []string{"A1", "A2", "A3", "A4"},
		Tickets: map[int]string{0: "#ROOT-1", 2: "#MID-3"}, Completed: 1},
	{Name: "gamma", Tasks: []string{"G1", "G2", "G3"},
		Tickets: map[int]string{1: "#MID-2"}, Active: true},
})
```

| Field | Meaning |
|---|---|
| `Name` | Initiative name. |
| `Tasks` | Task descriptions in chain order; each task depends on the previous one. Descriptions are unique across the silo. |
| `Tickets` | Task index, from 0, mapped to the external ticket linked to it. |
| `Completed` | Leading tasks completed with an `OUTCOME`. |
| `Active` | Starts the first task after the completed ones. |

`env` selects the project, session and database, as for `flow.Run`.
`Generate` validates the whole description before its first write, so an
invalid description leaves no database behind. It returns each task's short
UUID keyed by description; tests use that map to replace generated UUIDs
with stable placeholders.

The report pin in `report_pin_contract_test.go` is the first consumer; see
[characterization-tests.md](characterization-tests.md).

## `jczl-silo`

`jczl-silo` is a development tool built from `cmd/jczl-silo`. It is not part
of the engine, has no workflow commands, and is not built by `make build`
or shipped with releases. It generates uniform chains into a new database:

```bash
go run ./cmd/jczl-silo --database /tmp/bench/flow.sqlite3 \
  --chains 20 --tasks 20 --completed 8
```

| Flag | Default | Meaning |
|---|---|---|
| `--database` | none | New database file to create. Required. |
| `--project` | `silo` | Project ID inside the database. |
| `--session` | `silo` | Session ID used while generating. |
| `--chains` | `3` | Initiatives, each a chain of dependent tasks. |
| `--tasks` | `5` | Tasks per chain. |
| `--completed` | `0` | Leading tasks of each chain completed with an `OUTCOME`. |
| `--active` | off | Starts the first task after the completed ones in each chain. |
| `--ticket` | none | `POSITION=TICKET`, positions from 1, linked in every chain. Repeatable. |

Chains are named `chain-1`, `chain-2`, and so on, and tasks
`chain-1 task 1`; numbers are zero-padded to the width of the larger count.
On success, stdout names the database and the command to inspect it:

```bash
PROJECT_ID=silo JACAZUL_FLOW_DATABASE_PATH=/tmp/bench/flow.sqlite3 jczl-flow ponder --force
```

### Safety

`jczl-silo` cannot write into real workflow data:

- `--database` is required; there is no fallback to the default project
  database.
- An existing path is refused, whatever it holds, so the tool only creates
  new databases.
- Nothing is written outside the database file.

Errors go to stderr with an `ACTION:` line and exit status 1; invalid flags
exit with status 2. If generation fails after the database was created, the
partial file stays and the next run with the same path is refused; pass a
new path or delete the file.
