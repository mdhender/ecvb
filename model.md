# Proposed Data Model

This document describes the proposed game data model and its constraints.
Entity location rules are defined in [Entity Location](entity-location.md).

## Users and games

### `user`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `email` | User email address. Stored in lowercase and unique. |

### `game`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `code` | Game code. |
| `turn` | Required current turn number; defaults to 0. |
| `turn_state` | One of `open` or `resolved`; defaults to `open`. |
| `seed_high` | Required high half of the game's PCG seed; defaults to 19. |
| `seed_low` | Required low half of the game's PCG seed; defaults to 12. |

### `agent`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `code` | Stable engine-defined agent code; unique when set. |
| `description` | Human-readable description of the agent. |

Agents are implemented by the game engine. The `uncontrolled` agent generates
basic orders for entities that have no population.

### `faction`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `game_id` | The game containing the faction. |
| `user_id` | The user controlling the faction; null for an agent-controlled faction. |
| `agent_id` | The agent controlling the faction; null for a user-controlled faction. |

Exactly one of `user_id` and `agent_id` is set.

## Space

The spatial hierarchy is:

```text
game
└── stellium
    └── system
        └── planet
            └── deposit
```

The plural of *stellium* is *stellia*.

### `stellium`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `game_id` | The game containing the stellium. |
| `x` | X coordinate, from -15 through 15. |
| `y` | Y coordinate, from -15 through 15. |
| `z` | Z coordinate, from -15 through 15. |

The distance between stellia is their three-dimensional Euclidean distance,
rounded up to the next integer. For stellia at \((x_1, y_1, z_1)\) and
\((x_2, y_2, z_2)\), the distance is:

\[
\left\lceil
  \sqrt{(x_2-x_1)^2 + (y_2-y_1)^2 + (z_2-z_1)^2}
\right\rceil
\]

### `system`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `stellium_id` | The stellium containing the system. |
| `sequence` | System sequence, from `A` through `E`. |

`sequence` is unique within a stellium.

### `planet`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `system_id` | The system containing the planet. |
| `orbit` | Orbit number, from 1 through 10. |
| `kind` | One of `rocky`, `asteroid`, `gas-giant`, or `ice-giant`. |
| `habitability` | Habitability rating, from 0 through 25. |
| `faction_id` | The faction controlling the planet. |

`orbit` is unique within a system. Orbit numbers are sparse: a system does not
necessarily contain a planet in every orbit.

### `deposit`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `planet_id` | The planet containing the deposit. |
| `sequence` | Deposit sequence, from 1 through 45. |
| `resource` | One of `fuel`, `gold`, `metals`, or `minerals`. |
| `quality` | Yield percentage. |
| `initial_qty` | Initial resource quantity. |
| `current_qty` | Current resource quantity. |

`sequence` is unique within a planet.

## Entities

### `entity`

An entity's `unit` code identifies it as a ship or as one of the three colony
types.

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `unit` | One of `SHIP`, `COPN`, `CSFC`, or `CORB`. |
| `tech_level` | Required technology level, from 0 through 10. |
| `stellium_id` | The stellium containing the entity. Required. |
| `system_id` | The entity's system when it is at a planet; otherwise null. |
| `planet_id` | The entity's planet; optional for a ship and required for a colony. |
| `planet_ring` | The entity's ring at its planet; null when a ship is at the stellium level. |
| `faction_id` | The faction controlling the entity. |
| `enclosed_volume` | Raw volume enclosed by assembled structural components. |
| `mass` | Total mass of the entity's population and inventory. |

| Unit | Entity type |
| --- | --- |
| `SHIP` | Ship. |
| `COPN` | Open-air colony. |
| `CSFC` | Surface colony. |
| `CORB` | Orbital colony. |

The location columns must identify one valid location in the hierarchy. A
selected system and planet must belong to the selected stellium, and a selected
planet must belong to the selected system. See
[Entity Location](entity-location.md) for the unit-specific location rules.

Every entity has a controlling faction. An entity with population is controlled
by its player's faction. An entity without population is controlled by the
game's `uncontrolled` agent faction.

Only `STRC` and `STRL` inventory in the `component` section creates enclosed
volume. A structural unit at technology level \(t\) encloses \(t^2\) VU. Usable
enclosed space is the raw enclosed volume multiplied by the entity efficiency
and rounded down. The efficiencies are 1 for `COPN`, 0.2 for `CSFC`, and 0.1
for `CORB` and `SHIP`.

