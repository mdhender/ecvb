# CLAUDE-01 Game Rules

A search-and-capture game played on the CLAUDE-01 map by the ten player
factions. The gamemaster generates every faction's orders, resolves the turn,
and adjudicates the result.

## Rules

1. A faction may move zero or one time per turn.
2. A faction may jump zero or one time per turn.
3. A faction may probe as often as its assembled sensors allow.
4. A faction whose ship cannot afford to leave the stellium it is in is
   eliminated, and gives no further orders.
5. The winner is the first faction to place its ship in orbit around another
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

The practical search is therefore two turns a stellium, and the second of them
carries both orders. A ship arrives at the stellium orbit, and on the next turn
it probes system A orbit 4 **and moves onto it**: probes resolve at stage 13 and
movement at stage 15, so if the probe finds a home the move has already taken
it. Reading first and moving next turn would give the same answer a turn later.
If orbit 4 held no colony, the turn after is `move to orbit 11` and a jump,
which the engine also runs in that order.

## Jump range and fuel

Nothing caps how far a jump goes but the fuel it burns. The starting kit fits
every ship with 7 `HDRV-10`, which propels 73,150 MU against a ship of about
62,000, and 11,716 `FUEL`.

FUEL is drawn per assembled drive unit, so all seven draw on every jump
whatever its length:

| Order | FUEL |
| --- | --- |
| jump, per light year | 7 x 40 = **280** |
| move to another orbit of the same system | 7 x 4 = **28** |
| move across systems of one stellium | 7 x 8 = **56** |

A tank is therefore about 41 light years of jumping for the whole game, and a
checked stellium costs 56 FUEL besides -- in to orbit 4 and back out to the
stellium orbit. Distance is Euclidean, rounded up to the next whole light year.

A crossing takes ceil(d / 10) turns, 10 being the drive's technology level, so
any jump of 10 light years or less lands the turn it departs. The whole fuel
bill is drawn on departure.

### Elimination

A ship that cannot pay for the cheapest jump out of the stellium it is in is
stranded there, and its faction is eliminated: it can never reach another
faction's home, so it gives no further orders and the game goes on without it.

The gamemaster reads it off the map. A ship at stellium S with F FUEL is
stranded when

```text
F < 280 x (distance from S to its nearest neighbour)   (+ 28 if it sits at a planet)
```

This is what makes a route a decision rather than a walk. Jumping to the
nearest unvisited stellium is the cheapest way to check one, but the cheapest
hop *in* is not always a cheap hop *out*: a stellium on the rim of the map has
far neighbours, and a ship that arrives there on its last 300 FUEL has bought
one more look at one more orbit 4 and given up the game to do it. Before
jumping, a faction should keep back what the check and the next hop will cost.

## Result of CLAUDE-01

Played out over turns 0 through 9. Every faction ran the same search -- nearest
unvisited single-system stellium, then probe and move onto orbit 4 -- which
checks one stellium every two turns and puts all ten factions on the same
cadence. On turn 9, on their fifth checked stellium, four of them found a home
at once:

| Faction | Ship | Found | Ends in orbit |
| --- | --- | --- | --- |
| 4 | 12 | faction 11's home, stellium 93 | 71 |
| 8 | 28 | faction 9's home, stellium 88 | 91 |
| 9 | 32 | faction 3's home, stellium 79 | 16 |
| 11 | 40 | faction 4's home, stellium 83 | 95 |

A four-way win, and it is the cadence that makes it one: the loop is two turns
whoever runs it, so every faction checks its nth stellium on the same turn as
every other. Ten factions each checking five of the ninety single-system
stellia is fifty looks at nine homes, and four of them landing on the same turn
is what that arithmetic buys.

**No one was eliminated.** Every ship finished with 10 to 22 light years in the
tank, because ten searchers find a home long before one tank runs dry. The
elimination rule bites in the other game -- a long search, a rim stellium, a
faction that spent down to its last hop -- and it is adjudicated every turn
whether or not it fires.

## File naming

Orders are `orders/t{TURN}-f{FACTION}-orders-v1.txt`. Reports are
`reports/t{TURN}-f{FACTION}-turn-report.txt` before resolution,
`reports/t{TURN}-f{FACTION}-resolved-turn-report.txt` after, and
`reports/t{TURN}-f{FACTION}-orders-report.txt` for order outcomes. The engine
log for a turn is `reports/t{TURN}-engine.log`.
