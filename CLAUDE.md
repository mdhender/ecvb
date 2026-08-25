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
  one.** The last migration shows the house pattern for a destructive change — a guard
  table plus trigger that aborts with an actionable message when live data would be
  lost.
- Multi-step state changes (loading a game, adding a player with a kit, submitting
  orders, resolving a turn) run inside a single transaction and roll back whole.

## Domain model

Read `model.md` first; it is the authoritative description of the schema and is kept in
sync with the migrations. Supporting docs:

- `entity-location.md` — the location rules encoded as CHECK constraints on `entity`
  (`SHIP` may sit at stellium level or at a planet in ring 1–99; `COPN`/`CSFC` are ring
  0; `CORB` is ring 1). Any code that moves an entity must satisfy these.
- `units.md` — unit-code glossary plus the provisional mass/volume formulas the kit
  loader uses.
- `orders.md` — the order file format and check/submit semantics.
- `gamemaster-turn.md` — the operator runbook for resolving a turn.
- `distance_map.md` — design note on stellium distance; the decision is to keep
  squared-distance comparisons in SQL and skip `sqrt()`.

Spatial hierarchy: `game → stellium → system (A–E) → planet (orbit 1–10) → deposit`.
Plural of *stellium* is *stellia*. Distance between stellia is Euclidean, rounded up;
compare squared distances so no floating point enters the query.

## Adding an order

An order registers one `Spec` in `internal/orders/verbs.go` and nowhere else:

```go
register(&Spec{
    Verb:    "move",
    Summary: "move a ship inside its stellium, to a planet or to the stellium orbit",
    Syntax:  []string{"move ship SHIP-ID to orbit ORBIT", ...},
    Parse:   func(line *Line) (Order, error) { ... },
})
```

The parser tokenizes a line once and dispatches on its verb, so a line is only
ever measured against the forms of the order it names, and a mistyped verb is
told which orders exist. `Parse` consumes from a `*Line` using the shared field
readers in `token.go` -- `entityID`, `number`, `systemLetter`, `coordinates`,
`orbitList`, `quoted` -- rather than a regex per surface form.

Errors are of two kinds and are treated differently. A line that never matched
the shape of its order returns a `syntaxErr` (every `expect` does), and the
player is shown that verb's `Syntax`. A field that was read and found wrong
returns a plain error, which is reported as written.

`ec orders help [ORDER]` prints the registry, and a test fails if `orders.md`
does not document every registered form.

## Turn lifecycle

`game.turn_state` is `open` or `resolved`.

1. Players submit orders. `internal/orders` parses (`Parse`), validates against the
   database, and `Submit` atomically replaces that faction's pending `move_order` and
   `jump_order` rows for the current turn. `Check` does the same validation with no
   writes.
2. `ec turn resolve` runs `internal/engine.Resolve` in one transaction: **all MOVE
   orders resolve before any JUMP order**. Expected game-rule failures are recorded on
   the order row (`status = 'failed'` plus `error_message`, final location equal to
   start location) and do not abort the turn; database/state errors roll the turn back.
   State flips `open → resolved`; the turn number does not change.
3. `ec turn open` advances the turn number and purges order rows older than the most
   recently resolved turn, so the previous turn's outcomes stay readable via
   `ecrpt show orders --turn N`.

Order tables carry both `sequence` (engine resolution order: moves before jumps) and
`source_line` (position in the submitted file). The three-way status CHECK constraint
on `jump_order`/`move_order` is what enforces the pending/succeeded/failed shape — new
order kinds should follow the same table layout.

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
