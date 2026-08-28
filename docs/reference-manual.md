# Epimethean Challenge

## Copyright

Empyrean Challenge is owned by James Colombo.
The rules from the 1978 edition are used with his permission.

No part of this book may be reproduced in any form without his written permission.

## Introduction

Reference manual for Epimethean Challenge, a reboot of Empyrean Challenge.

This manual is written to the Diataxis style.

## The Map

A game's map is a set of stellia, fixed when the game is created. It does not
change.

### Stellia

A stellium is a point in space holding one through five systems. It has integer
coordinates x, y, z, each from -15 through 15. The plural of *stellium* is
*stellia*.

The distance between two stellia is their three-dimensional Euclidean distance,
rounded up to the next whole light year. For stellia at (x₁, y₁, z₁) and
(x₂, y₂, z₂):

    distance = ⌈ √((x₂-x₁)² + (y₂-y₁)² + (z₂-z₁)²) ⌉

A `jump` moves a ship between stellia and is the only movement that does. A
ship in a stellium but not at a planet is in the **stellium orbit**. A jump
departs from the stellium orbit and arrives in the destination's stellium
orbit.

The stellium orbit is not a planet and no planet occupies it. A `move` order
names it as orbit 11, which cannot be qualified with a system letter.

### Systems

A system is a group of planets within a stellium. Each is lettered `A` through
`E`, and the letter is unique within its stellium. A stellium holds one through
five of them.

A system holds planets. It holds nothing else: an entity is located at a
planet, never at a system.

### Planets

A planet occupies one orbit of a system, numbered 1 through 10. An orbit holds
at most one planet, and orbits are sparse — a system does not necessarily hold
a planet in every orbit.

Every planet has a kind and a habitability rating:

|Field       |Values                                          |
|------------|------------------------------------------------|
|Kind        |`rocky`, `asteroid`, `gas-giant`, or `ice-giant`|
|Habitability|0 through 25                                    |

A `COPN` may be placed only on a planet whose habitability is above 0.

#### Rings

An entity at a planet occupies one of its rings. Ring 0 is the planet's
surface; rings 1 through 99 are orbits around it.

|Ring|Occupied by      |
|----|-----------------|
|0   |`COPN` and `CSFC`|
|1   |`CORB`           |
|1–99|`SHIP`           |

A ship arriving at a planet under its own power draws a ring from 2 through 99,
leaving ring 1 to orbital colonies. A ship ordered to the planet it is already at stays there and draws a
new ring.

#### Deposits

A planet holds deposits of raw resource, numbered 1 through 45. The number is
unique within the planet.

|Field   |Values                                                |
|--------|------------------------------------------------------|
|Resource|`fuel`, `gold`, `metals`, or `minerals`               |
|Quality |Yield percentage                                      |
|Quantity|What the deposit held initially, and what it holds now|

A `MINE` work group works one deposit.

## Entities

### Inventory

An entity holds units in four sections. Cargo and unassembled inventory are
freight and take the same volume. Assembling a unit moves it into operational
or component inventory and costs volume: a unit occupies twice its freight
volume in operational inventory and four times its freight volume in component
inventory. The unit code determines which of the two sections a unit assembles
into. An order cannot choose the section.

### Enclosed Volume

`STRC` and `STRL` assembled in component inventory each enclose TL × TL VU of
raw volume. They are the only units that enclose volume.

An entity can use a fraction of the raw volume it encloses. The fraction is a
property of the entity kind:

|Kind|Entity         |Usable fraction|
|----|---------------|---------------|
|COPN|Open-air colony|1              |
|CSFC|Surface colony |1/5            |
|CORB|Orbital colony |1/10           |
|SHIP|Ship           |1/10           |

Everything an entity holds occupies its usable enclosed volume, with three
exceptions:

- `STRC` and `STRL` assembled in component inventory occupy none. Held in any
  other section they occupy the volume Appendix A gives.
- `FUEL`, `GOLD`, `METL`, and `MNRL` held as cargo on a `COPN` or a `CORB`
  occupy none.
- `STRC` and `STRL` held as cargo on an entity under construction occupy none.

An entity cannot hold more than its usable enclosed volume.

