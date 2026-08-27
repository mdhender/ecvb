# The `create` design — feedback, and a rewrite in our terms

## Context

`docs/plan/entity_build_bom_process.md` proposed a bill-of-materials build
system for `create`. It was written from outside this project's vocabulary, so
almost every noun in it was a word we do not use, and several of its rules
contradicted rules `docs/accepted-orders.md` and `docs/turn-sequence.md` had
already settled.

`create` is the last order blocked on this repository rather than on the 1978
rules (`docs/plan/beta-plan.md`), so settling its design unblocks batch 2 of
step 5. Six decisions shape the rewrite. Four were taken first:

1. **The build queue stands.** A build acquires its materials over turns rather
   than spending what the creating entity holds on the turn it is ordered. This
   overturns `docs/turn-sequence.md:106`.
2. **Orbital decay is out of scope for a build.** It was taken here as "dropped
   — rings are slots, not altitudes", and that turned out to be too strong:
   `SDRV` is the orbital drive and an entity that cannot hold its own mass up
   does fall a ring a turn and die at ring 0. But that is a **combat** rule, it
   is not yet written, and it does not touch an entity under construction. The
   design below is unaffected either way.
3. **A build's state lives in a table beside the order**, not in a fourth
   `game_order.status`. This is the `in_transit` shape, and it is the answer
   `docs/turn-sequence.md:477` left open.
4. **Documentation only this session. No code.**

Two more came out of the rules supplied afterwards, and they are the substantive
half of this note:

5. **A build's turn is three sweeps on three phases**, not one sweep at stage 5:
   it claims at stage 5, delivers at stage 9, and assembles at stage 10. See
   "How delivery reaches stage 9 and assembly stage 10" below.
6. **Nothing is banked.** Materials, transports, and workers are all taken per
   turn and released at the end of it. `with 1,500 CWKR` is a **per-turn cap**,
   not a reservation: the engine assigns up to that many from whatever is idle,
   and never more however many are standing about.
7. **A create is a commitment, not a purchase.** The order succeeds when it is
   given and the build then goes as fast as it can. Every shortfall is a rate,
   never a failure — which is the opposite of most orders in the game and is the
   thing to hold on to whenever a detail below looks odd.

---

## Part 1 — The feedback

### What the original got right, and what survives

- **Structure first.** Not a preference — forced. Usable enclosed space *is*
  assembled `STRC`/`STRL` × efficiency (`docs/model.md:158-162`), so until
  structure exists there is nowhere to put anything.
- **Priority, not a dependency chain.** Skipping a line that cannot progress is
  what keeps a badly ordered list from deadlocking a build.
- **Deterministic seniority** between competing builds.
- **"Completed", not "assembled"**, as the generic per-line verb — right for the
  reason it gives, and doubly right here, because population lives in
  `entity_population` and is never assembled.
- **Transporting construction workers.** I had this wrong; see item 4.

### Terminology — the doc's words, and ours

| Doc's word | Ours | Note |
| --- | --- | --- |
| `LIFE` | `LFSU` | The only life-support code. Supports \(t^2\) population units per assembled unit; an entity over capacity loses the excess. |
| `CWRK` | `CWKR` | Transposed. |
| `POPU` | `USK`, `SKW`, `SOL`, `NAS` | Four population classes in `entity_population`, not one unit in `inventory`. |
| `SDRV` | `SDRV` | Real code. It had no meaning in `docs/units.md` when this was written, so the doc's invented one — "keeps the entity from falling" — was a guess, and it turned out to be substantially right. `HDRV` is the jump drive. |
| `LASR`, `POWR`, `SHLD` | — | No weapon, power, or shield unit exists. |
| `CARG` | `TRAN` / `cargo` | `TRAN` is the transport; *cargo* is one of the four inventory **sections** (`component`, `operational`, `unassembled`, `cargo`), not a unit. |
| "asset" | **unit** | A unit code, optionally with a technology level. |
| "builder" | **the creating entity** | The order's subject: a ship or a colony. |
| "target" | **the new entity** | Under construction, an **unfinished entity**. |
| "BOM" | the `using` and `transfering` lists | The order already names them, in the player's words. |
| `target.controller` | `entity.faction_id` | |
| `IN_PROGRESS` / `COMPLETE` | — | `entity` has no status column. The presence of an `under_construction` row is the status. |
| "ring 99, falling" | — | The mechanic is real but belongs to combat, not to a build. `CORB` is ring 1 and `COPN`/`CSFC` ring 0 by CHECK (`migrations.go:114-124`), so only a `SHIP` has a ring to fall through. |

