# Building an entity — the `create` design

This is a design note, not a built thing. It replaces an earlier draft written
from outside this project's vocabulary; the words, the unit codes, and several
of its rules have been brought to what `docs/accepted-orders.md`,
`docs/turn-sequence.md`, `docs/entity-location.md`, and `docs/units.md` say.
`docs/plan/bom-rewrite.md` records what changed and why.

It covers **`create ship` and `create colony`** and nothing else. The three
group forms are a separate matter; see "The group create forms" at the end.

Six decisions shape it:

1. **A create is a commitment, not a purchase.** The order says *build this as
   fast as you can*, and it succeeds the moment it is given. Everything after
   that is rate, not failure. This is the opposite of most orders in the game,
   and it is the thing to hold on to when any detail below looks odd.
2. **Nothing is banked.** Materials are claimed, transports are used, and
   workers are assigned **per turn, and released at the end of it**. A build
   that got nothing this turn is not behind; it simply did not move, and next
   turn it competes again on equal terms.
3. **Nothing here falls out of orbit.** `SDRV` is the orbital drive and an
   entity that cannot hold its own mass up does descend a ring a turn and die at
   ring 0 — but that is a **combat** rule, it is not written yet, and it does not
   act on an entity under construction. For a build, a ring is just where the
   thing is.
4. **A build is live state beside the order, not a state of the order.** The
   `create` order departs and succeeds, the way `jump` does, and an
   `under_construction` row carries on. `game_order.status` needs no fourth
   value.
5. **A build's turn is three sweeps, on three stages** — it claims at 5,
   delivers at 9, and assembles at 10, each hung where the resource it competes
   for is settled.
6. **The two lists the order names are its recipe and its priority.** List order
   decides what gets scarce materials, transport, and workers first.

---

## The order

The accepted form is unchanged (`docs/accepted-orders.md`):

```text
ship 18 create ship
  using 60 STRC-8,
        61 HDRV-1, 5 SDRV-1
        , 5 LFSU-3, 1 SNSR-1
  transfering 25 FOOD, 5 SKW,  16,800 FUEL, 93 GOLD
  with 500 CWKR
end
```

Three clauses, and they do not mean the same thing:

- **`using`** names the units the new entity is made of. A create assembles what
  it is given, so the player writes no separate `assemble` order for them: they
  are set down in the new entity's `cargo`, which is what a transport does with
  anything, and the build's own workers move them into `component`, or into
  `operational` for a unit that works there.
- **`transfering`** names what is handed over rather than built in: cargo, and
  population. `5 SKW` is five units of skilled workers, which is five hundred
  people, and it goes to `entity_population` rather than to `inventory`.
- **`with` _quantity_ `CWKR`** is a **per-turn cap on labour**, not a
  reservation. It holds nothing back: every turn the engine assigns up to this
  many workers from whatever the creating entity has idle, and it may never
  assign more than this many however many thousands are standing about.
  `create` is the only order that names its workers, because it is the only
  assembly that runs for several turns and so the only one a player needs to be
  able to throttle.

Within each clause, the order the player wrote is the priority. There is no
explicit sequence in the grammar and none is wanted.

**Completion means two things**, one per clause. A `using` line is completed
when its units are assembled into the new entity. A `transfering` line is
completed when its units are stowed in cargo, or, for a population class, when
the people are aboard. Saying *completed* rather than *assembled* is what keeps
the model from having to assemble people.

---

## Construction workers commute

A `CWKR` is a cadre — a temporary assignment of population — but the population
it assigns is real, and it is two units, not one: **one `CWKR` is one `SKW` plus
one `USK`**, allocated by a draft.

```text
ship 18 draft 4,250 CWKR
```

That takes 4,250 `SKW` and 4,250 `USK` off the ship's rolls and makes a cadre of
4,250 construction workers. Drafting enough to cover a turn's expected work is
the faction's job, not an order's.

So workers have mass and take space like anyone else — 4 MU and 4 VU per `CWKR`
unit, being two population units — and they have to be carried to the site.
**But they never change owner.** They ride out on a transport, work the shift,
and come home. No population row moves, the creating entity's mass is the same
at the end of the turn as at the start, and the only thing the commute spends is
transport capacity and the fuel that goes with it.

