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

Six stages are pure sweeps (1, 2, 3, 18, 19, 21) and four are orders and a
sweep (4, 11, 14, 22). The rest are orders, except that stage 13 is one step of
orders and one step that is a sweep.

A lettered sub-step is a step of a stage, and steps run in the lettered order:
every order of step a resolves before any order of step b, whichever way round
a player wrote them. Within a step, the order of the lines in the file decides.
A step is therefore exactly a `Phase`, which is why move and jump are two
phases and not one, and why twenty-two stages come to forty-two phases.

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

**Orders.**

`create` -- ship, colony (open-air, enclosed, or orbital, optionally as a trade
station), factory group, farm group, and mine group.

Create orders are executed in the order they are written in the input.

This is the stage that was *set up*. Every `create` form resolves here, in one
stage, because a Spec carries one phase and `create` is one verb.

A create order assembles the units it is given, so it does not wait for
assembly at stage 10. It spends what the creating entity already holds:
creation is upstream of transfers (9), the market (11), and movement (15), so a
create cannot spend units that arrived this turn, and a ship cannot move
somewhere and plant a colony there in the same turn.

Creation is upstream of the group-change stages (7, 8), so a group created this
turn can be retooled or added to in the same file.

Creation may allocate and assign transports. This could reduce availability for later actions.

`create` is the only order that names its construction workers, and it names
them for two reasons that belong to it alone: it is the only assembly that may
need transports, and a create order may take several turns to finish. The
faction pre-allocates the `CWKR` cadre and it stays reserved for the duration.
Everywhere else the engine allocates construction workers itself -- see stage
8.

### 6. Dis-assembly orders are processed

**Orders.**

`unassemble`

Working units become unassembled inventory, optionally stowed to cargo. This is
before transfers (9) and the market (11) on purpose: units must be unassembled
to be transferred, bought, or sold, so the sequence is unassemble here, move
them there, assemble again at stage 10.

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
which is why no order but `create` names a cadre.

Removal unassembles what it takes out and optionally stows it, and the engine
salvages what it can of the work in progress. A mine group has no work in
progress, so its units idle at once; a factory or farm group's units keep
working until the line drains.

### 9. Transfers are processed

**Orders.**

`transfer`

Units and population move between two entities at the same location. The order
fails if they are not co-located when it executes, and is partially filled if
there are not enough transports. Transfers are after dis-assembly (6) and
before assembly (10), which is what makes the unassemble-move-assemble pipeline
work inside one turn.

Transfers are executed in the order they are written in the input.

### 10. Assembly orders are processed

**Orders.**

`assemble`

Unassembled units become working ones, usually taking more volume than they
did. Units that arrived by transfer at stage 9 can be assembled here in the
same turn.

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
`refuse trade` are administrative orders and resolve at stage 20, so a
permission granted this turn is first usable by the market next turn.

Inventory is updated at the end of the phase.
This prevents factions from selling and buying the same batch of items in a single turn.

### 12. Surveys are carried out

**Orders.**

`survey`

An entity reads the planet it is at. Like probes, a survey reads where the
entity stood when the turn began, because movement is at stage 15.

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
which stages 18 and 19 then resolve.

`neutralize` is the order that used to open with `attack`. It is not combat --
it spends spies against another faction's spies rather than committing a
percentage of an entity to a battle -- and it was renamed so that stage 4 and
stage 14 do not both claim the verb.

Steps a and b are separate steps because the engine allocates resources per
step. They were both `report` until the merge, which asked one `Spec` to carry
two phases; they are two verbs now, and neither reads as the default with the
other as a special case.

### 15. Ship movement occurs

  a. Move orders executed -- `move`
  b. Jump orders executed -- `jump`

Both sub-steps are orders. They are two phases and not one because every move
must finish before any jump begins: a ship moves inside its stellium, then
jumps between stellia, and a file that writes the jump first still moves first.

A jump begins from the stellium orbit, which is why step a is ahead of step b
rather than merely beside it: a ship at a planet has to be moved out in the
same turn before it can go. That much is built.

A jump of _d_ light years by a drive at technology level _t_ will take
\(\lceil d / t \rceil\) turns to complete. That is a pending change to the
order rather than a rule of the stage, and it lands when the engine is
implemented -- see *Orders that outlive their turn* below.

Everything that reads the world -- combat, surveys, probes, sensors, espionage
-- has already happened, so a turn's movement is what the *next* turn's reports
describe.

### 16. Draft and disband orders are processed

**Orders.**

  a. `draft`
  b. `disband`

Population is drafted into a cadre or released from one. Both are here rather
than earlier because a draft changes who is available to be paid at stage 17
and who counts at stages 18 and 21.

Population counts are updated at the end of the phase.

### 17. Pay and ration orders are entered

**Orders.**

  a. `pay`
  b. `rations`

The rates set here are the input to rebellion at stage 18 and to population
growth at stage 21, which is why they are the last thing a player says about
population before the game answers.

### 18. Rebellion occurs

**A sweep. No orders.**

What this turn's rebels do, given the pay and ration rates just entered.

### 19. Rebel increases take place

**A sweep. No orders.**

`REBL` is recalculated. It runs 0 through 99, so an entity is never wholly in
rebellion (`docs/units.md`).

