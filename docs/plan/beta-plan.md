# Make the order pipeline cheap to iterate on

## Context

Four orders exist (`MOVE`, `JUMP`, `PROBE`, `NAME`). Thirty-one remain, spanning
inventory & cargo, entities & groups, population, market, information, control &
diplomacy, and combat — then the production system and the combat system. The 1978 rules are being discovered by
playing, so orders get reworked after they land, not just added.

Adding one order used to touch ~33 places across 18 files (measured from
`git show 9d1b5c8`: 18 files, +1,542/−42). Four structural taxes cause that, and
this plan removes them before adding the orders.

**Status: steps 0 through 4 are done and the plan's own verification suite is
green. Step 5 has begun: NAME is built (`e3eada9`), batch 1 -- `assemble`,
`unassemble`, `transfer` -- is built, and `docs/accepted-orders.md`
is now accepted rather than proposed. The parser now reads that surface: every
order line is subject first, and `we` is a subject. The accepted verbs are
mapped onto `docs/turn-sequence.md`, which is twenty-two stages and forty-three
phases -- see "The turn sequence and the accepted doc are reconciled" below.**

**Step 5 is stopped after batch 1, and not for want of pipeline. The four structural taxes
this plan set out to remove are removed: an order is one `Spec`, a rule is
written once, a turn is its phase table, and every order is a row of one table.
What most of the thirty-one remaining verbs are waiting on is what they *do*.
`docs/accepted-orders.md` gives their surface forms and settles a good deal of
the surrounding detail, but it does not say what a group produces per turn, what
a spy costs, who the market's counterparty is, or what control confers -- and
writing those would be authoring the 1978 rules rather than implementing them.
See "What the code still has to be told" below for the list, verb by verb.
Two pieces were exceptions -- blocked on this repository rather than on the
rules -- and both are now cleared. The `jump` rework is **built**: a crossing
takes \(\lceil d / t \rceil\) turns, a ship in transit is nowhere and
unreachable, and the whole fuel bill is drawn on departure. `create` is
**designed**: `docs/plan/entity_build_bom_process.md` settles it end to end, and
the one schema change it was thought to need -- a fourth `game_order.status` --
turns out not to be needed at all.**

---

## Progress

| Step | State | Commit |
| --- | --- | --- |
| 0 — regression net | **done** | `816e476`, `6d46eaa`, `1803dab`, `cdabe4f` |
| 1 — tokenizer + order registry | **done** | `f48e0af` |
| 2 — one implementation of each rule | **done** | `3558c4a` |
| 3 — data-driven phases | **done** | `bb7a076` |
| 4 — one order table | **done** | `8e8215c`, `132e928` |
| 5 — add the 31 orders | **batch 1 built and under the golden net** (3 of 31); the rest blocked on rules | `e3eada9` (NAME), `b96420d`, the golden commit |

### What step 0 built

- **`internal/testdb`** — `New`, `NewAt`, `Exec`, `Tables`. Three hand-rolled
  fixture builders now share it. `cmd/db`'s `wantTables` is derived from the
  migrations instead of a frozen literal.
- **`internal/report`** — models every report as a title plus named tables of
  header and rows, rendered as either aligned text or JSON. The four report
  builders moved here from `cmd/ecrpt`, which dropped from 931 to 245 non-test
  lines. `ecrpt show --format json` is the golden-friendly output.
- **`internal/logging`** — `NewWithoutTime` drops the record's own time key;
  `ec turn resolve --no-log-timestamps` selects it.
- **`internal/replay`** — `TestReplay` plays a three-turn scripted game
  (submit → resolve → report → open) against a migrated database and diffs
  every engine log and report against `testdata/golden`. `-update` rewrites
  them. `TestSubmitRejects` covers the 11 failures that reject a whole file.
- **`games/claude/replay.sh [--json] [OUTDIR]`** — replays the seven-turn
  CLAUDE-01 corpus. With `--json`, two runs produce 147 byte-identical files.

### What step 1 built

Tokenize once, dispatch on verb. An order registers one `Spec` in
`internal/orders/verbs.go` (verb, summary, syntax, parse) and nothing else in
the pipeline needs to know it exists. Shared field readers live on `*Line` in
`token.go`. Errors split: a line that never matched its order's *shape* gets
that verb's syntax; a field read and found *wrong* reports itself. Added `#`
comments, spacing-insensitive coordinates, rejection of trailing words,
`ec orders help [ORDER]`, and a test that fails if `docs/orders.md` omits a
registered form.

### Decisions taken while executing (these changed the original plan)

- **The golden corpus is a purpose-built scenario, not the CLAUDE-01 corpus.**
  The plan assumed the corpus could seed the goldens. It could not: commit
  `e250d93` made the starting kit unable to jump at all, so every jump failed
  and the corpus was unreplayable. `internal/replay/testdata/scenario.sql` is a
  small game built to make each rule fire instead. The CLAUDE-01 replay stays
  as a slower on-demand end-to-end check.
- **The CLAUDE-01 kit was refitted** from 8 × `HDRV-8` + 120 `FUEL` to
  7 × `HDRV-10` + 11,716 `FUEL` + 2,421 `STRC-10` (structure was forced: `FUEL`
  takes 1 VU on a ship, and the kit loader wants enclosed space ≥ 1.1 ×
  occupied). All 147
  orders across turns 0–6 now succeed. The ship sits at exactly 100% of drive
  capacity; **the user has said not to future-proof kits** — they are cheap to
  update when needed.
- **JSON reports and the timestamp-free logger were user requests** folded into
  step 0. They are what make golden files possible.
- **Step 1 kept the flat `Order` struct.** Per-verb `Params` types were deferred
  to step 2, because `validate()` consumes `Order` and step 2 rewrites
  `validate()` anyway. Doing it in step 1 would have meant doing it twice.
- **Migrations have not been squashed yet.** Still 9 append-only migrations.
  The squash belongs with step 4, where the schema actually changes.

### What step 2 built

**`internal/world`** — one game's live state: entities (location, mass, fuel,
drive, sensors), stellia and the point index, plus `System`/`Planet` lookups
and the mutations `Move`, `BurnFuel`, `RecordProbe`, `SpendProbe`, which write
through to SQLite and keep the loaded copy in step. `world.Load(conn, gameCode)`
replaced both `orders.findGame` and `engine.requireGameTurn`'s query. The game
seed and `RingFor` moved here from the engine.

