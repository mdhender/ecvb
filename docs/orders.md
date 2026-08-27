# Order File Reference

An order file identifies one game turn and one submitting faction, followed by
zero or more orders.

Every order names its subject first and then what the subject is being told to
do:

```text
ship 18 jump to (-1,2,3)
colony 24 probe orbit 5
we name (-1,2,3) "Stellium Joe"
```

A subject is one of your ships, one of your colonies, or `we`, which is the
faction itself. `we` takes no id, because the file header already says which
faction is writing, and it is the subject of the orders that no ship or colony
carries out -- naming a place is the only one so far. An order given to a
subject that cannot be given it is rejected and says so: `MOVE` is given to a
ship, and `PROBE` to a ship or a colony.

A turn resolves in phases, and every order of one phase resolves before any
order of the next, whichever way round the file wrote them. File order decides
only between orders of the same phase, segment, and step. The phases, in the
order they happen:

| # | Phase | What resolves |
| --- | --- | --- |
| 1 | unassemble | every `UNASSEMBLE` order |
| 2 | transfer | every `TRANSFER` order |
| 3 | assemble | every `ASSEMBLE` order |
| 4 | probe | every `PROBE` order |
| 5 | sensor | every assembled `SNSR` reads the sky from where it stands; no order is given for it |
| 6 | move | every `MOVE` order |
| 7 | jump | every `JUMP` order departs |
| 8 | arrival | every ship whose crossing finished lands; no order is given for it |
| 9 | naming | every `NAME` order |

The three inventory phases are in that order on purpose: units have to be
unassembled to be carried, so one file may unassemble at one entity, transfer
to another, and assemble again there, all in the same turn.

Probes and passive sensors both read where things stood when the turn began, so
both settle before anything moves; a ship that jumps this turn reports its new
stellium in next turn's report. Departures settle before arrivals, so a ship
landing this turn cannot be caught by a jump order written the turn it arrives. `ec orders help` prints this table from the
same list the engine walks, so it cannot fall behind.

## Quantities

A quantity is a whole number greater than zero. Once it passes 999 it separates
every three digits with a comma: `5,000` is accepted and `5000` is refused.
The same comma separates the items of a list, which reads without ambiguity
because a quantity is always followed by a unit code and never by another
quantity:

```text
ship 18 transfer 4,500 GOLD, 18,000 FOOD to colony 24
```

A unit code is a code on its own, such as `GOLD`, or a code and a technology
level, such as `LFSU-7`. See [Unit Glossary](units.md).

## File names

The recommended file name is:

```text
t{TURN}-f{FACTION}-orders[-v{VERSION}].txt
```

For example, `t0-f1-orders-v1.txt` contains version 1 of faction 1's turn 0
orders. The version suffix is optional and has no effect on processing.

## File structure

The first physical line identifies the game and turn:

```text
game "BETA-001" turn 0
```

The second physical line identifies the player or faction:

```text
id player "user01@example.com"
```

or:

```text
id faction 1
```

Player email addresses are trimmed and converted to lowercase before lookup.
The identified player or faction must belong to the game.

Blank lines are permitted after the identity line. A `#` outside quotes begins
a comment that runs to the end of the line, so a comment may stand on its own
line or follow an order:

```text
# scout the neighbouring system
ship 2 probe system B orbit 4    # before anything moves
```

Keywords and system letters are case-insensitive. Game codes are matched
exactly against the database. IDs are positive integers. A line that does not
match any form of the order it names is rejected, and the error lists that
order's forms; `ec orders help` prints them all.

An order file containing only the two header lines represents an empty order
set.

## JUMP

```text
ship SHIP-ID jump to (X,Y,Z)
```

Example:

```text
ship 2 jump to (6,-9,8)
```

The ship must belong to the submitting faction. The coordinates must identify a
stellium in the named game; a jump cannot end in deep space. The ship arrives
orbiting the stellium rather than a planet: it crosses to a planet from there
under its own power, with a `MOVE`.

**A jump begins from the stellium orbit.** A ship at a planet cannot jump: send
it out first, in the same file, with `ship SHIP-ID move to orbit 11`.

```text
ship 2 move to orbit 11
ship 2 jump to (6,-9,8)
```

Every `MOVE` resolves before any `JUMP`, so those two lines work in either
order in the file. A move that fails -- for want of fuel, say -- leaves the
ship at its planet, and the jump behind it fails for the same reason.

**Distance does not limit a jump.** A drive's technology level does not cap how
far it goes, so any ship can be sent to any stellium in the game. What limits a
long jump is the `FUEL` it burns, which is 40 per assembled `HDRV` unit per
light year and so grows with the distance. The ship's mass must still be within
its drive's capacity. See [Unit Glossary](units.md) for the drive rules.

**Technology level decides how long the crossing takes.** A jump of _d_ light
years by a drive at technology level _t_ takes _d_ / _t_ turns, rounded up, and
never fewer than one. That is the whole of what a better drive buys: not a
longer reach, since every drive reaches everywhere, but fewer turns spent off
the board.

**The whole fuel bill is drawn on departure**, however many turns the crossing
takes, so a ship that cannot pay for all of it never leaves.

**A ship in transit is nowhere.** Until it arrives it is at no stellium, no
system, and no planet: it cannot be probed, it does not appear on a passive
sensor sweep, and it can be given no order of any kind. A crossing cannot be
recalled, redirected, or cancelled, and since the fuel is already spent, a jump
written to the wrong coordinates is not recoverable. The turn report's
`IN TRANSIT` section is where a crossing ship shows up: where it is bound, and
the turn it is due.

**A ship jumps once a turn.** A second `JUMP` for the same ship in one file is
rejected, and the whole file with it. Usually the ship is already in transit by
then and says so; when the first jump failed for want of fuel and left the ship
where it was, the second is refused for being the second. Both `orders check`
and the engine apply these limits, so a jump the check rejects is a jump the
engine would have failed.

## MOVE

```text
ship SHIP-ID move to orbit ORBIT
ship SHIP-ID move to system SYSTEM orbit ORBIT
```