This is what makes `with 500 CWKR` a real decision rather than a formality: 500
`CWKR` is 1,000 population units, which is 2,000 MU to move out and back every
turn the build runs.

### The cap is how labour is shared

Seniority alone would let the oldest build take every worker it could use, turn
after turn, and starve everything behind it. The `with` clause is what stops
that: it is the *only* lever a player has over how a builder's workers are
split, and it is why the number is worth thinking about rather than setting to
the size of the payroll.

A worked case. Two builds on one entity that has 1,100 `CWKR` idle: build 18
with `with 500 CWKR`, build 98 with `with 1,500 CWKR`. Build 18 is senior — ids
rise monotonically, so the lower one started first — and takes its cap of 500.
Build 98 takes what is left, 600, which is under its cap.

Next turn the sum is done again from scratch. If build 18 has finished, or the
faction has drafted more, build 98 takes up to its full 1,500. **Nothing was
lost and nothing was owed.** A lean turn costs a build that turn's work and
nothing else, which is what makes a shortfall a rate and not a wound.

---

## Transports

`TRAN` is the transport, at technology levels 1 through 10.

| | |
| --- | --- |
| Mass | 4 MU, flat — not \(2t\) |
| Volume | \(4t\) VU, both assembled and in cargo |
| Carries per turn | at most \(20t^2\) MU **and** at most \(60t^2\) VU |
| Crew | one `SKW` unit operates up to 10 transports a turn |
| Fuel per turn | \(\lceil \sum t^2 / 10 \rceil\) over the transports the entity used |

Three things about that table are deliberate and easy to "correct" by mistake.
`TRAN`'s mass does not scale with technology level. Its assembled and cargo
volumes are the *same* number, so the usual "operational is twice cargo"
multiplier does not apply to it. And the fuel is reckoned for all of an entity's
transports at once rather than one at a time, which is what keeps it in whole
numbers: ten `TRAN-1` and five `TRAN-2` in a turn is
\(\lceil (10 \times 1 + 5 \times 4)/10 \rceil = 3\) `FUEL`, and a single
`TRAN-1` costs \(\lceil 1/10 \rceil = 1\).

The crew is allocated by the engine. A player who wants 15 transports run needs
2 `SKW` units to be there; she does not have to say so.

Two properties worth knowing before anyone tunes these numbers:

- **Fuel per MU moved does not depend on technology level.** Capacity is
  \(20t^2\) and fuel is \(t^2/10\), so the ratio is flat. A better transport buys
  fewer hulls and fewer crew, not cheaper freight.
- **Mass is what binds, for nearly everything.** A transport carries 3 VU for
  every MU, and population (2 MU : 2 VU), the bulk resources (1:1), and an
  ordinary technology-level unit in cargo (\(2t\):\(2t\)) are all 1:1. The one
  exception in today's table is `TRAN` itself carried as freight, at 4 MU :
  \(4t\) VU, which becomes volume-bound from technology level 4 up.

Moving the 500 `CWKR` above, then: 2,000 MU, mass binding, is
\(\lceil 2000/20 \rceil = 100\) `TRAN-1`, 10 `SKW` to crew them, 10 `FUEL`. With
`TRAN-5` it is 4 transports, 1 `SKW`, and still 10 `FUEL`.

---

## A build is live state, not an order state

`in_transit` is the precedent, and the resemblance is exact. A jump order
departs — draws the whole fuel bill, takes the ship off the board, succeeds —
and a row of its own carries the crossing, purged by nothing and consumed by a
phase sweep. A create order does the same: it succeeds on the turn it is given,
and two tables carry the build.

```text
under_construction   entity_id  (primary key: the unfinished entity)
                     game_id
                     builder_entity_id       -- the entity feeding this build
                     cwkr_cap                -- the `with` clause: a per-turn
                                             --   ceiling, never a reservation
                     structure_complete

construction_item    entity_id, ordinal
                     clause                  -- 'using' or 'transfering'
                     unit, tech_level        -- or a population class
                     required
                     claimed                 -- this turn's call on the builder's
                                             --   stock; zero between turns
                     delivered               -- at the new entity, not yet done
                     completed               -- assembled, stowed, or aboard
```