### 20. Naming, control, and permission orders are processed

**Orders.**

  a. `release`
  b. `grant`
  c. `refuse`
  d. `name`
  e. `control`

Everything administrative, and the only stage where `we` -- the faction itself
-- is the subject of most of the orders.

`control` is a physical act and is given to an entity present at the place; it
fails against anything already controlled. It is downstream of movement (15),
so a ship that arrives this turn can take control of what it finds
uncontrolled. `release` is administrative, takes `we` as its subject, and needs
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

### 21. Population increases are calculated

**A sweep. No orders.**

Growth, given the rations set at stage 17 and the food on hand.

### 22. News service reports are compiled

**Orders and a sweep.**

`broadcast`

A broadcast is a message released at a place, so it is an order; compiling the
turn's news is a sweep. The stage is last because it reports on everything the
other twenty-one did.

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
- **`grant` and `refuse` resolve only at stage 20.** Trade permissions looked
  like market activity (11) and colonize permissions like control (20). Both
  are at 20, which keeps one phase per verb and gives permissions a consistent
  rule: granted this turn, in force next turn.
- **`create` is one step for five kinds of thing.** Ships, colonies, and the
  three group kinds all resolve at stage 5. The consequence is that a group is
  created before it can be retooled (7) or resized (8), which is the useful
  direction, and that nothing created this turn produces until the production
  stages of a later one -- a create may take several turns to finish.
- **`add` assembles what it adds.** A group holds working units -- `remove`
  unassembles what it takes out -- and `create` assembles what it is given.
  `add` does the same, so stage 8 does not have to wait on assembly at stage
  10. Assembly is what both orders need construction workers for.

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

## Orders that outlive their turn

Two orders are no longer finished when the turn that carried them resolves.
`create` may take several turns, which is why it pre-allocates its `CWKR` cadre
and holds it for the duration (stage 5), and `jump` is to become the same: the
crossing takes \(\lceil d / t \rceil\) turns, the distance over the drive's
technology level. That part of the jump rework is pending; the other two
changes -- that a jump begins from the stellium orbit, and that technology
level no longer caps the distance -- are built.

Nothing in the pipeline knows about such an order yet. Today `game_order.status`
is a three-way `CHECK` -- `pending`, `succeeded`, `failed` -- where `pending`
means submitted and not yet resolved, and where `ec turn open` purges rows older
than the most recently resolved turn. An order still running at the end of its
turn is a fourth thing: resolved, not failed, not done. Both the status column
and the purge have to learn about it before either order can be built.

## What the sequence still does not settle

The stages below are named but their rules are not written anywhere yet, so the
sequence says when they happen and nothing about what they do: production
(1, 2, 3), combat (4), the market's matching rule (11), espionage (14),
rebellion and rebel increases (18, 19), population growth (21), and the news
service (22). `docs/plan/beta-plan.md` carries the open questions per order.

One of them is settled rather than open: the engine allocates the `CWKR` cadre
for every assembly except `create`, and the faction's job is to have drafted
enough. What a construction worker costs, and what happens when a faction has
not drafted enough of them, are still unwritten.

## Stages, phases, and what is built

The engine's `phases` table is flat: it has one entry per *step*, not one per
stage, so the twenty-two stages below come to forty-two phases. Two of them are
built.

| Stage | Phases, in order | Shape | Built |
| --- | --- | --- | --- |
| 1. Mining production | `mining` | sweep | no |
| 2. Farming production | `farming` | sweep | no |
| 3. Factory production | `manufacturing` | sweep | no |
| 4. Combat | `raid`, `support`, `attack`, `invade` | orders + sweep | no |
| 5. Creation | `create` | orders | no |
| 6. Dis-assembly | `unassemble` | orders | no |
| 7. Build change | `retool` | orders | no |
| 8. Group change | `idle`, `remove`, `add`, `activate` | orders | no |
| 9. Transfers | `transfer` | orders | no |
| 10. Assembly | `assemble` | orders | no |
| 11. Market and trade stations | `sell`, `buy` | orders + sweep | no |
| 12. Surveys | `survey` | orders | no |
| 13. Probe and sensor reports | `probe`, `sensor` | orders, sweep | **yes** |
| 14. Espionage | `assess`, `detect`, `obtain`, `convert`, `incite`, `neutralize` | orders + sweep | no |
| 15. Ship movement | `move`, `jump` | orders | **yes** |
| 16. Draft and disband | `draft`, `disband` | orders | no |
| 17. Pay and rations | `pay`, `rations` | orders | no |
| 18. Rebellion | `rebellion` | sweep | no |
| 19. Rebel increases | `rebels` | sweep | no |
| 20. Naming, control, permissions | `release`, `grant`, `refuse`, `name`, `control` | orders | partly -- `name` only, and not the `we` forms |
| 21. Population increases | `population` | sweep | no |
| 22. News service | `news` | orders + sweep | no |

The phase names are the names a player sees in `ec orders help`, not
identifiers that exist yet. The two that do exist -- `probe` and `sensor` --
run in that order in `spec.go`, and `move`, `jump`, and `naming` after them,
which is stages 13, 15, and 20 with everything in between still to be written.
