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

## High: a one-turn crossing costs the ship nothing -- **done**

Found by faction 3, round 2. Closed by moving ship movement to the last stage
of the turn: see `docs/plan/movement-last.md`. A crossing now lands on
`departure + ceil(d/t)` and the ship is gone for every turn of it, so the
shortest crossing costs a turn of orders rather than nothing. The rings that
used to move with it are addressed through `internal/prng` and no longer
depend on where an order sits in the phase table.

### Issue

`docs/reference-manual.md`'s JUMP section says a crossing "takes *d* / *t*
turns, rounded up, and never fewer than one", and then says a ship in transit
is nowhere, cannot be probed, cannot be given an order, and appears in the turn
report's IN TRANSIT section with "the turn it is due". A player reads that as: a
jump costs at least one turn away.

It does not. The engine lands a crossing on `departure + ceil(d/t) - 1`, so the
shortest crossing arrives in the turn it departed, in the arrival phase that
follows the jump phase. Faction 1 sent ship 4 five light years on a `HDRV-10`
in turn 4 and had it at the destination in the same turn's report, never listed
IN TRANSIT. Three crossings from one drive make the pattern:

| Distance | ceil(d/t) | Departs | IN TRANSIT says | Turns the ship is gone |
| --- | --- | --- | --- | --- |
| 5 ly | 1 | turn 4 | nothing, ever | none |
| 11 ly | 2 | turn 4 | arrives 5 | one |
| 30 ly | 3 | turn 4 | arrives 6 | two |

Either the manual's arithmetic or the engine's is wrong, and the manual does
not say which turn a crossing lands on -- it gives the count and leaves the
player to guess where the count starts. "Never fewer than one" reads as a floor
on the cost and is in fact a floor on a number that is one greater than the
cost.

### Acceptance criteria

- **done.** The manual says a ship jumping on turn *N* is due on turn *N* plus
  the turns the crossing takes, and that the turn it is due is not a turn it
  can be given orders in.
- **done.** `ceil(d/t)` and the IN TRANSIT `ARRIVES` column agree, and every
  jump costs the ship the turns the crossing takes.
- **done.** `TestResolveExecutesMovesBeforeJumpsAndRecordsOutcomes` asserts a
  one-turn crossing leaves the ship nowhere at the end of the turn it departed,
  and the replay plays both its crossings to the turn they land in.

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

## Medium: NAME has two forms the manual does not give

Found by faction 5, round 2.

### Issue

The manual's NAME section gives three forms and says the order "Gives one of
the faction's own ships or colonies, or a stellium, a name." The engine accepts
two more, and faction 5 filed both:

```text
we name (-12,-7,11) system A "Hearth"
we name (-12,-7,11) system A orbit 4 "Hearth"
```

Both succeeded, and the turn report's NAMES section has been carrying
`system` and `planet` rows ever since:

```text
NAMES
SUBJECT   ID   NAME
planet    114  Hearth
stellium  10   Hearth Reborn
system    12   Hearth
```

A player reading the manual would not know a system or a planet can be named at
all, and the NAMES section prints subject kinds the manual never introduces.

### Acceptance criteria

- The NAME section gives every form the parser accepts, or the parser stops
  accepting the two it does not document.
- The forms say what they name: a system of a named stellium, and a planet of a
  named system.

## Medium: the manual does not describe the turn report

Found by faction 11, round 2.

### Issue

The turn report is the whole of what a player receives, and the manual names
two of its thirteen sections -- IN TRANSIT, in JUMP, and UNDER CONSTRUCTION, in
CREATE. It says nothing about CONTROLLED PLANETS, ENTITIES, NAMES, CENSUS,
INVENTORY, SENSOR SURVEY, SENSOR PLANETS, SYSTEM CONTACTS, PROBE CONTACTS,
PROBE DEPOSITS, ORDERS, or PROBES: not what they hold, not what their columns
mean, not when a row appears.

Three consequences a lawyer could hold up on their own:

- An entity under construction sits in ENTITIES next to finished ones, with a
  mass of 0 and an enclosed volume of 0 and nothing marking it. Only the
  UNDER CONSTRUCTION section below it says ship 41 is not a ship yet.
- SENSOR SURVEY prints an internal system id in a column headed `SYSTEM` --
  `99`, `35`, `47` -- where the manual says a system is lettered A through E.
  The PROBES section prints `83/94` for a stellium and system the player knows
  as coordinates and a letter.
- CENSUS reports people, INVENTORY reports units of 100 people, and nothing on
  either says so. That one the glossary happens to cover; the rest it does not.

### Acceptance criteria

- The manual has a section describing the turn report, section by section, with
  what each column holds.
