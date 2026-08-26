# Make the order pipeline cheap to iterate on

## Context

Four orders exist (`MOVE`, `JUMP`, `PROBE`, `NAME`). Twenty-six remain, spanning
inventory & cargo, entities & groups, population, market, information, control &
diplomacy, and combat — then the production system and the combat system. The 1978 rules are being discovered by
playing, so orders get reworked after they land, not just added.

Adding one order used to touch ~33 places across 18 files (measured from
`git show 9d1b5c8`: 18 files, +1,542/−42). Four structural taxes cause that, and
this plan removes them before adding the orders.

**Status: steps 0 through 4 are done. Step 5 has begun: NAME is built
(`e3eada9`) and `docs/accepted-orders.md` is now accepted rather than proposed.
It specifies a surface the built parser cannot read, so the parser rewrite comes
first, and mapping the accepted verbs onto `docs/turn-sequence.md` comes before
the order batches. The rest of step 5 is still waiting on rules the doc does not
settle -- see "What step 5 needs" below.**

---

## Progress

| Step | State | Commit |
| --- | --- | --- |
| 0 — regression net | **done** | `816e476`, `6d46eaa`, `1803dab`, `cdabe4f` |
| 1 — tokenizer + order registry | **done** | `f48e0af` |
| 2 — one implementation of each rule | **done** | `3558c4a` |
| 3 — data-driven phases | **done** | `bb7a076` |
| 4 — one order table | **done** | `8e8215c`, `132e928` |
| 5 — add the 25 orders | **in progress** | `e3eada9` (NAME) |

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
`ec orders help [ORDER]`, and a test that fails if `orders.md` omits a
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
  takes 1 VU on a ship, and enclosed space must be ≥ 1.1 × occupied). All 147
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
  phase, printed from the same table the engine walks; `orders.md` carries it
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
loop. Combat needs structure *inside* a phase (`orders.md:5` speaks
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
19. It cost one `Spec`, a section in `orders.md`, and tests -- no SQL in
`Submit` or the engine, no new pass in either loop.

Two gaps the pipeline had never been asked about turned up and were closed: an
order with **no actor at all** (naming a stellium leaves `actor_entity_id`
null, which the schema allowed and the engine did not), and fuel being logged
by verb rather than by whether the order moves anything.

Appending a phase cost nothing in the goldens: sequence numbers are handed out
in phase order, so a later phase only adds sequences at the end and every
existing ring draw is untouched. The golden diff was 216 lines added, none
removed.

**Not built:** `name player FACTION-ID (ship|colony) ID "NAME"`. It is unclear
whether `player 5` means faction 5 or player number 5, and `replay.sh` says
those differ.

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
`we`. `orders.md` moves with them, and `doc_test.go` fails until it does.

`units.md` and `model.md` specify the nouns completely -- the four inventory
sections, per-unit mass and volume in each, enclosed volume from assembled
`STRC`/`STRL` at \(t^2\) VU, the efficiencies (COPN 1, CSFC 0.2, CORB and
SHIP 0.1), the 1.1x excess-space rule, bulk resources in external depots. They
say nothing about what the batch-1 verbs *do*. Writing them would be authoring
rules, not implementing them, so the user is supplying the 1978 text.

### The turn sequence and the accepted doc no longer name the same orders

`docs/turn-sequence.md` is still the 1978 twenty-one stages, and the accepted
doc has moved. Three of its stages name orders that no longer exist -- set up
(4), build change (6), mining change (7); `retool` replaced build change, and a
mine group is now moved by taking it down and building it again. The accepted
doc has orders the sequence never names: `create` and its four kinds, the group
variants (`add`, `remove`, `idle`, `activate`), `disband`, and
`grant`/`refuse colonize`. Mapping the accepted verbs onto stages -- and
deciding which stages are sweeps rather than orders -- is the next piece of
work, and it settles the `phases` table in `spec.go` that the engine loops over.

### What the code still has to be told

The accepted doc gives the surface forms, so what is missing is semantics
rather than syntax:

- **ASSEMBLE / UNASSEMBLE** -- the doc says assembly usually increases a unit's
  volume and that unassembling optionally stows to cargo. Open: which sections
  it moves between; which units may be assembled and into which section; whether
  it costs labour, time, or a resource; any per-turn limit per entity; whether
  unassembling is lossless; what happens when unassembling `STRC` would drop
  enclosed space below what is occupied.
- **TRANSFER** -- the accepted form is `ship 18 transfer 4,500 GOLD, 18,000
  FOOD to colony 24`, and the doc now settles co-location (the order fails if
  the two entities are not in the same place), that the units must be in cargo
  and are stowed on arrival, and that a shortage of transports partially fills
  it. Still open: which inventory sections the units leave and arrive in, and
  whether it crosses factions. Population moves too: `500 SOL`. This is the
  closest of the lot to buildable.
- **CREATE** -- the doc gives the form, the `end` terminator, that the units are
  assembled automatically, and that a trade station is an orbital colony. Open:
  where the new entity appears and in which ring; what the `CWKR` cadre does and
  whether it is consumed; what the creating entity must be near; what happens
  when the units named are short.
- **GROUPS (add / remove / idle / activate / retool)** -- the doc settles the
  work-in-progress rules: a mine group has none, a farm group has some, retool
  drains the line before spending a turn, and an immediate retool discards it.
  Open: what a group produces per turn, what labour it needs, and how the farm
  table's `HN x 100,000` cap is applied.
- **MARKET (buy / sell)** -- the doc gives both forms, prices in `GOLD` or
  `CNGD`, whole-`GOLD` tech levels 1 through 10, the transport rules, and that
  every sale pays a commission. Open: who the counterparty is, how offers are
  matched, where the default commission comes from, and what a bought tech level
  does.
- **DRAFT / DISBAND / PAY / RATIONS** -- forms given, effects not. All four need
  a population system that does not exist.
- **SURVEY / SPY / BROADCAST** -- survey has a form. Spy and broadcast have
  examples and no grammar line at all, so their verbs are not settled: the spy
  examples read as five different verbs (`report`, `obtain`, `convert`,
  `incite`, `attack`), not as one `spy` verb with five objects.
- **CONTROL / RELEASE / GRANT / REFUSE** -- forms given; what control confers,
  and what a trade or colonize permission permits, are not.
- **COMBAT (attack / invade / raid / support)** -- forms given, effects not.
  Needs a combat system, and `support ... defending` now carries what used to be
  a separate defend order.

### Two things in the accepted doc to settle before they reach code

- **`attack` names two different orders**: combat (`colony 24 attack ship 18
  75%`) and espionage (`colony 24 attack faction 1 spies using 11 spies`). Same
  subject shape, same verb, different grammar and different phase. One `Spec`
  has to carry both, or one has to be renamed.
- **The cadres are not units.** `CWKR`, `PLCF`, `SPCF`, and `TRNE` are defined
  in the accepted doc and appear in no `units.md` entry, no migration, and no
  kit. `CWKR` is required by every `create`.

## Step 5 — add the orders

A new order becomes: one `Spec` (parse + bind + apply), a section in
`orders.md`, and tests.

The accepted doc carries thirty verbs. Four are built -- `move`, `jump`,
`probe`, `name` -- and `name` needs reworking for the faction subject. Twenty-six
remain, batched so that each exercises what the next needs:

1. **Inventory & cargo** — `assemble`, `unassemble`, `transfer`. Establishes
   `World`'s inventory mutations, which everything below moves units through.
2. **Entities & groups** — `create`, `add`, `remove`, `idle`, `activate`,
   `retool`. Establishes `CreateEntity`, the `work_group` tables, and is the
   first batch to exercise a multi-line order end to end.
3. **Population & upkeep** — `draft`, `disband`, `pay`, `rations`. A population
   system on the `internal/fuel` / `jumpdrive` / `sensors` package template.
4. **Market** — `buy`, `sell`, for units and for tech levels. Currency,
   commission, and the first order whose counterparty is the game rather than a
   faction.
5. **Information** — `survey`, the espionage verbs, `broadcast`. Cheap and
   largely independent of the four above; blocked on grammar rather than on
   rules.
6. **Control & diplomacy** — `control`, `release`, `grant`, `refuse`, and the
   `name` rework. First orders naming another faction's entities;
   `findEntity`'s hardcoded owner check (`orders.go:587`) becomes per-Spec
   policy, and `we` becomes a real subject.
7. **Combat** — `attack`, `invade`, `raid`, `support`. The
   phase-with-internal-structure case. Reuse the deterministic seeding already
   in place (`seed.ringFor` / `mix`, `engine.go:314-336`).

Load, unload, jettison, set up, build change, and mining change were in earlier
drafts of this plan and are not in the accepted doc. Found, abandon, scrap,
board, declare, gift, trade, message, assign, mine, farm, manufacture, and
recruit were guesses at verb names before the doc existed; the doc names none of
them.

## What deliberately does not change

- `zombiezen.com/go/sqlite` direct, `ff/v4`, no ORM, no JSON API.
- The `internal/<mechanic>` package template (`fuel`, `jumpdrive`, `sensors`).
- `entity`, `inventory`, `work_group`, `entity_population` and the location
  CHECK constraints in `entity-location.md`.
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

`orders.md` (per-order sections — a test enforces this), `model.md` (kept in
sync with the migrations), `CLAUDE.md` (the turn lifecycle, the migration rule,
new `internal/` packages), `AGENTS.md` for the migration-baseline rule, and
`gamemaster-turn.md` if the report shape changes. `docs/accepted-orders.md` is
accepted rather than a draft, so a change to it is a decision: bring `orders.md`
and the code to it rather than the other way round. `docs/turn-sequence.md` is
still the 1978 text and has not been reconciled with it.

This file is the plan of record. It began outside the repo, under
`~/.claude/plans/`; that copy is superseded.

## Working agreements

- Commit and push directly to `main`. Never create a branch.
- Do not future-proof the starting kits; they are cheap to update.
