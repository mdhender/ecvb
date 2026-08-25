# Order File Reference

An order file identifies one game turn and one submitting faction, followed by
zero or more orders. The engine resolves every `MOVE` order before any `JUMP`
order. File sequence controls orders for the same ship only when those orders
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

Blank lines are permitted after the identity line. Comments are not supported.
Keywords and system letters are case-insensitive. Game codes are matched
exactly against the database. IDs are positive integers.

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
planet in the requested orbit. Both forms place the ship in ring 99 at the
destination planet.

## Checking and submitting

Check syntax, identity, ownership, turn, and destinations without changing the
database:

```text
ec --db-path games/beta orders check games/beta/orders/t0-f1-orders-v1.txt
```

Submit an order file:

```text
ec --db-path games/beta orders submit games/beta/orders/t0-f1-orders-v1.txt
```

Submission parses and validates the file again. A valid submission atomically
replaces the faction's pending `move_order` and `jump_order` rows for the current
turn. An invalid submission leaves the existing order set unchanged. Errors
include the source line number when applicable.

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

The report displays each order's input, status, starting location, final
location, and error message. Pending orders have no outcome locations. For a
failed order, the starting and final locations are identical.