- Where a report prints an identifier, it prints the one the manual taught the
  player -- coordinates for a stellium, a letter for a system.
- An entity under construction is marked as such where it is listed.

## Medium: ENCLOSED VOLUME reports the raw figure, not the usable one

Found by faction 6, round 2.

### Issue

The manual is careful about this: `STRC` and `STRL` enclose TL x TL VU each, an
entity can use a fraction of what it encloses, and "An entity cannot hold more
than its usable enclosed volume." The fraction is 1 for a `COPN`, 1/5 for a
`CSFC`, and 1/10 for a `CORB` or a `SHIP`.

The turn report's column headed `ENCLOSED VOLUME` is the raw figure. Two
entities of the shipped kit make the problem plain:

```text
ID  UNIT  TECH  STELLIUM  SYSTEM  PLANET  RING  MASS   ENCLOSED VOLUME
13  COPN  1     10        A       114     0     6562   25000
14  CSFC  1     10        A       114     0     6602   25000
16  SHIP  1     10        A       114     64    73150  242100
```

Colony 13 can use all 25,000. Colony 14 can use 5,000 of the same number. Ship
16 can use 24,210 of the 242,100 it is shown. The number that decides whether
an order fails for want of room is in no report, and neither is how much of it
is occupied -- a player must total the INVENTORY section against Appendix A by
hand and then divide.

### Acceptance criteria

- The report shows the usable figure, or shows both and names them apart.
- Occupied volume is reported beside whichever figure is shown, so a player can
  see the room left before an order fails for want of it.
- The manual says which of the two the report gives.

## Medium: approximate mass and approximate quantity are an undocumented magnitude

Found by faction 4 and faction 11, round 2.

### Issue

`SYSTEM CONTACTS` gives an `APPROXIMATE MASS` and `PROBE DEPOSITS` an
`APPROXIMATE QUANTITY`. Neither is an approximation of anything a player can
read. They are orders of magnitude:

```text
SYSTEM CONTACTS
ENTITY  PLANET  ORBIT  CONTACT UNIT  RING  APPROXIMATE MASS
29      984     4      CORB          1     4          <- 73,607 MU
29      984     4      CORB          1     0          <- 0 MU, under construction
29      984     4      SHIP          64    4          <- 73,150 MU

PROBE DEPOSITS
PLANET  DEPOSIT  RESOURCE  APPROXIMATE QUANTITY
931     19531    fuel      7                          <- 10,000,000
```

A 73,150 MU ship and a 73,607 MU orbital colony read the same, and every
deposit in the game reads `7`. The manual promises a probe reports deposits
"with its resource and approximate quantity" and says nothing about the scale,
so a player takes `7` for seven units of ore. The PROBE CONTACTS section, by
contrast, gives exact masses, which the manual does promise.

### Acceptance criteria

- The manual says what an approximate figure is and on what scale, or the
  reports print a figure in the units the manual uses everywhere else.
- The two columns use the same scale as each other, and say so.

## Medium: what a passive sensor reads is unspecified

Found by faction 11, round 2.

### Issue

The manual's turn sequence gives phase 20 one line -- "every assembled `SNSR`
reads the sky; no order is given" -- and the SNSR entry in Appendix A gives its
mass and volume. Nothing says what a sweep returns. It returns three sections
of the turn report, and every one of them holds a rule the manual does not:

- `SENSOR PLANETS` gives the kind of every planet in every system of the
  entity's stellium. Ship 4, moved to a two-system stellium, gets twenty rows.
  A probe reads one planet at the cost of a probe; a sweep reads the lot for
  nothing. It gives no habitability, which a probe does.
- `SYSTEM CONTACTS` omits every ring-0 entity. A `COPN` and a `CSFC` on the
  surface are invisible to a sweep and plain to a probe.
- `SYSTEM CONTACTS` lists the observing entity among its own contacts, and has
  no identity column, so a player cannot tell their own ship from a stranger's
  at the same planet.

### Acceptance criteria

- The manual says what a sensor sweep reads, what it does not, and how far it
  reaches.
- It says whether an entity sees itself, and the report gives enough to tell a
  contact apart from the observer.

## Medium: a probe reports open-air colonies, which the manual's list excludes

Found by faction 4, round 2.

### Issue

The manual's PROBE section: "A probe reports, for the planet it reads: every
ship, orbital colony, and surface colony there, each with its identity and
exact mass". Three kinds, and the fourth named on purpose everywhere else in
the manual is missing. The engine reports all four:

```text
PROBE CONTACTS
PLANET  ENTITY  UNIT  RING  MASS
934     9       COPN  0     6562
934     10      CSFC  0     6602
934     11      CORB  1     73607
934     12      SHIP  64    73150
```

