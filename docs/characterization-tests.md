# Characterization Tests

A characterization test pins what the code does today so that a refactor can
change how it does it. The method comes from Michael Feathers, *Working
Effectively with Legacy Code*, where it is also called a golden master.

## When to use it

Use it when a change restructures existing behavior instead of adding new
behavior: a different query pattern, an extracted function, code moved
between packages, a cache introduced on a hot path.

The Red-Green-Refactor loop in [feature-parity.md](feature-parity.md) starts
with a failing test for missing behavior. A refactor has no valid Red step:
the current output is already correct, so a test written against it is born
green. A characterization test replaces the Red step with a pin, and the
mutation check becomes the only proof that the pin works.

Do not use it for new behavior. A golden recorded from new code pins whatever
the code happens to do, bugs included.

## Procedure

1. **Build the fixture through the real boundary.** Populate an isolated
   database in `t.TempDir()` with `flow.Run`, the way an agent would. Never
   write rows directly and never touch the real `TASKDATA`. Shape the fixture
   around what the refactor reaches: if it changes dependency loading, use
   chains; if it changes ticket resolution, place tickets at the head, in the
   middle, and nowhere.
2. **Use the exact output as the oracle.** Compare the full stdout of every
   command the refactor reaches, byte for byte. `strings.Contains` lets a
   wrong count, a swapped order, or a lost line pass.
3. **Normalize only what legitimately varies.** Generated UUIDs become stable
   placeholders such as `<A2>` or `<alpha>`. Never normalize anything the
   refactor could change: order, counts, spacing, or which ticket is shown.
4. **Prove the pin bites.** Break, one at a time, each rule the refactor
   touches, and confirm the pin fails. Then run the same mutations against
   the suite without the pin. The pin earns its place through what only it
   catches. Record both results on the task.
5. **Refactor with the pin frozen.** The golden does not change during the
   refactor. If it must change, the behavior changed: stop, move that change
   to its own task, and record a `DECISION` before updating the golden.
6. **Keep the pin afterwards.** Once the refactor lands, it stays as a
   regression test. A structural guard, such as a query count, may be added
   next to it but does not replace it.

## Determinism

A pin that flakes is worse than no pin, because agents learn to rerun it.
Before trusting a golden, check it for:

- ordering ties broken by a random value, such as a UUID;
- map iteration order reaching the output;
- values derived from the current time, such as "done today" counters,
  which change when a run crosses midnight;
- timestamps compared as strings in a format that trims trailing zeros.

Run the pin several times in a row before relying on it.

## Reference

`report_pin_contract_test.go` pins `status`, `plans`, `ponder` and `next`
before the query-count refactor of
[#13](https://github.com/jacazul-ai/flow/issues/13). Its mutation record: of
four mutations, the existing suite already caught three; only the pin caught
nearest-ancestor ticket inheritance. The fixture generator moves into
`internal/testharness` under
[#14](https://github.com/jacazul-ai/flow/issues/14) so later pins and the
benchmark fixture share it.
