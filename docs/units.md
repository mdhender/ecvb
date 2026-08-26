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

## `CNGD`

An inventory unit stored as cargo in the starting kit.

## `COPN`

**Open-air colony.** An entity located on a planet's surface in ring 0. Its
enclosed-space efficiency is 1. `GOLD`, `FUEL`, `METL`, and `MNRL` cargo is
stored in external depots and consumes no enclosed space.

## `CORB`

**Orbital colony.** An entity located in ring 1 around a planet. Its
enclosed-space efficiency is 0.1. `GOLD`, `FUEL`, `METL`, and `MNRL` cargo is
stored in external depots and consumes no enclosed space.

## `CSFC`

**Surface colony.** An entity located on a planet's surface in ring 0. Its
enclosed-space efficiency is 0.2.

## `CWKR`

**Construction Worker cadre.** Required to execute an `assemble` order, and so
named by every `create` order, because a create assembles what it is given.

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

A unit at technology level \(t\) has a mass of \(45t\) MU, jumps \(t\) units
of distance, and propels \(1045t\) MU through a jump.

A ship's drive is the sum of its assembled units. The **lowest** technology
level installed sets the range of the whole drive, because every unit has to
make the same jump. Capacity is the sum over the units of their own technology
levels, so a mixed drive still carries the mass its high-technology units can
propel. A ship carrying 10 `HDRV-1` and 3 `HDRV-2` jumps 1 unit and propels
\(10 \times 1045 \times 1 + 3 \times 1045 \times 2\) MU.

A jump fails when the ship has no assembled drive, when its mass exceeds the
drive's capacity, or when the distance exceeds the drive's range. A jump always
ends at a stellium; there is no deep space to stop in.

The same units move a ship inside a stellium. A move fails when the ship has no
assembled drive or when its mass exceeds the drive's capacity, but never for
range: the shortest-ranged drive reaches anywhere in a stellium. See
[Order File Reference](orders.md) for the move forms.

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
ship: its mass falls by 6 MU per unit, and a later order in the same turn
measures the drive against the lighter ship.

A move or jump fails when the ship cannot pay. `orders check` and `orders
submit` only warn, because fuel may still reach the ship before the turn
resolves; the engine decides.

## `LABR`

**Laborer cadre.** What it permits is not settled.

## `LFSU`

A technology-level component unit used by colonies and ships in the starting
kit.

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

## `SDRV`

A technology-level component unit used by colonies and ships in the starting
kit.

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

A technology-level operational inventory unit used by orbital colonies in the
starting kit.

## `TRNE`

**Trainee cadre.** Trainees and recruits. What it permits is not settled.

## `USK`

**Unskilled Worker population.** Population that a faction can assign to work
in farms, mines, and factories.
