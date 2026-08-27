# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`AGENTS.md` holds the project's binding coding standards (stack choices, auth rules,
SQLite practices, style). Read it before changing code; this file covers the parts of
the layout and workflow that only emerge from reading several files at once.

## Build, test, run

```sh
go build ./...
go test ./...
go test ./internal/orders -run TestSubmit      # narrowest relevant test first
gofmt -w <changed files>
```

Commands are run with `go run ./cmd/<name>` during development; `/db`, `/ec`, and
`/ecgen` at the repo root are gitignored build outputs.

`games/beta/rebuild.sh` (run from the repo root) tears down and rebuilds the beta game
end to end — create database, seed, create game, load map, add ten players, generate
reports, check/submit orders, resolve turn 0, open turn 1. It is the fastest way to
exercise the whole pipeline and to regenerate `games/beta/reports/*`. It deletes
`games/beta/ecvb.db` first.

## Binaries

There is no web server yet; `AGENTS.md`'s `cmd/server`, `html/template`, and HTMX rules
describe the intended shape of code that has not been written.

| Command | Responsibility |
| --- | --- |
| `cmd/db` | Only place allowed to create a database file or apply migrations (`create`, `migrate up`, `seed`). |
| `cmd/ecgen` | Generates map seed JSON: `stellia` → `systems` → `planets` → `deposits`, each reading the previous file from the target directory. Deterministic from `--stellia-seed`. |
| `cmd/ec` | Gamemaster mutations: `game create`, `load game`, `add player`, `orders check|submit`, `turn resolve|open`, `db verify`. |
| `cmd/ecrpt` | Read-only reporting: `show orders|stellium|system|turn`, opens the database `OpenReadOnly`. `show --format json` renders any report as JSON. |

All four take `--db-path DIR` (a *directory*; the file inside it is always
`database.Filename` = `ecvb.db`). `cmd/ec` and `cmd/ecgen` parse flags with
`ff.WithEnvVarPrefix("EC")`, so `--db-path` also reads `EC_DB_PATH`, `--game-seed`
reads `EC_GAME_SEED`, and so on.

## Database access invariants

- Every writing command calls `openVerifiedDatabase` (or the `cmd/db` equivalent):
  stat the file and require a regular file, open `OpenReadWrite` **without**
  `OpenCreate`, `PRAGMA foreign_keys = ON`, then check `application_id` equals
  `database.ApplicationID` (0x65637662, "ecvb") and `user_version` equals
  `database.SchemaVersion`. A mismatched version is a hard error, not an auto-migrate.
- `internal/database/migrations.go` is the single source of schema truth: an ordered
  `[...]string` of SQL, applied via `sqlitemigration`. `SchemaVersion` is derived from
  its length. **Add a migration by appending a new element; never edit an existing
  one.** The one exception is the baseline, which is currently the whole list: no
  database anyone cares about has been built from it, so it is still cheap to
  rewrite. **From the first live game onward the list is append-only without
  exception.** A destructive migration follows the house pattern — a guard table plus
  a trigger that aborts with an actionable message when live data would be lost.
- Multi-step state changes (loading a game, adding a player with a kit, submitting
  orders, resolving a turn) run inside a single transaction and roll back whole.

## Domain model

Read `docs/model.md` first; it is the authoritative description of the schema and is kept in
sync with the migrations. Supporting docs:

- `docs/entity-location.md` — the location rules encoded as CHECK constraints on `entity`
  (`SHIP` may sit at stellium level or at a planet in ring 1–99; `COPN`/`CSFC` are ring
  0; `CORB` is ring 1). Any code that moves an entity must satisfy these.
- `docs/units.md` — unit-code glossary plus the provisional mass/volume formulas the kit
  loader uses.
- `docs/orders.md` — the order file format and check/submit semantics.
- `docs/gamemaster-turn.md` — the operator runbook for resolving a turn.
- `distance_map.md` — design note on stellium distance; the decision is to keep
  squared-distance comparisons in SQL and skip `sqrt()`.

Spatial hierarchy: `game → stellium → system (A–E) → planet (orbit 1–10) → deposit`.
Plural of *stellium* is *stellia*. Distance between stellia is Euclidean, rounded up;
compare squared distances so no floating point enters the query.

## Adding an order

An order lives in `internal/orders/verbs.go` and nowhere else. It is three
things: a `Spec`, a `Params` type, and a `Bound` type.

```go
register(&Spec{
    Verb:     "move",
    Subjects: []string{SubjectShip},
    Summary:  "move a ship inside its stellium, to a planet or to the stellium orbit",
    Syntax:   []string{"ship SHIP-ID move to orbit ORBIT", ...},
    Phase:    PhaseMove,
    Parse:    func(subject Subject, line *Line) (Params, error) { ... },
})
```

