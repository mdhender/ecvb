# Unit Glossary

Unit tags with a technology level use the form `CODE-TL`, such as `FARM-2`.
Population and resources do not have technology levels.

The loader currently uses provisional mass and volume values for testing:

- A technology-level unit has a mass of \(2t\) MU and cargo volume of \(2t\)
  VU, where \(t\) is its technology level. `HDRV` is the exception: its mass is
  defined below, though its volumes are still provisional.
- A unit without a technology level has a mass of 6 MU and cargo volume of 6
  VU. The four bulk resources are the exception: `FUEL`, `GOLD`, `METL`, and
  `MNRL` each have a mass of 1 MU and a volume of 1 VU in every section.
- Operational inventory consumes twice its cargo volume. Component inventory
  consumes twice its operational volume. The bulk resources take 1 VU wherever
  they are held, so neither multiplier applies to them.
- Each population unit has a mass of 2 MU. Its cargo, operational, and component
  volumes are independent values, currently 2 VU each.
- One population unit represents 100 persons in census reports.

A **cadre** is not a unit. It is a temporary assignment of population that lets
the assigned units carry out orders they otherwise could not, so a cadre has no
mass or volume of its own. The cadres are `CWKR`, `LABR`, `PLCF`, `SPCF`, and
`TRNE`; they are listed below with the other codes because that is where a
reader looks one up. `CWKR` is required to execute an `assemble`. What the other
four permit, and which population may be assigned to each, is not settled.

`REBL` is neither a unit nor a cadre, and is listed below for the same reason
the cadres are. It is a percentage rather than a quantity, and nothing in the
schema holds it yet.

## `CNGD`

An inventory unit stored as cargo in the starting kit.

## `COPN`

**Open-air colony.** An entity located on a planet's surface in ring 0. Its
enclosed-space efficiency is 1. `GOLD`, `FUEL`, `METL`, and `MNRL` cargo is
stored in external depots and consumes no enclosed space.

A `COPN` may only be placed on a planet whose habitability number is greater
than 0, and **needs no `LFSU` at all**: the air outside is breathable, which is
what "open-air" means and what the efficiency of 1 is paying for. It is the one
entity that carries no life support and the one whose population is not capped
by it.

## `CORB`

**Orbital colony.** An entity located in ring 1 around a planet. Its
enclosed-space efficiency is 0.1. `GOLD`, `FUEL`, `METL`, and `MNRL` cargo is
stored in external depots and consumes no enclosed space.

## `CSFC`

**Surface colony.** An entity located on a planet's surface in ring 0. Its
enclosed-space efficiency is 0.2.

## `CWKR`

**Construction Worker cadre.** Required to execute an `assemble` order, and so
named by every `create` order, because a create assembles what it is given. One
`CWKR` is one `SKW` plus one `USK`.

**Its people are spoken for, and it cannot outlive them.** The `SKW` and `USK`
in a cadre are already counted in the entity's population -- that is what gives
a cadre its mass -- but they are not available to be given a second job. And if
the population falls below what the cadre assigned, the cadre falls with it: a
ship with 100 `SKW`, 100 `USK`, and 100 `CWKR` that loses three skilled workers
is left with 97 `SKW`, 100 `USK`, and 97 `CWKR`, the three unskilled workers
going back to being unassigned. A ship with only 50 `CWKR` loses nothing but the
workers. Taking the cadre and its people together -- which is what capturing or
disbanding one does -- is the same rule read the other way round, and waits on
combat and `disband`.

One unit does up to 500 MU of work a turn, where the work is the mass being
handled. Work of the same kind is **pooled across an entity**, so the workers it
needs are reckoned from one total rather than order by order; assembly and
unassembly are two pools that round up on their own and never pool with each
other. A worker does one task per turn, so the pool is drawn down as the turn
runs rather than refilled for each order.

**A shortage is a rate rather than a failure.** An order that asks for more than
the cadre can do this turn does what the workers paid for and says so; it is not
refused and nothing carries over. See [Order File Reference](orders.md).

A cadre is held in `entity_cadre`. Nothing forms or dissolves one yet -- that is
`draft` and `disband` -- so a starting kit is the only thing that assigns one.

## `FACT`

**Factory.** A technology-level inventory and work-group unit. A factory work
group does not work a resource deposit.

## `FARM`

**Farm.** A technology-level inventory and work-group unit. A farm work group
does not work a resource deposit.

