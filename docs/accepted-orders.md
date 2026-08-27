# Accepted Orders

## Definitions

_cadre_ is one of the cadre codes: `CWKR` construction worker, `LABR` laborer,
`PLCF` police force, `SPCF` special forces, and `TRNE` trainees and recruits.
A cadre is a temporary assignment of population, not a unit. The population it
assigns is real, so a cadre has the mass and volume of the people in it and has
to be carried like anyone else. One `CWKR` is one `SKW` plus one `USK`. What a
construction worker does is in
[What a construction worker does](#what-a-construction-worker-does).

_commission_ is an integer percentage amount, range 1 to 100, followed by a `%`.

_commitment_ is an integer percentage amount, range 1 to 100, followed by a `%`.

_coordinates_ are integer X, Y, Z coordinates, separated by commas and surrounded by `(` and `)`.
Interior ASCII spaces or tabs are ignored.

_depositNo_ is the sequence number of a deposit on the planet the entity is at, 1 through 45.

_goldPrice_ is a positive integer followed by `GOLD`, written with commas the same way as _quantity_.
For example, 800,000 GOLD.

_groupNo_ is the sequence number of a factory, farm, or mine group, starting at 1.

_payRate_ is an integer percentage amount, at least 0, followed by a `%`.

_population_ is one of the population classes: `USK` unskilled worker, `SKW`
skilled worker, `SOL` soldier, and `NAS` non-assignable. One population unit
stands for 100 persons.

_price_ is a positive number followed by either `GOLD` or `CNGD`.
It must include at least one digit before the decimal, if there is a decimal amount.
The digits before the decimal point are written with commas the same way as _quantity_.
For example, 1.0 GOLD, 3 CNGD, 0.1 GOLD, 25,600 CNGD.

_rationRate_ is an integer percentage amount, at least 0, followed by a `%`.

_quantity_ is an integer, greater than 0.
A quantity greater than 999 must separate every three digits with a comma: 5,000 is accepted and 5000 is rejected.

_techLevel_ is a technology level, written `TL`, a hyphen, and the level, 1 through 10.
For example, TL-4.

_stationNo_ is the sequence number of a trade station at a planet, starting at 1.

_unitCode_ is a unit code with an optional technology level (e.g., GOLD or LFSU-7).

`we` is the faction submitting the order file.
It is the subject of the orders that no ship or colony carries out, such as naming a place
or granting a permission, and it takes no id because the file header already names the faction.

## Combat orders

### Attack orders

(`ship` | `colony`) _id_ `attack` (`ship` | `colony`) _id_ _commitment_

> colony 24 attack ship 18 75%

### Invade Orders

(`ship` | `colony`) _id_ `invade` (`ship` | `colony`) _id_  _commitment_

> colony 24 invade ship 18 55%

### Raid Orders

(`ship` | `colony`) _id_ `raid` (`ship` | `colony`) _id_ `seeking` _unitCode_ (`,` _unitCode_)? _commitment_

> ship 18 raid colony 24 seeking GOLD, FUEL 22%

### Support Orders

(`ship` | `colony`) _id_ `support` (`ship` | `colony`) _id_ `attacking` ((`ship` | `colony`) _id_)? _commitment_

> ship 18 support ship 97 attacking 35%

> ship 18 support ship 97 attacking colony 24 35%

(`ship` | `colony`) _id_ `support` (`ship` | `colony`) _id_ `defending` (`against` (`ship` | `colony`) _id_)? _commitment_

> ship 18 support colony 14 defending 40%

> ship 18 support colony 14 defending against ship 33 45%

## Create Orders

Every create order assembles the units it is given automatically.
They do not have to be assembled first.

An `add` order assembles the units it puts into a group the same way, so those
do not have to be assembled first either.

Assembly is what both orders need construction workers for. Only the ship and
colony create form *names* a `CWKR` cadre, because it is the only one that runs
for more than a turn and so the only one worth throttling. Every other assembly
— a group create, an `add`, a bare `assemble` — draws on the same cadre pool at
the same entity, and the engine allocates from it without being told to.

**A construction worker does one task per turn.** A worker spent on a group
create is not available to a ship build, or to an `assemble` order, in that same
turn. The pool is drawn down in stage order as the turn runs, so a group create
at stage 5 takes its workers before an `add` at stage 8, and both before the
`assemble` orders and the ship and colony builds at stage 10.

### The two create orders finish very differently

This is the one thing about `create` most likely to catch a player out.

A **ship or colony create is a commitment.** It succeeds when it is given, the
entity appears unfinished, and the build runs for as many turns as it needs. A
turn with no materials, no transport, or no workers costs that turn's progress
and nothing else.

A **group create is kill-and-fill.** It runs once, in the turn it was given. The
engine assigns as much labour and fuel as it can, builds as much as that pays
for, reports what was built, and closes the order out. **Nothing carries over.**
A colony ordered to create 10,000,000 mines with the resources for 2 creates 2,
and the order is finished — it does not spend the next turns working through the
rest.

### The `with` clause is a cap, not a claim

The `with` _quantity_ `CWKR` clause of a ship or colony create sets a **ceiling
on the workers the build may use in a turn**. It reserves nothing and holds
nothing back.

Every turn, the engine assigns the build up to that many construction workers
from whatever the creating entity has idle at the time — and never more than
that many, however many thousands are standing about. A turn that cannot fill
the cap costs that turn's work and nothing else. A ship or colony create is a
standing commitment to build as fast as it can, not a purchase that fails when
it cannot be paid for in full.

The workers assigned to a build do the same 500 MU of work a turn as any other
construction worker; see [Assemble Orders](#assemble-orders).

**A build's assembly is its own pool.** The workers assigned to a build are at
the new entity doing that build's work, so their 500 MU a turn is reckoned for
that build alone. It does not pool with `assemble` orders the creating entity
gives in the same turn, and two builds fed by one entity do not pool with each
other either. Each pool rounds up on its own.

What they *do* share is the cadre itself. Every build and every `assemble` order
draws construction workers from the same idle pool at the creating entity, so a
large `assemble` order and a build can still starve each other of workers even
though their work is counted separately. An `assemble` order is served first,
because it was asked for by name; a build takes what is left and simply goes
slower that turn.

Because the cap is the only lever a player has over how one entity's workers are
divided between its builds, it is worth setting deliberately rather than setting
to the size of the payroll. Where two builds on one entity compete, the older
build is served first, and each takes up to its own cap.

## Ship and Colony Creation

(`ship` | `colony`) _id_ `create` (`ship` | ((`open-air` | `enclosed` | `orbital`) `colony` (`as` `trade-station`)?)) `using` _quantity_ _unitCode_ (`,` _quantity_ _unitCode_)* `transfering` _quantity_ _unitCode_ (`,` _quantity_ _unitCode_)* `with` _quantity_ `CWKR`

> # line breaks and spacing do not matter in this order
> ship 18 create ship
>   using 60 STRC-8,
>         61 HDRV-1, 5 SDRV-1
>         , 5 LFSU-3, 1 SNSR-1
>   transfering 25 FOOD, 5 SKW,  16,800 FUEL, 93 GOLD
>   with 500 CWKR
> end

The ship and colony create orders must be terminated with `end`.
This allows the order to span multiple lines.

An **open-air** colony may only be created on a planet whose habitability number
is greater than 0. That is what lets a `COPN` carry no `LFSU` at all — it
breathes the air outside. An enclosed or orbital colony has no such restriction
and must carry its own life support.

The new entity takes the technology level of the entity that created it.

### Factory Group Creation

A factory group must say what it will make.

Only colonies are permitted to create factory groups.

`colony` _id_ `create` `factory-group` `with` _quantity_ _unitCode_ (`,`_quantity_ _unitCode_)* `making` _unitCode_

> colony 24 create factory-group with 54,000 FACT-6 making CNGD

Units within a factory group may have different tech-levels.

`colony` _id_ `add` _quantity_ _unitCode_ (`,`_quantity_ _unitCode_)* `to` `factory-group` _groupNo_

> colony 24 add 63 FACT-9 to factory-group 3

`colony` _id_ `remove` _quantity_ _unitCode_ (`,`_quantity_ _unitCode_)* `from` `factory-group` _groupNo_ (`and` `stow`)?

> colony 24 remove 12,000 FACT-6, 63 FACT-9 from factory-group 3 and stow

Units removed from the factory group will be unassembled and optionally moved to cargo.
There are rules about the work in progress - the engine will salvage what it can and recycle what it can't.

`colony` _id_ `idle` _quantity_ _unitCode_ (`,`_quantity_ _unitCode_)* `in` `factory-group` _groupNo_

Units in the factory group will be idled but left in the group.
They will remain active until the work in progress is drained from them.

`colony` _id_ `activate` _quantity_ _unitCode_ (`,`_quantity_ _unitCode_)* `in` `factory-group` _groupNo_

Idle units in the factory group will be activated.
Production in them will resume immediately.

`colony` _id_ `retool` `factory-group` _groupNo_ `making` _unitCode_

The production line will drain (which may take 3 turns), then 1 turn will be spent retooling.
Production will resume on the next turn.

Optionally, the player may force an immediate retooling.
This discards the entire work in progress - all materials in the production line are recycled.
1 turn is spent retooling and production resumes the following turn.

`colony` _id_ `retool` `immediately` `factory-group` _groupNo_ `making` _unitCode_

### Farm Group Creation

(`ship` | `colony`) _id_ `create` `farm-group` `with` _quantity_ _unitCode_

> ship 18 create farm-group with 1,234,000 FARM-6

Note: The table below summarizes the tech-level of farming units that may created on an entity.
The inputs are entity type and orbit number.

| Entity | Orbits  | Allowed farming units | Notes |
| ------ | ------- | --------------------- | ----- |
| COPN   | 1 .. 5  | FARM-1                | Limited to Habitability Number (HN) x 100,000 per planet |
| COPN   | 1 .. 5  | FARM-2 .. FARM-5      | Hydroponic, solar-powered, consumes no fuel              |
| CSFC   | 1 .. 5  | FARM-2 .. FARM-5      | Hydroponic, solar-powered, consumes no fuel              |
| CORB   | 1 .. 5  | FARM-2 .. FARM-5      | Hydroponic, solar-powered, consumes no fuel              |
| COPN   | 6 .. 10 | FARM-6 .. FARM-10     | Hydroponic, artificial lights                            |
| CSFC   | 6 .. 10 | FARM-6 .. FARM-10     | Hydroponic, artificial lights                            |
| CORB   | 6 .. 10 | FARM-6 .. FARM-10     | Hydroponic, artificial lights                            |
| SHIP   | 1 .. 10 | FARM-6 .. FARM-10     | Hydroponic, artificial lights                            |

TODO: this table belongs in the rules reference, but is included here because the order validator uses it.

All units within a farm group must have the same tech-level.

(`ship` | `colony`) _id_ `add` _quantity_ _unitCode_ `to` `farm-group` _groupNo_

> ship 18 add 250,000 FARM-6 to farm-group 2

The units added must be at the group's tech-level.

(`ship` | `colony`) _id_ `remove` _quantity_ _unitCode_ `from` `farm-group` _groupNo_ (`and` `stow`)?

> colony 24 remove 40,000 FARM-3 from farm-group 1 and stow

Units removed from the farm group will be unassembled and optionally moved to cargo.
There are rules about the work in progress - the engine will salvage what it can and recycle what it can't.

(`ship` | `colony`) _id_ `idle` _quantity_ _unitCode_ `in` `farm-group` _groupNo_

> ship 18 idle 50,000 FARM-6 in farm-group 2

Units in the farm group will be idled but left in the group.
They will remain active until the work in progress is drained from them.

(`ship` | `colony`) _id_ `activate` _quantity_ _unitCode_ `in` `farm-group` _groupNo_

> ship 18 activate 50,000 FARM-6 in farm-group 2

Idle units in the farm group will be activated.
Production in them will resume immediately.

### Mine Group Creation

Only surface colonies are permitted to create mine groups.

`colony` _id_ `create` `mine-group` `with` _quantity_ _unitCode_ `working` `deposit` _depositNo_

> colony 83 create mine-group with 25,680 MINE-2 working deposit 18

All units within a mine group must have the same tech-level.

A mine group works the deposit it was created with and no order changes that.
To work a different deposit, remove the group and create a new one at the new deposit.

`colony` _id_ `add` _quantity_ _unitCode_ `to` `mine-group` _groupNo_

> colony 83 add 5,000 MINE-2 to mine-group 4

The units added must be at the group's tech-level.

`colony` _id_ `remove` _quantity_ _unitCode_ `from` `mine-group` _groupNo_ (`and` `stow`)?

> colony 83 remove 5,000 MINE-2 from mine-group 4 and stow

Units removed from the mine group will be unassembled and optionally moved to cargo.

`colony` _id_ `idle` _quantity_ _unitCode_ `in` `mine-group` _groupNo_

> colony 83 idle 10,000 MINE-2 in mine-group 4

Units in the mine group will be idled but left in the group.
A mine group has no work in progress, so they idle immediately.

`colony` _id_ `activate` _quantity_ _unitCode_ `in` `mine-group` _groupNo_

> colony 83 activate 10,000 MINE-2 in mine-group 4

Idle units in the mine group will be activated.
Production in them will resume immediately.

## Assemble Orders
Assembly turns unassembled units into working ones.
This usually increases the volume of the units.

Every form names the entity assembling, the unit code being assembled, and how many.
One order may assemble several kinds of units once.

(`ship` | `colony`) _id_ `assemble` _quantity_ _unitCode_ (`,` _quantity_ _unitCode_)*

> ship 18 assemble 6,000 SNSR-1

> colony 24 assemble 5 LFSU-1, 60 STRL-1

### What a construction worker does

A `CWKR` cadre is what carries out `assemble` and `unassemble` orders. One unit
does up to 500 MU of work a turn.

**Work of the same kind is pooled across an entity**, so the workers an entity
needs are reckoned from one total rather than order by order and unit by unit.
An entity assembling 15,120 MU of `HDRV` and 100 MU of `STRC-1` needs

> ceil((15,120 + 100) / 500) = **31** workers,
> not ceil(15,120 / 500) + ceil(100 / 500) = 31 + 1 = **32**.

Pooling matters because the rounding-up is per pool, not per line, so an entity
assembling many small lots is not charged a whole worker for each one.

Unassembly is pooled the same way, at the same rate.

**Assembly and unassembly do not pool with each other.** An entity assembling
100 MU of `STRC-1` and unassembling 100 MU of `LFSU-1` needs 2 `CWKR`, not 1:
each total is rounded up on its own.

## Unassemble Orders

Unassemble returns working units to unassembled inventory, optionally moving them to cargo.

Unassembly is work, and it is done by the same `CWKR` cadre at the same rate as
assembly — 500 MU a turn per unit, pooled across the entity's unassembly but
never pooled with its assembly. See
[What a construction worker does](#what-a-construction-worker-does).

(`ship` | `colony`) _id_ `unassemble` (`and` `stow`)? _quantity_ _unitCode_ (`,` _quantity_ _unitCode_)*

> ship 18 unassemble 1,000 SNSR-1

> colony 24 unassemble and stow 60 STRL-1, 5 LFSU-1

## Transfer Orders

(`ship` | `colony`) _id_ `transfer` _quantity_ _unitCode_ (`,` _quantity_ _unitCode_)* `to` (`ship` | `colony`) _id_

> ship 18 transfer 500 SOL to colony 24

> ship 18 transfer 4,500 GOLD, 18,000 FOOD to colony 24

Transfer orders fail if the two entities are not at the same location when the order is executed.

The order will be partially fulfilled if there are not enough transports available for the entire load.

Units must be in cargo to be transferred and will be automatically stowed by the receiving entity.

## Market Order

(`ship` | `colony`) _id_ `buy` _quantity_ _unitCode_ _price_ (`,` _quantity_ _unitCode_ _price_)* _commission_?

(`ship` | `colony`) _id_ `sell` _quantity_ _unitCode_ _price_ (`,` _quantity_ _unitCode_ _price_)* _commission_?

Units to be bought and sold must be unassembled.
Purchased units will automatically be stowed by the receiving entity.

The entity must have enough transports available for the amount bought or sold.
If there is a shortage when buying, the excess will be permanently lost.
If there is a shortage when selling, the entire order will be cancelled.

Every successful sale pays a commission to the market and there is no way to sell without paying one.
The default commission is set by the market, so an order that names none pays the default.
Sellers may offer a higher commission to increase the chances of their offer being accepted.
The commission is a percentage of the transaction amount and will automatically be deducted.

Buyers may offer an additional commission to increase the chances of their offer being accepted.
When they do, they are responsible for paying the extra fees.
The commission is a percentage of the transaction amount and will automatically be deducted.

The market will prefer to execute the transactions that return the highest commission to them.

A technology level is bought and sold in the same market, but it is not cargo.
It is paid for in `GOLD`, never `CNGD`, and the price is always a whole number.
It needs no transports, so the shortage rules above do not apply to it.
Selling one pays a commission like any other sale.

(`ship` | `colony`) _id_ `buy` `tech-level` _techLevel_ _goldPrice_ _commission_?

> ship 18 buy tech-level TL-6 1,000,000 GOLD

(`ship` | `colony`) _id_ `sell` `tech-level` _techLevel_ _goldPrice_ _commission_?

> colony 24 sell tech-level TL-4 800,000 GOLD 5%

A tech level order names no quantity, because a technology level is bought once.

## Survey Orders

(`ship` | `colony`) _id_ `survey`

> ship 18 survey

## Probe Orders

(`ship` | `colony`) _id_ `probe` (`system` _seq_)? `orbit` _orbitNo_+

> ship 18 probe orbit 1

> colony 24 probe system B orbit 3 1 8

## Spy Orders

> colony 24 assess rebels using 1 spies

> colony 24 detect spies using 4 spies

> colony 24 obtain information from ship 18 using 200 spies

> colony 24 convert rebels using 3 spies

> colony 24 incite rebels using 21 spies

> colony 24 neutralize faction 1 spies using 11 spies

## News Release

> ship 18 broadcast system B orbit 8 "message" "optional signature"

## Move Orders

`ship` _id_ `move` `to` (`system` _seq_)? `orbit` _orbitNo_

> ship 18 move to orbit 5

> ship 18 move to system B orbit 8

A ship moves once a turn. It may also jump in the same turn -- that is how a
ship at a planet leaves, since a jump begins from the stellium orbit -- but the
one move is the whole of what it does inside its stellium. The order is spent
whatever it goes on to do: a move that fails for want of fuel has still been
given.

## Jump Orders

`ship` _id_ `jump` `to` _coordinates_

> ship 18 jump to (-1,2,3)

A jump begins from the stellium orbit, so a ship at a planet must be moved out
to orbit 11 first. Every move resolves before any jump, so both orders may be
written in one file.

Technology level does not limit how far a drive jumps. It sets how long the
crossing takes: a jump of _d_ light years by a drive at technology level _t_
takes \(\lceil d / t \rceil\) turns to complete. `FUEL` is unchanged -- 40 per
assembled `HDRV` unit per light year -- so the fuel is what makes a long jump
expensive and the duration is what makes it slow.

The whole fuel bill is drawn on departure, in the turn the order executes,
however many turns the crossing then takes. A ship that cannot pay for the
whole crossing never leaves.

A ship in transit is nowhere. It is at no stellium, no system, and no planet
until it arrives, so it cannot be probed, does not appear on a passive sensor
sweep, and cannot be given an order of any kind. A crossing cannot be recalled,
redirected, or cancelled once it has begun, and because the fuel was drawn on
departure, a jump written to the wrong coordinates is not recoverable. That is
the intended cost of a long crossing and the reason to build a drive of a higher
technology level: a better drive is not a longer reach -- every drive reaches
everywhere -- it is fewer turns spent off the board.

An in-transit ship is a row of its own rather than a state of the order that
started it: the ship, the stellium it is bound for, and the turn it is due. The
row is written when the jump executes and deleted when the ship arrives, and it
is the only thing that knows where the ship went. The ship arrives at the
destination's stellium orbit at the end of ship movement on its due turn, so it
is on the board for the next turn's orders and appears in the next turn's
reports -- the same rule a single-turn jump already follows, because everything
that reads the world happens before movement.

A crossing of one turn is the same thing with nothing left over: the row is
written and deleted in the same turn, and the ship arrives where it always did.

A ship jumps once a turn, and a second jump order for the same ship in one file
is refused along with the file. Usually the ship is already in transit by then;
when the first jump failed for want of fuel and left the ship where it was, the
second is refused for being the second.

## Draft Orders

### Draft Orders

(`ship` | `colony`) _id_ `draft` _quantity_ (`SOL` | _cadre_) (`,` _quantity_ (`SOL` | _cadre_))*

> ship 18 draft 13 SOL

> colony 24 draft 3,600 SOL

> ship 18 draft 4,250 CWKR

> colony 24 draft 200 SOL, 1,000 CWKR

Only `SOL` and the cadres may be drafted. No other population class can be.

Drafting `SOL` **changes the type of a population unit**: it takes unskilled
workers off the rolls and makes them soldiers, one for one. It is the only draft
that changes anyone's type.

Drafting a cadre assigns population rather than converting it. One `CWKR` is one
`SKW` plus one `USK`, so `ship 18 draft 4,250 CWKR` assigns 4,250 of each to a
cadre of 4,250 construction workers, and they are skilled and unskilled workers
still while they serve in it.

**A draft partially fills.** Short population is not an error: the order drafts
as many as it can and says so. `colony 24 draft 3,600 SOL` on a colony with
1,000 `USK` drafts 1,000 soldiers. A cadre is limited by whichever of its
classes runs out first, so `ship 18 draft 4,250 CWKR` on a ship with 4,250 `SKW`
and 900 `USK` drafts 900 construction workers.

### Disband Orders

(`ship` | `colony`) _id_ `disband` _quantity_ (`SOL` | _cadre_) (`,` _quantity_ (`SOL` | _cadre_))*

> ship 18 disband 13 SOL

> colony 24 disband 3,600 SOL

> ship 18 disband 1,000 CWKR

Disband is the reverse of draft, and only the same things may be disbanded.

Disbanding `SOL` takes soldiers off the rolls and returns them as unskilled
workers, one for one. It is the only disband that changes anyone's type, and
`USK` is where they go — the same pool a draft took them from.

Disbanding a cadre returns the population it was assigned from to the pools it
came from, unchanged: `ship 18 disband 1,000 CWKR` returns 1,000 `SKW` and
1,000 `USK`.

## Pay Orders

(`ship` | `colony`) _id_ `pay` _population_ _payRate_ (`,` _population_ _payRate_)*

> ship 18 pay USK 120%

> colony 24 pay SKW 15%, USK 18%

## Ration Orders

(`ship` | `colony`) _id_ `rations` _rationRate_

> ship 18 rations 75%

> colony 24 rations 130%

## Control Orders

### Control Orders

(`ship` | `colony`) _id_ `control` (`ship` | `colony`) _id_

> ship 18 control colony 24

(`ship` | `colony`) _id_ `control` `system` _seq_ `orbit` _orbitNo_

> colony 8 control system A orbit 5

Control orders against an entity or planet that is already controlled automatically fail.

Note: control orders are given to an entity that is at the same location as the object (entity or planet) to be controlled.

### Release Control Orders

Taking control is a physical act, so it is given to an entity that is present.
Releasing it is administrative, so it is a faction order and needs no entity at the place.
A faction may release a planet whose garrison is gone.

`we` `release` (`ship` | `colony`) _id_

> we release ship 18

Release has a variant that allows you to release control of a planet.

`we` `release` _coordinates_ `system` _seq_ `orbit` _orbitNo_

> we release (-1,2,3) system A orbit 5

## Naming Orders

Names must be quoted text.
They may be no more than 24 characters long, including spaces.

Naming something you own is an order to the thing itself.
Naming anything else is a faction order, because no ship or colony carries it out.

You may name a stellium.
You are allowed to name stellia that you have not yet visited.

`we` `name` _coordinates_ _quotedText_

> we name (-1,2,3) "Stellium Joe"

You may name a system.
You are allowed to name systems that you have not yet visited.

`we` `name` _coordinates_ `system` _seq_ _quotedText_

> we name (-1,2,3) system A "Alpha Sur"

You may name a planet.
You are not allowed to name one that you have never had a report of.

`we` `name` _coordinates_ `system` _seq_ `orbit` _orbitNo_ _quotedText_

> we name (-1,2,3) system A orbit 8 "Headly's Gate"

You may name a ship or colony that you control.

(`ship` | `colony`) _id_ `name` _quotedText_

> ship 18 name "Jalopy"

> colony 24 name "Jingo"

You may name another player or faction.
You are not allowed to name one that you have not yet encountered.

`we` `name` (`player` | `faction`) _id_ _quotedText_

> we name faction 5 "The Hegemony"

You may name another player or faction's ships and colonies.
You are not allowed to name one that you have not yet encountered.

`we` `name` (`player` | `faction`) _id_ (`ship` | `colony`) _id_ _quotedText_

> we name player 5 ship 19 "Easy Target"

> we name player 5 colony 33 "Avoid"

## Trade Station Orders

`we` (`grant` | `refuse`) `trade` _coordinates_ `system` _seq_ `orbit` _orbitNo_ `station` _stationNo_ `to` `faction` _id_

> we grant trade (-1,2,3) system A orbit 5 station 4 to faction 1

> we refuse trade (-1,2,3) system A orbit 5 station 4 to faction 1

## Colonizing Permission

`we` (`grant` | `refuse`) `colonize` _coordinates_ `system` _seq_ `orbit` _orbitNo_ `to` `faction` _id_

> we grant colonize (-1,2,3) system A orbit 5 to faction 1

> we refuse colonize (-1,2,3) system A orbit 5 to faction 1

## General Notes

### Quoted Text

Quoted text may contain spaces.
Spaces includes the ASCII space character, but not tabs, line-feeds, carriage-returns, control characters, or other non-printing characters.

Leading and trailing spaces are not allowed and will cause the order to be rejected.

Runs of more than one space in are not allowed and will cause the order to be rejected.
