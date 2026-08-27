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
| `stellium_id` | The stellium containing the entity. Null only for a `SHIP` in transit, which is nowhere. |
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

The location columns must identify one valid location in the hierarchy, or no
location at all. A selected system and planet must belong to the selected
stellium, and a selected planet must belong to the selected system. All four
columns are null for a ship crossing between stellia; nothing else may be
nowhere, because nothing else travels. See
[Entity Location](entity-location.md) for the unit-specific location rules.

Every entity has a controlling faction. An entity with population is controlled
by its player's faction. An entity without population is controlled by the
game's `uncontrolled` agent faction.

A unit is assembled out of `unassembled` inventory and into one of the two
working sections, and which one is a property of the unit code rather than
anything an order says. `HDRV`, `SNSR`, `SDRV`, `LFSU`, `STRC`, and `STRL` go to
`component`, which is the only section they work in; everything else goes to
`operational`. Resources, population, and cadres are never assembled.

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

### `entity_cadre`

| Column | Description |
| --- | --- |
| `entity_id` | The entity the cadre is assigned at. |
| `cadre` | One of `CWKR`, `LABR`, `PLCF`, `SPCF`, or `TRNE`. |
| `quantity` | How many are assigned. |

`(entity_id, cadre)` is the primary key. A cadre is a temporary assignment of
population rather than a unit, so it has no mass and no volume of its own: the
people in it are already counted in `entity_population`, and this records what
they have been assigned to do. One `CWKR` is one `SKW` plus one `USK`, so an
entity's `CWKR` count is bounded by both; the kit loader checks that, and
nothing else does yet, because nothing else forms or dissolves a cadre. The
`draft` and `disband` orders are what will.

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

### `in_transit`

A ship crossing between stellia. The crossing is not the order that began it:
the jump order departs and succeeds, drawing the whole fuel bill and taking the
ship off the board, and this row is what continues after it.

| Column | Description |
| --- | --- |
| `game_id` | The game the crossing belongs to. |
| `entity_id` | The ship, and the primary key: a ship makes one crossing at a time. |
| `destination_stellium_id` | The stellium the ship is bound for. |
| `arrival_turn` | The turn the crossing finishes. |

While the row stands the ship's location columns are all null, so it cannot be
probed, does not appear on a sensor sweep, and can be given no order. The
arrival step of ship movement (stage 15c) lands every ship due and deletes its
row. Nothing purges this table: a crossing is live state, not turn history.

## Orders

Every order a player writes is a row of `game_order`, whatever its verb.

### `game_order`

| Column | Description |
| --- | --- |
| `game_id`, `turn`, `faction_id`, `sequence` | The order's identity. `sequence` is its place in the turn's resolution order. |
| `source_line` | Its position in the submitted file. |
| `verb` | Which order it is, lowercase: `move`, `jump`, `probe`. |
| `actor_entity_id` | The entity the order acts on; null for an order that acts on none. |
| `input` | The order rendered back in the words the player wrote. What the reports print and the engine log echoes. |
| `params` | Everything else the order said, as JSON, in the words the player wrote. |
| `fuel_spent` | The fuel the order would burn while it is pending, and the fuel it did burn once it resolved, which is zero for a failed order. |
| `status` | `pending`, `succeeded`, or `failed`. |
| `error_message` | Why a failed order failed. Null otherwise. |

`params` never holds an id. A `move` stores `{"orbit": 6}` or
`{"system": "B", "orbit": 4}`, a `jump` stores `{"x": 1, "y": 2, "z": 3}`, a
`probe` stores `{"orbits": [4]}`. The engine resolves those names against the
map again when the turn runs, so a name that no longer resolves is a failed
order rather than a corrupt row, and there is no id in the JSON for the
database to fail to enforce. The two ids worth enforcing are columns with
foreign keys on them: the faction plays this game, and the actor belongs to
that faction.

A CHECK ties the three statuses together: a failed order carries a non-empty
error message and burned no fuel, and a pending or succeeded one carries no
error message.

### `order_movement`

Where an order took its entity, for the orders that move one. Most do not and
have no row. The row is written once, when the turn resolves, and records the
complete location immediately before and after: stellium, and either a system,
planet, and ring together or none of the three. A trigger refuses a row whose
final location differs from its start when the order failed, because a failed
order goes nowhere.

A successful move to a planet places the ship in a ring drawn from 2 through
99; the draw is seeded from `game.seed_high`, `game.seed_low`, the turn, and
the order, so resolving a turn twice reaches the same rings. Distance inside a
stellium is not stored: it takes one of three fixed values, which the engine
reads off the start and destination systems, and fuel is the number a player
sees. A successful jump clears the ship's system, planet, and ring, because a
jump arrives in the destination's stellium orbit.

Orbit 11 is the stellium orbit, which no planet occupies. It resolves to no
system and no planet, and a move may not qualify it with a system letter.

### `order_survey`

What an order read, for the orders that read something. A probe writes one row
naming the stellium and system it read from, the planet it read, and that
planet's habitability. A probe that failed read nothing and has no row. One
probe order reads one orbit, so a line naming several orbits becomes several
orders and several rows.

### Submitting and resolving

Submission runs the turn against the database and rolls it back, then stores
the orders that run bound. It atomically replaces every pending order for the
faction and current turn, or stores nothing at all. Validation follows engine
resolution order, and the stored sequence records it.

Spending fuel deletes it from `inventory` and takes its mass off `entity.mass`,
so an entity's mass stays the total of its population and inventory.

Resolving a turn is atomic. The engine walks the turn's phases in order --
probe, sensor, move, jump, naming -- executing every order of one phase before any
order of the next, updating entities and order outcomes, and changing the game
from `open` to `resolved`. The turn number does not change until the gamemaster
opens the next turn. Opening the next turn retains the most recently resolved
order rows and purges older rows.

### `faction_name`

What a faction calls things.

| Column | Description |
| --- | --- |
| `game_id`, `faction_id` | Whose name it is. |
| `stellium_id`, `system_id`, `planet_id`, `entity_id` | What is named. A CHECK requires exactly one of the four. |
| `name` | At most 24 characters, trimmed, with no run of two spaces and nothing empty. |

A name is a label its owner reads, not a property of the thing named: naming a
ship does not change what anybody else's report calls it, and a place may be
named without ever having been visited. A partial unique index per subject kind
means one name per faction per thing, so naming something again renames it.

## Findings

What a faction has seen, recorded when it saw it. A finding is not an order and
outlives the order that produced it within the turn.

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