Every order line is **subject first**: `ship 18 jump to (-1,2,3)`,
`colony 24 probe orbit 5`, `we name (-1,2,3) "Stellium Joe"`. The parser
tokenizes the line once, reads the subject, then dispatches on the verb, so a
line is only ever measured against the forms of the order it names -- and only
against the forms its subject may be given. `Subjects` is that list, and a line
whose subject is not on it is refused before `Parse` ever runs, so no order
checks who it was given to. `we` is the faction itself and carries no id.

`Parse` receives the subject already read and consumes the rest from a `*Line`
using the shared field readers in `token.go` -- `entityID`, `number`,
`systemLetter`, `coordinates`, `orbitList`, `quoted` -- rather than a regex per
surface form, and returns the order's own `Params` type, so a field belongs to
the one order that has it.

Syntax errors are of two kinds and are treated differently. A line that never
matched the shape of its order returns a `syntaxErr` (every `expect` does), and
the player is shown that verb's `Syntax`. A field that was read and found wrong
returns a plain error, which is reported as written.

`ec orders help [ORDER]` prints the registry, and a test fails if `docs/orders.md`
does not document every registered form.

### Bind and Apply

Every game rule is written once, and which of the two halves it belongs in is
the only design question an order asks:

- **`Params.Bind`** settles what a turn cannot change -- ownership, the kind of
  entity, a destination that exists, a drive's range and capacity -- and turns
  names into ids. A `Bind` failure **rejects the whole order file**, because it
  will still be a failure when the turn resolves. It reports every reason, not
  the first, so a player fixing a file sees the list.
- **`Bound.Apply`** executes against live turn state, for what *can* change:
  fuel on hand, a budget another order spent, somewhere another faction reached
  first. An `Apply` failure is a **warning** at submission -- the order is kept
  -- and a `failed` order row when the turn resolves. A returned *error*, by
  contrast, is a database or state failure and rolls the whole turn back.

Both run in check, submit, and resolve. `Check` and `Submit` run them inside a
savepoint that is always rolled back (`discard`, `orders.go`), so a player's
file is measured by *doing* the turn rather than by simulating it; `Submit`
then stores the orders the rolled-back run bound. The test
`TestCheckPutsTheWorldBackTheWayItFoundIt` is what holds that rollback honest.

The engine binds a stored order again before applying it. That is not a second
implementation: the row carries the parameters the player wrote, not the ids
they resolved to, so `loadOrders` rebuilds `Params` and the same `Bind` runs.

`internal/world` is what both halves are written against: one game's entities,
stellia, systems, and planets, with mutations (`Move`, `BurnFuel`,
`RecordProbe`, `SpendProbe`, `RecordSensors`) that write through to SQLite and
keep the loaded copy in step. That is what makes the second order of a turn
measure a ship as the first order left it.

### Phases

A turn is the table in `spec.go` and nothing else:

```go
var phases = []*Phase{PhaseProbe, PhaseSensor, PhaseMove, PhaseJump, PhaseArrival, PhaseNaming}
```

`orders.Phases()` is what both `simulate` and `engine.resolve` loop over, so
adding a phase is an entry in that table plus a `Phase` on the Specs that
resolve in it — no `switch`, no new pass, nothing in the engine to edit. A
phase may carry a `Sweep func(*world.World, int) error`, which is what the
phase does apart from anyone's orders: `PhaseSensor` has no orders at all and
is only its sweep, and combat will be a phase that is mostly sweep, because a
battle is settled between the fleets that met rather than one order at a time.
Sweeps run in check and submit as well as in resolve, inside the rolled-back
savepoint, so a check stays a faithful dry run once a sweep starts changing the
world the later phases see.

`ec orders help` prints the phase list and tags each order with its phase, from
the same table, so the reference cannot fall behind the engine.

`docs/accepted-orders.md` is the accepted order set the game is heading for and
`docs/turn-sequence.md` the twenty-two stages it resolves in. Neither is the
spec of what exists: `docs/orders.md` is what is actually built, and a test fails
when a registered form is missing from it. The accepted doc writes every order
subject first -- `ship 18 jump to (-1,2,3)`, and `we` for the orders no ship
carries out -- and gives `create` multiple lines terminated by `end`. The built
parser reads neither, so the parser rewrite comes before the rest of the orders;
`docs/plan/beta-plan.md` carries the design.

The two docs are reconciled: every accepted verb lands on a step of a stage, and
`docs/turn-sequence.md` is the authority on *when* an order takes effect. Its
twenty-two stages are forty-three phases, because a stage's lettered steps run in
their letter order and a step is exactly a `Phase`. Six stages are pure sweeps;
five are orders *and* a sweep -- combat, the market, espionage, ship movement,
the news service -- where the orders declare intent and one sweep settles all of
them against each other.

Two effects outlive the turn that ordered them, and they are not the same shape.
A `jump` order departs and *succeeds*; what continues is a ship in transit, a
row of `in_transit` -- ship, destination stellium, arrival turn -- landed by the
`arrival` phase, so `jump` needed no new order state. That half is **built**: a
crossing takes \(\lceil d / t \rceil\) turns, the ship is nowhere while it
makes it (`entity.stellium_id` is null) and can be given no order, and the whole
fuel bill is drawn on departure. A `create` genuinely keeps running, which the
three-way `status` CHECK and `ec turn open`'s purge have to learn about first;
that is now a `create` prerequisite alone.

