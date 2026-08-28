# The Turn Sequence

A turn is twenty-two stages, resolved in the order below. This document says
what each stage does and which orders resolve in it.

It is a design target, not a description of what is built. `docs/orders.md` is
what exists today; `docs/accepted-orders.md` is the accepted order set and the
authority on what each order *says*. This document is the authority on *when*
each order takes effect, and it is what the `phases` table in
`internal/orders/spec.go` is derived from.

## How to read a stage

Every stage is one of three shapes, and which shape it is decides how it gets
built.

- **Orders.** The stage is its orders and nothing else. Every order of the
  stage resolves before any order of the next stage; the order of lines in a
  player's file decides only between orders of the same stage. Move and jump
  are stages of this shape today.
- **A sweep.** The stage is something the game does that nobody ordered:
  production is calculated, population grows, rebels multiply. No verb names
  it. `Phase.Sweep` is the hook, and the sensor stage is already built this
  way.
- **Orders and a sweep.** The orders declare intent -- who commits what, to
  which fight, at what price -- and the sweep settles all of them against each
  other. Combat, the market, and espionage are all this shape: a battle is
  settled between the fleets that met rather than one order at a time, and the
  market matches offers across every faction that made one.

Six stages are pure sweeps (1, 2, 3, 18, 19, 21) and seven are orders and a
sweep (4, 5, 9, 10, 11, 14, 22), where the orders declare intent and one sweep
settles all of them against each other. Seven more are orders and nothing else
(6, 7, 8, 12, 16, 17, 20). The last two, 13 and 15, mix the two a milder way: a
step of orders and then a whole step that is a sweep -- the passive sensor
reading in 13, the ships landing in 15 -- settling nothing between the orders,
only doing what the stage does apart from anyone's.

Stage 5 is the one to read carefully, because its sweep is not the combat or
market kind. What it settles is not this turn's orders against each other but
every standing build against the creating entity's stock: the same shape, a
different set of competitors.

A lettered sub-step is a step of a stage, and steps run in the lettered order:
every order of step a resolves before any order of step b, whichever way round
a player wrote them. Within a step, the order of the lines in the file decides.
A step is therefore exactly a `Phase`, which is why move and jump are two
phases and not one, and why twenty-two stages come to forty-five phases.

## The stages

### 1. Mining production is calculated

**A sweep. No orders.**

Every mine group works the deposit it was created with.

### 2. Farming production is calculated

**A sweep. No orders.**

Every farm group works the units it holds.

Both stages read the groups as they stood at the end of the previous turn. A
group created, resized, idled, or activated this turn does none of this work
until next turn, because the stages that change groups (5, 7, 8) are all
downstream of here, and the same is true of the factory groups at stage 3.

### 3. Factory production is calculated

**A sweep. No orders.**

Every factory group runs the line it was tooled for. Work in progress advances
and a retooling started on an earlier turn counts down.

Manufacturing is the last of the three production stages, so a line can draw on
what was mined and grown this turn.

### 4. Combat takes place

**Orders and a sweep.**

  a. `raid`
  b. `support`
  c. `attack`
  d. `invade`

The orders name a target and commit a percentage of the entity; the sweep
settles every battle between the entities that met. Combat is before movement
(15), so a fleet fights where it stood when the turn began -- a ship cannot
arrive and attack on the same turn -- and it is after production (1, 2, 3), so
a colony that falls this turn still delivered this turn's output.

`support ... attacking` and `support ... defending` are one verb; the defending
form is what used to be a separate defend order.

Combat may allocate and assign transports. This could reduce availability for later actions.

### 5. Creation orders are processed

**Orders and a sweep.**

`create` -- ship, colony (open-air, enclosed, or orbital, optionally as a trade
station), factory group, farm group, and mine group.

Create orders are executed in the order they are written in the input.

This is the stage that was *set up*. Every `create` form resolves here, in one
stage, because a Spec carries one phase and `create` is one verb.

**The five forms finish by two opposite rules.** A ship or colony create is a
commitment: it succeeds the moment it is given, an unfinished entity appears,
and the build runs for as many turns as it needs. A group create is
kill-and-fill: it runs once, here, and the engine builds as much as the labour
and fuel it can find will pay for and then closes the order out. A colony
ordered to create 10,000,000 mines with the resources for 2 creates 2, and the
order is finished. Nothing carries over.

A group create assembles the units it is given, so it does not wait for
assembly at stage 10. A ship or colony create does not: it acquires its
materials over turns, and they are assembled at stage 10 like everything else.