**A ship moves once a turn.** A second `MOVE` for the same ship in one file is
rejected, and the whole file with it. The one move is the whole of what a ship
does inside its stellium in a turn: it cannot cross to a planet and then carry
on to another, and it cannot visit a planet and then leave for the stellium
orbit. Doing both takes two turns.

A ship may still `MOVE` and `JUMP` in one turn, which is exactly how a ship at a
planet leaves: the move takes it out to the stellium orbit and the jump takes it
away. Those are the only two journeys a ship makes in a turn.

The order is spent whatever it goes on to do. A move that fails for want of fuel
has still been given, and the ship does not get another that turn.

Move to a planet in the ship's current system:

```text
ship SHIP-ID move to orbit ORBIT
```

Example:

```text
ship 2 move to orbit 6
```

The ship must currently have a system, and that system must contain a planet in
the requested orbit.

Move to a planet in a named system of the ship's current stellium:

```text
ship SHIP-ID move to system SYSTEM orbit ORBIT
```

Example:

```text
ship 2 move to system B orbit 4
```

The named system must exist in the ship's current stellium and must contain a
planet in the requested orbit. Both forms place the ship at the destination
planet in a ring the game draws at random from 2 through 99. Ring 0 is the
surface and ring 1 belongs to orbital colonies, so a ship arriving under its
own power never lands in either. The draw is seeded from the game and the
order, so re-resolving a turn puts the ship in the same ring.

Return to the stellium orbit:

```text
ship 2 move to orbit 11
```

Orbit 11 does not exist. It is a fiction that gives `MOVE` a way to say "leave
the planets", and it is the only orbit that is not a place: no planet occupies
it, and a probe cannot read it. Because the stellium orbit belongs to no
system, `ship 2 move to system A orbit 11` is an error. The ship ends the move
orbiting the stellium with no system, planet, or ring, which is where a jump
also leaves it -- and where a jump has to begin.

The ship's drive moves it. A move fails when the ship has no assembled `HDRV`
units or when its mass exceeds their capacity. Distance does not limit a move:
every move inside a stellium is well within the range of any drive.

A move costs one of three fixed amounts of `FUEL`, per assembled `HDRV` unit:

| Move | Fuel per unit |
| --- | --- |
| Stellium orbit to any planet of the stellium, or back | 4 |
| Planet to planet in the same system | 4 |
| Planet to the planet the ship is already at | 4 |
| Planet to planet in different systems of the stellium | 8 |
| Stellium orbit to the stellium orbit | 0 |

A move between systems costs two hops because the ship crosses the stellium
orbit on the way. A ship with more than one move order in a turn starts each
move from where the previous one left it. A failed move burns nothing.

Only one move is free. Ordering a ship in the stellium orbit to the stellium
orbit is a no-op: there is nowhere to go, so it burns nothing and the ship is
not touched. Ordering a ship to the planet it is **already at** is not a no-op:
it breaks orbit and settles again, which costs a hop and draws a fresh ring. It
is the one way to change a ship's ring without going anywhere.

## Fuel

Every `MOVE` and every `JUMP` burns `FUEL`, and every assembled `HDRV` unit
draws. A `PROBE` burns none.

A jump of X light years costs `40 * X` per unit. A move costs 4 or 8 per unit,
as the table above shows; those are the same rate applied to the tenth and the
fifth of a light year that a move covers, so nothing fractional is ever
measured or stored. See [Unit Glossary](units.md) for the drive rules and the
order the sections are drawn in.

A ship that cannot pay for an order does not stop the submission. `orders
check` and `orders submit` accept the file and warn:

```text
warning: line 5: ship 2 needs 960 FUEL to jump and holds 144; the order is kept in case that changes before the turn resolves
```

The warning comes from running the turn: each order is executed in resolution
order against the fuel the ship actually holds, so a ship that runs dry partway
through a file warns on every order after that, and an order it could not pay
for leaves the ship where it was for the orders that follow. A ship still short
of fuel when the engine reaches the order fails it, burning nothing and going
nowhere, and the turn carries on.

The warning is deliberately not an error, because fuel may reach the ship
between submission and resolution. Nothing moves fuel between entities yet;
transfer orders are not implemented.

## PROBE

```text
ship SHIP-ID probe orbit ORBIT ...
colony COLONY-ID probe orbit ORBIT ...
ship SHIP-ID probe system SYSTEM orbit ORBIT ...
colony COLONY-ID probe system SYSTEM orbit ORBIT ...
```

A probe is the one order a colony may be given. `MOVE` and `JUMP` are ship
orders.

Example:

```text
ship 2 probe orbit 6
```

One order may name several orbits, and spends one probe on each:

```text
ship 2 probe orbit 1 2 3 4 5 8 9 10
```

A probe may also name a system of the ship's current stellium:

```text
ship 4 probe system A orbit 1
ship 4 probe system A orbit 1 2 3
```

A probe that names no system reads the system the entity is in, which is why a
ship orbiting the stellium rather than a planet has to name one. A probe that
names a system reads any system of the entity's current stellium, so a ship in
system C of a three-system stellium can probe all three.

The entity must carry assembled `SNSR` units and have probes left this turn. A
colony is always at a planet, so it always has a current system. Each
named orbit must hold a planet of the probed system. See
[Unit Glossary](units.md) for how many probes a sensor array launches.

Probes resolve before anything moves, so a probe reads the system the ship is
in at the **start** of the turn. A ship cannot move into a system and probe it
in the same turn; it arrives on one turn and probes on the next. A probe reads
its planet before a move or jump carries the ship away.

Each probe reports, for the planet it reads:

- Every ship orbiting the planet, with its identity and exact mass.
- Every orbital colony, with its identity and exact mass.
- Every surface colony, with its identity and exact mass.
- Every natural resource deposit, with its type and approximate quantity.
- The habitability of the planet.

A probe does not move its ship, and its findings are recorded as of the moment
it read the planet.

## NAME

