# Proposed Orders

2026/08/25 - draft

## Combat orders

_commitment_ is an integer percentage amount, range 1 to 100, followed by a `%`.

_payRate_ is an integer percentage amount, at least 0, followed by a `%`.

_rationRate_ is an integer percentage amount, at least 0, followed by a `%`.

_quantity_ is an integer, greater than 0, and must include commas if greater than 999.

_unitCode_ is a unit code with an optional technology level (e.g., GOLD or LFSU-7).

### Bombard orders

`combat` (`ship` | `colony`) _id_ _commitment_ `bombard` (`ship` | `colony`) _id_

> combat ship 18 75% bombard colony 24

### Invade Orders

`combat` (`ship` | `colony`) _id_ _commitment_ `invade` (`ship` | `colony`) _id_

> combat ship 18 55% invade colony 24

### Raid Orders

`combat` (`ship` | `colony`) _id_ _commitment_ `raid` _unit_ (`ship` | `colony`) _id_

> combat ship 18 28% raid GOLD colony 24

### Support Orders

`combat` (`ship` | `colony`) _id_ _commitment_ `support` (`ship` | `colony`) _id_ `attacking` (`ship` | `colony`) _id_

> combat ship 18 35% support ship 97 attacking colony 24

### Defend Orders

`combat` (`ship` | `colony`) _id_ _commitment_ `defend` (`ship` | `colony`) _id_ (`from` (`ship` | `colony`) _id_)?

> combat ship 18 40% defend colony 14

> combat ship 18 45% defend colony 14 from ship 33

## Set Up Orders
17.2.2.1. Format
The order, location No. of the new ship or colony, type (ship or colony), ID No. of establishing colony or ship, "transfer," quantity and item, quantity and item, etc., END. The word END must be written at the end of set up orders only.

17.2.2.2. Examples
Set Up, 5.1, ship, 29, transfer, 58, 60 structural units, 5 space drives-1, 5 Life Supports-1, 5 Food, 5 Professionals, 1 sensor-1, 16,800 fuel, 61 hyper engines-1, END.
The "-1" in "space drives-1" refers to TL1.

## Assembly orders
17.2.3.1. Format
Factory assembly: Colony/ship ID No., "assemble," quantity of "factories," units the factory will make. Mine assembly: Colony/ship ID No., "assemble," quantity of "mines," location No. of deposit. Other assemblies: Colony/ship ID No., "assemble," quantity of units.

17.2.3.2. Examples
Factories
91, assemble, 54,000 factories-6, consumer goods.
Mines
83, assemble, 25,680 mine-2, 148.
Others
58, assemble, 6,000 missile launchers-1.

## Dis-assembly orders
Format and examples are the same as for assembly orders, with the word "dis-assemble" replacing the word "assemble."

## Build Change Orders
17.2.5.1. Format
Ship/colony ID No., "build change," factory group No., item to start building, (or retooling).

17.2.5.2. Examples
16, build change, 8, retool.
16, build change, 8, energy weapons-4.
17, build change, research.

## Transfer Orders

> transfer ship 18 SOL 500 colony 24

> transfer ship 18 GOLD 4,500 FOOD 18,000 colony 24

## Mining Change Orders
17.2.7.1. Format
ID No., "mining," mining group No., new deposit location No.

17.2.7.2. Examples
348, mining, 18, 92.

## Market Order
Format
ID No. , "buy"  , quantity , unit type, price each
ID No. , "sell" , quantity , unit type, price each
Examples
555, buy, 25600, structural, 0.01
721, sell, TL-4, 800000
44, sell, 4, space drive-3, 0.2
53, buy, TL-6, 1000000
Note: Quantity for TL must be omitted.

## Survey Orders

`survey` (`ship` | `colony`) _id_

> survey ship 18

## Probe Orders

`probe`  (`ship` | `colony`) _id_ (`system` _seq_)? `orbit` _orbitNo_+

> probe ship 18 orbit 1

> probe ship 18 system B orbit 5 8 2

> probe colony 24 orbit 1 2 3

> probe colony 24 system B orbit 3

## Spy Orders

> spy colony 24 report-on rebels 1

> spy colony 24 report-on spies 4

> spy colony 24 report-on information 18

> spy colony 24 convert rebels 3

> spy colony 24 incite rebels 21

> spy colony 24 attack spies 11

## News Release

> broadcast (-1,2,3) system B orbit 8 "message" "optional signature"

## Move Orders

`move` `ship` _id_ (`system` _seq_)? orbit _orbitNo_

> move ship 18 orbit 5

> move ship 18 system B orbit 8

## Jump Orders

`move` `ship` _id_ _coordinates_

> jump ship 18 (-1,2,3)

## Draft Orders

### Draft Orders

`draft` (`ship` | `colony`) _id_ (_population_ _quantity_)+

> draft ship 18 SOL 13

> draft colony 24 SOL 3600

### Disband Orders

`disband` (`ship` | `colony`) _id_ (_population_ _quantity_)+

> disband ship 18 SOL 13

> disband colony 24 SOL 3600

## Pay Orders

`draft` (`ship` | `colony`) _id_ (_population_ _payRate_)+

> pay ship 18 USK 120%

> pay colony 24 PRO 15%

## Ration Orders

`draft` (`ship` | `colony`) _id_ _rationRate_

> ration ship 18 75%

> ration colony 24 130%

## Control Orders

### Control Orders

Control orders against an entity or planet that is already controlled automatically fail.

`control` (`ship` | `colony`) _id_ (_coordinates_ `system` _seq_ `orbit` _orbitNo_)?

> control ship 18

> control colony 24

> control (-1,2,3) system A orbit 5

### Release Control Orders

`release` (`ship` | `colony`) _id_ (_coordinates_ `system` _seq_ `orbit` _orbitNo_)?

> release ship 18

> release colony 24

> release (-1,2,3) system A orbit 5

## Naming Orders

Names must be quoted text.
They may be no more than 24 characters long, including spaces.

`name` _coordinates (`system` _seq_ (`orbit` _orbitNo_)?)? _quotedText_

> name (-1,2,3) "Stellium Joe"

> name (-1,2,3) system A "Alpha Sur"

> name (-1,2,3) system A orbit 8 "Headly's Gate"

You may name a ship or colony that you control.

`name` (`ship` | `colony`) _id_ _quotedText_

> name ship 18 "Jalopy"

> name colony 24 "Jingo"

You may name another player's ships and colonies.

`name` `player` _id_ (`ship` | `colony`) _id_ _quotedText_

> name player 5 ship 19 "Easy Target"

> name player 5 colony 33 "Avoid"

## Trade Station Orders

> grant player 1 permission-to-trade (-1,2,3) system A orbit 5 station 4

> refuse player 1 permission-to-trade (-1,2,3) system A orbit 5 station 4

> revoke player 1 permission-to-trade (-1,2,3) system A orbit 5 station 4

## Colonizing Permission

> grant player 1 permission-to-colonize (-1,2,3) system A orbit 5

> refuse player 1 permission-to-colonize (-1,2,3) system A orbit 5

## General Notes

### Quoted Text

Quoted text may contain spaces.
Spaces includes the ASCII space character, but not tabs, line-feeds, carriage-returns, control characters, or other non-printing characters.

Leading and trailing spaces are not allowed and will cause the order to be rejected.

Runs of more than one space in are not allowed and will cause the order to be rejected.