What stage 5 settles for a build is the **claim** -- which build has first call
on what the creating entity holds. The sweep takes each creating entity's builds
in seniority order, which is the order of their entity ids, and walks each
build's lines in the order the player wrote them. A claim lives one turn and is
released at the end of it, so next turn's claiming runs afresh and a senior
build's priority is renewed rather than banked. Claiming moves nothing and needs
no transports: delivery is at stage 9 and assembly at stage 10.

Claiming is here because creation's ordering has always been settled here, and
it keeps what the old rule was protecting: creation is upstream of transfers
(9), the market (11), and movement (22), so a build cannot claim units that
arrived this turn, and a ship cannot move somewhere and plant a colony there in
the same turn.

Creation is upstream of the group-change stages (7, 8), so a group created this
turn can be retooled or added to in the same file.

`create` is the only order that names its construction workers, and it names
them for a reason that belongs to it alone: a ship or colony build runs for
several turns, so it is the only assembly a player needs to be able to throttle.
The `with` clause is a **ceiling on the workers a build may use in a turn**, not
a reservation. It holds nothing back: the engine assigns up to that many from
whatever is idle when stage 10 comes round, never more however many thousands
are standing about, and a turn that cannot fill the cap costs that turn's work
and nothing else. Everywhere else the engine allocates construction workers
itself -- see stage 8.

### 6. Dis-assembly orders are processed

**Orders.**

  a. `unassemble`
  b. `stow`

Both steps move units away from the sections they work in, which is what makes
them ready to go somewhere. Working units become unassembled inventory,
optionally stowed to cargo; a plain `stow` then moves unassembled inventory to
cargo, which is where a transport picks a load up.

The order of the two steps is what lets one file do both halves: a plain
`unassemble` at step a leaves units in unassembled inventory, and a `stow` at
step b can pick them up in the same turn. `unassemble` `and` `stow` does both at
step a for units that were assembled, and is construction work throughout at no
extra cost; a plain `stow` at step b is production labour moving freight. See
[Accepted Orders](accepted-orders.md).

This is all before transfers (9) and the market (11) on purpose: units must be
in cargo to be transferred and unassembled to be bought or sold, so the sequence
is unassemble and stow here, move them there, unstow or assemble at stage 10.

### 7. Build change orders are entered

**Orders.**

`retool`

What a factory group makes. The plain form drains the line first, which may
take three turns, and spends a turn retooling after that; the `immediately`
form discards the work in progress and spends the retooling turn now. Either
way production resumes at stage 1 of a later turn.

### 8. Group change orders are entered

**Orders.**

  a. `idle`
  b. `remove`
  c. `add`
  d. `activate`

This is the stage that was *mining change*, widened to every kind of group. A
mine group's deposit is fixed for its life, so moving a mine is now `remove`
followed by a fresh `create` next turn rather than an order of its own.

The four verbs are one stage because they are four verbs that read the same
three group kinds; splitting factory groups from mine and farm groups would ask
one Spec to carry two phases.

Retooling (7) is ahead of membership changes here, so a group can be retooled
and resized in the same turn and the resize applies to the retooled group.

`add` assembles the units it puts into a group, the way `create` does, so a
player does not have to assemble on one turn and add on the next even though
assembly is two stages further down at 10. That is also what makes both orders
need construction workers.

The engine allocates the `CWKR` cadre for an `add`, and for a bare `assemble`
at stage 10, without being told to. Drafting enough construction workers to
cover the turn's expected work is the faction's responsibility, not the order's,
which is why no order but `create` names a cadre -- and `create` names one
because it runs for several turns, not because it is allocated any differently.

**A construction worker does one task per turn**, so one entity's cadre pool is
drawn down in stage order as the turn runs: a group create at stage 5 takes its
workers first, then an `add` here, then the `assemble` orders and the ship and
colony builds at stage 10. No precedence rule is needed for that; the sequence
already decides it.

Removal unassembles what it takes out and optionally stows it, and the engine
salvages what it can of the work in progress. A mine group has no work in
progress, so its units idle at once; a factory or farm group's units keep
working until the line drains.

### 9. Transfers are processed

**Orders and a sweep.**

`transfer`

Units and population move between two entities at the same location. The order
fails if they are not co-located when it executes, and is partially filled if
there are not enough transports. Transfers are after dis-assembly (6) and
before assembly (10), which is what makes the unassemble-move-assemble pipeline
work inside one turn.

