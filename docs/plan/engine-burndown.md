# Engine and documentation burndown

This plan records the findings from `games/eagles`, where ten rule lawyers file
legal orders and then argue the outcome against `docs/`. Work is ordered by
severity.

A finding is a result the documentation does not support: the engine and a
document disagree, two documents disagree, or nothing says what should happen
at all. **Missing documentation is a finding**, and most of these are. The
engine came out of round 1 well; what it does is mostly right and mostly
unwritten.

Items marked **done** are closed. The lawyers are pointed at this file so a
round does not re-argue what is already written down here.

## Medium: UNDER CONSTRUCTION calls a ceiling "WORKERS"

Found by faction 9, round 1.

### Issue

`with 5 CWKR` on a create sets a **ceiling** on the cadre the build may draw --
`docs/accepted-orders.md:135` says so -- and it is stored as `cwkr_cap`. The
turn report prints that column under the heading `WORKERS`:

```text
UNDER CONSTRUCTION
ENTITY  KIND  BUILDER  WORKERS  CLAUSE  UNIT  TECH  REQUIRED  CLAIMED  DELIVERED  DONE
41      SHIP  29       5        using   STRC  10    60        0        0          0
```

Colony 29 has no `CWKR` at all. The build claimed nothing, delivered nothing,
and completed nothing on four consecutive turns, and will never finish. A
player reading that table sees five workers on the job and no progress, which
reads as a broken engine rather than an unstaffed build.

### Acceptance criteria

- The column is named for what it holds, or the report shows both the ceiling
  the order set and the cadre actually working the build.
- A build that made no progress this turn says why, in the words the inventory
  orders already use for the same shortage -- `its 0 CWKR had 0 MU of work left
  this turn`.
- A test covers a build whose builder has no cadre and asserts the report
  distinguishes "allowed five" from "had none".

## Medium: docs/orders.md contradicts itself about a second MOVE

Found by faction 1, round 1.

### Issue

`docs/orders.md:187`:

> **A ship moves once a turn.** A second `MOVE` for the same ship in one file is
> rejected, and the whole file with it.

`docs/orders.md:262`:

> A ship with more than one move order in a turn starts each move from where the
> previous one left it.

The second sentence is a leftover from before the once-a-turn rule and
describes a file the parser now refuses whole. The engine implements the first
(`Binder.once`), so the code is right and the reference has two answers.

### Acceptance criteria

- Only one of the two survives, and it is the one `Binder.once` implements.
- The fuel table above it still reads correctly without the sentence, which
  currently carries the explanation of why a cross-system move costs two hops.

## Medium: nothing documents what a starting kit can do

Found by factions 6 and 8, round 1.

### Issue

The shipped kit (`games/claude/home-planet-seed.json`) brings no `CWKR` cadre
and no `TRAN` transports. Three built, fully documented orders therefore do
nothing whatever a kit-started faction asks of them, and report `succeeded`:

```text
colony 17 unassembled 0 of 10 STRC-10; its 0 CWKR had 0 MU of work left this turn
colony 25 transferred 0 of 90 GOLD; it had no TRAN free this turn
```

Every part of that is correct. `docs/orders.md` documents the shortfall rule --
"A shortage is a rate rather than a failure" -- and the messages say exactly
how much was done. The gap is that no document says what a kit contains, so a
player has no way to know that `assemble`, `unassemble`, and `transfer` are
inert for them until they spend a turn finding out.

`docs/turn-sequence.md:209` compounds it: "Drafting enough construction workers
to cover the turn's expected work is the faction's responsibility". `DRAFT` is
not built. In a kit-started game the kit is the only source of a cadre, and the
kit loader does accept one (`"cadres": {"CWKR": n}`), but nothing points a
reader from that sentence to that fact.

### Acceptance criteria

- A document describes the starting kit: what it holds, and what a faction can
  and cannot do with it on turn 1.
- Where the docs make drafting the faction's responsibility, they say that
  `DRAFT` is not built yet and name the kit as the current source.
- The claim is checked against the kit rather than asserted: a test, or a
  documented command, that lists what a kit brings.

## Low: games/claude/game-rules.md claims a ring the engine never draws

Found by faction 1, round 1.

### Issue

`games/claude/game-rules.md:26`:

> A move places a ship in ring 99, so any successful move onto another faction's
> home planet wins.

`docs/orders.md:229` says a move places the ship "in a ring the game draws at
random from 2 through 99", and EAGLE-01 drew 63, 13, and 93 for three
consecutive moves. The conclusion the sentence draws is still true -- the win
is ring 1 through 99, so any ring qualifies -- but the reason given is wrong,
and it is the sentence a player would rely on to plan a winning move.

### Acceptance criteria

- The sentence says what the engine does, and still explains why any successful
  move onto the planet wins.

## Low: NAME says nothing about renaming, or about two things sharing a name

Found by faction 5, round 1.

### Issue

Both are legal and neither is written down. A second name replaces the first:

```text
ship 16 name "Kestrel"      turn 0
ship 16 name "Kestrel II"   turn 1   -> the ship carries only "Kestrel II"
```

And one name may be given to several different things at once. Faction 5
called a stellium, a system within it, and a planet within that all `Hearth`,
and all three stand:

```text
NAMES
SUBJECT   ID   NAME
planet    114  Hearth
stellium  10   Hearth
system    12   Hearth
```

`docs/orders.md`'s NAME section covers the forms, the length, and the
characters allowed, and says nothing about either question.

### Acceptance criteria

- The NAME section says that a later name replaces an earlier one for the same
  subject, and that names need not be unique.
- Tests cover both, so the documented answers are the engine's.

## What round 1 argued and lost

Recorded because a conformance game is only worth reading if it says what it
failed to find. All of these were probed and all of them match the docs:

- **MOVE** costs and rings. 28 FUEL for every hop including a move to the
  planet the ship is already at, 0 for stellium orbit to stellium orbit, and a
  fresh ring drawn each time -- exactly the table at `docs/orders.md:255`.
- **JUMP** fuel and crossings. 840 for three light years, 1,400 for five,
  4,760 for seventeen; the whole bill on departure; `ceil(d/10)` turns; a ship
  in transit reachable by nothing.
- **PROBE** budgets. Six probes from three `SNSR-2` and two from two `SNSR-1`,
  and an orbit named three times in one order spends three of them, which is
  what "spends one probe on each" says.
- **The group forms of CREATE** fail with `CREATE is accepted but not built
  yet`, which `docs/orders.md:843` documents as the answer for all of them.
- **Phase order.** Orders written in the reverse of the order they resolve are
  renumbered by phase in `sequence` and keep their place in `source_line`.

## Completion checks

- `go test ./...` passes.
- `games/eagles/replay.sh 0 3` refuses no docket. A refused docket has argued
  nothing and is a mistake in the docket, not a finding.