## Population

Population is people. It is counted in units of 100 persons. A unit of any
class masses 2 MU and occupies 2 VU in every inventory section.

Population is never assembled. A `transfer` order moves it between two entities
at the same place, and a `create` order delivers it to a new entity.

### Classes

|Code|Class           |Assignable to                                                                 |
|----|----------------|------------------------------------------------------------------------------|
|NAS |Non-assignable  |Nothing                                                                       |
|SKW |Skilled worker  |Operating farms, mines, and factories; crewing ships, colonies, and transports|
|SOL |Soldier         |Attacking and defending entities                                              |
|USK |Unskilled worker|Working farms, mines, and factories; moving freight                           |

### Assignment

Population is either unassigned or assigned to a cadre. Only unassigned
population may be given work, and only unassigned population counts towards
production labour and towards crewing transports.

Assigning population to a cadre does not move it. The people remain in the
entity's population and carry their mass there.

### Cadres

A cadre is a temporary assignment of population that lets an entity carry out
work it otherwise could not. The cadres are `CWKR`, `PLCF`, `SPCF`, and `TRNE`.
A cadre has no mass and no volume of its own and is held in no inventory
section.

A cadre cannot outlive the population it assigns. When an entity's population
falls, its cadres fall with it, and the population they were paired with
returns to being unassigned. A ship with 100 `SKW`, 100 `USK`, and 100 `CWKR`
that loses three skilled workers is left with 97 `SKW`, 100 `USK`, and 97
`CWKR`. The same ship with 50 `CWKR` loses nothing but the three workers.

#### CWKR

One `CWKR` assigns one `SKW` and one `USK`.

One `CWKR` does 500 MU of work a turn, measured as the mass handled. A worker
does one task per turn. Assembly and unassembly are separate pools: each is
rounded up to whole workers on its own, and neither draws on the other's
remainder. An entity assembling 100 MU and unassembling 100 MU needs two
workers.

An order that asks for more work than the cadre can do this turn does what the
workers cover and no more. It does not fail for want of them.

#### PLCF

Not yet specified.

#### SPCF

Not yet specified.

#### TRNE

Not yet specified.

### Production Labour

Production labour is an entity's unassigned `USK` plus TL for every assembled
`AUTO` it carries. Every kind of unskilled work is measured against it. A ship
with 75 unassigned `USK` and 150 assembled `AUTO-3` has a production labour
pool of 75 + 150 × 3 = 525.

Only assembled `AUTO` counts; an `AUTO` held in any other section is freight.
An `AUTO` is never assigned into a cadre or a group, and no order puts one in
or takes one out. Automation is not population: it is not fed, does not appear
in a census, and cannot fill the `USK` of a `CWKR`.

One unit of production labour moves 500 MU of freight a turn and does one task
per turn. Stowing and unstowing are separate pools and round up on their own,
as assembly and unassembly do for a `CWKR`. Neither pool draws on the
construction workers.

An order that asks for more freight than the labour can move this turn moves
what the labour covers and no more.

### Transports

One unassigned `SKW` unit operates 10 transports in a turn. A skilled worker
assigned to a cadre crews no hull. The engine allocates the crew; no order
names it.

### Life Support

An entity's assembled `LFSU` supports TL × TL population units per unit. The
capacity is the sum over its assembled units, so 5 `LFSU-3` support 45
population units, which is 4,500 persons. Only assembled units support anyone;
`LFSU` held in any other section is freight.

A `COPN` breathes the air outside. It carries no `LFSU` and its population is
not capped by one.

A `create` order will not deliver population to an entity that cannot support
it.

## Orders

A faction submits one order file for one turn. Every order names its subject
first and then what the subject is told to do.

    ship 482137 jump to (-1,2,3)
    colony 719042 probe orbit 5
    we name (-1,2,3) "Stellium Joe"

A subject is one of the faction's ships, one of its colonies, or `we`, which is
the faction itself and takes no id. An order given to a subject that may not be
given it is refused.