An open-air colony is the one entity a faction cannot hide, and it is the one
the manual's list leaves out. Either the list is missing a kind or the engine
is reporting one it should not.

### Acceptance criteria

- The manual's list of what a probe reports matches what a probe reports.
- A test asserts the kinds a probe returns for a planet holding all four.

## Medium: "uncontrolled" is used once and defined nowhere

Found by faction 8, round 2.

### Issue

The manual's TRANSFER section: "The recipient must be the faction's own entity
or an uncontrolled one." That is the only appearance of the word in the manual.
The glossary does not have it, no section says what makes an entity
uncontrolled, and nothing says one exists.

They do exist, and they are not obscure: every home planet in the game carries
an uncontrolled `CORB` of 73,607 MU in ring 1, and it is the largest thing at
the planet. Faction 8 transferred to it and the order bound and resolved, so
the manual's sentence is true. But a player cannot find the entity from the
manual: no report lists it, because reports list the faction's own entities,
and the only way faction 8 learned colony 27 was there was a probe.

### Acceptance criteria

- The manual says what an uncontrolled entity is, how a player finds one, and
  what may be done with it.
- Where the starting position is described, the uncontrolled orbital colony at
  the home planet is described with it.

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

## Low: NAME says nothing about two things sharing a name

Found by faction 5, round 1. Half closed by the reference manual, which now
says "Naming something again renames it"; faction 5 re-argued it in round 2 and
lost.

### Issue

One name may be given to several different things at once. Faction 5 called a
stellium, a system within it, and a planet within that all `Hearth`, and all
three stand:

```text
NAMES
SUBJECT   ID   NAME
planet    114  Hearth
stellium  10   Hearth
system    12   Hearth
```

The manual's NAME section covers the forms it admits to, the length, the
characters allowed, and renaming, and says nothing about whether a name has to
be unique.

### Acceptance criteria

- The NAME section says that names need not be unique.
- A test covers it, so the documented answer is the engine's.

## Low: `as trade-station` is recorded and never reported

Found by faction 9, round 2.

### Issue

The manual's CREATE section: "`as trade-station` is accepted on any of the
three colony kinds and recorded on the entity. What it confers is not
specified."

It is recorded -- faction 9 created colony 42 `as trade-station` and the
database carries the flag -- and no report mentions it. The order's own INPUT
text, echoed back in the ORDERS section, is the only place the words appear:

```text
SEQUENCE  LINE  ENTITY  VERB    INPUT
1         4     30      create  orbital colony as trade-station using 60 STRC-10 ...
```

Once the order rows for that turn are purged, a player has no way to tell a
trade station from a colony. Saying that a flag is recorded, and reporting the
flag, are two different promises, and the manual makes only the first.

### Acceptance criteria

- ENTITIES marks a trade station, or the manual says the flag is not visible
  until whatever it confers is specified.

## Low: the STOW section blames `AUTO` for a failure only an UNSTOW can cause

Found by faction 7, round 2.

### Issue

The manual's STOW section: "A stow that would leave the entity holding more
than its usable enclosed volume fails, and nothing moves. Only `AUTO` can bring
that about."

Appendix A says the opposite. `AUTO` occupies 2 x TL VU as cargo and 4 x TL VU
unassembled, and it is the only unit whose two freight volumes differ. A stow
moves units out of unassembled inventory into cargo, so stowing `AUTO` is the
one stow that certainly *frees* volume. Every other unit occupies the same in
both sections, and on a `COPN` or a `CORB` the four resources occupy nothing in
cargo at all -- which faction 7 used, moving 90 `GOLD` off a `COPN`'s cargo and
back again over four turns.

No stow can raise what an entity occupies. An unstow can, and `AUTO` is exactly
how: the sentence is true of the section on the other side of the transfer
phase.

### Acceptance criteria

- The `AUTO` sentence moves to UNSTOW, or STOW says a stow cannot exceed the
  usable volume and drops the failure with it.

## Low: population cannot reach cargo, and TRANSFER says it must

Found by faction 8, round 2.

### Issue

Three sentences of the manual do not fit together. TRANSFER: "Units must be in
cargo to be transferred and are set down in the recipient's cargo ... Population
moves the same way." STOW: "Population cannot be stowed, and neither can a
cadre; naming either is an error in the file." Appendix A gives population a
volume in every section, cargo among them.

So population must be in cargo to be transferred, and no order can put it
there. The engine does not agree with the requirement: faction 8 filed
`colony 25 transfer 10 USK to ship 28`, and it bound and resolved, stopped only
by the transports it had none of. Population is not held in an inventory
section at all -- it is reported in CENSUS, not INVENTORY.

### Acceptance criteria

- TRANSFER says the cargo requirement is about units, and says where population
  is taken from and set down.
