# Entity Location

Every entity belongs to a stellium. An entity may also be located at a specific
planet in that stellium. The entity's `unit` code determines which location
states are valid.

## Location hierarchy

An entity location uses these columns:

| Column | Meaning |
| --- | --- |
| `stellium_id` | Required stellium containing the entity. |
| `system_id` | System containing `planet_id`; null at the stellium level. |
| `planet_id` | Planet containing the entity; null at the stellium level. |
| `planet_ring` | Ring occupied at the planet; null at the stellium level. |

When a planet is selected:

- `system_id`, `planet_id`, and `planet_ring` are all set.
- `planet_id` belongs to `system_id`.
- `system_id` belongs to `stellium_id`.

When no planet is selected, `system_id`, `planet_id`, and `planet_ring` are all
null.

## Entity units

| Unit | Entity type |
| --- | --- |
| `SHIP` | Ship. |
| `COPN` | Open-air colony. |
| `CSFC` | Surface colony. |
| `CORB` | Orbital colony. |

The `unit` code fully identifies the entity type; separate entity-kind and
colony-subtype fields are not used.

## Valid locations

| Unit | `planet_id` | `planet_ring` | Meaning |
| --- | --- | --- | --- |
| `SHIP` | Null | Null | The ship is orbiting the stellium. |
| `SHIP` | Set | 1–99 | The ship is in the specified ring at a planet. |
| `CORB` | Set | 1 | The colony is in orbit around a planet. |
| `CSFC` | Set | 0 | The colony is on the planet's surface. |
| `COPN` | Set | 0 | The colony is on the planet's surface. |

Ring `0` represents a planet's surface. Rings `1` through `99` represent
planetary orbits.

Colonies always have a planet. Ships may be located either at a planet or at the
stellium level.

## Ship movement

A ship has two forms of movement:

- A **jump** moves the ship between stellia. After the jump, the ship is at the
  destination stellium level: `system_id`, `planet_id`, and `planet_ring` are
  null.
- **In-stellium movement** moves the ship to a planet in its current stellium.
  The destination system, planet, and a ring from 1 through 99 are set together.

A ship must move to a specific planet before interacting with a location that
requires planetary presence.

## State constraints

The following combinations are invalid:

- An entity without a `stellium_id`.
- An entity with only some of `system_id`, `planet_id`, and `planet_ring` set.
- An entity whose system does not belong to its stellium.
- An entity whose planet does not belong to its system.
- A `SHIP` at a planet with a ring outside 1 through 99.
- A `COPN`, `CSFC`, or `CORB` entity without a planet.
- A `COPN` or `CSFC` entity outside ring 0.
- A `CORB` entity outside ring 1.