An entity's id is the six-digit number the game gave it. It is printed in every
report, it belongs to the entity for as long as the entity exists, and it is
never given to anything else. It is not a count of anything: the third entity
built in a game is no more likely to be numbered 3 than 800,000, which is
deliberate -- an id an opponent can see would otherwise tell them how much has
been built. A faction's id is a small number counted from 1 within the game, and
it is on every report too. Each order below names the subjects it accepts and the
phase it resolves in; see Turn Sequence.

### The Order File

The first line names the game and turn. The second identifies the player or
faction. Both are required.

    game "BETA-001" turn 0
    id player "user01@example.com"

    game "BETA-001" turn 0
    id faction 1

`player` and `faction` name the same thing. An email address is trimmed and
lowercased before it is looked up. The player or faction must belong to the
game. A file holding only those two lines is an empty order set.

Keywords and system letters are case-insensitive. A game code is matched
exactly. An id is a positive integer.

Blank lines are permitted after the identity line. A `#` outside quotes begins a
comment that runs to the end of the line.

    # scout the neighbouring system
    ship 240118 probe system B orbit 4    # before anything moves

A quantity is a whole number above zero. Above 999 it separates every three
digits with a comma: `5,000` is accepted and `5000` is refused. A comma also
separates the items of a list.

A unit code is a code on its own, such as `GOLD`, or a code and a technology
level, such as `LFSU-7`.

Quoted text is closed on the line it opens. A quote with no closing `"` is
refused where it is written.

### ASSEMBLE

Given to a ship or a colony. Resolves in the assemble phase.

    ship SHIP-ID assemble QUANTITY UNIT, QUANTITY UNIT, ...
    colony COLONY-ID assemble QUANTITY UNIT, QUANTITY UNIT, ...

Puts unassembled units to work.

    ship 482137 assemble 6,000 SNSR-1
    colony 719042 assemble 5 LFSU-1, 60 STRL-1

It draws from unassembled inventory first and from cargo after it. The unit code
determines the section a unit is assembled into; the order does not say. `HDRV`,
`SDRV`, `SNSR`, `LFSU`, `STRC`, and `STRL` assemble into component inventory and
everything else into operational inventory. Resources, population, and cadres
cannot be assembled, and naming one is an error in the file.

A `CWKR` cadre does the work at 500 MU a turn. An order that asks for more than
the cadre can do this turn, or for more than the entity holds, does what it can
and reports how much that was. It is not refused and nothing carries over.

An assemble that would leave the entity holding more than its usable enclosed
volume fails, and nothing moves.

### CREATE

Given to a ship or a colony. Resolves in the create phase.

    ship SHIP-ID create ship using ... transfering ... with QUANTITY CWKR end
    ship SHIP-ID create (open-air | enclosed | orbital) colony [as trade-station] using ... transfering ... with QUANTITY CWKR end
    colony COLONY-ID create ship using ... transfering ... with QUANTITY CWKR end
    colony COLONY-ID create (open-air | enclosed | orbital) colony [as trade-station] using ... transfering ... with QUANTITY CWKR end

Begins building a ship or a colony.

A create is a commitment rather than a purchase. It succeeds the moment it is
given, and the build runs for as many turns as it needs. Everything after that
is a rate: a build that gets no materials, no transport, or no workers does not
move that turn, and nothing carries over against it.

It is the one order that may run over several lines, so it is terminated by
`end`. Line breaks and spacing inside it mean nothing.

    ship 482137 create ship
      using 60 STRC-8,
            61 HDRV-1, 5 SDRV-1
            , 5 LFSU-3, 1 SNSR-1
      transfering 25 FOOD, 5 SKW, 16,800 FUEL, 93 GOLD
      with 500 CWKR
    end

All three clauses are required and each names at least one line.

|Clause       |Names                                                 |
|-------------|------------------------------------------------------|
|`using`      |what the new entity is made of; the build assembles it|
|`transfering`|what is handed over: cargo and population             |
|`with`       |a ceiling on the construction workers a turn may use  |

`with` holds nothing back. Each turn the engine assigns the build up to that many
workers from whatever the builder has idle, and never more. Within each clause,
the order the lines are written in is their priority.

A build's turn is three steps, hung on three phases:

|Phase       |What the build does                                             |
|------------|----------------------------------------------------------------|
|5, create   |claims what the builder holds and has not already promised      |
|13, transfer|delivers the claim on the builder's transports, with the workers|
|15, assemble|the workers assemble what is on site                            |

A build claims from the builder's cargo, which is the only section a transport
loads from. A claim lives one turn: what is not carried is released. Materials
are carried before workers. Where two builds of one entity compete, the older is
served first.

Until every structural `using` line is completed, only the `STRC` and `STRL`
lines are eligible. A population line is eligible only while the new entity's
assembled `LFSU` supports the people aboard and the people arriving. A line that
cannot make progress is skipped, and the build goes on to the next.

The new entity appears at the builder's planet, in the ring its kind requires,
and takes the technology level of the entity that created it. A ship created
from an entity in the stellium orbit is created there. A colony requires the
builder to be at a planet, and an open-air colony requires that planet's
habitability to be above 0; both are refused at submission.

The entity exists from the moment the order is given. It has a mass, and probes
and sensors read it. It can be given no order, and nothing but its own build may
deliver to it. Its progress is reported in the turn report's UNDER CONSTRUCTION
section.

`as trade-station` is accepted on any of the three colony kinds and recorded on
the entity. What it confers is not specified.

### JUMP

Given to a ship. Resolves in the jump phase.

    ship SHIP-ID jump to (X,Y,Z)

Sends a ship from the stellium orbit to another stellium.

    ship 240118 jump to (6,-9,8)

The coordinates must identify a stellium in the game. A jump begins from the
stellium orbit: a ship at a planet must `move to orbit 11` first, and may do
both in one turn because every `MOVE` resolves before any `JUMP`. Both are in
the last stage of the turn, so a ship carries out everything else it was told to
do before it goes anywhere.

Distance does not limit a jump. Any ship may be sent to any stellium in the
game. The ship's mass must be within its drive's capacity.

A jump burns 40 `FUEL` per assembled `HDRV` unit per light year. **The whole
bill is drawn on departure**, however many turns the crossing takes, so a ship
that cannot pay never leaves.

A crossing of *d* light years by a drive at technology level *t* takes *d* / *t*
turns, rounded up, and never fewer than one — **and it costs the ship every one
of them.** A ship that jumps on turn *N* is due on turn *N* + the turns the
crossing takes, and it is gone for every turn in between. The shortest crossing
there is costs a turn.

A ship in transit is nowhere: at no stellium, no system, and no planet. It
cannot be probed, does not appear on a sensor sweep, and can be given no order.
A crossing cannot be recalled, redirected, or cancelled. The turn report's
IN TRANSIT section gives its destination and the turn it is due.

**The turn it is due is not a turn it can be given orders in.** A ship lands in
the last phase of that turn, after every order has resolved, so it is still
nowhere when the turn's file is read. The report for that turn shows it at its
destination and it takes its next order the turn after.

A ship jumps once a turn. A second `JUMP` for the same ship in one file is
refused, and the whole file with it. The order is spent whatever it goes on to
do: a jump that failed for want of fuel has still been given.

### MOVE

Given to a ship. Resolves in the move phase.

    ship SHIP-ID move to orbit ORBIT
    ship SHIP-ID move to system SYSTEM orbit ORBIT

Moves a ship inside its stellium.

    ship 240118 move to orbit 6
    ship 240118 move to system B orbit 4
    ship 240118 move to orbit 11

The first form names a planet in the ship's current system, and the ship must
have one. The second names a planet in any system of the ship's current
stellium. Both place the ship at the destination planet in a ring drawn from 2
through 99. The draw is seeded from the game and the order, so re-resolving a
turn puts the ship in the same ring.

Orbit 11 is the stellium orbit. A move to it leaves the ship orbiting the
stellium with no system, planet, or ring. It may not be qualified with a system
letter.

The ship's drive moves it. A move fails when the ship has no assembled `HDRV`
units or when its mass exceeds their capacity. Distance does not enter into a
move: every move is one of three kinds, each costing a fixed amount of `FUEL`
per assembled `HDRV` unit.