This is also where a build's **delivery** happens: what it claimed at stage 5
moves from the creating entity to the unfinished one, and the build's
construction workers ride out on the same transports. One sweep settles every
transfer order and every build's delivery against the transports each entity
has, so file order cannot silently decide who got the last hull. That is what
makes `transfer` an intent-declaring order in the sense combat and the market
already use.

> **Explicitly ordered work outranks a standing commitment.** A `transfer` order
> is served before a build's claim, here, and an `assemble` order before a
> build's own assembly at stage 10. A build takes what is left, which is all it
> ever needs to do: a build never fails for want, it only slows.

Everything a transport carries is set down in `cargo`, so delivery does not
assemble. Assembling it is stage 10's work, whether an order asked for it or a
build is working down its own list.

Transfers are executed in the order they are written in the input, which is the
tie-break inside the sweep rather than the whole of the rule.

### 10. Assembly orders are processed

**Orders and a sweep.**

  a. `unstow`
  b. `assemble`, and the build sweep

Both steps move units toward the sections they work in, which is the mirror of
stage 6. `unstow` takes units out of cargo and puts them in unassembled
inventory; `assemble` puts them to work.

`unstow` is here rather than earlier because what it moves is usually what a
transfer set down at stage 9, and it is before the market (11) because units
must be unassembled to be bought or sold. It is **not** needed to assemble
anything: an `assemble` draws from unassembled inventory first and from cargo
after it, so a load delivered at stage 9 can be assembled at step b of the same
turn without being unstowed first.

Unassembled units become working ones, usually taking more volume than they
did.

A build **completes** here too: the workers carried out at stage 9 work off what
is on site, in the order the player wrote the lines, and then go home. The stage
is a sweep for the same reason stage 9 is -- the `assemble` orders and the
builds draw construction workers from one pool at the creating entity -- and it
is settled by the same rule. An `assemble` order is served first because it was
asked for by name; a build takes what is left and goes slower that turn.

The workers are shared but the **work is not pooled**. A build's workers are at
the new entity doing that build's work, so what they get through is reckoned for
that build alone and rounds up on its own. It does not pool with the creating
entity's `assemble` orders, nor with a sibling build.

### 11. All market and trade station activity takes place

**Orders and a sweep.**

  a. `sell`
  b. `buy`

Both forms trade units and technology levels.

Each order is an offer with a price and a commission. The sweep matches them,
preferring the transactions that return the market the highest commission, so
no order is settled on its own. Units bought or sold must be unassembled, which
is why dis-assembly (6) and assembly (10) sit either side of this stage.

A technology level trades in the same market but is not cargo: it is paid for
in whole `GOLD`, needs no transports, and is bought once rather than by
quantity.

Trade station permissions are *not* granted here. `grant trade` and
`refuse trade` are administrative orders and resolve at stage 19, so a
permission granted this turn is first usable by the market next turn.

Inventory is updated at the end of the phase.
This prevents factions from selling and buying the same batch of items in a single turn.

### 12. Surveys are carried out

**Orders.**

`survey`

An entity reads the planet it is at. Like probes, a survey reads where the
entity stood when the turn began, because nothing moves until the last stage
of the turn.

### 13. Probe and sensor reports are compiled

  a. Probe orders executed -- `probe`
  b. Passive sensor reports compiled -- **a sweep, no orders**

Both read the world as it stood before anything moved. A ship that jumps this
turn reports its new stellium next turn. The two sub-steps are independent --
neither reads what the other writes -- so their order between themselves does
not matter; the engine runs probe first.

### 14. Espionage activity takes place

**Orders and a sweep.**

  a. `assess rebels`
  b. `detect spies`
  c. `obtain information from`
  d. `convert rebels`
  e. `incite rebels`
  f. `neutralize`

Spies are committed by the order and the outcomes are contested, so this stage
has the same shape as combat: the orders say who spent how many spies on what,
and a sweep settles them against each other. Three of the six act on `REBL`,
which stages 17 and 18 then resolve.

`neutralize` is the order that used to open with `attack`. It is not combat --
it spends spies against another faction's spies rather than committing a
percentage of an entity to a battle -- and it was renamed so that stage 4 and
stage 14 do not both claim the verb.

Steps a and b are separate steps because the engine allocates resources per
step. They were both `report` until the merge, which asked one `Spec` to carry
two phases; they are two verbs now, and neither reads as the default with the
other as a special case.

### 15. Draft and disband orders are processed

**Orders.**

  a. `draft`
  b. `disband`

