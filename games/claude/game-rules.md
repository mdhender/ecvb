# CLAUDE-01 Game Rules

A search-and-capture game played on the CLAUDE-01 map by the ten player
factions. The gamemaster generates every faction's orders, resolves the turn,
and adjudicates the result.

## Rules

1. A faction may move zero or one time per turn.
2. A faction may jump zero or one time per turn.
3. The winner is the first faction to place its ship in orbit around another
   faction's home planet.
4. Everyone loses if two factions end up in the same stellium in any turn.
   Check for winners before checking for losers.
5. One faction may cheat and issue zero, one, or two jumps per turn. The
   faction is chosen randomly before the game starts.
6. One faction may cheat and query the database for another faction's home
   planet. It may look up one faction per turn. The faction is chosen randomly
   before the game starts.
7. One faction may not hold both cheats.

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

### Losing

Only ships count toward the mutual-loss check. Immobile home colonies and the
uncontrolled faction's orbital colonies are ignored. Without this restriction
the game is unwinnable: a win requires jumping into the target stellium on one
turn and moving onto the planet on the next, and the jump turn itself would end
the game in a loss.

The check is on the end-of-turn state. Two or more ships from different
factions in the same stellium after resolution means everyone loses. A stellium
that a ship only passes through during a double jump does not count.

### Information

Public knowledge, available to every faction:

- The coordinates of all 100 stellia, and how many systems each one holds.
  Homes only ever occur in single-system stellia, so a faction may restrict its
  search to those 90.
- Its own entities, orders, and order outcomes.

A faction learns which planets a stellium holds, and whether any of them is
controlled, only for a stellium its ship occupies. A double-jumping ship
observes both stellia it visits during the turn.

Only the rule-6 cheater may look up a home planet it has not visited, one
faction per turn.

### Cheats

Chosen randomly before turn 0, and recorded here for review:

- Rule 5, double jump: **faction 8** (user07@example.com).
- Rule 6, database query: **faction 4** (user03@example.com).

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

A faction must therefore plan a route. A faction holding a target travels one
hop per turn toward it, and the rule-5 cheat is now worth a great deal, since
two hops per turn both doubles search speed and halves travel time.

## File naming

Orders are `orders/t{TURN}-f{FACTION}-orders-v1.txt`. Reports are
`reports/t{TURN}-f{FACTION}-turn-report.txt` before resolution,
`reports/t{TURN}-f{FACTION}-resolved-turn-report.txt` after, and
`reports/t{TURN}-f{FACTION}-orders-report.txt` for order outcomes. The engine
log for a turn is `reports/t{TURN}-engine.log`.
