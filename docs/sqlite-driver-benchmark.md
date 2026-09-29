# SQLite Driver Benchmark

Spike for [#12](https://github.com/jacazul-ai/flow/issues/12), run on
2026-09-23 against commit `f71aa9e`.

## Question

`jczl-flow` uses the pure-Go `modernc.org/sqlite` driver and builds with
`CGO_ENABLED=0`. Is it measurably slower than the cgo driver
`github.com/mattn/go-sqlite3` for the workload this engine actually runs,
and is the difference large enough to give up static, toolchain-free builds?

Decision rule agreed before measuring: if the per-command wall-time
difference is under about 10 ms, keep the pure-Go driver; otherwise record
the numbers and take the question to a separate design decision.

## Verdict

**Keep `modernc.org/sqlite`. The query pattern was the first bottleneck, and
it is now bounded.**

- The baseline cgo driver was 1.5x to 2.3x faster in process (geomean -45%),
  but uncached dashboards amplified that per-query difference.
- The post-fix rerun reduced the 400-task `status --force` median from
  1335.71 ms to 50.45 ms on `modernc`, with one task-list query and one
  dependency query instead of 4600 dependency queries.
- Across `status`, `plans`, `ponder`, and `next`, the post-fix modernc versus
  mattn median delta stayed below 10 ms on every path. The driver question is
  closed for this workload; static, cgo-free builds still win operationally.
- Against the reference `tw-flow` on a fixture of the same shape,
  `jczl-flow` remains 2x to 173x faster per command. The query-bound fix
  removes the Go port's quadratic ticket-resolution artifact.

The benchmark records the driver result rather than silently discarding it:
keep `modernc.org/sqlite` and revisit the choice only with a new workload or
an explicit driver decision.

## Environment

| Item | Value |
|---|---|
| CPU | AMD Ryzen 5 1600, 12 threads |
| OS | Linux 7.2.6 (Fedora 44), x86-64 |
| Go | 1.27.0 |
| `modernc.org/sqlite` | v1.54.0, `CGO_ENABLED=0`, static binary, 13.3 MB |
| `mattn/go-sqlite3` | v1.14.52, `CGO_ENABLED=1`, dynamic binary, 11.2 MB |
| Load average | about 1.7 to 2.0 during the runs; the machine was not idle |

Both binaries were built from the same commit. The cgo variant lived in a
disposable worktree that changed only the driver import and the
`sql.Open` driver name (`sqlite` to `sqlite3`); it never touched `master`.
`go version -m` confirmed each binary links only its own driver. The
pragmas are issued with `Exec`, so both drivers run the same configuration:
`busy_timeout`, `foreign_keys` and WAL journal mode.

## Fixture

An isolated database in the session sandbox, never the real `TASKDATA`,
built through the real CLI:

- 20 initiatives with 20 chained tasks each: 400 tasks;
- two notes per task (`DECISION`, `RESEARCH`): 800 annotations;
- the first 8 tasks of every initiative completed with an `OUTCOME`:
  160 completed, 240 pending, 960 annotations in total.

Both builds rendered byte-identical output for `status`, `plans`, `next`,
`ponder --force` and `context` on this fixture, so the comparison measures
the same work.

## Baseline end-to-end wall time (before query-bound changes)

Each command ran as a real process with a controlled environment
(`PROJECT_ID`, `JACAZUL_SESSION_ID`, `JACAZUL_FLOW_DATABASE_PATH`,
`JACAZUL_HOME` and `HOME` all pointing into the sandbox). The two builds
were interleaved on every iteration so machine drift affected both evenly.
Each command had 5 warm-up runs and 100 measured runs. `context` and `note`
target a pending task in the middle of a chain (initiative 10, task 12).

| Command | modernc median | mattn median | Delta | modernc p95 | mattn p95 |
|---|---:|---:|---:|---:|---:|
| `help` | 3.34 ms | 3.20 ms | +0.14 ms | 3.92 ms | 3.68 ms |
| `status` (cached) | 5.20 ms | 4.27 ms | +0.93 ms | 10.59 ms | 6.54 ms |
| `ponder` (cached) | 5.21 ms | 4.29 ms | +0.92 ms | 6.45 ms | 5.35 ms |
| `note` (write) | 31.65 ms | 27.35 ms | +4.30 ms | 40.75 ms | 37.11 ms |
| `context` | 16.80 ms | 9.74 ms | +7.06 ms | 27.26 ms | 22.24 ms |
| `next` | 27.28 ms | 17.30 ms | +9.98 ms | 45.05 ms | 30.14 ms |
| `plans --force` | 81.36 ms | 62.47 ms | +18.89 ms | 113.54 ms | 79.34 ms |
| `ponder --force` | 157.09 ms | 119.87 ms | +37.22 ms | 203.18 ms | 151.39 ms |
| `status --force` | 1335.71 ms | 707.79 ms | +627.92 ms | 1404.48 ms | 756.77 ms |

`help` does not open the database, so its 0.14 ms is the startup cost of
the larger static binary. The cached rows show the fixed cost of opening the
database, applying the pragmas and checking migrations: under 1 ms.

A first run measured `status` and `plans` without `--force` and reported
about 5 ms for both. Those numbers were the cache path, not the query path.
The table above separates the two explicitly.

## Post-fix end-to-end wall time

The query-bound rerun was performed on 2026-09-29 at commit `200f125`,
using the same 20-initiative, 400-task fixture. The modernc and mattn
binaries were interleaved for 100 measured runs after five warm-ups. Timing
used a nanosecond wall clock around each isolated process invocation.

| Command | modernc median | mattn median | Delta | modernc p95 | mattn p95 |
|---|---:|---:|---:|---:|---:|
| `status --force` | 50.45 ms | 46.38 ms | +4.07 ms | 61.39 ms | 54.29 ms |
| `plans --force` | 51.16 ms | 49.29 ms | +1.87 ms | 57.42 ms | 53.62 ms |
| `ponder --force` | 52.02 ms | 49.57 ms | +2.45 ms | 79.62 ms | 76.78 ms |
| `next` | 13.29 ms | 10.05 ms | +3.24 ms | 23.42 ms | 20.35 ms |

Outputs were byte-identical across both drivers for every command.

## Baseline in-process store cost

`go test -bench` against the same fixture, copied into a temporary
directory per benchmark, `-benchmem -count 10`, compared with `benchstat`.
Every difference is significant at `p=0.000`.

| Benchmark | modernc | mattn | Change |
|---|---:|---:|---:|
| `Open` (configure + migrate) | 1119.6 µs | 605.8 µs | -45.89% |
| `PointQuery` (one indexed `COUNT`) | 31.80 µs | 18.31 µs | -42.43% |
| `GetTask` | 340.3 µs | 146.2 µs | -57.03% |
| `ListTasks` (all 400) | 20.87 ms | 14.01 ms | -32.87% |
| `ListInitiatives` (ponder core) | 47.71 ms | 32.78 ms | -31.29% |
| `InheritedAnnotations` (context) | 9.731 ms | 4.734 ms | -51.35% |
| `AddAnnotation` | 372.0 µs | 194.6 µs | -47.70% |
| geomean | 1.715 ms | 947.6 µs | -44.75% |

Allocations were within about 10% of each other; neither driver wins on
memory in a way that matters here.

## Post-fix in-process store cost

The in-process rerun used `go test -benchmem -count=10` and `benchstat` on
copies of the same fixture. The benchmark isolates store calls after opening
the database.

| Benchmark | modernc | mattn | Change |
|---|---:|---:|---:|
| `ListInitiatives` (fixture) | 9.695 ms | 5.649 ms | -41.73% |
| `ListTasks` (fixture) | 7.807 ms | 4.707 ms | -39.71% |
| geomean | 8.700 ms | 5.157 ms | -40.73% |

Allocations remained effectively equal: 1.392 MiB versus 1.392 MiB for
`ListInitiatives`, and 725.9 KiB versus 726.2 KiB for `ListTasks`.

## Baseline query count

A throwaway build counted calls to the per-task dependency loader
(`Store.dependencies`) and to `Store.ListTasks` for one uncached run of each
command on the 400-task fixture:

| Command | `ListTasks` calls | Dependency queries |
|---|---:|---:|
| `status --force` | 1 | 4600 |
| `ponder --force` | 81 | 2000 |
| `plans --force` | 40 | 800 |
| `next` | 1 | 400 |
| `context` | 0 | 25 |

`ListTasks` loads the task rows in one query, then issues one more query
per task for its dependencies. `ListInitiatives` calls `ListTasks` twice
per initiative, once directly and once through `ReadyTasks`. `status`
reaches 4600 dependency queries through the ticket lookup explained in the
next section.

The dependency queries alone cost about 32 µs each on `modernc` and 18 µs on
`mattn`. That per-query gap, multiplied by the query count, is the
end-to-end gap. The count grows with the number of tasks, so the gap grows
with the project. A driver change shrinks the multiplier; bounding the
query count removes it.

## Baseline `status --force` cost

`status` resolves the ticket for every listed task through
`Store.FindExternalTicket`. When a task has no ticket, the lookup climbs
its dependency chain with one `GetTask` per ancestor until it finds one or
reaches the root. The fixture has no tickets, so every task climbs to the
root, and each task repeats the climb its predecessor just made:

- pending tasks sit at positions 9 to 20 of a 20-task chain: 9 + 10 + ... +
  20 = 174 lookups per initiative;
- completed tasks sit at positions 1 to 8: 1 + 2 + ... + 8 = 36 lookups per
  initiative;
- 210 lookups per initiative across 20 initiatives is 4200 `GetTask` calls.
  Each loads its own dependencies, which together with the 400 from
  `ListTasks` gives exactly the 4200 + 400 = 4600 dependency queries counted
  above.

The cost grows with the square of the chain length, not linearly. At
340 µs per `GetTask` on `modernc` and 146 µs on `mattn`, 4200 lookups
account for about 1.4 s and 0.6 s, which matches the measured 1.34 s and
0.71 s. This is the worst case: in a real project a ticket usually sits
near the head of the chain and the climb stops there. The repetition is
real in every case.

## Post-fix query count and status path

A disposable instrumented build counted store calls for one uncached run on
the 400-task fixture. Both drivers produced the same counts:

| Command | `ListTasks` calls | Dependency queries | `GetTask` calls |
|---|---:|---:|---:|
| `status --force` | 1 | 1 | 0 |
| `plans --force` | 1 | 1 | 0 |
| `ponder --force` | 1 | 1 | 0 |
| `next` | 1 | 1 | 0 |

`status` now resolves inherited tickets from the loaded task graph with a
memoized depth-first traversal. The prior per-task `GetTask` climb is gone;
its old 4600 dependency-query count is retained above as the historical
baseline.

## Comparison with the reference `tw-flow`

To put the numbers in context, the reference Python engine was measured on
a fixture of the same shape: 20 initiatives, 400 chained tasks, 800 notes,
160 completed with an outcome, built through `tw-flow` itself. It ran in an
isolated Taskwarrior sandbox using the isolation recipe of the reference
test suite (`TASKDATA`, `TASKRC`, `JACAZUL_HOME` and `PROJECT_ID` pointed at
the sandbox, `JACAZUL_SESSION_ID` unset). `jczl-flow` used the `modernc`
build. The engines were interleaved on every iteration: 2 warm-up runs and
20 measured runs per command.

| Command | `jczl-flow` median | `tw-flow` median | Ratio |
|---|---:|---:|---:|
| `help` | 7.5 ms | 96.9 ms | 13x |
| `plans --force` | 73.5 ms | 161.0 ms | 2.2x |
| `ponder --force` | 173.9 ms | 583.5 ms | 3.4x |
| `context` | 24.5 ms | 288.4 ms | 12x |
| `note` (write) | 34.3 ms | 435.8 ms | 13x |
| `next` | 33.4 ms | 1544.1 ms | 46x |
| `status --force` | 1354.5 ms | 233662.4 ms | 173x |

The `tw-flow status --force` median is not a typo. A separate single run
took 243 s wall time and rendered a valid list of 240 pending and 160
completed tasks. The reference `find_ticket` in
`jacazul/cli/flow.py` implements the same climb, and there each step is a
`task export` subprocess: 4200 subprocesses at about 58 ms each is about
243 s. The Go port inherited the algorithm; it is 173 times faster only
because a SQLite lookup is cheaper than starting a process.

Two conclusions follow. The migration to Go already removes most of the
cost the agents pay today, with either driver. And the quadratic ticket
climb is a reference-behavior artifact worth removing in both engines, not
a property of the storage layer.

## Recommendation

1. Keep `modernc.org/sqlite` and `CGO_ENABLED=0`. Static binaries and
   cross-compilation for every launcher without a C toolchain are worth
   more than the measured sub-10 ms end-to-end driver gap.
2. The query-bound work is complete: dependencies are loaded in one query,
   task snapshots are reused across dashboard sections, and inherited
   tickets resolve once per task graph.
3. The driver question is closed for this fixture and workload. Reopen it
   only for a new measured workload or an explicit architecture decision.
4. If a driver change is ever reconsidered, measure
   `github.com/ncruces/go-sqlite3` as well. It runs real SQLite compiled to
   WebAssembly on `wazero` and also needs no cgo; it was not measured here.

## Reproducing

The harness is not in the repository; it was disposable spike code. To
reproduce:

1. Build both binaries from the same commit: the normal `make build`, and a
   copy of the tree with the driver import swapped to
   `github.com/mattn/go-sqlite3`, the `sql.Open` driver name changed to
   `sqlite3`, built with `CGO_ENABLED=1`.
2. Build the fixture described above with the CLI, pointing
   `JACAZUL_FLOW_DATABASE_PATH` and `JACAZUL_HOME` at a sandbox directory,
   never at real runtime data.
3. Time each command as a subprocess, interleaving the two builds, with
   warm-up runs discarded. Use `--force` for every command that has an
   output cache, or the result measures the cache.
4. For the in-process numbers, add `testing.B` benchmarks in
   `internal/storage/sqlite` that open a copy of the fixture, and compare
   with `benchstat` at `-count 10`.
