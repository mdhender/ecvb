# Entity Location

Every entity belongs to a stellium, with one exception: a ship crossing between
stellia is nowhere at all until it arrives. An entity may also be located at a
specific planet in its stellium. The entity's `unit` code determines which
location states are valid.

## Location hierarchy

An entity location uses these columns:

| Column | Meaning |
| --- | --- |
| `stellium_id` | The stellium containing the entity. Null only for a `SHIP` in transit. |
| `system_id` | System containing `planet_id`; null at the stellium level. |
| `planet_id` | Planet containing the entity; null at the stellium level. |
| `planet_ring` | Ring occupied at the planet; null at the stellium level. |

When a planet is selected:

- `system_id`, `planet_id`, and `planet_ring` are all set.
- `planet_id` belongs to `system_id`.
- `system_id` belongs to `stellium_id`.

When no planet is selected, `system_id`, `planet_id`, and `planet_ring` are all
null.

When a ship is in transit, all four columns are null. See
[the crossing](#a-ship-in-transit-is-nowhere) below.

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

| Unit | `stellium_id` | `planet_id` | `planet_ring` | Meaning |
| --- | --- | --- | --- | --- |
| `SHIP` | Null | Null | Null | The ship is crossing between stellia and is nowhere. |
| `SHIP` | Set | Null | Null | The ship is orbiting the stellium. |
| `SHIP` | Set | Set | 1–99 | The ship is in the specified ring at a planet. |
| `CORB` | Set | Set | 1 | The colony is in orbit around a planet. |
| `CSFC` | Set | Set | 0 | The colony is on the planet's surface. |
| `COPN` | Set | Set | 0 | The colony is on the planet's surface. |

Ring `0` represents a planet's surface. Rings `1` through `99` represent
planetary orbits.

Colonies always have a planet, and always have a stellium: a colony does not
travel. Ships may be at a planet, at the stellium level, or nowhere.

## Ship movement

A ship has two forms of movement:

- A **jump** moves the ship between stellia. It departs from the stellium level
  and takes \(\lceil d / t \rceil\) turns to cross _d_ light years at drive
  technology level _t_, never fewer than one. On arrival the ship is at the
  destination stellium level: `system_id`, `planet_id`, and `planet_ring` are
  null. In between it is nowhere.
- **In-stellium movement** moves the ship to a planet in its current stellium.
  The destination system, planet, and a ring are set together; a ship arriving
  under its own power draws a ring from 2 through 99, leaving ring 1 to orbital
  colonies. A move to orbit 11, the fictional stellium orbit, instead clears all
  three and returns the ship to the stellium level without leaving the stellium.
  A ship ordered to the planet it is already at stays there but draws a new
  ring; a ship already in the stellium orbit and ordered there is not touched.

A ship must move to a specific planet before interacting with a location that
requires planetary presence.

### A ship in transit is nowhere

While a crossing is under way the ship's `stellium_id` is null along with the
other three columns, and an `in_transit` row holds the destination stellium and
the turn the ship is due. Nothing can see the ship and no order can reach it:
a crossing cannot be recalled, redirected, or cancelled. The arrival step of
ship movement lands every ship due that turn in the destination's stellium orbit
and deletes its row.

A ship therefore makes one crossing at a time, since even the shortest crossing
occupies the turn it began in.

## State constraints

The following combinations are invalid:

- Any entity other than a `SHIP` without a `stellium_id`.
- An entity with a `system_id`, `planet_id`, or `planet_ring` but no
  `stellium_id`: a ship that is nowhere is nowhere in all four columns.
- An entity with only some of `system_id`, `planet_id`, and `planet_ring` set.
- An entity whose system does not belong to its stellium.
- An entity whose planet does not belong to its system.
- A `SHIP` at a planet with a ring outside 1 through 99.
- A `COPN`, `CSFC`, or `CORB` entity without a planet.
- A `COPN` or `CSFC` entity outside ring 0.
- A `CORB` entity outside ring 1.
