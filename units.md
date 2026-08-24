# Unit Glossary

Unit tags with a technology level use the form `CODE-TL`, such as `FARM-2`.
Population and resources do not have technology levels.

The loader currently uses provisional mass and volume values for testing:

- A technology-level unit has a mass of \(2t\) MU and cargo volume of \(2t\)
  VU, where \(t\) is its technology level.
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

A technology-level component unit used by ships in the starting kit.

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

A technology-level component unit used by ships in the starting kit.

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
