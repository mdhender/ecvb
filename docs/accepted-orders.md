# Accepted Orders

## Definitions

_cadre_ is one of the cadre codes: `CWKR` construction worker, `LABR` laborer,
`PLCF` police force, `SPCF` special forces, and `TRNE` trainees and recruits.
A cadre is a temporary assignment of population, not a unit.

_commission_ is an integer percentage amount, range 1 to 100, followed by a `%`.

_commitment_ is an integer percentage amount, range 1 to 100, followed by a `%`.

_coordinates_ are integer X, Y, Z coordinates, separated by commas and surrounded by `(` and `)`.
Interior ASCII spaces or tabs are ignored.

_depositNo_ is the sequence number of a deposit on the planet the entity is at, 1 through 45.

_goldPrice_ is a positive integer followed by `GOLD`, written with commas the same way as _quantity_.
For example, 800,000 GOLD.

_groupNo_ is the sequence number of a factory, farm, or mine group, starting at 1.

_payRate_ is an integer percentage amount, at least 0, followed by a `%`.

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

(`ship` | `colony`) _id_ `raid` (`ship` | `colony`) _id_ `seeking` _unit_ (`,` _unit_) _commitment_

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
colony create form names a `CWKR` cadre today; where the construction workers
for a group create, and for an `add`, are drawn from is not yet settled.

## Ship and Colony Creation

(`ship` | `colony`) _id_ `create` (`ship` | ((`open-air` | `enclosed` | `orbital`) `colony` (`as` `trade-station`)?)) `using` _quantity_ _unit_ (`,` _quantity_ _unit_) `transfering` _quantity_ _unit_ (`,` _quantity_ _unit_) `with` _quantity_ `CWKR`

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

## Unassemble Orders

Unassemble returns working units to unassembled inventory, optionally moving them to cargo.

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

## Jump Orders

`ship` _id_ `jump` `to` _coordinates_

> ship 18 jump to (-1,2,3)

## Draft Orders

### Draft Orders

(`ship` | `colony`) _id_ `draft` _quantity_ _population_ (`,` _quantity_ _population_)*

> ship 18 draft 13 SOL

> colony 24 draft 3,600 SOL

### Disband Orders

(`ship` | `colony`) _id_ `disband` _quantity_ _population_ (`,` _quantity_ _population_)*

> ship 18 disband 13 SOL

> colony 24 disband 3,600 SOL

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