## `FOOD`

An inventory unit without a technology level, stored as cargo in the starting
kit.

## `FUEL`

**Fuel resource.** A unit without a technology level, massing 1 MU and taking
1 VU in every section. Fuel cargo on `COPN` and `CORB` entities contributes
mass but consumes no enclosed space.

## `GOLD`

**Gold resource.** A unit without a technology level, massing 1 MU and taking
1 VU in every section. Gold cargo on `COPN` and `CORB` entities contributes
mass but consumes no enclosed space.

## `HDRV`

**Jump drive.** A technology-level component unit used by ships. Only units
assembled in component inventory propel a ship; `HDRV` held in any other
section is freight.

A unit at technology level \(t\) has a mass of \(45t\) MU and propels
\(1045t\) MU through a jump.

A ship's drive is the sum of its assembled units. The **lowest** technology
level installed is the level the whole drive runs at, because every unit has to
make the same jump. Capacity is the sum over the units of their own technology
levels, so a mixed drive still carries the mass its high-technology units can
propel. A ship carrying 10 `HDRV-1` and 3 `HDRV-2` runs at technology level 1
and propels \(10 \times 1045 \times 1 + 3 \times 1045 \times 2\) MU.

**Technology level does not limit how far a drive jumps.** Any ship can be sent
to any stellium in the game; what a long jump costs is fuel, and the fuel grows
with the distance. A jump always ends at a stellium; there is no deep space to
stop in.

A jump fails when the ship has no assembled drive, when its mass exceeds the
drive's capacity, or when the ship is at a planet rather than in the stellium
orbit -- a jump begins from the stellium orbit, so a ship at a planet has to
move out first.

The same units move a ship inside a stellium. A move fails when the ship has no
assembled drive or when its mass exceeds the drive's capacity. Distance never
enters into a move: every move inside a stellium is one of three fixed kinds.
See [Order File Reference](orders.md) for the move forms.

### Fuel

A drive burns 40 `FUEL` per assembled unit per light year jumped. Every
assembled unit draws: a ship cannot idle part of its drive to save fuel, so a
large drive is expensive to run even on a short hop. A ship with 21 assembled
units jumping 3 light years burns \(21 \times 3 \times 40 = 2520\) `FUEL`.

Distance inside a stellium is never measured. A move is one of three kinds,
each with a fixed cost per assembled unit:

| Kind | Fuel per unit | The fraction of a light year it stands for |
| --- | --- | --- |
| A hop: the stellium orbit to a planet, or two planets of one system | 4 | 0.1 |
| Crossing systems: planets of different systems of one stellium | 8 | 0.2 |
| Going nowhere | 0 | 0.0 |

Naming the kinds rather than measuring tenths keeps the arithmetic in whole
numbers everywhere: the fractions never reach the engine, the schema, or a
report. A ship with 21 assembled units hopping burns \(21 \times 4 = 84\)
`FUEL`.

Fuel is burned, not assembled, so it counts from any section. It is drawn
**operational** first, then **unassembled**, and **cargo** last, so a hold of
spare fuel survives until the working supply is gone. Burned fuel leaves the
ship: its mass falls by 1 MU per unit, `FUEL` being a bulk resource, and a
later order in the same turn measures the drive against the lighter ship.

A move or jump fails when the ship cannot pay. `orders check` and `orders
submit` only warn, because fuel may still reach the ship before the turn
resolves; the engine decides.

## `LABR`

**Laborer cadre.** What it permits is not settled.

## `LFSU`

**Life support.** A technology-level component unit. Only units assembled in
component inventory support anyone; `LFSU` held in any other section is freight.

A unit at technology level \(t\) has a mass of \(8t\) MU, takes \(8t\) VU
assembled and \(4t\) VU in cargo, and supports \(t^2\) population units. An
entity's capacity is the sum over its assembled units, so 5 `LFSU-3` support
\(5 \times 9 = 45\) population units, which is 4,500 persons.

Life support runs at 100% or not at all; it cannot be throttled to save fuel.
Each unit burns \(t\) `FUEL` per turn, so those 5 `LFSU-3` cost 15 `FUEL` a turn.

On a `SHIP`, `CORB`, or `CSFC`, life support has **first call on the entity's
fuel**, ahead of everything else it might be spent on. A `COPN` is open to the
air and is not on that list.