|Move                                                 |Fuel per unit|
|-----------------------------------------------------|-------------|
|Stellium orbit to any planet of the stellium, or back|4            |
|Planet to planet in the same system                  |4            |
|Planet to the planet the ship is already at          |4            |
|Planet to planet in different systems of the stellium|8            |
|Stellium orbit to the stellium orbit                 |0            |

A failed move burns nothing. Ordering a ship in the stellium orbit to the
stellium orbit moves it nowhere and burns nothing. Ordering a ship to the planet
it is already at costs a hop and draws a fresh ring; it is the one way to change
a ship's ring without going anywhere.

A ship moves once a turn. A second `MOVE` for the same ship in one file is
refused, and the whole file with it. **The order is spent whatever it goes on to
do**: a move that failed for want of fuel, and a move to the stellium orbit from
the stellium orbit that went nowhere and burned nothing, have both been given,
and the ship gets no other move that turn.

A ship may `MOVE` and `JUMP` in one turn, and those are the only two journeys it
makes.

### NAME

Given to a ship, a colony, or `we`. Resolves in the naming phase.

    ship SHIP-ID name "NAME"
    colony COLONY-ID name "NAME"
    we name (X,Y,Z) "NAME"

Gives one of the faction's own ships or colonies, or a stellium, a name.

    ship 482137 name "Jalopy"
    colony 719042 name "Jingo"
    we name (-1,2,3) "Stellium Joe"

A name is private to the faction that gave it. Naming a ship does not change
what another faction's report calls it. A stellium may be named without having
been visited, but it must exist. Naming something again renames it.

Naming something the faction owns is an order to the thing itself, so the ship
or colony is the subject. Naming a stellium is a faction order, because no ship
or colony carries it out, so `we` is the subject.

A name is quoted text of at most 24 characters, counting spaces. It may not be
empty, may not begin or end with a space, may not hold two spaces in a row, and
may not hold control characters.

### PROBE

Given to a ship or a colony. Resolves in the probe phase.

    ship SHIP-ID probe orbit ORBIT ...
    colony COLONY-ID probe orbit ORBIT ...
    ship SHIP-ID probe system SYSTEM orbit ORBIT ...
    colony COLONY-ID probe system SYSTEM orbit ORBIT ...

Reads planets with an entity's sensors.

    ship 240118 probe orbit 6
    ship 240118 probe orbit 1 2 3 4 5 8 9 10
    ship 403556 probe system A orbit 1 2 3

One order may name several orbits and spends one probe on each. A probe that
names no system reads the system the entity is in, so a ship orbiting the
stellium must name one. A probe that names a system reads any system of the
entity's current stellium.

The entity must carry assembled `SNSR` units and have probes left this turn. It
launches one probe per technology level of each assembled `SNSR`. Each named
orbit must hold a planet of the probed system.

Probes resolve before anything moves, so a probe reads the system the entity was
in at the start of the turn. A ship cannot move into a system and probe it in
the same turn.

A probe reports, for the planet it reads: every ship, orbital colony, and
surface colony there, each with its identity and exact mass; every deposit, with
its resource and approximate quantity; and the planet's habitability.

A probe burns no `FUEL` and does not move its entity.

### STOW

Given to a ship or a colony. Resolves in the stow phase.

    ship SHIP-ID stow QUANTITY UNIT, QUANTITY UNIT, ...
    colony COLONY-ID stow QUANTITY UNIT, QUANTITY UNIT, ...

Moves units out of unassembled inventory into cargo.

    ship 482137 stow 18,000 FOOD, 800 HDRV-1

Units must be in cargo to be transferred, so a `STOW` readies a load for a
`TRANSFER`. It is not needed to assemble anything: `ASSEMBLE` draws from cargo
as well.

It neither assembles nor unassembles, so it reaches the four resources, which an
`ASSEMBLE` never can. Population cannot be stowed, and neither can a cadre;
naming either is an error in the file.

Production labour does the work at 500 MU a turn. A stow that asks for more than
the entity holds, or for more than its production labour moves this turn, moves
what it can and reports how much that was.

A stow that would leave the entity holding more than its usable enclosed volume
fails, and nothing moves. Only `AUTO` can bring that about.

### TRANSFER

