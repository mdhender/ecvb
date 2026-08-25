# Unit Glossary

Unit tags with a technology level use the form `CODE-TL`, such as `FARM-2`.
Population and resources do not have technology levels.

The loader currently uses provisional mass and volume values for testing:

- A technology-level unit has a mass of \(2t\) MU and cargo volume of \(2t\)
  VU, where \(t\) is its technology level. `HDRV` is the exception: its mass is
  defined below, though its volumes are still provisional.
- A unit without a technology level has a mass of 6 MU and cargo volume of 6
  VU.
- Operational inventory consumes twice its cargo volume. Component inventory
  consumes twice its operational volume.
- Each population unit has a mass of 2 MU. Its cargo, operational, and component
  volumes are independent values, currently 2 VU each.
- One population unit represents 100 persons in census reports.

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

**Fuel resource.** A unit without a technology level. Fuel cargo on `COPN` and
`CORB` entities contributes mass but consumes no enclosed space.

## `GOLD`

**Gold resource.** A unit without a technology level. Gold cargo on `COPN` and
`CORB` entities contributes mass but consumes no enclosed space.

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

## `LFSU`

A technology-level component unit used by colonies and ships in the starting
kit.

## `METL`

**Metal resource.** A unit without a technology level. Metal cargo on `COPN`
and `CORB` entities contributes mass but consumes no enclosed space.

## `MINE`

**Mine.** A technology-level inventory and work-group unit. A mine work group
works a resource deposit.

## `MNRL`

**Mineral resource.** A unit without a technology level. Mineral cargo on
`COPN` and `CORB` entities contributes mass but consumes no enclosed space.

## `NAS`

**Non-Assignable population.** Population that a faction cannot assign to work
in farms, mines, or factories.

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

## `USK`

**Unskilled Worker population.** Population that a faction can assign to work
in farms, mines, and factories.