`ordinal` is the line's place in its clause as the player wrote it, which is its
priority. What is still wanted on a line is
\(required - claimed - delivered - completed\); it is derived, not stored.

The presence of an `under_construction` row is the entity's status. `entity` has
no status column and does not need one: when the last item completes, the two
rows go and what is left is an ordinary entity.

**Seniority is `entity_id`, the row id.** A row id is unique, rises
monotonically, and is never reused, so the unfinished entities of one builder
are already in the order their builds started — within a turn, because create orders execute in the order they
were written, and across turns for the obvious reason. Nothing has to be stored
to settle seniority, and nothing has to reference `game_order`, which
`ec turn open` purges anyway.

Nothing purges either table, for the reason nothing purges `in_transit`: a build
is live state, not turn history.

---

## A build's turn is three sweeps

A build competes for three different things, and they are settled at three
different stages of the turn. So the build does not run in one place: each of
its three acts is a sweep on the phase where its scarce resource is decided.

| Stage | Phase | What the build does there |
| --- | --- | --- |
| 5. Creation | `create` — orders, then sweep | **Claim** |
| 9. Transfers | `transfer` — orders, then sweep | **Deliver**, and carry the workers out |
| 10. Assembly | `assemble` — orders, then sweep | **Complete**, and send the workers home |

A `Phase` already carries an optional sweep — what the phase does apart from
anyone's orders — and sweeps run in check, submit, and resolve alike, so this
needs no new machinery. The whole cost in bookkeeping is the one `claimed`
column above, written at stage 5 and consumed at stage 9.

### Stage 5 — claim

For each creating entity, take its builds in seniority order. For each build,
walk its eligible lines in `ordinal` order and put a claim on what the creating
entity holds and has not already promised:

```text
want   = required - claimed - delivered - completed
claim  = min(want, what the creating entity holds unclaimed)
```

Claiming needs no transport and moves nothing. It is the priority decision, and
it belongs at stage 5 because that is where creation's ordering has always been
settled. It also keeps the property the old rule was protecting: creation is
upstream of transfers (9), the market (11), and movement (15), so **a build
cannot claim units that only arrived this turn**, and a ship still cannot move
somewhere and plant a colony there in the same turn.

### Stage 9 — deliver

One sweep settles every claim and every `transfer` order against the transports
each entity has. What moves is taken out of the creating entity's inventory and
put into the new entity's. The construction workers ride out on the same
transports.

**Everything lands in `cargo`, and delivery does not assemble.** Cargo is where
a transport puts things down — the same rule `transfer` already states — so a
`using` unit sits in the new entity's cargo until stage 10, and a `transfering`
unit sits there because that is where it was going anyway. "A create assembles
the units it is given" means the player never writes a separate `assemble` order
for them, not that assembly happens the instant they arrive. If it did, stage 10
would have nothing to do and transport alone would set the build rate.

**What does not fit is released.** A claim lives for one turn; there is no
reservation carried over. Next turn's claiming runs afresh, in seniority order,
so a senior build's priority is renewed rather than banked.

This is why stage 9 has to be a sweep and not a run of orders one at a time: a
`transfer` order and a build's delivery at the same entity draw on the same
transports, and settling them one at a time would let file order silently decide
who got the last hull. It makes `transfer` an intent-declaring order, in the
sense `docs/turn-sequence.md` already uses for combat and the market.

### Stage 10 — complete

The workers carried out at stage 9 work off what is on site, in `ordinal` order,
and then go home.

How many there are is the smallest of three numbers: the build's `cwkr_cap`,
what the creating entity has idle after the senior builds have taken theirs, and
what the transports actually carried out. A worker who could not be carried
cannot work.

What they get through is 500 MU a turn each, pooled across everything the build
is assembling — one total, rounded up once, not a rounding per line. So a build
with 31 workers on site completes up to 15,500 MU of its `using` lines that
turn, in `ordinal` order, and stops mid-line when the work runs out.

Because the cadre is drawn per turn from the creating entity's idle workers,
this sweep **does** have to be settled against the `assemble` orders in the same
stage — they draw on the same pool. Stage 10 is therefore the same shape as
stage 9: orders and a sweep, settled together.