## Turn lifecycle

`game.turn_state` is `open` or `resolved`.

1. Players submit orders. `internal/orders` parses (`Parse`), binds and applies
   them against a `world.World` inside a savepoint it rolls back, and `Submit`
   then atomically replaces that faction's pending order rows for the current
   turn. `Check` runs exactly the same thing and keeps nothing.
2. `ec turn resolve` runs `internal/engine.Resolve` in one transaction, walking
   `orders.Phases()` in order: **every order of one phase resolves before any
   order of the next**, today probe, sensor, move, jump, arrival, naming. Expected game-rule
   failures are recorded on the order row (`status = 'failed'` plus
   `error_message`, final location equal to start location) and do not abort the
   turn; database/state errors roll the turn back. State flips
   `open → resolved`; the turn number does not change.
3. `ec turn open` advances the turn number and purges order rows older than the most
   recently resolved turn, so the previous turn's outcomes stay readable via
   `ecrpt show orders --turn N`. `in_transit` is deliberately not purged: a
   crossing is live state that outlives the turn it began in.

Every order is a row of `game_order`, whatever its verb: `verb`, `actor_entity_id`,
`input` (the order in the words the player wrote), and `params` (everything else it
said, as JSON, also in the player's words and never as an id). It carries both
`sequence` (engine resolution order: earlier phases first) and `source_line` (position
in the submitted file). The three-way status CHECK is written once there.

Two child tables hang off it, keyed on the same four columns, for the orders that
record more than a status: `order_movement` (where the order took its actor — set
`Movement: true` on the Spec) and `order_survey` (the planet it read). A new order kind
needs neither unless it records that kind of thing.

Because `params` holds no ids, SQLite no longer checks that a jump's destination is in
this game. It does not have to: `Bind` resolves the coordinates against the game's own
stellia when the turn runs, so a destination that is not there is a failed order rather
than a corrupt row. The compound foreign keys that do fire — faction in game, actor
belongs to faction — are still columns.

Resolution writes one structured `slog` record per order to stderr; the runbook
captures it as `2>reports/tN-engine.log`. `ec turn resolve --no-log-timestamps`
drops the wall clock from those records, so the same turn logs the same bytes.

## Reproducible output

Two things exist so a turn's result can be compared against a golden file:
`ecrpt show --format json` renders a report as structured JSON rather than
column-aligned text, and `ec turn resolve --no-log-timestamps` writes an engine
log with no wall clock in it. `internal/report` models every report as a title
plus named tables of header and rows, and renders that one model as either
text or JSON, so a report is never written twice.

`games/claude/replay.sh [--json] [OUTPUT-DIR]` replays the whole committed
CLAUDE-01 corpus, seven turns and ten factions, from a fresh database. With
`--json` two runs produce byte-identical output.

## The regression net

`internal/replay` is the gate a change to the order pipeline has to pass.
`TestReplay` plays a three-turn scripted game -- submit, resolve, report, open
the next turn -- against a database built from the migrations, and compares
every engine log and report against `testdata/golden`. A refactor that
preserves behavior leaves every golden untouched; one that does not names the
report and the line. Rewrite the goldens with:

```sh
go test ./internal/replay -update
```

Read that diff before committing it: it is the record of what the rules now do.

The scenario in `testdata/scenario.sql` is built so the rules fire. Two kinds
of failure are covered in two places, because they behave differently. A
shortfall of `FUEL` is a warning at submission and a failed order at
resolution, since fuel may still reach the ship; that is in the replay.
Everything that cannot change between submission and resolution -- a missing
drive, a ship too heavy for it, a jump out of range, a destination that does
not exist, a spent probe budget -- rejects the whole file, so it is in
`TestSubmitRejects`, which also checks that a rejected file stores nothing.

## Environment files

`internal/dotenv.Load(env)` accepts exactly `development`, `test`, `production`, or
`agent`, selected by `EC_ENV` (default `development`). It loads, highest precedence
first: `.env.{env}.local`, `.env.local`, `.env.{env}`, `.env`. The `.local` files are
gitignored and may hold secrets; `.env` and `.env.{env}` are committed and must not.
The `agent` environment is reserved for coding-agent work — use it rather than pointing
a coding session at the developer's `games/beta` database.

## Tests

Tests live beside the code and drive commands through the package-level `run(ctx, args,
stdout, stderr)` function rather than `main`. They build databases in `t.TempDir()`,
often by hand-writing `PRAGMA application_id` / `PRAGMA user_version` to simulate a
wrong-version or corrupt database. Cover the path-handling boundaries the way the
existing tests do: missing parent directory, non-regular file, database that already
exists, version newer than supported.
