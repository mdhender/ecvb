# CLAUDE-01 Game Rules

A search-and-capture game played on the CLAUDE-01 map by the ten player
factions. The gamemaster generates every faction's orders, resolves the turn,
and adjudicates the result.

## Rules

1. A faction may move zero or one time per turn.
2. A faction may jump zero or one time per turn.
3. A faction may probe as often as its assembled sensors allow.
4. The winner is the first faction to place its ship in orbit around another
   faction's home planet.

The game is a draw after 15 turns.

## Adjudication

These readings resolve the rules against the engine's mechanics.

### Winning

A ship is in orbit around a planet when it is at that planet in a ring from 1
through 99. A move places a ship in ring 99, so any successful move onto
another faction's home planet wins.

Home planets are always orbit 4 of system A in a single-system stellium, so the
winning order is always:

```text
move ship SHIP-ID to system A orbit 4
```

The winner is evaluated from the end-of-turn entity state. A faction that
expects to win must not also jump that turn, because the engine resolves every
move before any jump and the jump would carry the ship back out.

### Information

Public knowledge, available to every faction:

- The coordinates of all 100 stellia, and how many systems each one holds.
  Homes only ever occur in single-system stellia, so a faction may restrict its
  search to those 90.
- Its own entities, orders, and order outcomes.

Everything else a faction learns, it learns from its sensors. There is no
lookup of another faction's home planet; a faction finds a home by travelling
to its stellium and reading it.

### Sensors

Both readings a faction gets are taken before anything moves, so both describe
where its entities stood at the **start** of the turn. A ship that jumps into a
new stellium on turn 3 reports that stellium in its turn 4 report, not its turn
3 report.

The starting kit fits the ship with 3 `SNSR-2` and the open-air home colony
with 2 `SNSR-1`, so a faction launches six probes from its ship and two from
its colony each turn. See [Unit Glossary](../../units.md) for the sensor rules
and [Order File Reference](../../orders.md) for the `probe` order.

**Passive readings** cost nothing and are taken every turn by every
sensor-equipped entity:

- In a stellium, they report the number of systems it holds, and the orbit and
  kind of every planet in each of them.
- At a planet, they also report every ship and orbital colony orbiting any
  planet of that system, with each mass rounded down to its order of magnitude
  in base 10. A ship of 55,226 MU reads as 4.

A passive reading is therefore enough to find the candidate planets in a
stellium and to tell that something of a ship's size is sitting in orbit
somewhere in the system, but not whose it is.

**Probes** are ordered, cost one probe per orbit named, and read one planet
exactly:

```text
probe ship SHIP-ID orbit ORBIT
probe ship SHIP-ID system A orbit 1 2 3
probe colony COLONY-ID orbit ORBIT
```

A probe reads a planet of the entity's current system, or of any system of its
current stellium when it names one. It reports every ship, orbital colony, and
surface colony at the planet with exact identity and mass, every deposit with
type and approximate quantity, and the planet's habitability. A home is
identified by the colony sitting on it.

Probes resolve before anything moves, so a ship cannot arrive in a system and
probe it on the same turn: it arrives on one turn and probes on the next. A
probe does not move its ship, and its findings are recorded as of the moment it
read the planet.

The practical search is therefore: jump into a single-system stellium, read its
planets passively on arrival, then spend the next turn probing orbit 4 — or the
whole system — to see whether the colony there is a home.

## Jump range

Jumps are limited by the ship's assembled HDRV units, so a faction can no longer
cross the map in one turn. See [Unit Glossary](../../units.md) for the drive
rules. The starting kit gives every ship 8 HDRV-8: a range of 8 units of
distance and a capacity of 66,880 MU against a ship mass of 55,226 MU.

Range 8 was chosen against this map. It leaves all ten homes in one connected
component, with 4.6 hops between a pair of homes on average and 8 at worst, so
crossing the map is a multi-turn journey that fits inside the 15-turn limit.
Shorter ranges fragment the map: at range 6 only 4 of the 10 homes remain
mutually reachable, and at range 3 most stellia are isolated.

A faction must therefore plan a route, and every hop costs a turn of search as
well as a turn of travel.

## File naming

Orders are `orders/t{TURN}-f{FACTION}-orders-v1.txt`. Reports are
`reports/t{TURN}-f{FACTION}-turn-report.txt` before resolution,
`reports/t{TURN}-f{FACTION}-resolved-turn-report.txt` after, and
`reports/t{TURN}-f{FACTION}-orders-report.txt` for order outcomes. The engine
log for a turn is `reports/t{TURN}-engine.log`.
