# Move ship movement to the end of the turn

## Context

**Status: closed on 2026-08-27.** All three parts are done, in the three commits
this plan called for: ring draws addressed through `internal/prng`, the scripted
game lengthened by a turn, and ship movement moved to the last stage. The
burndown's High item is closed against it. What follows is the record.

Round 2 of EAGLE-01 found that a one-turn crossing costs a ship nothing: the
engine lands a jump on `departure + ceil(d/t) - 1` and arrival is phase 29 of
39, so the shortest crossing is written and consumed inside one turn. That is
the High item in `docs/plan/engine-burndown.md`.

The fix is the one later editions of the game used: **ship movement becomes the
last stage of the turn** — move, then jump departures, then jump arrivals.
Every other order of turn N resolves with the ship still where it began; it
leaves at the end of the turn; it lands at the very end of turn N+T, when there
is nothing left to process. The "freeze time and resume mid-turn" problem never
arises, because nothing follows the arrival.

Two things fall out of putting the whole stage last rather than only the jump:

- **Rebellion gets to stop a ship leaving.** Stages 18 and 19 are rebellion and
  rebel increases; with movement at 15 they resolve after a ship has already
  moved. Last, and they resolve first. Neither is built yet, so this decides
  where they land when they are.
- **Stages 16–22 see every ship where it started the turn**, which is what
  "a turn's movement is what the *next* turn's reports describe" has always
  meant.

There is no half-order to keep anywhere. A jump is whole at departure
(`jumpBound.Apply`, `internal/orders/verbs.go:668`): it draws the fuel, writes
an `in_transit` row, nulls the location and returns `succeeded`. `in_transit`
(`internal/database/migrations.go:138`) is the continuation, and arrival is a
sweep with no orders in it (`PhaseArrival`, `internal/orders/spec.go:98`).
None of that changes.

**But the phase table cannot be reordered safely yet**, which is why this is
three commits rather than one. See part 1.

## What the model gives

A crossing of T turns costs the ship exactly T turns of orders:

| Turn | What the ship does |
| --- | --- |
| N | all its orders resolve normally; moves and departs in the last stage |
| N+1 … N+T | nowhere; can be given no order (Bind refuses, as today) |
| N+T | lands in the last step of the last stage; the report shows it arrived |
| N+T+1 | orders normally from its new stellium |

Today the cost is T−1. `Binder.actor` (`internal/orders/order.go:180`) already
tells the player exactly this model — "it arrives on turn %d and can be given
orders from turn %d", with `ArrivalTurn+1` — so the guard stops papering over a
mismatch and becomes literally true. Reachability stays one fact for a whole
turn, which is what keeps it a Bind rule.

---

## Part 1 — address ring draws through `internal/prng`

**This is a prerequisite, not a nicety.** Both ring draws are addressed by the
order's `sequence`:

```go
// internal/orders/verbs.go:566 and :1761
t.World.Game().Seed.RingFor(t.Number, t.FactionID, t.Sequence)
```

`sequence` is a position in the phase walk (`internal/orders/orders.go:191`).
So reordering the phase table does not merely renumber a column — it silently
redraws every ring in every game. Worse, the phase table is designed to be
extended, so **every future phase insertion rewrites the rings of every live
game**. `internal/prng`'s own doc names this failure mode: instance keys must
be intrinsic, "never SQLite autoincrement row ids, whose values depend on
insertion order". A sequence number is weaker than a row id.

`internal/prng` is built, golden-pinned and frozen, and wired to nothing:
`grep 'prng\.'` outside the package returns no hits. `world.Seed`
(`internal/world/seed.go`) is a separate ad-hoc PCG + SplitMix, and `RingFor`
is the only random draw in the live engine.

### The address

```go
prng.Key{TagRing, x, y, z, sys, orbit, turn, factionNo, entityID}
```

The planet by coordinates, the turn so a repeat hop draws fresh — the manual's
one documented way to change a ring without going anywhere — and the faction
and entity so two ships settling at one planet in one turn differ.

### The work

- **`internal/prng/tags.go`** — append `TagRing` (8) to the frozen block, and
  add the sentence that admits ids of *game things*: an entity id and a faction
  number are assigned by the game, shown to the player in every order and
  report, and never reused or renumbered, so they address draws about that
  thing. Map objects still use coordinates, which are strictly better.
- **`internal/prng/testdata/golden.json`** — pin a vector for the new tag's
  stream, as tags.go instructs for every added tag.
- **`internal/world/seed.go`** — `Seed{High, Low int64}` becomes `prng.Seeds`
  via `prng.New`, and `RingFor(turn, factionID, sequence)` is replaced by a
  `World` method that can reach the planet's coordinates, the system's sequence
  letter (A=1) and the orbit, all of which `World` already loads.
- **`internal/orders/verbs.go:566`** — the move calls the new draw.
- **`internal/world/build.go`, `CreateEntity`** — the created-ship ring moves
  *into* `CreateEntity`, after the insert. It cannot stay at
  `verbs.go:1761`, because the address needs the entity id and the id does not
  exist until `LastInsertRowID()` at `build.go:139`. Insert, then draw, then set
  the ring.
- **`internal/world/seed_test.go`** — rewritten against the new address.

Every ring in the goldens changes once, deliberately, and nothing else does.
That is the whole of this commit's diff.

## Part 2 — lengthen the scripted game to turn 3