### Where the original collides with what is settled

1. **`using` and `transfering` are collapsed into one list.** The accepted form
   has three clauses (`docs/accepted-orders.md:94`) and they mean different
   things: `using` is assembled into the new entity, `transfering` is cargo and
   population handed over. The doc's own "Generic BOM Completion" section needs
   this distinction; it just did not know the grammar draws it.

2. **Population is put in the build list.** In the accepted grammar it rides the
   `transfering` clause (`5 SKW`), and it lands in `entity_population`.

3. **Ring 99 and orbital decay.** This was the criticism I got most wrong, and
   it is worth keeping the record straight: the original doc's mechanic — start
   high, fall until a drive comes online, so put the drive high in the list —
   was substantially correct, and `SDRV` is exactly the unit it supposed. What
   is true of the criticism is narrower. The mechanic belongs to **combat**, so
   it does not act on a build; and it could never have applied to three of the
   four kinds `create` builds, since a `COPN`, `CSFC`, or `CORB` has a ring
   fixed by CHECK and only a `SHIP` has one to fall through.

4. ~~**CWKR cannot be transported.**~~ **Wrong — withdrawn.** A `CWKR` is a
   cadre, but a cadre is an assignment of *real population*: one `CWKR` unit is
   one `SKW` plus one `USK`, created by `ship 18 draft 4,250 CWKR`. Two
   population units massing 2 MU each is 4 MU and 4 VU of people to move, so the
   original doc's "Transporting CWRK" section was right in substance and only
   wrong in spelling. The correction that stands is smaller and better: **the
   workers never change owner.** They ride out on a transport, work the shift,
   and come home, so no population row ever moves — the commute spends transport
   capacity and fuel and nothing else.

5. **"The target is created immediately when the build order is accepted."**
   *Accepted* is ambiguous here. `Check` and `Submit` run the real `Apply` inside
   a savepoint that is always rolled back (`orders.go:143`), so the entity comes
   into existence when the order **executes at resolution**, never at submission.

6. **Seniority.** Settled, and more cheaply than I proposed: **the new entity's
   `id` is the proxy.** It is unique, monotonic, and never reused, so it orders
   builds by when they started without storing anything. This drops the
   `started_turn` and `sequence` columns I had proposed, and with them the
   wrinkle that `ec turn open` purges `game_order` and so a foreign key would not
   have survived a long build.

7. **The line invariant is wrong as written.** Four fields, then an equation over
   five: `required = needed + claimed + on_site + completed`. `needed` is not a
   field; it is `required − claimed − on_site − completed`, which the claiming
   pseudocode gets right. Two readings of one doc.

8. **The group create forms are missing.** Settled: the build process applies to
   `create ship` and `create colony` only. **No conflict** — one `Spec` may bind
   a different `Bound` per form, so `create` stays one verb in one phase, and the
   three group forms complete within stage 5, which is what stages 7 and 8 need
   (a group created this turn can be retooled and resized in the same file).
   One sentence has to split, though: `docs/turn-sequence.md:105` says "A create
   order assembles the units it is given, so it does not wait for assembly at
   stage 10." That stays true of the group forms and becomes false of the other
   two, which now assemble at stage 10 like everything else.

9. **A build competing with itself for transport.** Settled the other way from
   the original, and more simply: **a claim is released if it cannot be moved
   that turn.** There is no reservation carried between turns. What moves is
   removed from the creating entity's inventory and added to the new entity's;
   what does not move is simply unclaimed again, and the next turn's claiming
   runs afresh in seniority order. The original's "Unmoved claimed assets remain
   aboard the builder and stay reserved for the target" is overturned.

### The schema collisions

