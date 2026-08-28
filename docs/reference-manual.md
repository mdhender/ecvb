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

|Field       |Values                                            |
|------------|--------------------------------------------------|
|Kind        |`rocky`, `asteroid`, `gas-giant`, or `ice-giant`  |
|Habitability|0 through 25                                      |

A `COPN` may be placed only on a planet whose habitability is above 0.

#### Rings

An entity at a planet occupies one of its rings. Ring 0 is the planet's
surface; rings 1 through 99 are orbits around it.

|Ring |Occupied by      |
|-----|-----------------|
|0    |`COPN` and `CSFC`|
|1    |`CORB`           |
|1–99 |`SHIP`           |

A ship arriving at a planet under its own power draws a ring from 2 through 99,
leaving ring 1 to orbital colonies. A ship ordered to the planet it is already at stays there and draws a
new ring.

#### Deposits

A planet holds deposits of raw resource, numbered 1 through 45. The number is
unique within the planet.

|Field   |Values                                       |
|--------|---------------------------------------------|
|Resource|`fuel`, `gold`, `metals`, or `minerals`      |
|Quality |Yield percentage                             |
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

|Kind|Entity          |Usable fraction|
|----|----------------|---------------|
|COPN|Open-air colony |1              |
|CSFC|Surface colony  |1/5            |
|CORB|Orbital colony  |1/10           |
|SHIP|Ship            |1/10           |

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

|Code|Class            |Assignable to                                                                 |
|----|-----------------|------------------------------------------------------------------------------|
|NAS |Non-assignable   |Nothing                                                                       |
|SKW |Skilled worker   |Operating farms, mines, and factories; crewing ships, colonies, and transports|
|SOL |Soldier          |Attacking and defending entities                                              |
|USK |Unskilled worker |Working farms, mines, and factories; moving freight                           |

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

Order file layout...

List of implemented rules...

## Turn Sequence

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