The scenario has two crossings and both were built to land inside its three
turns: `ship 105` (`HDRV-1`, 3 ly, T=3) lands on turn 0+3−1 = 2, and `ship 100`
(`HDRV-3`, 3 ly, T=1) lands on turn 2+1−1 = 2, "and lands in it" as the order
file says. Under N+T both are due on turn 3 while `lastTurn` is 2, so
`LandArrivals` would fire zero times in the whole scripted game and the goldens
would record two ships stuck in transit for good. **The gate would still pass**,
having quietly lost the only coverage that a crossing ever completes.

So `lastTurn` (`internal/replay/replay_test.go:44`) goes to 3, on its own,
under today's rules — a quiet turn in which nothing happens. `submitOrders`
skips a turn with no order file, and turn 3 cannot have orders for those ships
in any case: at bind time they are still crossing, because they land in its
last phase. Five new goldens, and nothing existing moves.

## Part 3 — movement resolves last

Two edits in the engine:

1. **`internal/orders/spec.go:135`** — in the `phases` slice, move
   `PhaseMove, PhaseJump, PhaseArrival` from between the espionage phases and
   `PhaseDraft` to the end, after `PhaseBroadcast`. `phase.order` is derived
   from position (`init()`, line 151), so nothing else computes a number. Two
   comments above the slice need rewriting: the one about probes and sensors
   reading the start of the turn "because movement is at stage 15", and the one
   about departures preceding arrivals, which stays true and gains the reason
   the pair is last.
2. **`internal/orders/verbs.go:679`** — `t.Number + o.turns - 1` becomes
   `t.Number + o.turns`, and the comment above it, which currently explains the
   degenerate one-turn crossing, is replaced by the reason it is now T.

Nothing else in the engine. No new status, no new table, no Bind/Apply move.

Resulting phase numbers for the manual's table: create 5, unassemble 6, stow 7,
transfer 13, unstow 14, assemble 15, probe 19, sensor 20, **naming 34, move 37,
jump 38, arrival 39**. Everything from draft to broadcast moves down by three.

Two unit tests pin the arithmetic, both in `internal/engine/engine_test.go`:
the multi-turn crossing test (`due != 5`, and its `turn := 3; turn <= 4` loop)
and `TestResolveRefusesOrdersToAShipInTransit`, which pins "arrives on turn 5
and can be given orders from turn 6". Both shift by one. The second is also the
proof that the in-transit guard is enforced twice — whole file at submit, a
`failed` row at resolve — and neither path changes.

The goldens move two ways, and **because part 1 landed first, rings are not one
of them**:

- Arrival state: t0 and t1 `IN TRANSIT` read `ARRIVES 3` instead of `2`; t2
  gains two IN TRANSIT rows where it now has none, and ship 100's location goes
  from stellium 11 to nowhere; turn 3 becomes the turn both ships land.
- Sequence renumbering, since naming moves from 37 to 34, ahead of move and
  jump:

  | | today | after |
  | --- | --- | --- |
  | t1 | `8 move`, `9/10/11 names` | `8/9/10 names`, `11 move` |
  | t2 | `7 move`, `8 jump`, `9 rations`, `10/11 names` | `7 rations`, `8/9 names`, `10 move`, `11 jump` |

  This one is player-visible with no rule behind it: the orders report lists a
  turn in resolution order, so naming now prints above movement.

## Documents

- **`docs/turn-sequence.md`** — stage 15 leaves its 1978 position and becomes
  the last stage, keeping its three steps. The paragraph calling a one-turn
  crossing "the degenerate case rather than a special one" is the rule being
  replaced and goes. Stages 18 and 19 gain a sentence saying a rebellion is
  settled before anything moves.
- **`docs/reference-manual.md`** — the phase table, and the JUMP section, which
  must now say which turn a crossing lands on and that the ship takes no orders
  in it. The MOVE section's claim that a ship may move and jump in one turn is
  unaffected.
- **`docs/plan/engine-burndown.md`** — the High item closes.

## Consequences worth stating out loud

- A ship that moves or jumps this turn takes part in everything else this turn
  first — naming, draft, pay, rations, and the sweeps at 18, 19, 21 and 22.
- A ship that lands this turn is at its destination in this turn's report, with
  IN TRANSIT empty, and still takes no orders until next turn. Worth a sentence
  in the manual so it does not read as a bug.
- Departures still precede arrivals, so a ship cannot be caught by a jump order
  written the turn it arrives. Now trivially, since neither is reachable.
- Probes and sensors (19, 20) read the start of the turn as they always did;
  with movement last, that is now structural rather than a rule to remember.

## Verification

Three commits, each with one cause, so each golden diff reads as one thing.

1. **Rings through prng.** `go test ./internal/prng` (the frozen vectors must
   still pass — a failure there means the addressing or hashing drifted, not
   that the ring moved), then `./internal/world ./internal/orders`, then
   `go test ./internal/replay -update`. The diff is every ring, and nothing
   else.
2. **Scenario to turn 3.** `go test ./internal/replay -update`. The diff is
   five new files and no change to any existing one.
3. **Movement last.** `go test ./internal/orders ./internal/engine` first, then
   `go test ./internal/replay -update`. The diff is arrival turns, sequence
   renumbering, and turn 3 becoming the turn the ships land — with no ring
   churn, which is the proof part 1 did its job.

Then, outside the tests:

- `games/eagles/replay.sh 0 6` — faction 3's three crossings from one `HDRV-10`
  (5, 11 and 30 ly) are the live demonstration: IN TRANSIT `ARRIVES` should
  read 5, 6 and 7 where it now reads 4, 5 and 6, and faction 1's five-ly hop
  stops being free.
- `ec orders help` prints the phase list from the same table, so it should show
  move, jump and arrival last with no further edit.