Given to a ship or a colony. Resolves in the transfer phase.

    ship SHIP-ID transfer QUANTITY UNIT, ... to ship SHIP-ID
    ship SHIP-ID transfer QUANTITY UNIT, ... to colony COLONY-ID
    colony COLONY-ID transfer QUANTITY UNIT, ... to ship SHIP-ID
    colony COLONY-ID transfer QUANTITY UNIT, ... to colony COLONY-ID

Hands units or population to another entity at the same place.

    ship 482137 transfer 500 SOL to colony 719042
    ship 482137 transfer 4,500 GOLD, 18,000 FOOD to colony 719042

The recipient must be the faction's own entity or an uncontrolled one, and the
two must be at the same stellium, system, and planet when the order runs. They
may be in different rings. A transfer to an entity somewhere else fails.

Units must be in cargo to be transferred and are set down in the recipient's
cargo. The recipient may assemble them in the same turn, because `ASSEMBLE`
resolves after every transfer. Population moves the same way. A cadre cannot be
transferred; transfer the population instead.

The sending entity's transports carry the load and it pays their fuel. One
`TRAN` at technology level *t* has two separate capacities for the turn:
**20*t*² MU of mass and 60*t*² VU of volume.** A load must fit under both.
Exceeding either one stops it, whatever room is left under the other.

A `TRAN-1` carries 20 MU and 60 VU. It takes a load of 10 MU and 60 VU, which
fills the volume exactly and uses half the mass. It does not take a load of 21
MU and 8 VU, which is one MU over on mass with 52 VU to spare.

Capacities add across the hulls a transfer uses. A transport goes there and
comes back for one charge, and one unassigned `SKW` unit crews ten of them. The
fuel is reckoned over every transport the entity used in the turn at once, so a
second transfer sharing the round trip pays only what it adds.

A transfer that asks for more than is in cargo, or for more than the transports
carry, moves what it can and reports how much that was. A transfer that cannot
pay its transports' fuel fails.

### UNASSEMBLE

Given to a ship or a colony. Resolves in the unassemble phase.

    ship SHIP-ID unassemble QUANTITY UNIT, QUANTITY UNIT, ...
    colony COLONY-ID unassemble QUANTITY UNIT, QUANTITY UNIT, ...
    ship SHIP-ID unassemble and stow QUANTITY UNIT, QUANTITY UNIT, ...
    colony COLONY-ID unassemble and stow QUANTITY UNIT, QUANTITY UNIT, ...

Takes working units apart and returns them to unassembled inventory.

    ship 482137 unassemble 1,000 SNSR-1
    colony 719042 unassemble and stow 60 STRL-1, 5 LFSU-1

It is lossless: what comes apart is what went together.

`and stow` puts the units down in cargo instead. It costs no more than a plain
`unassemble`: the whole order is charged to the construction workers and none of
it to production labour.

The same `CWKR` cadre does the work at the same rate as assembling. Assembly and
unassembly are separate pools and round up on their own.

An unassemble that would leave the entity holding more than its usable enclosed
volume fails, and nothing moves. Unassembling `STRC` or `STRL` takes enclosed
volume away at the same time as the units coming apart need somewhere to go.

Unassembling `LFSU` reduces what the entity supports.

### UNSTOW

Given to a ship or a colony. Resolves in the unstow phase.

    ship SHIP-ID unstow QUANTITY UNIT, QUANTITY UNIT, ...
    colony COLONY-ID unstow QUANTITY UNIT, QUANTITY UNIT, ...

Moves units out of cargo into unassembled inventory.

    colony 719042 unstow 800 HDRV-1, 18,000 FOOD

It reaches the same units a `STOW` does and refuses the same two. It is not
needed to assemble anything.

Unstowing resolves after every transfer, so what a transfer set down this turn
can be unstowed in the same turn.

Production labour does the work at 500 MU a turn. Stowing and unstowing are
separate pools: one unit of production labour does one task a turn, so labour
that stowed cannot also unstow.

An unstow that asks for more than the entity holds, or for more than its
production labour moves this turn, moves what it can and reports how much that
was. An unstow that would leave the entity holding more than its usable enclosed
volume fails.