That gives one rule for both stages:

> **Explicitly ordered work outranks a standing commitment.** A `transfer` order
> is served before a build's claim, and an `assemble` order before a build's
> own assembly. A build takes what is left, which is all it ever needs to do,
> because a build never fails for want — it only slows.

### What a lean turn costs

Materials, transport, and workers limit a build independently, and a build that
gets none of them simply makes no progress. **After the entity exists, a
shortage is never a failure** — not of the order, which succeeded when it was
given, and not of the build, which is a standing commitment and will try again
next turn.

A build with no transports at all is the extreme case and it is still not an
error: the order succeeds, the entity exists, and every turn it delivers nothing
and completes nothing until transports reach the creating entity. That is a
correct outcome to see in a turn report, not a defect to guard against.

What still fails is only what could not have changed: a unit code that is not a
unit code, a colony ordered from an entity that is not at a planet. Those are
`Bind` failures and they reject the file, because they will still be true when
the turn resolves. **A shortfall of anything is never one of them**, because
every shortfall is a per-turn condition and the next turn may cure it.

---

## Eligibility, and why a line may be skipped

A line that cannot make progress this turn is **skipped**, and the sweep goes on
to the next one. An unavailable line never freezes the build.

> The order a player wrote the list in decides priority among the work that can
> be done now. A line that cannot be worked is passed over, not waited on.

This is what stops a badly ordered list from deadlocking a build while still
letting a badly ordered list cost the player turns. As soon as a skipped line can
be worked again it takes precedence over everything below it.

Two things make a line ineligible.

### Structure comes first

Until the structure is complete, only the `STRC` and `STRL` lines of `using` are
eligible.

This is not a preference; it is forced. A structural unit at technology level
\(t\), assembled in component inventory, creates \(t^2\) VU of raw enclosed
volume and consumes none. Usable enclosed space is that raw volume times the
entity's efficiency — 1 for `COPN`, 0.2 for `CSFC`, 0.1 for `CORB` and `SHIP` —
and everything else an entity holds consumes it. Before there is structure there
is nowhere to put anything.

`structure_complete` is set when every structural `using` line is completed. The
rest of the build becomes eligible then.

**The exemption that makes this possible is narrow: `STRC` and `STRL` only.**
Structural units delivered to an entity under construction sit in its cargo
consuming no enclosed space, which they could not otherwise do — the space they
would occupy is the space they have not yet created. Nothing else is exempt.

That is a tighter rule than "an unfinished entity ignores enclosed space", and
it is better, because it means **structure-first is not merely a priority — it
is forced.** Every other unit needs real enclosed space to be delivered into, so
it cannot arrive until structure has made some.

The bootstrapping is hand-waved and should be read that way: the workers are
assembling girders on a bare frame with no life support and no air. The game
does not model how, and does not need to.

### Population needs life support first

A population line is eligible only while the new entity's **assembled** `LFSU`
supports the people already aboard as well as the ones about to arrive:

```text
room = life support of assembled LFSU - population already aboard
move = min(still wanted, room, available at the creating entity, transport)
```

Delivered-but-unassembled `LFSU` supports nobody. This is a restriction on the
*delivery*, not on some later step: unsupported people never leave the creating
entity.

Because a population line is skipped rather than waited on, a player who writes
population above life support loses turns rather than the build:

```text
transfering 500 SKW, 10 LFSU-3, ...
```

The people are ineligible at first, the sweep works the `LFSU` line instead, and
as life support comes online the higher line becomes eligible again and takes
priority back.

Note that construction workers are not subject to this. They are not aboard —
they are on shift, and they sleep at home.

---

## Where the new entity appears

At the creating entity's planet, in the ring its kind requires:

| Created | Ring |
| --- | --- |
| `COPN`, `CSFC` | 0, the planet's surface |
| `CORB` | 1 |
| `SHIP` | drawn from 2 through 99, the way an arriving ship draws one |

A ship's ring is drawn from the game seed, the turn, and the order, so resolving
a turn twice reaches the same ring.

It follows that **a colony create requires the creating entity to be at a
planet**, and `Bind` refuses it otherwise — the same shape as a jump refusing a
ship that is not in the stellium orbit. A ship may be created from an entity at
the stellium level; it is created there.