Population is drafted into a cadre or released from one. Both are here rather
than earlier because a draft changes who is available to be paid at stage 16
and who counts at stages 17 and 20.

Population counts are updated at the end of the phase.

### 16. Pay and ration orders are entered

**Orders.**

  a. `pay`
  b. `rations`

The rates set here are the input to rebellion at stage 17 and to population
growth at stage 20, which is why they are the last thing a player says about
population before the game answers.

### 17. Rebellion occurs

**A sweep. No orders.**

What this turn's rebels do, given the pay and ration rates just entered.

This is settled before anything moves, which is the reason ship movement is the
last stage rather than the jump alone: a rebellion gets its say about a ship
before the ship can leave.

### 18. Rebel increases take place

**A sweep. No orders.**

`REBL` is recalculated. It runs 0 through 99, so an entity is never wholly in
rebellion (`docs/units.md`).

### 19. Naming, control, and permission orders are processed

**Orders.**

  a. `release`
  b. `grant`
  c. `refuse`
  d. `name`
  e. `control`

Everything administrative, and the only stage where `we` -- the faction itself
-- is the subject of most of the orders.

`control` is a physical act and is given to an entity present at the place; it
fails against anything already controlled. It is *upstream* of movement (22),
so a ship takes control of what it found when the turn began rather than of
what it reaches this turn: a ship that arrives claims next turn. That follows
from movement being last and is worth stating, because the order of these two
stages used to say the opposite. `release` is administrative, takes `we` as its subject, and needs
no entity at the place at all -- a faction may release a planet whose garrison
is gone.

`grant` and `refuse` carry both the trade-station permission and the colonize
permission. Both resolve here rather than at stage 11 for the same reason
`release` does: no ship or colony carries them out. A permission granted this
turn is in force from the next.

Naming something you own is an order to the thing itself; naming a place, a
faction, or another faction's ship is a faction order.

Two things follow from the order of the steps rather than from any rule: a
faction may `release` an entity at step a and take it back with `control` at
step e in the same turn, and it cannot change permissions on something it only
gained control of at step e.

### 20. Population increases are calculated

**A sweep. No orders.**

Growth, given the rations set at stage 16 and the food on hand.

### 21. News service reports are compiled

**Orders and a sweep.**

`broadcast`

A broadcast is a message released at a place, so it is an order; compiling the
turn's news is a sweep. The stage is last because it reports on everything the
other twenty-one did.

### 22. Ship movement occurs

  a. Move orders executed -- `move`
  b. Jump orders executed -- departures
  c. Jump orders executed -- arrivals

Steps a and b are orders. Step c is a sweep: nobody writes an arrival, so there
is nothing in it to order.

**This is the last stage of the turn, and that is a rule rather than a
convenience.** A ship resolves every other order of the turn where it began, it
leaves at the end of the turn, and it lands at the very end of the turn it is
due -- when there is nothing left to process, so nothing has to ask whether it
has landed yet. The 1978 sequence moved ships at stage 15 and a later edition
moved the stage to the end; this is that edition's answer, and everything below
follows from it.

They are three phases and not one because each has to finish before the next
begins. Every move finishes before any jump: a ship moves inside its stellium,
then jumps between stellia, and a file that writes the jump first still moves
first. A jump begins from the stellium orbit, which is why step a is ahead of
step b rather than merely beside it -- a ship at a planet has to be moved out in
the same turn before it can go.

A jump of _d_ light years by a drive at technology level _t_ takes
\(\lceil d / t \rceil\) turns to complete, never fewer than one, and **the
crossing costs the ship every one of them**: it departs in step b of turn _N_
and lands in step c of turn _N_ + \(\lceil d / t \rceil\), so it is gone for
the whole of every turn in between and for the whole of the turn it arrives in.
The shortest crossing there is still costs a turn of orders. **The crossing is
not the order**, either. Step b is the whole of the order: it draws the
whole fuel bill, takes the ship off the board, and succeeds. What it leaves
behind is an `in_transit` row saying which ship is bound for which stellium and
on which turn it is due. Step c reads those rows and lands every ship due this
turn in the destination's stellium orbit, deleting the row as it goes.

Departures come before arrivals, so this turn's jumps are settled before this
turn's landings and a ship cannot be caught by a jump order written the turn it
arrives. Between the two steps a ship is nowhere at all -- no stellium, no
system, no planet -- so nothing sees it and no order reaches it.