- The manual says whether population occupies a section at all, since Appendix
  A gives it four volumes and no order moves it into three of them.

## Low: a probe from the stellium orbit is a rejection the manual's table does not cover

Found by faction 4, round 2. No docket was filed: the file was refused at
check, and a refused docket argues nothing.

### Issue

The manual's PROBE section says a probe naming no system "reads the system the
entity is in, so a ship orbiting the stellium must name one", which is exactly
what the engine says back:

```text
ec: line 4: ship 8 is orbiting the stellium; name a system to probe
```

The rule holds. What the manual does not say is that this refuses the whole
file. Its table splits failures by whether a turn can change the cause --
rejection for "what a turn cannot change", warning for "what a turn can" -- and
by that criterion this belongs on the warning side: a ship in the stellium
orbit can reach a system within the turn, by a `MOVE`. It cannot reach one *in
time*, because the probe phase is 19 and the move phase is 27, and that is the
real reason. The criterion the manual gives is not the criterion the engine
uses.

### Acceptance criteria

- The rejection/warning table says the test is whether the cause can change
  before the order's own phase, not merely within the turn.
- PROBE says that a probe with no system, given by an entity that has none, is
  a rejection.

## What round 1 argued and lost

Recorded because a conformance game is only worth reading if it says what it
failed to find. All of these were probed and all of them match the docs:

- **MOVE** costs and rings. 28 FUEL for every hop including a move to the
  planet the ship is already at, 0 for stellium orbit to stellium orbit, and a
  fresh ring drawn each time -- exactly the table at `docs/orders.md:255`.
- **JUMP** fuel and crossings. 840 for three light years, 1,400 for five,
  4,760 for seventeen; the whole bill on departure; `ceil(d/10)` turns; a ship
  in transit reachable by nothing. The turn count stands; round 2 found that
  the manual never says which turn the count lands on, which is the High item
  above.
- **PROBE** budgets. Six probes from three `SNSR-2` and two from two `SNSR-1`,
  and an orbit named three times in one order spends three of them, which is
  what "spends one probe on each" says.
- **The group forms of CREATE** fail with `CREATE is accepted but not built
  yet`, which `docs/orders.md:843` documents as the answer for all of them.
- **Phase order.** Orders written in the reverse of the order they resolve are
  renumbered by phase in `sequence` and keep their place in `source_line`.

## What round 2 argued and lost

Round 2 was the manual's first adversarial reading, and it held up better than
the count of findings above suggests. Everything here was probed against
`docs/reference-manual.md` alone and matched it:

- **The MOVE fuel table, in full.** The cross-system row had never been tested,
  because every home stellium in the shipped map holds one system. Ship 4
  jumped to a two-system stellium and moved A-to-B for 56 `FUEL` -- 8 per
  assembled `HDRV` against the 4 of every other hop, both at 7 units.
- **The JUMP fuel rule at three distances.** 1,400 for five light years, 3,080
  for eleven, 8,400 for thirty; 40 per assembled `HDRV` per light year every
  time, and the whole bill drawn on departure, including the 8,400 that left
  ship 8 with 208 `FUEL` in the tank.
- **Phase order against file order, twice.** Faction 11 wrote `jump` above
  `move` and the move resolved first; faction 7 wrote `unstow` above `stow` and
  the stow resolved first. Both were renumbered in `sequence` and kept their
  `source_line`.
- **A shortfall succeeds and says how much it did.** The manual's checking
  section now states this outright -- an order short of workers, labour,
  transports, or stock "succeeded, did what it could, and reports how much that
  was" -- and `assemble`, `unassemble`, `stow`, `unstow`, and `transfer` all
  behave that way. Round 1 could only find this in the orders reference.
- **NAME's rules for a stellium.** A stellium may be named without being
  visited: faction 5 named (4,1,-2), fourteen light years from anything it
  owns. Naming again renames: `Hearth` became `Hearth Reborn`. The manual says
  both, which answers half of round 1's NAME finding.
- **A created entity takes the builder's technology level** and appears at the
  builder's planet in the ring its kind requires -- colony 42, created by a
  tech-1 `CSFC`, is a tech-1 `CORB` in ring 1.
- **An uncontrolled entity is a legal transfer recipient**, as TRANSFER says.
- **Probes and sensors read the start of the turn.** Ship 4's own sweep
  reported it at the ring it left, not the one it moved to.
- **The group forms of CREATE**, all three of them now, fail with `CREATE is
  accepted but not built yet`, which the manual documents.

## Completion checks

- `go test ./...` passes.
- `games/eagles/replay.sh 0 3` refuses no docket. A refused docket has argued
  nothing and is a mistake in the docket, not a finding.