**An open-air colony also requires a habitable planet.** A `COPN` breathes the
air outside, so it may only be created where the habitability number is above 0.
That is a second `Bind` refusal, and it is knowable at submission because a
planet's habitability does not change.

**The new entity's technology level is the creating entity's.** That settles the
`NOT NULL` column with nothing to look up and nothing for the player to write.

**Nothing falls while it is being built**, so the ring a new entity draws is not
a countdown. `SDRV` does hold a `SHIP` or `CORB` up, and one that cannot produce
thrust equal to its mass descends a ring a turn and is destroyed at ring 0 — but
that is a combat rule, unwritten, and it does not act on an entity under
construction. A build may therefore take as many turns as it takes without its
`SDRV` line being a race.

There is a loose end in this for whoever writes the combat rule, and it is worth
recording where it was noticed. If a ring is an altitude, then a ship ordered to
the planet it is already at drawing a *new* ring (`docs/entity-location.md`) is
a random change of altitude, and a ship arriving under its own power drawing 2
through 99 is a random amount of margin. Both read oddly once rings mean height.
Neither affects this design.

---

## The unfinished entity, while it is unfinished

- **It can be given no order.** `Binder.actor` already refuses a ship in
  transit; refusing an entity under construction is the same guard with one more
  clause, written once for every order there will ever be.
- **It belongs to the faction that is building it**, from the moment it exists.
  This is an exception to "an entity without population is controlled by the
  `uncontrolled` agent faction", and the exception is the point: an unfinished
  entity has no population yet by definition.
- **It is visible.** It sits at a planet and it has mass, so probes read it and
  passive sensors report its approximate mass like anything else. Hiding it would
  be a new rule and there is no reason to want one.
- **It grows.** Its `mass` rises as material is delivered and its
  `enclosed_volume` rises as structure is assembled, both maintained the way every
  other write-through mutation is.

---

## Completion

A build is complete when every line is completed:

```text
for every construction_item:  completed == required
```

The two rows are deleted and the entity becomes ordinary — it can be given
orders, it produces, it is counted. Nothing has to be released: the workers were
never held, only assigned a turn at a time, so the turn the build finishes is
simply the first turn they are all idle again.

---

## The group create forms

`create` is one verb with five forms, and only two of them build an entity. The
factory, farm, and mine group forms have no `using` list, no `transfering`
clause, and no `CWKR` clause; they name units and a purpose:

```text
colony 24 create factory-group with 54,000 FACT-6 making CNGD
ship 18 create farm-group with 1,234,000 FARM-6
colony 83 create mine-group with 25,680 MINE-2 working deposit 18
```

**Nothing in this note applies to them**, and the reason is sharper than mere
scope: they finish by the opposite rule.

A ship or colony create is a **commitment** — it succeeds at once and the build
runs for as many turns as it needs. A group create is **kill-and-fill**. It runs
once, in the turn it was given: the engine assigns as much labour and fuel as it
can find, builds as much as that pays for, reports what was built, and closes the
order. Nothing carries over. Ask for 10,000,000 mines with the resources for 2
and you get 2, and the order is done — it does not spend the next turns working
through the rest.

So `create` is one verb with two completion models, chosen by form. That costs
the pipeline nothing — one `Spec` may bind a different `Bound` per form — but it
is the thing to say plainly in player-facing documentation, because the two
behave as differently as any two orders in the game.

They complete within stage 5, which is what stages 7 and 8 need: a group created
this turn can be retooled and resized in the same file.

They also take their workers first. Every assembly draws on one cadre pool at the
creating entity and **a worker does one task per turn**, so the pool is drawn
down in stage order — a group create at stage 5, then an `add` at stage 8, then
the `assemble` orders and the ship and colony builds at stage 10. No new
precedence rule is needed; the turn sequence already decides it.

It does split one sentence in `docs/turn-sequence.md`, which says a create
"assembles the units it is given, so it does not wait for assembly at stage 10".
That stays true of the three group forms and becomes false of the other two,
which now assemble at stage 10 like everything else.

Their own rules are unsettled, and one schema gap remains: `work_group` has no
column for what a factory group is `making`.

---

## The rules this waited on, and how they landed