**Population dies back to capacity.** An entity carrying more population than
its assembled `LFSU` supports loses the excess. This is what makes life support
worth its mass, and it has two sharp edges. Only assembled units support anyone,
so **unassembling `LFSU` can kill people**. And a create order will not deliver
population to a new entity that cannot yet support it — see
[Accepted Orders](accepted-orders.md) — which is the gate that keeps a build
from walking into this.

A `COPN` is the exception to all of this. It sits on a planet with a habitability
number above 0, breathes the air outside, and neither carries `LFSU` nor has its
population capped by one.

Note that the assembled volume is twice the cargo volume rather than four times
it, so the general "component inventory consumes twice its operational volume"
rule does not apply to `LFSU`.

## `METL`

**Metal resource.** A unit without a technology level, massing 1 MU and taking
1 VU in every section. Metal cargo on `COPN` and `CORB` entities contributes
mass but consumes no enclosed space.

## `MINE`

**Mine.** A technology-level inventory and work-group unit. A mine work group
works a resource deposit.

## `MNRL`

**Mineral resource.** A unit without a technology level, massing 1 MU and taking
1 VU in every section. Mineral cargo on `COPN` and `CORB` entities contributes
mass but consumes no enclosed space.

## `NAS`

**Non-Assignable population.** Population that a faction cannot assign to work
in farms, mines, or factories.

## `PLCF`

**Police Force cadre.** What it permits is not settled.

## `REBL`

**Rebels.** The percentage of an entity's population that is discontent with the
faction controlling it and may engage in acts of rebellion. It runs from 0 to
99, so an entity is never wholly in rebellion. `docs/turn-sequence.md` puts
rebellion at stage 18 and rebel increases at stage 19, and three of the
espionage orders act on rebels.

## `SDRV`

**Orbital drive engine.** A technology-level component unit that holds a `SHIP`
or a `CORB` up in its orbit. Only units assembled in component inventory produce
thrust; `SDRV` held in any other section is freight.

A unit at technology level \(t\) has a mass of \(25t\) MU, takes \(25t\) VU
assembled, and produces \(3000t^2\) MU of thrust per turn. In cargo it takes
\(12.5t\) VU — the one volume in the game that is not a whole number per unit.
Reckon it over the whole stack and round up once, \(\lceil 25tn/2 \rceil\) VU
for \(n\) units, so the half is never stored.

One `SKW` unit crews 100 units that are **run**, not 100 that are installed, and
each unit that is run burns \(t\) `FUEL` per turn. Spare drives therefore cost
nothing but their mass and volume to carry. The engine works out how many units
to run and assigns the `SKW` itself; neither is ordered.

Its call on that fuel depends on the ring it is in. In rings 1 through 10 it
comes second, behind life support and ahead of everything else; from ring 11 up
it comes last.

**None of that is drawn yet.** Nothing in the game today asks an entity to run
its drives, so assembled `SDRV` is mass and volume and nothing more. The fuel,
the crew, and the priority above all arrive with the rule below.

### Falling is a combat rule

A `SHIP` or `CORB` at a planet is meant to run enough units to produce thrust at
least equal to its own mass, and one that cannot **descends one ring a turn**
until it reaches ring 0 and is destroyed.

**That penalty belongs to combat and is not written yet.** It does not apply to
an entity under construction, and it is not a standing check made against every
entity every turn. An entity carrying too little `SDRV` for its mass is
under-powered rather than doomed, and stays that way until there is a combat
system to make it matter.

The ring-dependent fuel priority above is what makes the rule bite when it
arrives: an entity that has already been driven down to ring 10 has ten turns
left, and from there its drives outrank everything but life support.

As with `LFSU`, the assembled volume is twice the cargo volume rather than four
times it, so the general component-volume multiplier does not apply.

## `SHIP`

**Ship.** An entity located either at the stellium level or in a planetary ring
from 1 through 99. Starting ships are placed at their home planet in ring 64.
Its enclosed-space efficiency is 0.1.

## `SKW`

**Skilled Worker population.** Population that a faction can assign to operate
farms, mines, and factories or to crew ships and colonies.

## `SNSR`

**Sensor.** A technology-level component unit. Only units assembled in
component inventory sense anything; `SNSR` held in any other section is
freight.

A unit at technology level \(t\) has a mass of \(40t\) MU and launches \(t\)
probes per turn. An entity's probes are the sum over its units, so 5 `SNSR-1`
and 3 `SNSR-2` launch \(5 \times 1 + 3 \times 2 = 11\) probes in a turn.