**`Params` / `Bind` / `Apply`.** The flat `Order` became `Order{Line, Verb,
Params}` with `MoveParams`, `JumpParams`, `ProbeParams`. `Params.Bind` returns
`[]Bound` (a probe line binds one order per orbit) and `Bound.Apply` executes.
Everything about an order -- Spec, Params, Bind, Bound, Apply, and its INSERT
-- now lives in `verbs.go`.

- Deleted: the `locations` shadow simulation, `spendProjectedFuel`,
  `moveProblem`, `jumpProblem`, `findEntity`, `findStellium`, `findSystem`,
  `findPlanet`, the three reload preambles, `checkMoveDrive`, `checkDrive`,
  `checkFuel`, `burnFuel`, `destination`, `executeMove/Jump/Probe`,
  `updateMove/JumpOutcome`, `loadEntities`, `loadStellia`. `engine.go` went
  934 → 508 lines; `orders.go` 645 → 269.
- **`Check` and `Submit` run the real `Apply` inside a savepoint that is always
  rolled back** (`discard`). `Submit` then stores the orders that rolled-back
  run bound. `TestCheckPutsTheWorldBackTheWayItFoundIt` guards the rollback.
- **The engine binds a stored order again before applying it.** `loadOrders`
  rebuilds `Params` from the row (the player's words, not the ids they resolved
  to), so the same `Bind` runs at resolve. The stored `destination_*` columns
  became write-only; step 4 drops them into `params`.

Verified: `internal/replay` goldens untouched; all 147 CLAUDE-01 report files
byte-identical to the pre-change binary (diffed against a stashed build), and
identical across two runs; `games/beta/rebuild.sh` run green in a throwaway
worktree rather than against the developer's beta database.

Deliberate behavior changes, all in wording or in the submit-side projection:

- Probe failure messages named the entity `entity N` in the engine and `ship N`
  in the check. Both now name it by what it is: `ship 40` / `colony 41`.
- The fuel warning was `... and will hold 36; the order fails unless fuel
  reaches the ship first`. It is now the engine's own message plus a general
  caveat: `... and holds 36; the order is kept in case that changes before the
  turn resolves`. There is one fuel message now, not two.
- A fuel-short order used to *succeed optimistically* in the submit-side shadow
  sim, so later orders were checked from where it would have gone. It now fails
  the dry run like it will fail the turn, and the ship stays put for the orders
  that follow. This is what deleting the second world model costs, and it is
  the more honest answer.
- `"destination system is not in the ship's current stellium"` is gone: the
  destination is recomputed from the ship's live location, so it cannot arise.
- The orders test fixture had ships of mass 0 carrying 500 FUEL. Running the
  real burn tripped `CHECK (mass >= 0)`, so the fixture now masses 3,000.

**Step 3 partly landed with step 2.** Writing three identical phase loops
purely to keep the step boundary would have been silly, so `orders.Phase`,
`orders.Phases()`, `Spec.Phase`, and the single loop in both `simulate` and
`engine.resolve` came with step 2; the rest followed in `bb7a076`.

### What step 3 built

`Phase` stopped being an int and became a value with a `Name` and an optional
`Sweep func(*world.World, int) error` -- what the phase does apart from
anyone's orders. The turn is one table:

```go
var phases = []*Phase{PhaseProbe, PhaseSensor, PhaseMove, PhaseJump}
```

- `PhaseSensor` has no orders and is only its sweep. `recordPassiveSensors`
  moved out of the engine to `world.RecordSensors`, beside `RecordProbe`, and
  the `if phase == PhaseProbe` that used to wedge it between two loops is gone.
- Both `simulate` and `engine.resolve` run each phase's orders and then its
  sweep. Adding a phase is an entry in the table plus a `Phase` on the Specs
  that resolve in it; there is no `switch` left in either.
- **Sweeps run in check and submit too**, inside the rolled-back savepoint.
  Nothing observes the sensor sweep at submission, but combat's sweep will
  change the world the later phases see, and a check that skipped it would stop
  being the dry run step 2 made it. Measured cost on the seven-turn CLAUDE-01
  replay: none (4.05s before, 4.05s after).
- `ec orders help` opens with the phase list and tags each order with its
  phase, printed from the same table the engine walks; `docs/orders.md` carries it
  as a table rather than a sentence kept in step by hand.
- New tests: `TestTheTurnIsItsPhaseTable`,
  `TestEveryOrderResolvesInAPhaseTheTurnHas`,
  `TestHelpNamesEveryPhaseAndPlacesEveryOrder`, and
  `TestCheckPutsTheWorldBackTheWayItFoundIt` extended to the sensor tables, so
  a sweep that escaped the rollback fails a test.

`engine.go` is 473 lines, down from 934 before step 2. Goldens untouched and
the 147 CLAUDE-01 reports byte-identical throughout.

**Left for combat, not for step 3:** a phase with segments and steps inside it.
`Sweep` is the seam it will use, but nothing needs the internal structure until
there is a battle to resolve.

### What step 4 built