- **`CWKR` now has a home to build.** Not a fifth `entity_population.class` — it
  is an assignment of one `SKW` and one `USK`, made by `draft` and persisting
  until disbanded. That wants a table (`entity_id, cadre, quantity`) that
  generalises to `LABR`, `PLCF`, `SPCF`, and `TRNE`, and `under_construction`
  records how much of it a given build holds.
- **`entity.tech_level` and `entity.enclosed_volume` are `NOT NULL` with no
  default**, and neither is a field on `world.Entity`. A create must supply both,
  and `enclosed_volume` has to grow as structure is assembled.
- **`model.md:154-156`: "An entity without population is controlled by the game's
  `uncontrolled` agent faction."** An unfinished entity starts with no
  population, so this rule would hand every new build to the uncontrolled
  faction. It needs an exception.

---

## Part 2 — How delivery reaches stage 9 and assembly stage 10

This was the open question, and the engine already has the seam for it. A
`Phase` carries an optional `Sweep func(*world.World, int) error` — what the
phase does apart from anyone's orders — and sweeps run in check, submit, and
resolve alike. So a build's turn is **not one sweep at stage 5. It is three
sweeps on three phases**, each hung on the stage where the resource it competes
for is settled:

| Stage | Phase | The build's share of it |
| --- | --- | --- |
| 5. Creation | `create` — orders, then sweep | **Claim.** Builds in seniority order, lines in the order written: decide what each build has a call on. |
| 9. Transfers | `transfer` — orders, then sweep | **Deliver.** Settle every claim and every transfer order against the entity's transports. Move the workers out. Release what did not fit. |
| 10. Assembly | `assemble` — orders, then sweep | **Complete.** The build's own cadre works off what is on site. The workers go home. |

Total bookkeeping: **one column.** `claimed` on `construction_item`, written at
stage 5, consumed at stage 9, and zero at every turn boundary because an unfilled
claim is released. It is a database column rather than in-memory turn state on
`world` for a specific reason: `world`'s in-memory maps (`probes`, `ordered`) are
*not* rolled back by the savepoint `Check` and `Submit` always discard, so a
claim held there would leak out of a dry run.

Two consequences worth taking now, while neither order is written:

- **`transfer` becomes intent-declaring.** Stage 9 becomes "orders and a sweep"
  in the sense `docs/turn-sequence.md` already uses for combat and the market:
  the orders say what is wanted, and one sweep settles all of them against a
  shared pool. It has to, because a `transfer` order and a build's delivery at
  the same entity draw on the same transports — executing transfers one at a
  time and delivering afterwards would let file order silently decide who got the
  last transport.
- **Stage 10 has to settle against `assemble` orders too.** I first wrote the
  opposite, on the reading that a create *holds* its cadre for the duration
  while a bare `assemble`'s workers are engine-allocated from what is left. The
  cadre is a per-turn cap rather than a reservation, so both draw on the same
  idle pool and stage 10 is the same shape as stage 9: orders and a sweep,
  settled together.

That yields one rule covering both stages, which is better than two ad-hoc
calls: **explicitly ordered work outranks a standing commitment.** A `transfer`
order is served before a build's claim and an `assemble` order before a build's
own assembly; the build takes what is left, which is all it ever needs, because
a build never fails for want — it only slows.

Keeping the claim at stage 5 also keeps the argument the old rule was making:
creation is still upstream of transfers, the market, and movement, so a build
cannot claim units that only arrived this turn.

---

## Part 3 — The transport rules, restated

Supplied, and recorded in the rewritten design doc:

- `TRAN` has technology levels 1–10. **Mass 4 MU, flat** — not \(2t\).
- **Volume \(4t\) VU both assembled and in cargo**, which is deliberate and
  unusual: it is the same number in two sections, so the standard "operational
  is twice cargo" multiplier does not apply to it.
- **Capacity is dual and per turn**: at most \(20t^2\) MU *and* at most
  \(60t^2\) VU.
- **Crew**: one `SKW` unit operates up to 10 transports a turn, allocated by the
  engine rather than by the player.
- **Fuel**: reckoned for all of an entity's transports together, not one at a
  time. In whole numbers, which is the house style
  (`docs/units.md:132-135`): \(\lceil \sum t^2 / 10 \rceil\) `FUEL`.

