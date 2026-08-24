# How to execute a game turn

Use this procedure after player order submission has closed. Replace the game
code, turn, faction IDs, and paths with the values for the game you are running.

## 1. Verify the submitted orders

Review each faction's current order set:

```sh
go run ./cmd/ecrpt --db-path games/beta show orders \
  --game BETA-001 --faction 1
```

Repeat for every player faction. A faction with no orders requires no special
submission; it will take no actions during the turn.

If you have the source order files, check them without changing the submitted
orders:

```sh
go run ./cmd/ec --db-path games/beta orders check \
  games/beta/orders/t0-f1-orders-v1.txt
```

Resolve submission errors before continuing.

## 2. Resolve the turn

Create the report directory if it does not already exist, then resolve the
expected turn and save the structured engine log:

```sh
go run ./cmd/ec --db-path games/beta turn resolve \
  --game BETA-001 --turn 0 \
  2>games/beta/reports/t0-engine.log
```

The command prints a count of succeeded and failed orders. The engine executes
all `MOVE` orders before any `JUMP` orders. It records every outcome in the
database and writes one structured log entry per order.

If the command returns an error, the transaction was rolled back. Review the
error and engine log, correct the problem, and run the same command again.

Do not rerun a successfully resolved turn. It remains in the `resolved` state
until you open the next turn.

## 3. Review order outcomes

Generate an order report for each faction:

```sh
go run ./cmd/ecrpt --db-path games/beta show \
  --output games/beta/reports/t0-f1-orders.txt \
  orders --game BETA-001 --turn 0 --faction 1
```

Inspect every order whose status is `failed`. Its error column describes the
failure, and its final location will be the same as its starting location.

Use the engine log when you need the complete chronological audit:

```sh
less games/beta/reports/t0-engine.log
```

## 4. Generate final turn reports

Generate a turn report for each faction while turn 0 is still resolved:

```sh
go run ./cmd/ecrpt --db-path games/beta show \
  --output games/beta/reports/t0-f1-turn-report.txt \
  turn --game BETA-001 --faction 1
```

Confirm that the entity locations and order outcomes match the engine summary.
Distribute or archive the reports before opening the next turn.

## 5. Open the following turn

After review and report generation are complete, open turn 1:

```sh
go run ./cmd/ec --db-path games/beta turn open \
  --game BETA-001 --turn 0
```

Players can now submit turn 1 orders. The turn 0 order outcomes remain available
throughout turn 1. To review them again, specify their turn explicitly:

```sh
go run ./cmd/ecrpt --db-path games/beta show orders \
  --game BETA-001 --turn 0 --faction 1
```

When you later open turn 2, the engine purges turn 0 orders and retains turn 1
outcomes.