### Orders That Parse and Do Not Act

The parser accepts the whole accepted order set. Twenty-seven verbs are not
built: an order naming one is stored like any other and fails when the turn
resolves, with the reason. The rest of the file is unaffected.

    activate, add, assess, attack, broadcast, buy, control, convert,
    detect, disband, draft, grant, idle, incite, invade, neutralize,
    obtain, pay, raid, rations, refuse, release, remove, retool, sell,
    support, survey

Two built orders have forms that behave the same way:

- The group forms of `CREATE`: `factory-group`, `farm-group`, and `mine-group`.
- The forms of `NAME` that name another faction, or another faction's ship or
  colony.

### Checking and Submitting

Checking and submitting both run the turn against the game's live state and
report what would happen. Checking keeps nothing; submitting replaces the
faction's pending orders for that turn.

A file fails in one of two ways:

|Kind     |What causes it                                                                                                             |Effect                                                           |
|---------|---------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------|
|Rejection|What a turn cannot change: ownership, a destination that does not exist, a drive out of capacity, a second `MOVE` or `JUMP`|The whole file is refused and nothing is stored                  |
|Warning  |What a turn can change: fuel on hand, a budget another order spent                                                         |The order is kept, and fails at resolution if it still cannot run|

A rejection reports every reason, not the first.

An order that ran short of workers, labour, transports, or stock is neither. It
succeeded, did what it could, and reports how much that was.

## Turn Sequence

A turn resolves in phases. **Every order of one phase resolves before any order
of the next.** The order of the lines in a file decides only between orders of
the same phase.

The phases that carry a built order, in the order they happen:

|Phase|Name      |What resolves                                          |
|-----|----------|-------------------------------------------------------|
|5    |create    |every `CREATE`, then a build's claim                   |
|6    |unassemble|every `UNASSEMBLE`                                     |
|7    |stow      |every `STOW`                                           |
|13   |transfer  |every `TRANSFER`, then a build's delivery              |
|14   |unstow    |every `UNSTOW`                                         |
|15   |assemble  |every `ASSEMBLE`, then a build's assembly              |
|19   |probe     |every `PROBE`                                          |
|20   |sensor    |every assembled `SNSR` reads the sky; no order is given|
|34   |naming    |every `NAME`                                           |
|37   |move      |every `MOVE`                                           |
|38   |jump      |every `JUMP` departs                                   |
|39   |arrival   |every crossing that finished lands; no order is given  |

The numbers are positions in the turn's full list of 39 phases. Those not listed
carry no built order.

A phase's own work runs after its orders, so explicitly ordered work outranks a
standing commitment: a `TRANSFER` order is served before a build's claim, and an
`ASSEMBLE` order before a build's own assembly.

**Nothing moves until the end of the turn.** A ship carries out every other
order it was given where it began the turn, and only then moves, departs, or
lands. Two things follow. Probes and passive sensors read where things stood at
the start of the turn because everything reads before anything moves. And a
ship that lands this turn lands after the last order of it has resolved, so it
can be given no order until the next turn.

Departures settle before arrivals.

## Appendix A - Tables

### Units

A unit code either carries a technology level, written `CODE-TL`, or does not.
`TL` in the table below is the technology level of the unit.

