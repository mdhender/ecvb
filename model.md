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

### `agent`

| Column | Description |
| --- | --- |
| `id` | Primary key. |
| `description` | Human-readable description of the agent. |

Agents are implemented by the game engine. The data model stores no other agent
attributes.

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
| `enclosed_volume` | The entity's enclosed volume. |

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

### `inventory`

| Column | Description |
| --- | --- |
| `entity_id` | The entity holding the inventory. |
| `section` | One of `component`, `operational`, `unassembled`, or `cargo`. |
| `unit` | Unit code, such as `TRAN`. |
| `tech_level` | Required technology level, from 0 through 10. |
| `quantity` | Quantity held. |

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

## Order entry

### `order_entry`

`order_entry` holds only the orders submitted for the current turn. Orders are
replaced as a complete set for each faction and removed after the game advances.
The table does not retain order history.

| Column | Description |
| --- | --- |
| `game_id` | The game in which the order is issued. Required. |
| `faction_id` | The faction submitting the order. Required. |
| `sequence` | The order's deterministic sequence within the faction's submission. Required. |
| `entity_id` | The entity receiving the order. Required. |
| `verb` | Engine-defined order verb, such as `jump`, `bombard`, or `draft`. Required. |
| `target_entity_id` | Optional entity targeted by the order. |
| `support_entity_id` | Optional entity supported by the order. |
| `parameters` | Raw parameter text for the game engine. Required; defaults to an empty string. |

`(faction_id, sequence)` is the primary key. There is no `turn` column because
the table contains orders only for the current turn. `game_id` allows games to
advance and clear orders independently.

The database enforces the required columns and foreign keys. The game engine is
responsible for validating that:

- The faction belongs to the game.
- The issuing entity belongs to the submitting faction and game.
- Target and support entities are valid for the order.
- The verb is recognized.
- The parameters have the format required by the verb.
- The order satisfies verb-specific fit, ownership, range, and capability rules.

Replacing a faction's orders is atomic. In one transaction, the engine validates
the complete submission, deletes the faction's existing orders, inserts the new
set, and commits. A failure rolls back the replacement and preserves the prior
order set.

Advancing a game turn is also atomic. The engine processes the game's current
orders, applies their results, increments `game.turn`, deletes all
`order_entry` rows for that game, and commits the transaction.