```text
ship SHIP-ID name "NAME"
colony COLONY-ID name "NAME"
we name (X,Y,Z) "NAME"
we name (X,Y,Z) system SYSTEM "NAME"
we name (X,Y,Z) system SYSTEM orbit ORBIT "NAME"
```

Examples:

```text
ship 18 name "Jalopy"
colony 24 name "Jingo"
we name (-1,2,3) "Stellium Joe"
we name (-1,2,3) system A "Alpha Sur"
we name (-1,2,3) system A orbit 8 "Headly's Gate"
```

### Naming another faction

```text
we name (player | faction) FACTION-ID "NAME"
we name (player | faction) FACTION-ID (ship | colony) ID "NAME"
```

**Not built yet.** These two parse and fail when the turn resolves; see
[Orders that are accepted and not built yet](#orders-that-are-accepted-and-not-built-yet).
A faction may only name one it has encountered, and nothing records an
encounter yet.

`player` and `faction` mean the same thing.

```text
we name faction 5 "The Hegemony"
we name player 5 ship 19 "Easy Target"
```

A name is yours. Naming your ship does not change what anybody else's report
calls it, and a stellium, system, or planet may be named without ever having
been visited -- though it has to exist. Naming something again renames it.

Naming something you own is an order to the thing itself, so the ship or colony
is the subject. Naming a place is a faction order, because no ship or colony
carries it out, so `we` is the subject: the stellium at those coordinates, or
one of its systems, or the planet in an orbit of one of its systems. An orbit
may only follow a system, because only a system holds planets.

A name is quoted text of at most 24 characters, counting spaces. It may not be
empty, may not begin or end with a space, may not hold two spaces in a row, and
may not hold control characters.

Names resolve in the naming phase, which is the last phase of the turn.

## CREATE

```text
ship SHIP-ID create ship using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end
ship SHIP-ID create (open-air | enclosed | orbital) colony [as trade-station] using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end
colony COLONY-ID create ship using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end
colony COLONY-ID create (open-air | enclosed | orbital) colony [as trade-station] using QUANTITY UNIT, ... transfering QUANTITY UNIT, ... with QUANTITY CWKR end
```

A create begins building a ship or a colony. **It is a commitment, not a
purchase**: the order says *build this as fast as you can*, it succeeds the
moment it is given, and the build runs for as many turns as it needs.
Everything after that is rate rather than failure. This is the opposite of most
orders in the game, and it is the thing to hold on to when a rule below looks
odd.

A create is the one order that may run over several lines, so it is terminated
by `end`. Line breaks and spacing inside it mean nothing.

Examples:

```text
ship 18 create ship
  using 60 STRC-8,
        61 HDRV-1, 5 SDRV-1
        , 5 LFSU-3, 1 SNSR-1
  transfering 25 FOOD, 5 SKW, 16,800 FUEL, 93 GOLD
  with 500 CWKR
end

colony 24 create orbital colony as trade-station
  using 200 STRC-4, 20 LFSU-2
  transfering 40 USK
  with 100 CWKR
end
```

All three clauses are required and each names at least one line.

### The three clauses

`using` names the units the new entity is **made of**. A create assembles what
it is given, so no separate `ASSEMBLE` is written for them: they are set down in
the new entity's cargo, which is what a transport does with anything, and the
build's own workers move them into component or operational inventory. A line
here must name something that can be assembled; a resource or a population class
belongs in the other list.

`transfering` names what is **handed over** rather than built in: cargo, and
population. `5 SKW` is five units of skilled workers, which is five hundred
people, and they go aboard rather than into inventory. A `transfering` line is
finished the moment it arrives, because cargo is where it was going.

`with QUANTITY CWKR` is a **ceiling on the workers a turn may use**, not a
reservation. It holds nothing back. Every turn the engine assigns the build up
to that many construction workers from whatever the builder has idle, and never
more however many thousands are standing about. A turn that cannot fill the cap
costs that turn's work and nothing else.

`create` is the only order that names its workers, because it is the only
assembly that runs for several turns and so the only one worth throttling. It is
also the only lever over how one entity's workers are split between its builds,
so it is worth setting deliberately rather than setting to the size of the
payroll.

**Within each clause, the order you wrote the lines in is their priority.**
List order decides what gets scarce materials, transport, and workers first.

### What a build does each turn

A build competes for three different things and they are settled at three
different stages, so its turn is three steps rather than one:

| Stage | What the build does |
| --- | --- |
| 5, creation | **Claims** what the builder holds and has not already promised. Claiming moves nothing and needs no transport. |
| 9, transfers | **Delivers** the claim on the builder's transports, and carries the construction workers out with it. |
| 10, assembly | **Completes**: the workers assemble what is on site, and then go home. |

Claiming is upstream of transfers and the market, so a build cannot claim units
that arrived this turn. A claim lives for **one turn**: what does not get
carried is released, and next turn's claiming runs afresh in seniority order, so
a senior build's priority is renewed rather than banked.

A build claims from the builder's **cargo**, which is the only section a
transport loads from -- the same rule a `TRANSFER` follows. `STOW` is what
readies a load for a build.

Materials are carried before workers. Delivered material keeps across turns
while a worker who could not be carried costs only that turn's shift, so filling
the hold with workers first would be the more expensive mistake.

> **Explicitly ordered work outranks a standing commitment.** A `TRANSFER` order
> is served before a build's claim, and an `ASSEMBLE` order before a build's own
> assembly. A build takes what is left, which is all it ever needs to do,
> because a build never fails for want -- it only slows.

A build's assembly is **its own pool**: its workers are at the new entity doing
that build's work, so their 500 MU a turn is reckoned for that build alone and
rounds up on its own. It does not pool with the builder's `ASSEMBLE` orders, nor
with a sibling build. What they share is the cadre they are drawn from.

Where two builds of one entity compete, the **older is served first** and each
takes up to its own cap.

### Structure comes first

Until every structural `using` line is completed, only the `STRC` and `STRL`
lines are eligible. This is forced rather than preferred: assembled structure is
what creates enclosed volume, and everything else an entity holds consumes it,
so before there is structure there is nowhere to put anything.

The one exemption that makes a build possible at all is narrow: `STRC` and
`STRL` delivered to an entity under construction sit in its cargo consuming no
enclosed space. **Nothing else is exempt.**

### Population needs life support first

A population line is eligible only while the new entity's **assembled** `LFSU`
supports the people already aboard as well as the ones about to arrive.
Delivered-but-unassembled life support supports nobody. Unsupported people never
leave the entity handing them over. An open-air colony breathes the air outside
and is not capped at all.

A line that cannot make progress this turn is **skipped**, and the build goes on
to the next one. An unavailable line never freezes a build; as soon as it can be
worked again it takes precedence over everything below it. So a player who
writes population above life support loses turns rather than the build.

### Where the new entity appears

At the builder's planet, in the ring its kind requires: ring 0 for an open-air
or enclosed colony, ring 1 for an orbital one, and a ring drawn from 2 through
99 for a ship, the way an arriving ship draws one. A ship created from an entity
in the stellium orbit is created there.

A colony therefore requires the builder to be **at a planet**, and an open-air
colony requires that planet's habitability to be **above 0**. Both are refused
at submission, because neither can change before the turn resolves.

The new entity takes the **technology level of the entity that created it**.

`as trade-station` is accepted on any of the three colony kinds and recorded on
the entity. What it confers is the market's business and is not built yet.

### While it is unfinished

The entity **exists from the moment the order is given**. It belongs to the
faction building it, it has a mass, and probes and passive sensors read it like
anything else. It grows: its mass rises as material is delivered and its
enclosed volume as structure is assembled.

It can be **given no order**, and nothing but its own build may deliver to it.
When the last line completes, the build is over and what is left is an ordinary
entity.

Read a build's progress in the **UNDER CONSTRUCTION** section of the turn
report. It is the only place: a build outlives the create order that began it,
and that order row is purged two turns on.

**A lean turn is not a failure.** A build that gets no materials, no transport,
or no workers simply does not move that turn, and nothing carries over against
it. A build with no transports at all is the extreme case and is still not an
error: the order succeeds, the entity exists, and it delivers nothing until
transports reach the builder. What still fails is only what could not have
changed -- a unit code that is not one, or a colony ordered from an entity that
is not at a planet.

### The group forms

```text
colony COLONY-ID create factory-group with QUANTITY UNIT, ... making UNIT
ship SHIP-ID create farm-group with QUANTITY UNIT
colony COLONY-ID create farm-group with QUANTITY UNIT
colony COLONY-ID create mine-group with QUANTITY UNIT working deposit DEPOSIT-NO
```

**Not built yet.** These parse and fail when the turn resolves; see
[Orders that are accepted and not built yet](#orders-that-are-accepted-and-not-built-yet).

They are the same verb and **nothing above applies to them**, because they
finish by the opposite rule. A ship or colony create is a commitment that runs
for as many turns as it needs. A group create is **kill-and-fill**: it runs once,
in the turn it was given, builds as much as the labour and materials pay for,
reports what was built, and closes out. Nothing carries over. Ask for 10,000,000
mines with the resources for 2 and you get 2, and the order is finished.

They take no `end`, because they are one line. A factory group must say what it
will make; a mine group's deposit is fixed for its life, which is why moving a
mine is a `REMOVE` and a fresh create rather than an order of its own. Only a
colony may hold a factory or a mine group; a farm group may be worked from a
ship.

Examples:

```text
colony 24 create factory-group with 54,000 FACT-6 making CNGD
ship 18 create farm-group with 1,234,000 FARM-6
colony 83 create mine-group with 25,680 MINE-2 working deposit 18
```

## UNASSEMBLE

```text
ship SHIP-ID unassemble QUANTITY UNIT, QUANTITY UNIT, ...
colony COLONY-ID unassemble QUANTITY UNIT, QUANTITY UNIT, ...
ship SHIP-ID unassemble and stow QUANTITY UNIT, QUANTITY UNIT, ...
colony COLONY-ID unassemble and stow QUANTITY UNIT, QUANTITY UNIT, ...
```

Unassembling takes working units apart and returns them to unassembled
inventory. It is lossless: what comes apart is what went together.

Examples:

```text
ship 18 unassemble 1,000 SNSR-1
colony 24 unassemble and stow 60 STRL-1, 5 LFSU-1
```

`and stow` puts the units down in cargo instead, which is what a `TRANSFER`
needs: units must be in cargo to be carried. Either section can be assembled
from, so stowing costs nothing but the room a unit takes.

Unassembling is work, and the same `CWKR` cadre does it at the same rate as
assembling. See [What construction workers do](#what-construction-workers-do).

An unassemble that would leave the entity overpacked **fails**, and nothing
moves. Enclosed space comes from assembled `STRC` and `STRL`, so unassembling
structure takes room away at the same time as the units coming apart need
somewhere to be put; an entity cannot hold more than it encloses.

Unassembling `LFSU` can kill people: only assembled life support supports
anyone. See [Unit Glossary](units.md).

## STOW

```text
ship SHIP-ID stow QUANTITY UNIT, QUANTITY UNIT, ...
colony COLONY-ID stow QUANTITY UNIT, QUANTITY UNIT, ...
```

Stowing moves units out of unassembled inventory and into cargo, which is where
a transport picks a load up: units must be in **cargo** to be transferred, so a
`STOW` is what readies a load for a `TRANSFER`.

Examples:

```text
ship 18 stow 18,000 FOOD, 800 HDRV-1
colony 24 stow 12,000 METL
```

It neither takes a unit apart nor puts one together, so it reaches things an
`ASSEMBLE` never can: `GOLD`, `FUEL`, `METL`, and `MNRL` are freight like
anything else. Population is not -- people ride a transport rather than being
loaded onto one -- and a cadre is not a thing at all; naming either is a
mistake in the file.

`UNASSEMBLE ... AND STOW` already does both halves in one order and is charged
to the construction workers throughout, so it is the cheaper route whenever the
units were assembled to begin with. A plain `STOW` is for units that never
were: bought, unstowed, or left over from a build.

Stowing is not needed to assemble anything. `ASSEMBLE` draws from cargo as well
as from unassembled inventory.

**A shortage fills the order partway rather than failing it.** A stow that asks
for more than the entity holds, or for more than its production labour moves
this turn, moves what it can and says how much that was.

Stowing resolves at stage 6b of the turn, after `UNASSEMBLE` and before any
transfer, so one file may unassemble at step a, stow at step b, and transfer
the load away in the same turn.

## UNSTOW

```text
ship SHIP-ID unstow QUANTITY UNIT, QUANTITY UNIT, ...
colony COLONY-ID unstow QUANTITY UNIT, QUANTITY UNIT, ...
```

Unstowing moves units out of cargo and back into unassembled inventory. Units
must be **unassembled** to be bought or sold, so an `UNSTOW` is what readies
what a transfer set down for the market.

Examples:

```text
colony 24 unstow 800 HDRV-1, 18,000 FOOD
colony 24 unstow 9,000 METL
```

It reaches the same things a `STOW` does and refuses the same two, and it is
subject to the same shortage rule.

Unstowing resolves at stage 10a of the turn, after every transfer and before
`ASSEMBLE`, so what a transfer set down this turn can be unstowed in the same
turn. It is **not** needed to assemble anything.

### What production labour does

`STOW` and `UNSTOW` are freight handling rather than construction, so it is not
the `CWKR` cadre that carries them out and no cadre is drafted for them.
**Production labour** does: an entity's unassigned `USK` plus *t* for every
assembled `AUTO` it carries. Automation stands in for an unskilled worker
wherever unskilled work is done, and moving freight is unskilled work however
the term reads. A worker already assigned to a cadre is not production labour
at all, having been spoken for; automation is the other way round, being
production labour and never a cadre. See [Unit Glossary](units.md).

One unit of it moves **500 MU a turn**, the same rate a construction worker
assembles at, where the work is the mass being handled. Work of the same kind
is **pooled across an entity**, the way construction work is.

**Stowing and unstowing never pool with each other.** One unit does one task a
turn, so labour that stowed cannot also unstow: each total rounds up on its
own, and an entity that stowed 1 MU has spent a whole worker on it.

An order that outran the labour reports **what was left when it ran**, not what
a whole turn of it is worth, because the pool is drawn down in phase order and
an order that came second is answered by whatever the first one left it.

A stow or an unstow that would leave the entity overpacked **fails**, and
nothing moves. Only `AUTO` can bring that about, being the one unit that takes
more room unassembled than it does in cargo.

## TRANSFER

```text
ship SHIP-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to ship SHIP-ID
ship SHIP-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to colony COLONY-ID
colony COLONY-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to ship SHIP-ID
colony COLONY-ID transfer QUANTITY UNIT, QUANTITY UNIT, ... to colony COLONY-ID
```

A transfer hands units and population to another entity at the same place. The
sending entity's transports do the carrying and it pays their fuel.

Examples:

```text
ship 18 transfer 500 SOL to colony 24
ship 18 transfer 4,500 GOLD, 18,000 FOOD to colony 24
```

The recipient must be one of your own entities or an uncontrolled one, and the
two must be at the same stellium, system, and planet **when the order runs**.
They may be in different rings of that planet. A transfer to an entity
somewhere else fails; there is no partial answer to being in the wrong place.

Units must be in **cargo** to be transferred, and are stowed in the recipient's
cargo on arrival: a transport sets down freight and does not assemble it. The
recipient may assemble it in the same turn, because `ASSEMBLE` draws from cargo
too and resolves after every transfer.
Population moves the same way and is charged the same mass and volume. A cadre
cannot be transferred, because it is an assignment of people rather than a
thing; transfer the population instead.

**A shortage fills the order partway rather than failing it.** A transfer that
asks for more than is in cargo, or for more than the transports carry, moves
what it can and says how much that was. One `TRAN` at technology level *t*
carries 20*t*² MU **and** 60*t*² VU in a turn, there and back for one charge;
both limits hold. One `SKW` crews ten of them -- one that is free to, because a
skilled worker already assigned to a cadre is doing a job and does not crew a
hull as well. The fuel is reckoned over every
transport the entity used in the turn at once, so a second transfer that shares
the round trip pays only what it adds. See [Unit Glossary](units.md).

A transfer that cannot pay its transports' fuel fails.

## ASSEMBLE

```text
ship SHIP-ID assemble QUANTITY UNIT, QUANTITY UNIT, ...
colony COLONY-ID assemble QUANTITY UNIT, QUANTITY UNIT, ...
```

Assembling puts units to work. It usually costs volume: a unit takes twice its
cargo volume operational and four times it as a component.

It draws from **unassembled inventory first and from cargo after it**.
Unassembled inventory is where units are kept to be worked on -- it is what an
`UNASSEMBLE` leaves behind and what the market deals in -- and cargo is where a
transport sets its load down, so a `TRANSFER` at stage 9 can be assembled at
stage 10 of the same turn.

Examples:

```text
ship 18 assemble 6,000 SNSR-1
colony 24 assemble 5 LFSU-1, 60 STRL-1
```

Nothing in the order says where a unit goes; the unit code does. `HDRV`,
`SNSR`, `SDRV`, `LFSU`, `STRC`, and `STRL` are assembled into **component**
inventory, because that is the only section they work in. Everything else is
assembled into **operational** inventory. Resources, population, and cadres are
never assembled, and naming one is a mistake in the file.

Only assembled units do anything: a drive that is not in component inventory is
freight, and so is a sensor. Assembling either changes what the entity can do
in the same turn, but a later phase has to be the one that uses it -- assembly
resolves before probes and before movement.

An assemble that would leave the entity overpacked **fails**, and nothing
moves.

### What construction workers do

A `CWKR` cadre carries out `ASSEMBLE` and `UNASSEMBLE`. One does 500 MU of work
a turn, where the work is the mass being handled.

**Work of the same kind is pooled across an entity**, so the workers it needs
are reckoned from one total rather than order by order and unit by unit. An
entity assembling 15,120 MU of `HDRV` and 100 MU of `STRC-1` needs
`ceil((15,120 + 100) / 500)` = 31 workers, not 31 + 1 = 32.

**Assembly and unassembly never pool with each other.** An entity assembling
100 MU and unassembling 100 MU needs 2 workers, not 1: each total rounds up on
its own.

**A shortage is a rate rather than a failure.** An order that asks for more
than the cadre can do this turn, or for more than the entity holds, does what
it can and says how much that was. It is not refused and it does not carry
over. The engine allocates the cadre without being told to.

The message reports **what the cadre had left when the order ran**, so a second
order that stopped short is answered by the number that explains it rather than
by the turn's whole rate.

## Orders that are accepted and not built yet

The twenty-seven orders below **parse**. A malformed one is refused at
submission with the same messages every other order gives, `ec orders help`
prints their forms, and ownership is still checked -- an order given to a ship
that is not yours is refused whether or not the verb is finished.

What none of them can be told yet is what to *do*. What a spy costs, what a work
group produces per turn, who the market's counterparty is, what control confers:
those are game rules that are not written, and guessing at them here would be
authoring the game rather than building it.

So each of them **fails when the turn resolves**, with

```text
VERB is accepted but not built yet; the rules it needs are not written
```

and the rest of the file is unaffected. A file that contains one is a file you
can still submit: the failure is a warning at submission and a failed order row
afterwards, which is where every other game-rule failure goes.

The same is true of two forms of orders that are otherwise built: the three
group forms of `CREATE`, and the two forms of `NAME` that name another faction
or its ships.

## ATTACK

```text
ship SHIP-ID attack (ship | colony) ID PERCENT%
colony COLONY-ID attack (ship | colony) ID PERCENT%
```

**Not built yet**, and resolves at stage 4 of the turn when it is.

An attack commits a share of the entity to a battle against another. The
commitment is a percentage from 1 to 100, and it is a share of the entity rather
than a count of anything.

Examples:

```text
colony 24 attack ship 18 75%
```

## INVADE

```text
ship SHIP-ID invade (ship | colony) ID PERCENT%
colony COLONY-ID invade (ship | colony) ID PERCENT%
```

**Not built yet**, and resolves at stage 4 of the turn when it is.

An invasion commits a share of the entity to taking another by landing on it,
rather than to destroying it.

Examples:

```text
colony 24 invade ship 18 55%
```

## RAID

```text
ship SHIP-ID raid (ship | colony) ID seeking UNIT, UNIT PERCENT%
colony COLONY-ID raid (ship | colony) ID seeking UNIT, UNIT PERCENT%
```

**Not built yet**, and resolves at stage 4 of the turn when it is.

A raid commits a share of the entity to taking named units off another. It
seeks one unit or two and no more: it is a snatch rather than a shopping
list.

Examples:

```text
ship 18 raid colony 24 seeking GOLD, FUEL 22%
```

## SUPPORT

```text
ship SHIP-ID support (ship | colony) ID attacking [(ship | colony) ID] PERCENT%
ship SHIP-ID support (ship | colony) ID defending [against (ship | colony) ID] PERCENT%
colony COLONY-ID support (ship | colony) ID attacking [(ship | colony) ID] PERCENT%
colony COLONY-ID support (ship | colony) ID defending [against (ship | colony) ID] PERCENT%
```

**Not built yet**, and resolves at stage 4 of the turn when it is.

Support commits a share of the entity to somebody else's battle, on one side or
the other. Naming the other side is optional either way: support given without
one is given against whoever turns up. The defending form says `against` first,
because otherwise `defending ship 33` would read as defending that ship rather
than defending from it.

Examples:

```text
ship 18 support ship 97 attacking 35%
ship 18 support ship 97 attacking colony 24 35%
ship 18 support colony 14 defending 40%
ship 18 support colony 14 defending against ship 33 45%
```

## RETOOL

```text
colony COLONY-ID retool factory-group GROUP-NO making UNIT
colony COLONY-ID retool immediately factory-group GROUP-NO making UNIT
```

**Not built yet**, and resolves at stage 7 of the turn when it is.

Retooling changes what a factory group makes. The plain form drains the
production line first, which may take three turns, and spends a turn retooling
after that. The `immediately` form discards the work in progress and spends the
retooling turn now. Either way production resumes a turn later.

Examples:

```text
colony 24 retool factory-group 3 making CNGD
colony 24 retool immediately factory-group 3 making FOOD
```

## IDLE

```text
ship SHIP-ID idle QUANTITY UNIT, ... in (factory-group | farm-group | mine-group) GROUP-NO
colony COLONY-ID idle QUANTITY UNIT, ... in (factory-group | farm-group | mine-group) GROUP-NO
```

**Not built yet**, and resolves at stage 8 of the turn when it is.

Idling stops units in a work group without taking them out of it. They keep
working until the work in progress drains out of them.

Examples:

```text
colony 24 idle 5,000 FACT-6 in factory-group 3
```

## REMOVE

```text
ship SHIP-ID remove QUANTITY UNIT, ... from (factory-group | farm-group | mine-group) GROUP-NO [and stow]
colony COLONY-ID remove QUANTITY UNIT, ... from (factory-group | farm-group | mine-group) GROUP-NO [and stow]
```

**Not built yet**, and resolves at stage 8 of the turn when it is.

Removal takes units out of a work group. It unassembles what it takes out and
may stow it, the same way an `UNASSEMBLE` order may, and the engine salvages
what it can of the work in progress.

Examples:

```text
colony 24 remove 12,000 FACT-6, 63 FACT-9 from factory-group 3 and stow
colony 24 remove 40,000 FARM-3 from farm-group 1
```

## ADD

```text
ship SHIP-ID add QUANTITY UNIT, ... to (factory-group | farm-group | mine-group) GROUP-NO
colony COLONY-ID add QUANTITY UNIT, ... to (factory-group | farm-group | mine-group) GROUP-NO
```

**Not built yet**, and resolves at stage 8 of the turn when it is.

Adding puts units into a work group and assembles them on the way, so no
separate `ASSEMBLE` is written for them. That is what makes it need construction
workers.

Examples:

```text
colony 24 add 63 FACT-9 to factory-group 3
```

## ACTIVATE

```text
ship SHIP-ID activate QUANTITY UNIT, ... in (factory-group | farm-group | mine-group) GROUP-NO
colony COLONY-ID activate QUANTITY UNIT, ... in (factory-group | farm-group | mine-group) GROUP-NO
```

**Not built yet**, and resolves at stage 8 of the turn when it is.

Activating sets idle units in a work group working again. Production in them
resumes at once.

Examples:

```text
colony 24 activate 5,000 FACT-6 in factory-group 3
```

## SELL

```text
ship SHIP-ID sell QUANTITY UNIT PRICE (GOLD | CNGD), ... [PERCENT%]
ship SHIP-ID sell tech-level TL-N PRICE GOLD [PERCENT%]
colony COLONY-ID sell QUANTITY UNIT PRICE (GOLD | CNGD), ... [PERCENT%]
colony COLONY-ID sell tech-level TL-N PRICE GOLD [PERCENT%]
```

**Not built yet**, and resolves at stage 11 of the turn when it is.

A sale is an offer of units, or of a technology level, at a price. Units must be
**unassembled** to be sold, which is what `UNSTOW` is for.

Every sale pays a commission to the market and there is no way to sell without
paying one. An order that names none pays the default the market sets; a seller
may offer more to make the offer likelier to be taken.

A technology level trades in the same market but is not cargo. It is paid for in
whole `GOLD`, never `CNGD`, needs no transports, and is bought once rather than
by quantity.

Examples:

```text
ship 18 sell 4,500 GOLD 1.0 CNGD
colony 24 sell tech-level TL-4 800,000 GOLD 5%
```

## BUY

```text
ship SHIP-ID buy QUANTITY UNIT PRICE (GOLD | CNGD), ... [PERCENT%]
ship SHIP-ID buy tech-level TL-N PRICE GOLD [PERCENT%]
colony COLONY-ID buy QUANTITY UNIT PRICE (GOLD | CNGD), ... [PERCENT%]
colony COLONY-ID buy tech-level TL-N PRICE GOLD [PERCENT%]
```

**Not built yet**, and resolves at stage 11 of the turn when it is.

A bid is an offer to take units, or a technology level, at a price. Purchased
units are stowed in the buyer's cargo.

A buyer may offer an additional commission to make the bid likelier to be
taken, and pays the extra themselves.

Examples:

```text
ship 18 buy 100 FOOD 3 CNGD
ship 18 buy tech-level TL-6 1,000,000 GOLD
```

## SURVEY

```text
ship SHIP-ID survey
colony COLONY-ID survey
```

**Not built yet**, and resolves at stage 12 of the turn when it is.

A survey reads the planet the entity is already at. Like a probe, it reads where
the entity stood when the turn began, because movement is later.

Examples:

```text
ship 18 survey
```

## ASSESS

```text
ship SHIP-ID assess rebels using QUANTITY spies
colony COLONY-ID assess rebels using QUANTITY spies
```

**Not built yet**, and resolves at stage 14 of the turn when it is.

Spends spies reading how rebellious a place is.

Examples:

```text
colony 24 assess rebels using 1 spies
```

## DETECT

```text
ship SHIP-ID detect spies using QUANTITY spies
colony COLONY-ID detect spies using QUANTITY spies
```

**Not built yet**, and resolves at stage 14 of the turn when it is.

Spends spies finding another faction's spies.

Examples:

```text
colony 24 detect spies using 4 spies
```

## OBTAIN

```text
ship SHIP-ID obtain information from (ship | colony) ID using QUANTITY spies
colony COLONY-ID obtain information from (ship | colony) ID using QUANTITY spies
```

**Not built yet**, and resolves at stage 14 of the turn when it is.

Spends spies reading another entity.

Examples:

```text
colony 24 obtain information from ship 18 using 200 spies
```

## CONVERT

```text
ship SHIP-ID convert rebels using QUANTITY spies
colony COLONY-ID convert rebels using QUANTITY spies
```

**Not built yet**, and resolves at stage 14 of the turn when it is.

Spends spies turning rebels back.

Examples:

```text
colony 24 convert rebels using 3 spies
```

## INCITE

```text
ship SHIP-ID incite rebels using QUANTITY spies
colony COLONY-ID incite rebels using QUANTITY spies
```

**Not built yet**, and resolves at stage 14 of the turn when it is.

Spends spies raising rebels somewhere.

Examples:

```text
colony 24 incite rebels using 21 spies
```

## NEUTRALIZE

```text
ship SHIP-ID neutralize faction FACTION-ID spies using QUANTITY spies
colony COLONY-ID neutralize faction FACTION-ID spies using QUANTITY spies
```

**Not built yet**, and resolves at stage 14 of the turn when it is.

Spends spies against another faction's spies. It is **not** combat -- it spends
spies rather than committing a share of an entity to a battle -- and it is named
`neutralize` so that combat and espionage do not both claim `attack`.

Examples:

```text
colony 24 neutralize faction 1 spies using 11 spies
```

## DRAFT

```text
ship SHIP-ID draft QUANTITY (SOL | CADRE), ...
colony COLONY-ID draft QUANTITY (SOL | CADRE), ...
```

**Not built yet**, and resolves at stage 16 of the turn when it is.

A draft makes soldiers or a cadre out of the population an entity already
carries. Only `SOL` and the cadres may be drafted; no other class can be.

Drafting `SOL` **changes the type of a population unit**, taking unskilled
workers off the rolls and making them soldiers one for one. Drafting a cadre
assigns population rather than converting it: one `CWKR` is one `SKW` plus one
`USK`, and they are skilled and unskilled workers still while they serve.

**A draft partially fills.** Short population is not an error.

Examples:

```text
ship 18 draft 4,250 CWKR
colony 24 draft 200 SOL, 1,000 CWKR
```

## DISBAND

```text
ship SHIP-ID disband QUANTITY (SOL | CADRE), ...
colony COLONY-ID disband QUANTITY (SOL | CADRE), ...
```

**Not built yet**, and resolves at stage 16 of the turn when it is.

Disband is the reverse of draft, and only the same things may be disbanded.
Disbanding `SOL` returns soldiers as unskilled workers, which is the pool a
draft took them from. Disbanding a cadre returns its population unchanged.

Examples:

```text
ship 18 disband 13 SOL
ship 18 disband 1,000 CWKR
```

## PAY

```text
ship SHIP-ID pay CLASS PERCENT%, ...
colony COLONY-ID pay CLASS PERCENT%, ...
```

**Not built yet**, and resolves at stage 17 of the turn when it is.

Sets what a class of population is paid, as a share of the standard rate. There
is no ceiling: a faction may overpay.

Examples:

```text
ship 18 pay USK 120%
colony 24 pay SKW 15%, USK 18%
```

## RATIONS

```text
ship SHIP-ID rations PERCENT%
colony COLONY-ID rations PERCENT%
```

**Not built yet**, and resolves at stage 17 of the turn when it is.

Sets what everyone aboard is fed, as a share of the standard ration. There is no
ceiling here either.

Examples:

```text
ship 18 rations 75%
colony 24 rations 130%
```

## RELEASE

```text
we release (ship | colony) ID
we release (X,Y,Z) system SYSTEM orbit ORBIT
```

**Not built yet**, and resolves at stage 20 of the turn when it is.

Releasing control is administrative, so it is a faction order and needs no
entity at the place: a faction may release a planet whose garrison is gone.

Examples:

```text
we release ship 18
we release (-1,2,3) system A orbit 5
```

## GRANT

```text
we grant trade (X,Y,Z) system SYSTEM orbit ORBIT station STATION-NO to faction FACTION-ID
we grant colonize (X,Y,Z) system SYSTEM orbit ORBIT to faction FACTION-ID
```

**Not built yet**, and resolves at stage 20 of the turn when it is.

Gives a faction leave to trade at a station, or to colonize a planet. A
permission granted this turn is in force from the next, which is why it resolves
here and not with the market.

Examples:

```text
we grant trade (-1,2,3) system A orbit 5 station 4 to faction 1
we grant colonize (-1,2,3) system A orbit 5 to faction 1
```

## REFUSE

```text
we refuse trade (X,Y,Z) system SYSTEM orbit ORBIT station STATION-NO to faction FACTION-ID
we refuse colonize (X,Y,Z) system SYSTEM orbit ORBIT to faction FACTION-ID
```

**Not built yet**, and resolves at stage 20 of the turn when it is.

Takes back a faction's leave to trade or to colonize. It is `GRANT` read the
other way round and has the same two forms.

Examples:

```text
we refuse trade (-1,2,3) system A orbit 5 station 4 to faction 1
we refuse colonize (-1,2,3) system A orbit 5 to faction 1
```

## CONTROL

```text
ship SHIP-ID control (ship | colony) ID
ship SHIP-ID control system SYSTEM orbit ORBIT
colony COLONY-ID control (ship | colony) ID
colony COLONY-ID control system SYSTEM orbit ORBIT
```

**Not built yet**, and resolves at stage 20 of the turn when it is.

Taking control is a **physical act**, so it is given to an entity that is at the
place. It fails against anything already controlled. It is downstream of
movement, so a ship that arrives this turn can take control of what it finds.

Examples:

```text
ship 18 control colony 24
colony 8 control system A orbit 5
```

## BROADCAST

```text
ship SHIP-ID broadcast system SYSTEM orbit ORBIT "MESSAGE" ["SIGNATURE"]
colony COLONY-ID broadcast system SYSTEM orbit ORBIT "MESSAGE" ["SIGNATURE"]
```

**Not built yet**, and resolves at stage 22 of the turn when it is.

Releases a message at a place for the news service to carry. The signature is
optional: a broadcast may be anonymous.

Examples:

```text
ship 18 broadcast system B orbit 8 "message" "optional signature"
```

## Checking and submitting

Check syntax, identity, ownership, turn, and destinations without changing the
database. The check runs the turn for real -- burning the fuel, moving the
ships, reading the planets -- and then rolls all of it back, so what it reports
is what the engine will do rather than a second opinion about it:

```text
ec --db-path games/beta orders check games/beta/orders/t0-f1-orders-v1.txt
```

Submit an order file:

```text
ec --db-path games/beta orders submit games/beta/orders/t0-f1-orders-v1.txt
```

Submission runs the same check and then, having rolled it back, atomically
replaces the faction's pending order rows for the current turn. An invalid
submission leaves the existing order set unchanged and stores nothing. Errors
include the source line number when applicable.

An order file is refused for anything that cannot change between now and the
turn resolving: a ship that is not yours, an entity of the wrong kind, an order
given to a subject that cannot be given it, a destination that does not exist,
a drive that cannot lift the ship, or a jump ordered from a planet. Anything
that can still change -- fuel above all -- is a warning, and the order is
stored.

## Reporting submitted orders

Show the orders currently stored for a faction:

```text
ecrpt --db-path games/beta show orders --game BETA-001 --faction 1
```

The player email may be used instead of the faction ID:

```text
ecrpt --db-path games/beta show orders --game BETA-001 --email user01@example.com
```

Use `--turn NUMBER` to review the retained orders from the previous turn after
the next turn has opened.

The report displays each order's input, fuel, status, starting location, final
location, and error message. Fuel is the one number that prices an order,
whether it is a move or a jump. Pending orders have no outcome locations, and
their fuel is what the order would burn. For a failed order, the starting and
final locations are identical and the fuel is zero.