The example given reads "10 `TRAN-2` and 5 `TRAN-2`" but its arithmetic says
\(10 \times 0.1 \times 1 \times 1 + \ldots\), so the first group is `TRAN-1`.
Both forms agree: \(10 \times 1 + 5 \times 4 = 30\), and \(\lceil 30/10 \rceil =
3\) `FUEL`. A single `TRAN-1` costs \(\lceil 1/10 \rceil = 1\).

Two properties fall out that are worth knowing before anyone tunes them:

- **Fuel per MU moved is constant across technology levels.** Capacity is
  \(20t^2\) and fuel is \(t^2/10\) per transport, so the ratio does not depend on
  \(t\). A higher technology level buys fewer hulls and fewer crew, not cheaper
  freight.
- **Mass is what binds, for nearly everything.** The capacity ratio is 3 VU per
  MU, and population (2 MU : 2 VU), the bulk resources (1:1), and an ordinary
  technology-level unit in cargo (\(2t\):\(2t\)) are all 1:1. The one exception in
  today's table is `TRAN` itself carried as cargo, which is 4 MU : \(4t\) VU and
  so becomes volume-bound at technology level 4 and above.

**A worked commute**, because this is the number that makes `with 500 CWKR` a
real decision. 500 `CWKR` is 500 `SKW` + 500 `USK` = 1,000 population units =
2,000 MU and 2,000 VU. Mass binds. With `TRAN-1` that is
\(\lceil 2000/20 \rceil = 100\) transports, 10 `SKW` to crew them, and 10 `FUEL`.
With `TRAN-5`, \(\lceil 2000/500 \rceil = 4\) transports, 1 `SKW`, and still
10 `FUEL`.

### What this does to the starting kit

`games/beta/home-planet-seed.json` puts all 600 `TRAN-1` on the `CORB`, and that
`CORB` has no population at all — so it is owned by the `uncontrolled` faction
and can crew none of them. No player-controlled entity in the kit holds a single
transport.

I first called that a blocker. It is not: the create order succeeds, the entity
appears, and the build sits at zero progress until transports reach the creating
entity. That is the correct behaviour and a good first thing to watch work. What
it does mean is that the first end-to-end test of a build that *finishes* needs
a kit with transports on a crewed entity.

The mass change (\(2t\) → 4 flat) moves every entity that holds `TRAN`. The
replay goldens are safe — `internal/replay/testdata/scenario.sql` has no `TRAN`
— but `games/beta` and the CLAUDE-01 corpus both move, so
`games/beta/rebuild.sh` and `games/claude/replay.sh` want rerunning when the
rule lands.

---

## Part 4 — What is still not settled

- ~~What `SDRV` does.~~ **Settled: the orbital drive.** The original design was
  right about the mechanic — hold your mass up or fall a ring a turn and die at
  ring 0 — but **that is a combat rule and does not touch a build.** An
  unfinished entity does not fall and `SDRV` draws no fuel or crew yet, so the
  design above is unchanged and the ring a new entity appears in stays cosmetic.
- ~~What `LFSU` supports.~~ **Settled: \(t^2\) population units per assembled
  unit, and an entity over capacity loses the excess.** That makes the
  population gate load-bearing rather than decorative: without it a build would
  deliver people into a hull that kills them. Open only for `COPN`.
- ~~Whether a commute is charged once or twice.~~ **Settled: once** — a
  transport gives there-and-back service.
- ~~Whether explicitly ordered work outranks a standing commitment.~~
  **Settled: it does**, at stage 9 and stage 10 alike.
- ~~What one `CWKR` completes in a turn.~~ **Settled: 500 MU a turn, pooled
  across an entity's work of the same kind, never across assembly and
  unassembly, and a build's assembly is its own pool.** Now in
  `docs/accepted-orders.md`, including the part that would otherwise surprise a
  player: separate pools still draw on one shared cadre, so a build and an
  `assemble` order can starve each other of workers without pooling their work.
- ~~On site but not yet worked.~~ **Settled, and narrower than I recommended:
  `STRC` and `STRL` alone are exempt from the enclosed-space rule on an entity
  under construction.** Better than the blanket exemption, because it makes
  structure-first *forced* rather than merely preferred — everything else needs
  real space to be delivered into. The bootstrapping is a deliberate hand-wave.
