# Order File Reference

An order file identifies one game turn and one submitting faction, followed by
zero or more orders. The engine resolves every `PROBE` order, then reads passive
sensors, then resolves every `MOVE` order, then every `JUMP` order. File sequence controls orders for the same ship only when those orders
resolve in the same phase, segment, and step.

## File names

The recommended file name is:

```text
t{TURN}-f{FACTION}-orders[-v{VERSION}].txt
```

For example, `t0-f1-orders-v1.txt` contains version 1 of faction 1's turn 0
orders. The version suffix is optional and has no effect on processing.

## File structure

The first physical line identifies the game and turn:

```text
game "BETA-001" turn 0
```

The second physical line identifies the player or faction:

```text
id player "user01@example.com"
```

or:

```text
id faction 1
```

Player email addresses are trimmed and converted to lowercase before lookup.
The identified player or faction must belong to the game.

Blank lines are permitted after the identity line. A `#` outside quotes begins
a comment that runs to the end of the line, so a comment may stand on its own
line or follow an order:

```text
# scout the neighbouring system
probe ship 2 system B orbit 4    # before anything moves
```

Keywords and system letters are case-insensitive. Game codes are matched
exactly against the database. IDs are positive integers. A line that does not
match any form of the order it names is rejected, and the error lists that
order's forms; `ec orders help` prints them all.

An order file containing only the two header lines represents an empty order
set.

## JUMP

```text
jump ship SHIP-ID to (X,Y,Z)
```

Example:

```text
jump ship 2 to (6,-9,8)
```

The ship must belong to the submitting faction. The coordinates must identify a
stellium in the named game; a jump cannot end in deep space. When executed, the
ship moves to that stellium and orbits the stellium rather than a planet.

The ship's jump drive limits the jump. The distance to the destination, which is
the Euclidean distance rounded up, must be within the range of the ship's
assembled `HDRV` units, and the ship's mass must be within their capacity. See
[Unit Glossary](units.md) for the drive rules.

A ship with more than one jump order in a turn measures each jump from where the
previous one left it. Both `orders check` and the engine apply these limits, so a
jump the check rejects is a jump the engine would have failed.

## MOVE

```text
move ship SHIP-ID to orbit ORBIT
move ship SHIP-ID to system SYSTEM orbit ORBIT
```

Move to a planet in the ship's current system:

```text
move ship SHIP-ID to orbit ORBIT
```

Example:

```text
move ship 2 to orbit 6
```

The ship must currently have a system, and that system must contain a planet in
the requested orbit.

Move to a planet in a named system of the ship's current stellium:

```text
move ship SHIP-ID to system SYSTEM orbit ORBIT
```

Example:

```text
move ship 2 to system B orbit 4
```

The named system must exist in the ship's current stellium and must contain a
planet in the requested orbit. Both forms place the ship at the destination
planet in a ring the game draws at random from 2 through 99. Ring 0 is the
surface and ring 1 belongs to orbital colonies, so a ship arriving under its
own power never lands in either. The draw is seeded from the game and the
order, so re-resolving a turn puts the ship in the same ring.

Return to the stellium orbit:

```text
move ship 2 to orbit 11
```

Orbit 11 does not exist. It is a fiction that gives `MOVE` a way to say "leave
the planets", and it is the only orbit that is not a place: no planet occupies
it, and a probe cannot read it. Because the stellium orbit belongs to no
system, `move ship 2 to system A orbit 11` is an error. The ship ends the move
orbiting the stellium with no system, planet, or ring, which is where a jump
also leaves it.

The ship's drive moves it. A move fails when the ship has no assembled `HDRV`
units or when its mass exceeds their capacity. Distance does not limit a move:
every move inside a stellium is well within the range of any drive.

A move costs one of three fixed amounts of `FUEL`, per assembled `HDRV` unit:

| Move | Fuel per unit |
| --- | --- |
| Stellium orbit to any planet of the stellium, or back | 4 |
| Planet to planet in the same system | 4 |
| Planet to the planet the ship is already at | 4 |
| Planet to planet in different systems of the stellium | 8 |
| Stellium orbit to the stellium orbit | 0 |

A move between systems costs two hops because the ship crosses the stellium
orbit on the way. A ship with more than one move order in a turn starts each
move from where the previous one left it. A failed move burns nothing.

Only one move is free. Ordering a ship in the stellium orbit to the stellium
orbit is a no-op: there is nowhere to go, so it burns nothing and the ship is
not touched. Ordering a ship to the planet it is **already at** is not a no-op:
it breaks orbit and settles again, which costs a hop and draws a fresh ring. It
is the one way to change a ship's ring without going anywhere.