Every open question this design had is now answered. They are kept struck
through rather than deleted, because two of them were answered *against* what
this note first recommended and that is worth being able to find later.

- ~~What `SDRV` does.~~ **Settled: it is the orbital drive**, and the earlier
  draft of this note was right about the mechanic after all — an entity that
  cannot hold itself up falls a ring a turn and dies at ring 0. But **that is a
  combat rule and does not apply to a build.** An unfinished entity does not
  fall, `SDRV` draws no fuel and no crew until combat exists, and where a new
  entity appears stays the cosmetic choice settled above. Nothing in this design
  changes.
- ~~What `LFSU` supports.~~ **Settled: \(t^2\) population units per assembled
  unit**, and an entity over capacity loses the excess. The population gate is
  now implementable, and it earns its keep: without it a build would deliver
  people into a hull that kills them. Open only for `COPN`, which may need no
  life support at all.
- ~~Whether a commute is charged once or twice.~~ **Settled: once.** A transport
  provides there-and-back service, so a turn's capacity covers the round trip.
- ~~Whether explicitly ordered work outranks a standing commitment.~~
  **Settled: it does**, at stage 9 and stage 10 alike. In
  `docs/accepted-orders.md`.
- ~~What one `CWKR` completes in a turn.~~ **Settled: 500 MU a turn, and a
  build's assembly is its own pool** — it does not pool with the creating
  entity's `assemble` orders, nor with a sibling build, because the work happens
  at the new entity. The cadre they draw from is still shared, so they can
  starve each other of workers without pooling their work. In
  `docs/accepted-orders.md`.
- ~~On site but not yet worked.~~ **Settled: `STRC` and `STRL` delivered to an
  entity under construction consume no enclosed space, and nothing else is
  exempt.** `delivered` stays a real state, and structure-first becomes forced
  rather than merely preferred.
- ~~Where a group create's construction workers come from.~~ **Settled: the same
  cadre pool, drawn in stage order**, and a worker does one task per turn. Group
  creates are kill-and-fill — see "The group create forms" above.

---

## What this costs to build

Recorded here so the size is visible; none of it is written yet.

- Three phases with sweeps in `spec.go` — `create`, and sweeps added to
  `transfer` and `assemble` when those land.
- One `Spec` in `verbs.go`, plus the first multi-line parse: `Line` holds one
  physical line today and `Parse` is a per-line scanner loop, so the
  `end`-terminated form needs a continuation somewhere.
- Two readers the tokenizer does not have: a quantity with mandatory thousands
  separators (`54,000` currently tokenizes as three tokens) and a unit code with
  an optional technology level.
- Inventory mutation in `internal/world`, which has none. Today the only writes
  to `inventory` are `fuel`'s private draw-down and the kit loader, and no code
  path creates an `entity` row during a turn.
- A cadre table, so a drafted `CWKR` has somewhere to live. It is an assignment
  of `SKW` and `USK`, not a fifth `entity_population.class`, and it should
  generalise to `PLCF`, `SPCF`, and `TRNE`.
- Migrations for `under_construction`, `construction_item`, and that cadre table.
- The mass and volume rules, which exist once already as `metricsForUnit`,
  `parseUnitTag`, `isBulkResource`, and `usableEnclosedSpace` in
  `cmd/ec/kit.go`. They belong in an `internal/units` package on the
  `internal/fuel` template, so the kit loader and the order share one
  implementation rather than two. `TRAN`'s defined mass goes in
  `metricsForUnit` beside `jumpdrive.UnitMass` and `sensors.UnitMass`.
  `spaceWithTenPercentExcess` **stays where it is**: it is headroom the loader
  asks of a seed file so an agent-built kit can be hand-edited afterwards, not a
  rule of the game, and a build has no use for it.

**The starting kit is rework, and that is expected.** No player-controlled entity
in `games/beta/home-planet-seed.json` holds a transport, and life support is far
under the population aboard. Neither stops a create order — it succeeds and the
build simply makes no progress — but a kit that can finish a build is a
prerequisite for testing one.

Nothing here needs a fourth `game_order.status`, a change to `loadOrders`, or a
change to `ec turn open`'s purge list. That is what decision 4 bought.