- ~~Where a group create's construction workers come from.~~ **Settled: the same
  cadre pool.** A worker does one task per turn and the pool is drawn down in
  stage order, so no new precedence rule was needed. Group creates are
  kill-and-fill and finish in the turn they were given — the opposite of the
  commitment model, and the sharpest thing in `create` for a player to trip over.

Two discrepancies were found while here. Both are now recorded where they were
found rather than silently resolved, because one of them is the user's call:

- **The trade station is a decision, still open.** `docs/plan/beta-plan.md` says
  a trade station is an orbital colony; the accepted grammar hangs
  `as trade-station` off all three colony kinds equally.
  `docs/accepted-orders.md` is accepted, so it wins unless that is deliberately
  overturned. The beta plan now says so instead of asserting the narrower rule.
- **The 1.1x excess-space rule is not a game rule**, and this note was wrong to
  read it as one. The beta plan credited it to `docs/units.md` and
  `docs/model.md`; it is a check the kit loader makes on a seed file
  (`spaceWithTenPercentExcess`, `cmd/ec/kit.go:316`), so that a kit an agent
  built has headroom for a human to edit it afterwards. It constrains no play —
  kits have deliberately been built optimised for mass rather than space, to
  test mini-games — so there is nothing to record in `docs/units.md`. The beta
  plan now says what it actually is.

---

## Files to change

| File | Change |
| --- | --- |
| `docs/plan/entity_build_bom_process.md` | **Rewritten — done.** |
| `docs/units.md` | **Done.** `TRAN`, `LFSU`, `SDRV`, and `COPN` have real rules; `CWKR` is "one `SKW` plus one `USK`, made by `draft`"; the cadre preamble no longer says a cadre has no mass, because the population it assigns does. The 1.1x rule is *not* recorded here — see above; it is kit-loader guidance, not a rule of the game. |
| `docs/turn-sequence.md` | **Reconciled — done.** Stages 5, 9, and 10 are orders + sweep; the shape tallies and the stage table follow; the assembly sentence is split so it holds for group creates only; the claim rule replaces "spends what the creating entity already holds"; the `with` clause is a cap rather than a reservation held for the duration; "What outlives the turn that ordered it" now says `create` takes `jump`'s shape and needs no fourth status. |
| `docs/plan/beta-plan.md` | **Reconciled — done.** The `create` prerequisite is answered and the fourth order status retired everywhere it appeared; the CREATE bullet says designed rather than open; batch 2 is unblocked and now names its real dependency on batch 1; the cadre section specifies `CWKR`; the stale 1.1x claim names its real home. |
| `docs/accepted-orders.md` | The `draft` form now has a worked meaning (`ship 18 draft 4,250 CWKR`). Trade-station scope, only as a deliberate decision. |
| `docs/model.md` | `entity_population` gains the cadre-assignment table when it is designed. |

No code and no migration yet. When the build starts, the reuse to plan for is
`cmd/ec/kit.go` — `metricsForUnit`, `parseUnitTag`, `isBulkResource`,
`usableEnclosedSpace`, and `spaceWithTenPercentExcess` are the mass-and-volume
rules `create` needs, and they belong in an `internal/units` package on the
`internal/fuel` / `jumpdrive` / `sensors` template so the kit loader and the
order share one implementation. `metricsForUnit` is also where `TRAN`'s defined
mass goes, beside `jumpdrive.UnitMass` and `sensors.UnitMass`.

## Verification

Documentation, so the checks are consistency ones:

```sh
go test ./internal/orders      # doc_test.go: every registered form is in docs/orders.md
go test ./...                  # nothing here should move it
```

Then read for consistency, which is what actually matters:

- every unit code the rewritten doc names appears in `docs/units.md`
  (`grep -o '\b[A-Z]\{3,4\}\b' docs/plan/entity_build_bom_process.md | sort -u`
  against the glossary headings);
- the doc's account of stages 5, 9, and 10 matches `docs/turn-sequence.md`'s, in
  both directions;
- nothing contradicts `docs/entity-location.md`'s ring table or
  `docs/accepted-orders.md`'s grammar — the accepted doc wins.