**A ship travels twice a turn at most, and never the same way twice.** One move
in step a and one jump in step b, which is how a ship at a planet leaves: the
move takes it out to the stellium orbit and the jump takes it away. Two moves
for one ship, or two jumps, are refused along with the file that wrote them.
The order is spent whatever it goes on to do, so a move that failed for want of
fuel has still been the ship's move for that turn.

A ship in transit can be given no order for any turn of its crossing, the turn
it lands included: it is nowhere when that turn's orders bind, and it is still
nowhere at every stage that could name it, because the stage that lands it is
the last one. That is why "is this ship reachable?" has one answer for a whole
turn and can be settled when the file is read rather than as the turn runs.

Everything that reads the world -- combat, surveys, probes, sensors, espionage
-- has already happened, so a turn's movement is what the *next* turn's reports
describe.

## What the merge settled

A `Spec` carries one verb and one phase. Laying the accepted order set over the
sequence is what turned that into four questions, and all four are now answered.

- **The espionage `attack` became `neutralize`.** Combat
  (`colony 24 attack ship 18 75%`) at stage 4 and espionage
  (`colony 24 neutralize faction 1 spies using 11 spies`) at stage 14 were the
  same subject shape and the same verb, with different grammar, in different
  stages. The espionage one was renamed because it is not combat: it spends
  spies rather than committing a percentage of an entity, and the spy orders
  already read as six verbs rather than as one `spy` verb with six objects.
- **`grant` and `refuse` resolve only at stage 19.** Trade permissions looked
  like market activity (11) and colonize permissions like control (20). Both
  are at 20, which keeps one phase per verb and gives permissions a consistent
  rule: granted this turn, in force next turn.
- **`create` is one step for five kinds of thing.** Ships, colonies, and the
  three group kinds all resolve at stage 5. The consequence is that a group is
  created before it can be retooled (7) or resized (8), which is the useful
  direction, and that nothing created this turn produces until the production
  stages of a later one. What one step does not make one is *finishing*: a
  group create closes out inside stage 5, and a ship or colony create runs for
  as many turns as its build needs.
- **`add` assembles what it adds.** A group holds working units -- `remove`
  unassembles what it takes out -- and a group create assembles what it is
  given. `add` does the same, so stage 8 does not have to wait on assembly at
  stage 10. Assembly is what both orders need construction workers for. A ship
  or colony create is the exception on both counts: its materials arrive over
  turns rather than being on hand when the order is given, so it assembles at
  stage 10 like everything else.

No step of any stage holds more than one verb. `create` is the only verb whose
several *forms* share a step, and that is unremarkable: `name` has six forms in
one step, `add` and `remove` read three group kinds each, `support` covers
attacking and defending, and `buy` and `sell` each trade units and technology
levels. Every one of those is one verb, one phase.

- **`report` became `assess` and `detect`.** `report on rebels` and
  `report on spies` were one verb across two steps, which asks a `Spec` for two
  phases. Merging the steps was not available -- the engine allocates resources
  per step -- so the grammar moved instead: `assess rebels` at 14a and
  `detect spies` at 14b. The espionage stage was already six distinct verbs
  rather than one `spy` verb with six objects, so two more names cost nothing
  structural, and these are genuinely different acts spending separately
  allocated resources.

Renaming was the smaller change. The alternative was moving `Phase` off `Spec`
onto the bound order -- a hook that picks the phase from the parsed parameters,
with `PhaseOf` reading the stored `params` alongside the `verb`, both of which
are columns. That would have preserved the accepted grammar exactly and would
also let `grant` and `refuse` split across stages 11 and 20 if that ever
matters, but every other order would pay for it in reading to buy one order's
grammar.

## What outlives the turn that ordered it

Two effects outlast the turn that ordered them, and they turned out to be the
same shape.

`jump` is the smaller of the two, because the crossing is not the order. The
order departs and is done -- fuel drawn, ship off the board, `succeeded` -- and
what continues is a ship in transit, which is a row of its own naming the ship,
the stellium it is bound for, and the turn it is due. Stage 15's arrival sweep
consumes those rows. **So `jump` needs no new order state at all**: it needs the
in-transit row, a nullable location on `entity` so a crossing ship is genuinely
nowhere, and a sweep on the jump phase, which is a seam the engine already has.