## Fuel

Every `MOVE` and every `JUMP` burns `FUEL`, and every assembled `HDRV` unit
draws. A `PROBE` burns none.

A jump of X light years costs `40 * X` per unit. A move costs 4 or 8 per unit,
as the table above shows; those are the same rate applied to the tenth and the
fifth of a light year that a move covers, so nothing fractional is ever
measured or stored. See [Unit Glossary](units.md) for the drive rules and the
order the sections are drawn in.

A ship that cannot pay for an order does not stop the submission. `orders
check` and `orders submit` accept the file and warn:

```text
warning: line 5: ship 2 needs 960 FUEL to jump and holds 144; the order is kept in case that changes before the turn resolves
```

The warning comes from running the turn: each order is executed in resolution
order against the fuel the ship actually holds, so a ship that runs dry partway
through a file warns on every order after that, and an order it could not pay
for leaves the ship where it was for the orders that follow. A ship still short
of fuel when the engine reaches the order fails it, burning nothing and going
nowhere, and the turn carries on.

The warning is deliberately not an error, because fuel may reach the ship
between submission and resolution. Nothing moves fuel between entities yet;
transfer orders are not implemented.

## PROBE

```text
probe ship SHIP-ID orbit ORBIT ...
probe colony COLONY-ID orbit ORBIT ...
probe ship SHIP-ID system SYSTEM orbit ORBIT ...
probe colony COLONY-ID system SYSTEM orbit ORBIT ...
```

A probe is the one order a colony may give. `MOVE` and `JUMP` are ship orders.

Example:

```text
probe ship 2 orbit 6
```

One order may name several orbits, and spends one probe on each:

```text
probe ship 2 orbit 1 2 3 4 5 8 9 10
```

A probe may also name a system of the ship's current stellium:

```text
probe ship 4 system A orbit 1
probe ship 4 system A orbit 1 2 3
```

A probe that names no system reads the system the entity is in, which is why a
ship orbiting the stellium rather than a planet has to name one. A probe that
names a system reads any system of the entity's current stellium, so a ship in
system C of a three-system stellium can probe all three.

The entity must carry assembled `SNSR` units and have probes left this turn. A
colony is always at a planet, so it always has a current system. Each
named orbit must hold a planet of the probed system. See
[Unit Glossary](units.md) for how many probes a sensor array launches.

Probes resolve before anything moves, so a probe reads the system the ship is
in at the **start** of the turn. A ship cannot move into a system and probe it
in the same turn; it arrives on one turn and probes on the next. A probe reads
its planet before a move or jump carries the ship away.

Each probe reports, for the planet it reads:

- Every ship orbiting the planet, with its identity and exact mass.
- Every orbital colony, with its identity and exact mass.
- Every surface colony, with its identity and exact mass.
- Every natural resource deposit, with its type and approximate quantity.
- The habitability of the planet.

A probe does not move its ship, and its findings are recorded as of the moment
it read the planet.

## Checking and submitting

Check syntax, identity, ownership, turn, and destinations without changing the
database. The check runs the turn for real -- burning the fuel, moving the
ships, reading the planets -- and then rolls all of it back, so what it reports
is what the engine will do rather than a second opinion about it:

```text
ec --db-path games/beta orders check games/beta/orders/t0-f1-orders-v1.txt
```

Submit an order file:

```text
ec --db-path games/beta orders submit games/beta/orders/t0-f1-orders-v1.txt
```

Submission runs the same check and then, having rolled it back, atomically
replaces the faction's pending order rows for the current turn. An invalid
submission leaves the existing order set unchanged and stores nothing. Errors
include the source line number when applicable.

An order file is refused for anything that cannot change between now and the
turn resolving: a ship that is not yours, an entity of the wrong kind, a
destination that does not exist, a drive that cannot reach or cannot lift.
Anything that can still change -- fuel above all -- is a warning, and the order
is stored.

## Reporting submitted orders

Show the orders currently stored for a faction:

```text
ecrpt --db-path games/beta show orders --game BETA-001 --faction 1
```

The player email may be used instead of the faction ID:

```text
ecrpt --db-path games/beta show orders --game BETA-001 --email user01@example.com
```

Use `--turn NUMBER` to review the retained orders from the previous turn after
the next turn has opened.

The report displays each order's input, fuel, status, starting location, final
location, and error message. Fuel is the one number that prices an order,
whether it is a move or a jump. Pending orders have no outcome locations, and
their fuel is what the order would burn. For a failed order, the starting and
final locations are identical and the fuel is zero.