Passive sensors are read after probes and before anything moves, so they report
from where the entity stands at the **start** of the turn. A ship that jumps
into a new stellium on turn 3 reports that stellium in its turn 4 report, not
its turn 3 report. Passive sensors report:

- In a stellium, they report the number of systems, and the orbit and kind of
  every planet in each of them.
- At a planet, they also report every `SHIP` and `CORB` orbiting any planet of
  that system, with the approximate mass of each.

**Approximate mass** is a mass reported as its order of magnitude in base 10,
rounded down. A ship of 1999 MU reads as 3. A passive reading carries only that
much precision; a probe reads exact masses and identities.

A probe is ordered with the `probe` verb by a ship or a colony, and reads one
planet, either in the entity's current system or in any system of its current
stellium. See [Order File Reference](orders.md).

## `SOL`

**Soldier population.** Population that a faction can assign to attack or
defend entities.

## `SPCF`

**Special Forces cadre.** What it permits is not settled.

## `STRC`

**Structural unit.** A technology-level unit. When assembled in component
inventory, each unit at technology level \(t\) creates \(t^2\) VU of raw
enclosed volume and consumes no enclosed space. In any other inventory category
it consumes enclosed space normally.

## `STRL`

**Structural unit.** It has the same enclosed-volume behavior as `STRC`.

## `TRAN`

**Transport.** A technology-level unit that moves units and population between
two entities at the same place.

A unit at technology level \(t\) has a mass of 4 MU — flat, not \(2t\) — and
takes \(4t\) VU both assembled and in cargo, so the usual component and
operational multipliers do not apply to it. In a turn one unit carries at most
\(20t^2\) MU **and** at most \(60t^2\) VU; both limits hold, and with today's
unit table it is nearly always the mass that binds.

One **unassigned** `SKW` unit operates up to 10 transports in a turn, allocated
by the engine rather than by the player. A skilled worker in a cadre is already
doing a job and does not crew a hull as well, so a ship with 100 `SKW` and 100
`CWKR` runs no transports at all and one with 50 `CWKR` runs 500. Fuel is reckoned over all the transports an entity
used, not one at a time, which keeps it in whole numbers:
\(\lceil \sum t^2 / 10 \rceil\) `FUEL`. Ten `TRAN-1` and five `TRAN-2` in one
turn cost \(\lceil (10 + 20)/10 \rceil = 3\) `FUEL`; a single `TRAN-1` costs 1.

**A transport goes there and comes back.** A turn's capacity covers the round
trip, so carrying a `CWKR` cadre out to a build site and home again at the end
of the shift is charged once, not twice. (Combat may destroy a transport on the
way; that is a combat rule and is not written yet.)

A `transfer` takes the fewest hulls that carry its load, highest technology
level first, and the fuel is reckoned over every hull the entity used in the
turn at once, so a second transfer that shares the round trip pays only what it
adds to the total. **A shortage fills the order partway rather than failing it.**
See [Order File Reference](orders.md).

The flat 4 MU and the \(4t\) VU above are not what the code weighs yet. The
mass and volume table in `internal/units` is still the provisional one this
document opens with, for `TRAN` as for every other unit, and correcting one unit
of it would change every kit built from it.

Fuel per MU moved does not depend on technology level: capacity is \(20t^2\) and
fuel is \(t^2/10\), so the ratio is flat. A better transport buys fewer hulls and
fewer crew, not cheaper freight.

## `TRNE`

**Trainee cadre.** Trainees and recruits. What it permits is not settled.

## `USK`

**Unskilled Worker population.** Population that a faction can assign to work
in farms, mines, and factories.

## Summary
Agents must ignore this summary. It is a work in progress.

|Code|Class     |Mass MU|Component VU|Cargo VU|
|----|----------|-------|------------|---------|
|STRC|Structural|1 × TL |-1          |1 × TL   |
|STRL|Structural|1 × TL |-1          |1 × TL   |
|HDRV|Propulsion|45 × TL|45 × TL     |22.5 × TL|
|SDRV|Propulsion|25 × TL|25 × TL     |12.5 × TL|
|NAS |Population|1      |N/A         |N/A      |
|SKW |Population|1      |N/A         |N/A      |
|SOL |Population|1      |N/A         |N/A      |
|USK |Population|1      |N/A         |N/A      |
|CWKR|Cadre     |2      |N/A         |N/A      |