`create` is the larger one and takes the same shape after all. A ship or colony
create succeeds the moment it is given, and it holds nothing: the `with` clause
is a per-turn cap rather than a reservation, so no cadre is tied up between
turns. What continues is not the order but the build -- an unfinished entity,
with rows of its own naming who is feeding it, what it still wants, and how many
workers it may use in a turn. The sweeps at stages 5, 9, and 10 consume those
rows, and the last line to complete deletes them. **So `create` needs no new
order state either**, and in particular no fourth `game_order.status`: nothing
changes in the status `CHECK`, in `loadOrders`, or in what `ec turn open`
purges.

Splitting the two was still worth the paragraph. It was written here as one
problem blocking both orders; it was two questions with one answer, and `jump`
could go first and alone. The open question -- whether `create` is better
modelled as work in progress recorded beside the order than as a state of it --
is settled the way `jump` was: beside it, and the fourth status is not needed.
`docs/plan/entity_build_bom_process.md` carries the design.

## What the sequence still does not settle

The stages below are named but their rules are not written anywhere yet, so the
sequence says when they happen and nothing about what they do: production
(1, 2, 3), combat (4), the market's matching rule (11), espionage (14),
rebellion and rebel increases (18, 19), population growth (21), and the news
service (22). `docs/plan/beta-plan.md` carries the open questions per order.

Construction workers are no longer among them. The engine allocates the `CWKR`
cadre for every assembly except `create`, and the faction's job is to have
drafted enough; a worker does one task and 500 MU of work a turn; and a
shortfall is a rate rather than a failure, costing a build that turn's progress
and a kill-and-fill order some of what it asked for. `docs/accepted-orders.md`
carries those rules.

## Stages, phases, and what is built

The engine's `phases` table is flat: it has one entry per *step*, not one per
stage, so the twenty-two stages below come to forty-five phases. Twelve of those
phases are built -- `create`, `unassemble`, `stow`, `transfer`, `unstow`,
`assemble`, `probe`, `sensor`, `move`, `jump`, `arrival`, `name` -- and stages
6, 9, 10, 13, and 22 are built entire, sweeps and all. Stage 5 has the ship and
colony forms of `create` and its claim sweep; the three group forms are a
different order with a different completion model and are not built.

A stage that is orders and a sweep gets no extra entry for the sweep:
`Phase.Sweep` runs after that phase's own orders, so creation's claim, the
delivery sweep at 9, and the assembly sweep at 10 ride on the `create`,
`transfer`, and `assemble` phases the way combat's and the market's do. Only a
sweep that is a lettered *step* of its own is a phase of its own -- `sensor` in
13, `arrival` in 22. Forty-five stands.

| Stage | Phases, in order | Shape | Built |
| --- | --- | --- | --- |
| 1. Mining production | `mining` | sweep | no |
| 2. Farming production | `farming` | sweep | no |
| 3. Factory production | `manufacturing` | sweep | no |
| 4. Combat | `raid`, `support`, `attack`, `invade` | orders + sweep | parses; the sweep is not written |
| 5. Creation | `create` | orders + sweep | ship and colony built; the three group forms parse |
| 6. Dis-assembly | `unassemble`, `stow` | orders | **yes** |
| 7. Build change | `retool` | orders | parses |
| 8. Group change | `idle`, `remove`, `add`, `activate` | orders | parses |
| 9. Transfers | `transfer` | orders + sweep | **yes** |
| 10. Assembly | `unstow`, `assemble` | orders + sweep | **yes** |
| 11. Market and trade stations | `sell`, `buy` | orders + sweep | parses; the matching sweep is not written |
| 12. Surveys | `survey` | orders | parses |
| 13. Probe and sensor reports | `probe`, `sensor` | orders, sweep | **yes** |
| 14. Espionage | `assess`, `detect`, `obtain`, `convert`, `incite`, `neutralize` | orders + sweep | parses; the sweep is not written |
| 15. Draft and disband | `draft`, `disband` | orders | parses |
| 16. Pay and rations | `pay`, `rations` | orders | parses |
| 17. Rebellion | `rebellion` | sweep | no |
| 18. Rebel increases | `rebels` | sweep | no |
| 19. Naming, control, permissions | `release`, `grant`, `refuse`, `name`, `control` | orders | `name` built but for its two faction forms; the rest parse |
| 20. Population increases | `population` | sweep | no |
| 21. News service | `news` | orders + sweep | parses; the sweep is not written |
| 22. Ship movement | `move`, `jump`, `arrival` | orders, orders, sweep | **yes** |

The phase names are the names a player sees in `ec orders help`, not
identifiers that exist yet. Thirty-nine of them now do: `spec.go` carries every
phase that has an order, in the order this document gives them, and
`ec orders help` prints the list.