|Class       |Code|Mass MU|Cargo VU|Unassembled VU|Assembled VU|Assembles into|
|------------|----|-------|--------|--------------|------------|--------------|
|Automation  |AUTO|4 × TL |2 × TL  |4 × TL        |4 × TL      |Operational   |
|Consumable  |CNGD|6      |6       |6             |12          |Operational   |
|Consumable  |FOOD|6      |6       |6             |12          |Operational   |
|Life support|LFSU|2 × TL |2 × TL  |2 × TL        |8 × TL      |Component     |
|Population  |NAS |2      |2       |2             |N/A         |N/A           |
|Population  |SKW |2      |2       |2             |N/A         |N/A           |
|Population  |SOL |2      |2       |2             |N/A         |N/A           |
|Population  |USK |2      |2       |2             |N/A         |N/A           |
|Propulsion  |HDRV|45 × TL|2 × TL  |2 × TL        |8 × TL      |Component     |
|Propulsion  |SDRV|2 × TL |2 × TL  |2 × TL        |8 × TL      |Component     |
|Resource    |FUEL|1      |1       |1             |N/A         |N/A           |
|Resource    |GOLD|1      |1       |1             |N/A         |N/A           |
|Resource    |METL|1      |1       |1             |N/A         |N/A           |
|Resource    |MNRL|1      |1       |1             |N/A         |N/A           |
|Sensor      |SNSR|40 × TL|2 × TL  |2 × TL        |8 × TL      |Component     |
|Structural  |STRC|2 × TL |2 × TL  |2 × TL        |8 × TL      |Component     |
|Structural  |STRL|2 × TL |2 × TL  |2 × TL        |8 × TL      |Component     |
|Transport   |TRAN|2 × TL |2 × TL  |2 × TL        |4 × TL      |Operational   |
|Work group  |FACT|2 × TL |2 × TL  |2 × TL        |4 × TL      |Operational   |
|Work group  |FARM|2 × TL |2 × TL  |2 × TL        |4 × TL      |Operational   |
|Work group  |MINE|2 × TL |2 × TL  |2 × TL        |4 × TL      |Operational   |

Exceptions to the section multipliers:

- `FUEL`, `GOLD`, `METL`, and `MNRL` occupy 1 VU in every section. They cannot
  be assembled. Held as cargo on a `COPN` or a `CORB` they occupy no enclosed
  space; they count towards the entity's mass in every section.
- `NAS`, `SKW`, `SOL`, and `USK` occupy 2 VU in every section and cannot be
  assembled. One unit is 100 persons in a census.
- `STRC` and `STRL` occupy no enclosed space when assembled in component
  inventory. See Enclosed Volume.
- `AUTO` occupies 2 × TL VU as cargo and 4 × TL VU unassembled. It is the only
  unit whose cargo and unassembled volumes differ.

## Appendix B - Glossary

**Cadre** — A temporary assignment of an entity's population to work it could
not otherwise carry out. The cadres are `CWKR`, `PLCF`, `SPCF`, and `TRNE`. A
cadre is not a unit: it has no mass and no volume, and is held in no inventory
section. The population a cadre assigns is counted in the entity's population
and carries its mass there.

**Cargo** — The inventory section holding freight. A transport sets its load
down in cargo.

**Component inventory** — The inventory section in which `STRC`, `STRL`,
`HDRV`, `SDRV`, `SNSR`, and `LFSU` do their work. A unit of those codes held in
any other section is freight.

**Enclosed volume** — The volume an entity encloses, created by its assembled
`STRC` and `STRL`. An entity can use a fraction of it, set by its kind, and
cannot hold more than it can use. See Enclosed Volume.

**Entity** — A ship or a colony. The kinds are `SHIP`, `COPN` (open-air
colony), `CSFC` (surface colony), and `CORB` (orbital colony). An entity holds
units, carries population, and is given orders.

**Freight** — A unit held in cargo or unassembled inventory. Freight does no
work.

**Mass Unit (MU)** — The unit of mass. Every unit an entity holds has a mass in
MU, and the total is what a jump drive must propel.

**Operational inventory** — The inventory section in which every assemblable
unit that is not a component unit does its work.

**Production labour** — An entity's unassigned `USK` plus TL for every
assembled `AUTO` it carries. Every kind of unskilled work is measured against
it. It is not a cadre: nothing is drafted for it and nothing is assigned into
it.

**Section** — One of the four places an entity holds a unit: cargo,
unassembled, operational, or component inventory.

**Technology Level (TL)** — An integer from 0 through 10 carried by a unit
code and written `CODE-TL`, as in `FARM-2`. It scales the unit's mass, its
volume, and what it does. Population and the four resources carry no technology
level and are written bare.

**Unassembled inventory** — The inventory section holding units that are not
assembled. A unit is assembled out of unassembled inventory and unassembled
back into it.

**Unit** — A quantity of one code, the thing an entity's inventory is counted
in. One population unit is 100 persons.

**Volume Unit (VU)** — The unit of volume. What one unit occupies depends on
the section it is held in; see Appendix A.