One table. Every order is a row of `game_order` -- `verb`, `actor_entity_id`,
`input` (the order in the words the player wrote), `params` (everything else it
said, as JSON, also in the player's words) -- with two child tables keyed on the
same four columns for the orders that record more than a status:
`order_movement` and `order_survey`.

- **`params` holds no ids, which is what makes the JSON safe.** The plan
  expected to trade away "SQLite checks this destination is in this game"; step
  2 had already made that trade unnecessary, because the engine re-binds the
  player's words when the turn runs. Nothing that wanted a foreign key was
  going into the JSON. The compound keys that do fire -- faction in game, actor
  belongs to faction -- stayed columns.
- **No `phase` column.** The plan had one; it is derivable from the verb via
  the registry, and `docs/turn-sequence.md` makes clear the phase list will
  churn for a long while yet. Storing an ordinal whose meaning shifts would
  have meant a migration every time a stage moved.
- **The status block says more than it did**: a failed order carries a message
  *and burned no fuel*. The two triple-consistency location CHECKs are written
  once. "A failed order goes nowhere" outlived the CHECK it lived in, because
  status is on the spine and locations are in the child, so it became a guard
  trigger -- the house pattern.
- **An order lost its INSERT.** It declares `Movement` on its Spec and `Decode`
  beside its `Parse`; storing an order is written once.
- **Probe findings went to `order_survey`, not to a `result` JSON column.**
  They are id-shaped and want foreign keys, the same reason `order_movement`
  exists. No `result` column was added: nothing needs one yet.
- **Migrations 1-9 squashed to one baseline.** Verified structurally identical
  to what the nine produced for every table but the order tables -- column by
  column, foreign key by foreign key, index by index. `ec db verify` reports
  version 1. The rule is now append-only from the first live game (CLAUDE.md,
  AGENTS.md).
- One commit before it (`8e8215c`) unified the two renderings of an order --
  the engine log said `system current orbit 6` where the report said
  `orbit 6` -- so that the schema commit had a clean golden signal. Four golden
  lines changed there; none changed in the schema commit.

Collapsed as promised: one DELETE and one INSERT in `Submit`, one `loadOrders`
and one `updateOutcome` in the engine, one entry per table in the purge list,
one `SELECT ... LEFT JOIN` per report section instead of `UNION ALL` arms, and
one table in `gameHasData`, which no longer silently omits probes.
`engine.go` is 412 lines, down from 934 before step 2.

**Left over:** the orders report still splits its sections on `verb = 'probe'`.
That is now a question about the shape of the report rather than the shape of
the schema, and it wants revisiting when there are twenty-five verbs.

---

## Step 2 — one implementation of each rule (done)

The largest remaining win. Every game rule is currently written twice, against
two separate world models: `validate()`'s `locations map[int64]shipLocation`
shadow simulation (`internal/orders/orders.go:228`) and the engine's
`entities map[int64]*entity` (`internal/engine/engine.go:160`).

| Rule | Check side | Resolve side |
| --- | --- | --- |
| move legality | `moveProblem` `orders.go:443` | `checkMoveDrive` `engine.go:542` |
| jump legality | `jumpProblem` `orders.go:457` | `checkDrive` `engine.go:594` |
| fuel | `spendProjectedFuel` `orders.go:96` | `checkFuel` `engine.go:612` |
| destination | inline `orders.go:325-360` | `destination` `engine.go:529` |

The test suites mirror too — seven `TestCheck…`/`TestResolve…` pairs.

**Introduce `internal/world`** holding what `engine.resolve` already loads
(`loadEntities` `engine.go:338`, `loadStellia` `:386`) plus lookup and mutation
methods: `Entity`, `System`, `Planet`, `Stellium`, `Move`, `BurnFuel`,
`AddInventory`, `RemoveInventory`, `CreateEntity`, `DestroyEntity`. Every
mutation writes through to SQLite and keeps `entity.mass` in step, as
`burnFuel` (`engine.go:622`) does today.

**Then split each order's logic once**, along the line the current behavior
already draws — and which `TestSubmitRejects` vs `TestReplay` already encodes:

- **`Bind`** — legality that cannot change between submission and resolution:
  ownership, entity kind, destination exists, drive range and capacity. A
  `Bind` error **rejects the file**. Runs in check, submit, and resolve.
- **`Apply`** — execution against live turn state, for what *can* change: fuel
  on hand, probe budget, another faction got there first. An `Apply` failure is
  a `failed` order row at resolve and a **warning** at check.

`Check` already opens a transaction and discards it (`orders.go:115`), so it
can run the real `Apply` path for free. This deletes the `locations` shadow
simulation, the three duplicated reload preambles, and the mirrored rule
functions; the seven mirrored test pairs collapse to one each.

Add `Bind`/`Apply` to `Spec`, and replace the flat `Order` with per-verb
`Params` types at the same time.

## Step 3 — data-driven phases (mostly landed with step 2)

```go
func Phases() []Phase { return []Phase{PhaseProbe, PhaseMove, PhaseJump} }
// production and combat append here
```

Done in step 2: `simulate` and `engine.resolve` are each one loop over
`orders.Phases()`, and the `len(a)+len(b)+len(c)` sums are gone.

Left to do: the passive sensor snapshot still runs from a hardcoded
`if phase == orders.PhaseProbe` in `engine.resolve`; it wants to be
`PhaseSensor`, a phase whose resolution is a function rather than a per-order
loop. Combat needs structure *inside* a phase (`docs/orders.md:5` speaks
of phase, segment, step), so model that as a phase whose resolution is a
function rather than a per-order loop.

## Step 4 — one order table

Squash migrations 1–9 into a single baseline and land the new schema in it.
Update the CLAUDE.md rule to "append-only from the first live game".

```sql
CREATE TABLE game_order (
    game_id, turn, faction_id, sequence, source_line,
    verb TEXT NOT NULL,
    phase INTEGER NOT NULL,
    actor_entity_id INTEGER,           -- NULL for faction-level orders
    input TEXT NOT NULL,               -- canonical rendering, what the report prints
    params TEXT NOT NULL DEFAULT '{}',
    fuel_spent INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','succeeded','failed')),
    error_message TEXT,
    result TEXT,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (faction_id, game_id) REFERENCES faction(id, game_id),
    FOREIGN KEY (actor_entity_id, faction_id) REFERENCES entity(id, faction_id),
    CHECK (...the tri-state block, written once...)
);

CREATE TABLE order_movement (   -- move and jump only
    game_id, turn, faction_id, sequence,
    start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
    final_stellium_id, final_system_id, final_planet_id, final_planet_ring,
    PRIMARY KEY (game_id, turn, faction_id, sequence),
    FOREIGN KEY (game_id, turn, faction_id, sequence) REFERENCES game_order(...),
    ...the location FKs and the two triple-consistency CHECKs, written once...
);
```

Most of the 25 remaining orders change no location, so the eight start/final
columns belong in the child table, not the spine.

**The trade:** order-specific IDs move into `params`, so SQLite no longer
enforces "this destination stellium is in this game". `Bind` resolves them and
`Apply` re-reads through `World`, so a vanished ID produces a failed order, not
a corrupt row. The compound FKs that actually fire — faction in game, actor
belongs to faction — stay on the spine.

`probe_contact`, `probe_deposit`, `sensor_survey`, `sensor_contact` stay as they
are; they record findings, not orders.

Collapses: one DELETE and one INSERT in `Submit`; one `loadOrders` and one
`updateOutcome` in the engine; one entry in `OpenNextTurn`'s purge list
(`engine.go:245`); one `SELECT ... LEFT JOIN order_movement` in
`internal/report/orders.go` instead of `UNION ALL` arms; one table in
`gameHasData` (`cmd/ec/load.go:167`), which today silently omits `probe_order`.

### NAME is built

`docs/accepted-orders.md` specifies naming completely, so it went in as the
first order since the rework: five forms, a new `faction_name` table, and a
`naming` phase last in the turn where `docs/turn-sequence.md` puts it at stage
20. It cost one `Spec`, a section in `docs/orders.md`, and tests -- no SQL in
`Submit` or the engine, no new pass in either loop.

Two gaps the pipeline had never been asked about turned up and were closed: an
order with **no actor at all** (naming a stellium leaves `actor_entity_id`
null, which the schema allowed and the engine did not), and fuel being logged
by verb rather than by whether the order moves anything.

Appending a phase cost nothing in the goldens: sequence numbers are handed out
in phase order, so a later phase only adds sequences at the end and every
existing ring draw is untouched. The golden diff was 216 lines added, none
removed.

**Not built:** naming another faction, or another faction's ships and colonies
-- `we name faction 5 "The Hegemony"` and `we name player 5 ship 19 "Easy
Target"`. The ambiguity this plan used to record here has narrowed: the accepted
doc writes the two forms as `(player | faction) _id_`, one production with one
id, so it reads them as two words for the same thing rather than two numbering
schemes. That still wants confirming against `replay.sh`, where a player number
and a faction id differ.

Two things are missing besides. `faction_name` can name a stellium, a system, a
planet, or an entity, and there is no column for a named *faction*, so the first
form is a migration. And the accepted doc gates both on not naming a faction
"you have not yet encountered", which nothing defines: `probe_contact` and
`sensor_contact` record the entity seen but not who owns it, so an encounter is
derivable only by joining back through `entity`, and whether one sighting
counts forever is a rule, not a query.

## What step 5 needs before the rest can start

### The parser rewrite comes first

`docs/accepted-orders.md` is accepted, and it specifies a surface the built
parser cannot read. Two changes, cheaper together than apart: both land in
`parseOrder`, and both churn the same corpora and docs.

**Subject first.** Every grammar line in the accepted doc opens with its
subject, and there are exactly three: `ship` _id_, `colony` _id_, and `we`.
`parseOrder` inverts -- read the subject, read the verb, check the verb accepts
that subject, hand the rest to `spec.Parse`. `Spec` grows a `Subjects` field,
which is what today lives inside each parser as `expect("ship")`, and `Parse`
takes the subject rather than reading it. `token.go` gains a subject reader.
Nothing outside `internal/orders` changes: `Input()` renders parameters without
the verb or the actor, so the replay goldens hold still, and a golden that does
move is a real behaviour change rather than churn.

**`we` is the faction subject.** Naming a place, releasing control, and granting
a permission are faction orders. `actor_entity_id` stays nullable and null keeps
meaning "the faction itself", which is what it already means for a place name,
so there is no migration.

**Multi-line orders.** `create` is terminated by `end` and ignores line breaks.
Keep tokenizing one physical line at a time and let `Line` pull the next when an
order is unfinished; `Spec` declares `Multiline`, and dispatch has already
happened by then, so nothing has to pre-scan. `game_order.source_line` keeps
holding the start line, which is the number a player looks for, so the column
does not change.

**Errors, in four tiers.** A line that never named a verb cannot be answered
with a verb's syntax, but the subject vocabulary is closed, so the error is
still precise: `expected ship, colony, or we`. Once the verb is known,
`verbList()` answers a typo as it does today. A verb that does not take the
subject it was given is new, and better than what is built: `jump is a ship
order` rather than `expected ship`. After the verb the existing split is
untouched -- a `syntaxErr` shows that verb's `Syntax`, a plain error reports as
written. Inside a multi-line order an error names both numbers: `line 12: in the
create order beginning on line 8: ...`.

**Resynchronising after a missing `end`.** Without this, one forgotten `end`
swallows the rest of the file and the player gets forty errors describing one
mistake. `we` is a hard sync point: it appears nowhere in the grammar except as
a subject, so a `we` opening a line inside an unterminated block is a new order.
Stop there and report `the create order beginning on line 8 was never terminated
with end`. `ship` and `colony` are only a hint, even first on a line: they
appear mid-order as targets, and `create` takes `ship` as the thing being
created while ignoring line breaks, so

```text
ship 18 create
  ship
  using 60 STRC-8, ...
end
```

is legal and its second line opens with `ship`. Note the position, keep parsing,
and if the block fails or reaches EOF unterminated, offer it as a guess rather
than a verdict: `... was never terminated with end; line 12 looks like the start
of a new order`.

**The corpora.** 168 order lines across 73 files (`games/claude/orders` x70,
`games/beta/orders` x1, `internal/replay/testdata/orders` x3), rewritten by
script: move the subject behind the verb, prefix the place-naming lines with
`we`. `docs/orders.md` moves with them, and `doc_test.go` fails until it does.

`docs/units.md` and `docs/model.md` specify the nouns completely -- the four
inventory sections, per-unit mass and volume in each, enclosed volume from
assembled `STRC`/`STRL` at \(t^2\) VU, the efficiencies (COPN 1, CSFC 0.2,
CORB and SHIP 0.1), bulk resources in external depots. The 1.1x excess-space
check was listed here as one of them and is not a game rule at all: it is a
check the kit loader makes on a seed file (`spaceWithTenPercentExcess`,
`cmd/ec/kit.go:316`), so that a kit an agent built has room for a human to edit
it afterwards. It constrains nobody's play and belongs in neither doc. What the
two docs say nothing about is what the batch-1 verbs *do*. Writing that would be
authoring rules, not implementing them, so the user is supplying the 1978 text.

### The turn sequence and the accepted doc are reconciled

`docs/turn-sequence.md` was the 1978 twenty-one stages and the accepted doc had
moved away from it. It is now twenty-two stages and forty-three phases, with every
accepted verb landing on a step and every step saying whether it is orders, a
sweep, or both. That table is what `phases` in `spec.go` grows into.

What moved on the sequence's side: production split into three stages -- mining,
farming, factory. *Set up* (old 4) became **creation** and holds every `create`
form. *Build change* (old 6) became **retool**, which is what it always meant.
*Mining change* (old 7) became **group change** and widened to `add`, `remove`,
`idle`, and `activate` over all three group kinds, because a mine group's
deposit is fixed for its life. Trade and colonize permissions both resolve in
the naming-and-control stage rather than in the market, which keeps one phase
per verb and makes a permission granted this turn take effect next turn.

What moved on the accepted doc's side, because a `Spec` carries one verb and one
phase: the espionage `attack` became **`neutralize`** (it spends spies rather
than committing a percentage of an entity, so it never belonged in combat), and
`report on rebels` / `report on spies` became **`assess`** and **`detect`** --
one verb across two steps that the engine allocates separately for. The
alternative was moving `Phase` off `Spec` onto the bound order; renaming two
words was smaller than making every other order pay for one order's grammar.

Three shapes came out of it that the engine has to be built for. Six stages are
pure sweeps; five are **orders and a sweep** -- combat, the market, espionage,
ship movement, and the news service -- where orders declare intent and one sweep
settles all of them against each other. That third shape is not just combat:
matching market offers by commission is structurally the same problem as a
battle between the fleets that met. Ship movement joined the list late and for a
milder reason: its sweep settles nothing between the orders, it only lands the
ships whose crossing finished this turn.

### The parser reads the accepted surface

Every order line is subject first -- `ship 18 jump to (-1,2,3)`,
`colony 24 probe orbit 5`, `we name (-1,2,3) "Stellium Joe"`. `Spec` gained
`Subjects`, the list of subjects an order may be given to, and the parser
refuses a line whose subject is not on it before `Parse` runs, so no order
checks who it was given to. `Spec.Parse` takes the subject already read.
`we` is a real subject now, which is the half of the `name` rework that the
place forms needed; naming another faction's ships still waits on batch 6.

Not yet read: the multi-line `create` form terminated by `end`, and the
`quantity` grammar with its mandatory thousands separators. Both arrive with
the orders that use them.

### All three jump changes are built

A jump now **begins from the stellium orbit**: a ship at a planet has to be
moved out to orbit 11 in the same file, which works because every MOVE resolves
before any JUMP. It binds rather than applies, because by the time the jump
phase runs the moves have happened, so the file is refused when the ship will
still be at a planet and the order fails at resolution when a move it needed
failed for fuel.

**Technology level no longer caps the distance.** `Drive.Range` became
`Drive.TechLevel` and `Drive.Reaches` is gone; any ship can be sent to any
stellium in the game. `FUEL` is unchanged and was never a function of
technology level -- 40 per assembled `HDRV` unit per light year -- so it is the
only thing that limits a long jump, and it grows linearly with the distance.
What the technology level does instead is divide the distance to give the turns
the crossing takes, which is the third change and is also built; it is described
below.

### Two effects outlive the turn that ordered them

This section used to read *two orders*, and used to say that one prerequisite
blocked both. Both halves of that were wrong, and the decisions below are what
showed it.

A ship or colony create may take several turns to finish, and `jump` takes
\(\lceil d / t \rceil\) turns to cross _d_ light years at technology level
_t_. **Neither is an order that keeps running.** A jump *departs* -- and what
outlives the turn is a ship in transit, which is not the order at all. A create
*commits* -- and what outlives the turn is an unfinished entity, which is not
the order either.

`create` was thought to be the one that needs something the pipeline lacks.
`game_order.status` is a three-way CHECK -- `pending`, `succeeded`, `failed` --
where `pending` means submitted and not yet resolved, and `ec turn open` purges
rows older than the most recently resolved turn. An order still running when its
turn resolves would be a fourth thing: resolved, not failed, not done. **No
order turned out to be that thing.** The `create` design settled it the way
`jump` was settled: the order succeeds on the turn it is given and holds nothing
-- the `with` clause is a per-turn cap on workers, not a cadre reserved for the
duration -- and the build carries on in rows of its own, consumed by sweeps at
stages 5, 9, and 10. The status column and the purge stay as they are.

Two rules that were open here were settled, and both are now built:

- **A jump's fuel is drawn in full on departure.** The whole bill -- 40 per
  assembled `HDRV` unit per light year -- is charged in the turn the order
  executes, however many turns the crossing then takes. This is what a
  single-turn jump already does, so no built behaviour changes and no golden
  moves; it also means a ship that cannot pay never leaves, which is the answer
  that needs no rule for running dry halfway. `docs/accepted-orders.md` carried
  this as a TODO and now carries the decision.
- **A ship in transit is nowhere, and a crossing is a row of its own.**
  `entity.stellium_id` is nullable and the location CHECK has an arm for a ship
  with no stellium, no system, no planet, and no ring; an `in_transit` table
  holds the ship, the stellium it is bound for, and the turn it is due.
  The row is written when the jump executes and deleted when the ship arrives,
  and it is the only thing that knows where the ship went. A crossing ship is
  invisible to probes, to passive sensors, and to the turn report, and can be
  given no order -- it cannot be recalled or redirected, which is deliberate:
  a higher-technology drive does not reach further, since every drive reaches
  everywhere, it spends fewer turns off the board. That is what makes building
  one worth doing.

**This is what split `jump` from `create`, and the split outlived its reason.**
The crossing is not the order: the jump order departs, burns the whole fuel
bill, and *succeeds*. So `jump` needed no fourth status and no change to the
purge -- and neither, in the end, does `create`. The two effects that outlive
their turn are the same shape after all, and nothing is waiting on a schema
change to `game_order`.

### The jump rework is built

Stage 15 is three steps rather than two -- move, departures, arrivals -- so
`PhaseArrival` is a sweep-only phase beside `PhaseSensor`, and the turn is still
nothing but its table. What it cost:

- `entity.stellium_id` is nullable with a CHECK arm for a ship that is nowhere,
  and `in_transit` holds the ship, its destination, and the turn it is due. The
  baseline migration was edited rather than appended to, which the rule still
  allows because no live game has been built from it; **`games/beta/rebuild.sh`
  has to be run against any database made before this.**
- `world` gained `Depart` and `LandArrivals` and a `Transit` on `Entity`;
  `Drive.TurnsForJump` is the whole of the rule.
- `Binder.actor` refuses a ship in transit, once, for every order there will
  ever be.
- The turn report gained an `IN TRANSIT` section, because otherwise a ship
  simply vanishes for the length of its crossing.

Two things fell out that were not designed for. A one-turn crossing is written
and consumed within the same turn, so it is the *same* path as a long one and
every existing golden held still -- the only diff in the replay was the new
report section and the new ship. And a ship can no longer chain jumps within a
turn to cross the map, because the shortest crossing still occupies the turn it
began in; two of the old tests existed to assert the opposite and now assert the
refusal.

The nullable location was the half expected to be expensive, and was not: the
probe sweep already filtered on `planet_id`, `locationAttr` already treated a
zero stellium as nowhere, and only the sensor sweep, the entity report, and
`order_movement` for a failed order needed anything said to them.

### What the code still has to be told

The accepted doc gives the surface forms, so what is missing is semantics
rather than syntax:

- **ASSEMBLE / UNASSEMBLE / TRANSFER** -- **built.** See "Batch 1 is built"
  below. The four rules that were open here were answered by the user and are
  now written down in `docs/orders.md`, `docs/units.md`, and `docs/model.md`.
- **CREATE** -- **designed, and no longer waiting on rules.**
  `docs/plan/entity_build_bom_process.md` settles it end to end and
  `docs/plan/bom-rewrite.md` records what changed on the way. Every question
  that stood here is answered: a ship or colony create is a standing commitment
  that succeeds when it is given and claims its materials over turns; the `with`
  clause is a per-turn cap rather than a cadre held for the duration; the new
  entity appears at the creating entity's planet in the ring its kind requires;
  the cadre is neither consumed nor released, because it was never held; and a
  shortage of anything is a slower build rather than a failed order. The three
  group forms finish by the opposite rule -- kill-and-fill, closing out inside
  stage 5 -- and their production rules are still unwritten, as is a
  `work_group` column for what a factory group is `making`. One item here is a
  decision rather than a gap: this plan says a trade station is an orbital
  colony, and the accepted grammar hangs `as trade-station` off all three colony
  kinds. `docs/accepted-orders.md` wins unless that is deliberately overturned.
- **GROUPS (add / remove / idle / activate / retool)** -- the doc settles the
  work-in-progress rules: a mine group has none, a farm group has some, retool
  drains the line before spending a turn, and an immediate retool discards it.
  `add` assembles what it puts into a group, the way a *group* create does, and
  the engine allocates the construction workers for it; a ship or colony create
  is the one that assembles at stage 10 instead, because its materials arrive
  over turns rather than being on hand. Open: what a group produces
  per turn, what labour it needs, and how the farm table's `HN x 100,000` cap is
  applied.
- **MARKET (buy / sell)** -- the doc gives both forms, prices in `GOLD` or
  `CNGD`, whole-`GOLD` tech levels 1 through 10, the transport rules, and that
  every sale pays a commission. Open: who the counterparty is, how offers are
  matched, where the default commission comes from, and what a bought tech level
  does.
- **DRAFT / DISBAND / PAY / RATIONS** -- forms given, effects not. All four need
  a population system that does not exist.
- **SURVEY / SPY / BROADCAST** -- survey has a form. Spy and broadcast still
  have examples and no grammar line, but the verbs are settled: six of them --
  `assess`, `detect`, `obtain`, `convert`, `incite`, `neutralize` -- each its
  own step of stage 14, because the engine allocates resources per step. Three
  act on rebels; `REBL` is a percentage of an entity's population, 0 through 99
  (`docs/units.md`), and no column holds it. Rebellion is stages 18 and 19 of
  the turn sequence and is a system that does not exist.
- **CONTROL / RELEASE / GRANT / REFUSE** -- forms given; what control confers,
  and what a trade or colonize permission permits, are not.
- **COMBAT (attack / invade / raid / support)** -- forms given, effects not.
  Needs a combat system, and `support ... defending` now carries what used to be
  a separate defend order.

### Batch 1 is built

`assemble`, `unassemble`, and `transfer` are three `Spec`s, three doc sections,
and tests -- which is what steps 0 through 4 were for. What they actually cost
was everything underneath them, because `internal/world` neither loaded nor
mutated inventory and `internal/fuel` was the only code that touched the table.

**`internal/world/inventory.go` is now the only thing that reads or writes
`inventory`.** An entity's mass, enclosed volume, drive, sensors, and fuel are
derived from what it holds, so every mutation writes the row and corrects all
five. `fuel.Available`, `fuel.AvailableAll`, `fuel.Spend`, `jumpdrive.Load(All)`
and `sensors.Load(All)` are gone with their SQL; `fuel` keeps `DrawOrder`, which
is the rule world spends by, and the other two keep their arithmetic. The
mutations are `ShiftAll` (between the sections of one entity), `Hand` and
`HandPopulation` (between two entities), `RoomAfter` (what a set of shifts would
leave, as arithmetic rather than a trial and a rollback), and `BurnFuel`, which
is now a draw through the same code.

Two new mechanic packages on the `internal/<mechanic>` template:
**`internal/cadre`** (the 500 MU a turn, `WorkersFor`, and `WorkAllowed`, which
is what makes the two pools round up separately) and **`internal/transport`**
(`Capacity`, `CrewedHulls`/`Limit`, `Pack`, and `Fuel`). Both are pure
arithmetic with no SQL, and `world` holds the per-turn pools they are asked
about, beside the probe budget.

**One migration**, appended to the baseline the rule still allows editing:
`entity_cadre`. A cadre is an assignment of population rather than a unit, so it
carries no mass of its own and its people stay in `entity_population`; the kit
loader gained a `cadres` block and checks that a kit assigning _n_ `CWKR`
carries _n_ `SKW` and _n_ `USK`. **`games/beta/rebuild.sh` has to be run against
any database made before this.**

**The four open rules, as the user settled them:**

- **Which sections, and which units.** `HDRV`, `SNSR`, `SDRV`, `LFSU`, `STRC`,
  and `STRL` assemble into `component`, because `docs/units.md` says of each of
  them that a unit held anywhere else is freight. Everything else assembles into
  `operational`. Resources, population, and cadres are never assembled. The
  order says nothing about the section; the unit code decides.
- **Unassembly is lossless.** What comes apart is what went together.
- **Too little space fails the order**, and nothing moves. It is the one thing
  that fails an assemble or an unassemble outright, because doing less of the
  order would not fix it. The same check runs on `assemble`, which usually costs
  volume, for the same reason.
- **A transfer reaches your own entities and uncontrolled ones**, and no other
  faction's. `world.Game().Uncontrolled` is loaded for it.

**A fifth rule turned up and was settled the same way.** `CWKR` is made by
`draft`, which is batch 3, so nothing had any. Gating on it would have killed
`assemble` until batch 3 and `create` with it. The user's answer was that an
order short of what it needs *does not fail*: it assembles what the workers paid
for and no more. That is now the shape of all three orders -- short of workers,
short of stock, short of transports -- and it is `Outcome.Note`, a warning at
check and submit and a `note` on the engine log line rather than an
`error_message`, because the order succeeded. Kits may assign a cadre so that
there is one to draw on before `draft` exists.

**A sixth rule, corrected after the fact.** Transport crew was first written as
the entity's whole `SKW`, on the grounds that `docs/units.md` says nothing about
a cadre competing for it. The user's answer is that it does: a cadre is an
assignment of real people, so those people are not available to be given a
second job, and a cadre cannot outlive them either. `cadre.Composition` says
which population a cadre assigns, `Entity.Unassigned` is what may be given other
work, and `world.setPopulation` settles every cadre back to what is left to fill
it -- 100 `SKW` and 100 `CWKR` losing three skilled workers leaves 97 `SKW`, 100
`USK`, and 97 `CWKR`. Taking the cadre and its people together is the same rule
read the other way round and arrives with combat and `disband`.

**Judgment calls, flagged rather than hidden:**

- **Co-location ignores the ring.** A ring is drawn afresh every time a ship
  settles at a planet, so requiring the same one would make a transfer nearly
  impossible. Same stellium, system, and planet is what `sameBerth` compares.
  **Confirmed by the user:** rings matter for combat and not for this.
- **Nothing checks the recipient's space.** The accepted doc gives one reason a
  transfer is partly filled -- transports -- and says outright that what arrives
  is stowed in cargo. Whether a recipient can refuse for want of room is not
  written anywhere, and neither is what `LFSU` does to population that arrives.
- **`TRAN`'s own mass and volume are still the provisional table's.**
  `docs/units.md` gives it a flat 4 MU and \(4t\) VU; `internal/units` gives
  every unit the provisional \(2t\), and correcting one unit of it would change
  every kit built from it and every golden with it. Noted in `docs/units.md`.

**What batch 1 wants and did not get:** a partly filled order has nowhere on the
order row to say so. `game_order`'s status block keeps a message and a success
apart, correctly, and a note is not an error. The engine log and the submission
warnings carry it; a player reading the orders report does not see it. That is
the `result` column step 4 deliberately did not add, and batch 2 will want it,
because a build's progress is the same shape.

**Batch 1 landed in two commits, and the split was deliberate.** The first left
`internal/replay`'s scenario and the CLAUDE-01 corpus alone, so the goldens were
the clean signal that a large change to `world` had moved nothing: green with no
`-update`, and 147 byte-identical report files. The second put the three orders
under that net.

The scenario gained entity **107**, a depot colony with a `CWKR` cadre, twenty
`TRAN-1`, a hundred `STRC-10`, and stock to work on, and each of the three turns
exercises a different half of the rules: turn 0 an assemble rationed by the
cadre (62 of 100, hand-checkable at 40 MU a sensor against 2,500 MU of cadre)
and a transfer between two rings of one planet; turn 1 the whole pipeline in one
turn -- unassemble and stow at stage 6, carry at stage 9, assemble at stage 10 --
with the second assemble exactly covered by what the first order left of the
cadre; turn 2 an unassemble refused for want of room (3,040 VU into 0 VU), a
transfer filled partway by both the stock and the transports, and one refused
for not being co-located. `TestSubmitRejects` gained the seven failures that
reject a whole file, including the quantity written without its separators.

Every existing golden moved, in one way that is worth knowing about: **rings
shifted**. A ring is drawn from the game seed and the order's sequence number,
sequence numbers are handed out in phase order, and the three new phases come
*before* the old ones. Appending a phase costs nothing in the goldens, as the
NAME work found; prepending one renumbers everything after it. Nothing else in
the old lines changed.

**A seventh rule, found by writing the goldens.** `docs/turn-sequence.md` says
at stage 10 that "units that arrived by transfer at stage 9 can be assembled
here in the same turn". As first built they could not: a transport sets
everything down in `cargo`, `assemble` drew only from `unassembled`, and no
order moved one to the other, so the pipeline the turn sequence is arranged
around stopped one step short at its far end. **The user's answer: `assemble`
draws from `unassembled` first and from `cargo` after it.** Unassembled
inventory is the section units are kept in to be worked on -- what an
`unassemble` leaves behind and what the market deals in -- so it goes first; the
cadre does not care which section a unit came out of, and rations the two as one
pool. The scenario's turn 1 now runs the whole pipeline to its end: 107
unassembles and stows, carries to 101, and 101 assembles what arrived, in that
turn.

### The cadres are named, and one of them is specified

`CWKR`, `LABR`, `PLCF`, `SPCF`, and `TRNE` have `docs/units.md` entries. A cadre
is a temporary assignment of population rather than a unit, so it needs no row
in `entity_population` -- but the population it assigns is real, and carries
that population's mass and volume, so a cadre can be transported. Nothing models
an assignment yet; a cadre table is one of `create`'s costs.

`CWKR` is specified now. It is **one `SKW` plus one `USK`**, made by `draft`
(`ship 18 draft 4,250 CWKR`); one does **500 MU of work a turn**; and a worker
does **one task per turn**, so an entity's pool is drawn down in stage order --
a group create at 5, an `add` at 8, then the `assemble` orders and the ship and
colony builds at 10. The engine allocates it for an `add` and for a bare
`assemble` without being told to, and drafting enough to cover the turn's
expected work is the faction's job. `create` is the exception that names its
own, because a ship or colony build runs for several turns and so is the only
assembly worth throttling. What happens when a faction has not drafted enough is
settled too: a shortfall is a rate rather than a failure, costing a build that
turn's progress and a kill-and-fill order some of what it asked for. The other
four cadres are still names.

## Step 5 — add the orders

A new order becomes: one `Spec` (parse + bind + apply), a section in
`docs/orders.md`, and tests.

**Where this stopped, and what starts it again.** None of the thirty-one are
built, and none of them are waiting on the pipeline: an order costs a `Spec`, a
doc section, and tests now, which is what steps 0 through 4 were for. They are
waiting on what the verbs *do*. That is the list in "What the code still has to
be told", and it is the 1978 text the user is supplying, verb by verb. Nothing
in the code has to be prepared for it first, so a batch can be built the week
its rules land.

Two pieces were blocked on this repository rather than on the rules, and neither
is now. The **`jump` rework** is built -- see "The jump rework is built" above.
**`create`** is designed: `docs/plan/entity_build_bom_process.md` is its plan of
record, the fourth order status turned out not to be needed, and what it waits
on is implementation rather than a decision. That cost is real and is listed
there -- inventory mutation in `internal/world`, which has none today; the first
multi-line parse; `under_construction`, `construction_item`, and a cadre table;
and the mass-and-volume rules lifted out of `cmd/ec/kit.go` into an
`internal/units` package.

**Batch 1 is built.** It was the place to start because everything below moves
units through the mutations it establishes, and because it was the only batch
whose rules were settled. Four more were asked and answered while building it;
see "Batch 1 is built" above. Everything else still waits on rules.

The accepted doc carries thirty-five verbs. Seven are built -- `move`, `jump`,
`probe`, `name`, and batch 1's `assemble`, `unassemble`, and `transfer` -- and
one of those still needs reworking: `name`, for naming another faction's ships
and colonies. `jump` is finished, crossings and all. Twenty-eight remain,
batched so that each exercises what the next needs:

1. **Inventory & cargo** — `assemble`, `unassemble`, `transfer`. **Built.**
   `world` now owns the inventory table outright, `internal/cadre` and
   `internal/transport` hold the two mechanics, and `entity_cadre` holds the
   assignment a kit may make. See "Batch 1 is built" above for the rules that
   were settled on the way and the four judgment calls that were not.
2. **Entities & groups** — `create`, `add`, `remove`, `idle`, `activate`,
   `retool`. Establishes `CreateEntity`, the `work_group` tables, and is the
   first batch to exercise a multi-line order end to end. **No longer blocked on
   the fourth order status**, which `create`'s design does without. It does
   depend on batch 1 rather than merely following it: a build claims, delivers,
   and assembles, so `world` needs batch 1's inventory mutations before `create`
   can move anything.
3. **Population & upkeep** — `draft`, `disband`, `pay`, `rations`. A population
   system on the `internal/fuel` / `jumpdrive` / `sensors` package template.
4. **Market** — `buy`, `sell`, for units and for tech levels. Currency,
   commission, and the first order whose counterparty is the game rather than a
   faction.
5. **Information** — `survey`, the six espionage verbs, `broadcast`. Cheap and
   largely independent of the four above. The verbs are settled now; what is
   missing is `REBL` and what a spy costs.
6. **Control & diplomacy** — `control`, `release`, `grant`, `refuse`, and the
   `name` rework. First orders naming another faction's entities;
   `findEntity`'s hardcoded owner check (`orders.go:587`) becomes per-Spec
   policy, and `we` becomes a real subject.
7. **Combat** — `attack`, `invade`, `raid`, `support`. The
   orders-and-a-sweep case, which the market and espionage share. Reuse the
   deterministic seeding already in place (`seed.ringFor` / `mix`,
   `engine.go:314-336`).

Load, unload, jettison, set up, build change, and mining change were in earlier
drafts of this plan and are not in the accepted doc. Found, abandon, scrap,
board, declare, gift, trade, message, assign, mine, farm, manufacture, and
recruit were guesses at verb names before the doc existed; the doc names none of
them.

## What deliberately does not change

- `zombiezen.com/go/sqlite` direct, `ff/v4`, no ORM, no JSON API.
- The `internal/<mechanic>` package template (`fuel`, `jumpdrive`, `sensors`).
- `entity`, `inventory`, `work_group`, `entity_population` and the location
  CHECK constraints in `docs/entity-location.md`.
- The findings tables and the sensor snapshot semantics.
- The order file format players write, apart from `#` comments.

---

## Verification

Run at the end of every step:

```sh
go test ./internal/orders/... ./internal/engine/...   # narrowest first
go test ./...
go test ./internal/replay                             # the golden gate
./games/claude/replay.sh --json /tmp/candidate        # 147 files, byte-identical
./games/beta/rebuild.sh
gofmt -l ./cmd ./internal
```

`go test ./internal/replay` **must stay green through steps 2–4.** A diff is
either a bug or a deliberate rules change that needs `-update` and a note in the
commit message. To confirm the net still bites, perturb `FuelForHop` in
`internal/jumpdrive/jumpdrive.go` and check the test names the engine log, the
orders report, and the ship's fuel in the turn report.

For step 4 specifically: `ec db verify` on a fresh database, and confirm the
tri-state CHECK still rejects a hand-written bad row (`status='failed'` with a
NULL `error_message`).

## Docs to update as you go

`docs/orders.md` (per-order sections — a test enforces this), `docs/model.md` (kept in
sync with the migrations), `CLAUDE.md` (the turn lifecycle, the migration rule,
new `internal/` packages), `AGENTS.md` for the migration-baseline rule, and
`docs/gamemaster-turn.md` if the report shape changes. `docs/accepted-orders.md` is
accepted rather than a draft, so a change to it is a decision: bring `docs/orders.md`
and the code to it rather than the other way round. `docs/turn-sequence.md` is
reconciled with it and is the authority on *when* an order takes effect.

This file is the plan of record. It began outside the repo, under
`~/.claude/plans/`; that copy is superseded.

## Working agreements

- Commit and push directly to `main`. Never create a branch.
- Do not future-proof the starting kits; they are cheap to update.