### `inventory`

| Column | Description |
| --- | --- |
| `entity_id` | The entity holding the inventory. |
| `section` | One of `component`, `operational`, `unassembled`, or `cargo`. |
| `unit` | Unit code, such as `TRAN`. |
| `tech_level` | Required technology level, from 0 through 10. |
| `quantity` | Quantity held. |

Assembled `STRC` and `STRL` components create enclosed volume and do not consume
enclosed space. Structural units in other inventory sections consume space like
other inventory. `GOLD`, `FUEL`, `METL`, and `MNRL` cargo on `COPN` and `CORB`
is stored in external depots: it contributes mass but consumes no enclosed
space.

### `entity_population`

| Column | Description |
| --- | --- |
| `entity_id` | The entity containing the population. |
| `class` | One of `USK`, `SKW`, `SOL`, or `NAS`. |
| `quantity` | Population units; one unit represents 100 persons. |

`(entity_id, class)` is the primary key. Population contributes to entity mass
and occupied enclosed space. Census reports multiply `quantity` by 100.

## Work groups

### `work_group`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `entity_id` | The entity containing the work group. |
| `unit` | One of `FACT`, `FARM`, or `MINE`. |
| `sequence` | Sequence from 1 through 99. |
| `deposit_id` | The deposit worked by a mining group; nullable. |

`sequence` is unique by `(entity_id, unit, sequence)`.

The game engine requires `deposit_id` for a `MINE` group and requires it to be
null for `FACT` and `FARM` groups. The database model does not enforce this
unit-specific rule.

### `work_group_units`

| Column | Description |
| --- | --- |
| `work_group_id` | The work group containing the units. |
| `tech_level` | Required technology level, from 0 through 10. |
| `quantity` | Number of units at the technology level. |

## Orders

Orders are stored in intent-specific tables. Each of `jump_order`,
`move_order`, and `probe_order` identifies the game, turn, faction, source line,
sequence, and ship. Their status
is one of `pending`, `succeeded`, or `failed`.

Resolved rows record the ship's complete location immediately before and after
the order. A failed order also records an error message, and its final location
equals its starting location.

### `jump_order`

A jump order stores the requested X, Y, and Z coordinates and the resolved
destination stellium ID. A successful jump clears the ship's system, planet,
and ring.

### `probe_order`

A probe order names the entity that probes, which may be a ship or a colony,
rather than a ship as `move_order` and `jump_order` do. It stores the requested
orbit, the optional requested system, and, once resolved, the stellium and
system the ship read from, the planet it read, and that planet's habitability.
One row records one probed orbit, so an order naming several orbits stores
several rows.

### `sensor_survey` and `sensor_contact`

Passive sensor readings are snapshotted when they are taken, which is after
probes and before anything moves. `sensor_survey` records, for each
sensor-equipped entity, the stellium and system it read from and how many
systems that stellium holds; the planets themselves are derived from the map,
which does not change. `sensor_contact` records each ship and orbital colony the
entity read, with its exact mass. Reports render those masses as approximate
masses.

An entity that moves or jumps during the turn keeps the reading it took before
it left, so a stellium entered this turn is first reported next turn.

### `probe_contact` and `probe_deposit`

A probe's findings are snapshotted when it resolves, because the ship may jump
away later in the same turn and the entities it saw may move. `probe_contact`
records every entity at the probed planet with its unit, ring, and exact mass.
`probe_deposit` records every deposit with its resource and quantity. Reports
render deposit quantities as approximate quantities.

### `move_order`

A move order stores the optional requested system letter, requested orbit, and
the resolved destination stellium, system, and planet IDs. A successful move
places the ship in ring 99.

Submission atomically replaces every kind of pending order for the faction and
current turn. Semantic validation follows engine resolution order: all probes
are validated, then all moves, then all jumps. The stored sequence records that resolution
order; the source line records the order's position in the submitted file.

Resolving a turn is atomic. The engine executes all probes, reads passive
sensors, then executes all moves and all jumps, updates entities and order
outcomes, and changes the game from `open` to
`resolved`. The turn number does not change until the gamemaster opens the next
turn. Opening the next turn retains the most recently resolved order rows and
purges older rows.
