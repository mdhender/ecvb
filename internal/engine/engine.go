// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package engine resolves game turns.
package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mdhender/ecvb/internal/orders"
	"github.com/mdhender/ecvb/internal/world"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Result summarizes a resolved turn.
type Result struct {
	GameCode  string
	Turn      int
	Orders    int
	Succeeded int
	Failed    int
}

// OpenResult summarizes opening the next turn.
type OpenResult struct {
	GameCode string
	Turn     int
}

// storedOrder is one submitted order read back out of the database. It carries
// the parameters the player wrote rather than the ids they resolved to, so the
// engine binds and applies the order with the same code that checked it.
type storedOrder struct {
	verb      string
	factionID int64
	sequence  int
	line      int
	// input is the order in the words the player wrote, as stored. The log
	// echoes it rather than rebuilding it, so an order that fails to bind
	// still says what it asked for.
	input string
	// movement says whether this kind of order records where its actor began
	// and ended.
	movement bool
	params   orders.Params
}

type outcome struct {
	orderType string
	factionID int64
	sequence  int
	line      int
	actorID   int64
	input     string
	status    string
	message   string
	start     world.Location
	final     world.Location
	// movement says whether where the order began and ended is recorded.
	movement bool
	// fuelSpent is the FUEL the drive burned. An order that failed burned
	// none. Probes do not burn fuel.
	fuelSpent int64
	// survey is the planet the order read, for the orders that read one.
	survey *orders.Survey
	// note is what an order that succeeded still had to say: that it did less
	// than it was asked for. It is not an error, so it does not go on the
	// order row, where the schema keeps a message and a success apart.
	note string
}

// Resolve resolves an open turn. Every order of one phase resolves before any
// order of the next. Expected game-rule failures are recorded on the order and
// do not stop the turn; database and state errors roll back the entire turn.
func Resolve(ctx context.Context, logger *slog.Logger, conn *sqlite.Conn, gameCode string, turn int) (Result, error) {
	if logger == nil {
		return Result{}, fmt.Errorf("logger is required")
	}
	result, outcomes, err := resolve(ctx, conn, gameCode, turn)
	if err != nil {
		logger.ErrorContext(ctx, "resolve turn", "game", gameCode, "turn", turn, "error", err)
		return Result{}, err
	}
	for _, item := range outcomes {
		attrs := []slog.Attr{
			slog.String("game", gameCode),
			slog.Int("turn", turn),
			slog.String("order_type", item.orderType),
			slog.Int64("faction_id", item.factionID),
			slog.Int("sequence", item.sequence),
			slog.Int("source_line", item.line),
			actorAttr(item),
			slog.String("request", item.input),
			slog.String("status", item.status),
			locationAttr("start", item.start),
			locationAttr("final", item.final),
		}
		// An order reports its fuel when moving something is what it does, so
		// that a move that burned nothing still says so, and otherwise only
		// when it actually burned some -- a transfer moves no entity and still
		// runs its transports.
		if item.movement || item.fuelSpent != 0 {
			attrs = append(attrs, slog.Int64("fuel_spent", item.fuelSpent))
		}
		if item.message != "" {
			attrs = append(attrs, slog.String("error", item.message))
		}
		if item.note != "" {
			attrs = append(attrs, slog.String("note", item.note))
		}
		logger.LogAttrs(ctx, slog.LevelInfo, "order resolved", attrs...)
	}
	return result, nil
}

func resolve(ctx context.Context, conn *sqlite.Conn, gameCode string, turn int) (result Result, outcomes []outcome, err error) {
	if gameCode == "" {
		return Result{}, nil, fmt.Errorf("game is required")
	}
	if turn < 0 {
		return Result{}, nil, fmt.Errorf("turn must be nonnegative")
	}
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return Result{}, nil, fmt.Errorf("begin resolve turn transaction: %w", err)
	}
	defer end(&err)

	loaded, err := requireGameTurn(conn, gameCode, turn, "open")
	if err != nil {
		return Result{}, nil, err
	}
	game := loaded.Game()
	submitted, count, err := loadOrders(conn, game.ID, turn)
	if err != nil {
		return Result{}, nil, err
	}

	result = Result{GameCode: gameCode, Turn: turn, Orders: count}
	// The turn is its phases, in the order the table gives them. Adding a
	// phase is an entry in that table, not a case here.
	for _, phase := range orders.Phases() {
		for _, order := range submitted[phase] {
			if err := ctx.Err(); err != nil {
				return Result{}, nil, err
			}
			item, err := execute(conn, loaded, turn, order)
			if err != nil {
				return Result{}, nil, err
			}
			outcomes = append(outcomes, item)
			if item.status == orders.StatusSucceeded {
				result.Succeeded++
			} else {
				result.Failed++
			}
		}
		if phase.Sweep == nil {
			continue
		}
		if err := phase.Sweep(loaded, turn); err != nil {
			return Result{}, nil, fmt.Errorf("resolve the %s phase of game %q turn %d: %w",
				phase.Name, gameCode, turn, err)
		}
	}
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET turn_state = 'resolved'
		WHERE id = ? AND turn = ? AND turn_state = 'open';`, &sqlitex.ExecOptions{
		Args: []any{game.ID, turn},
	}); err != nil {
		return Result{}, nil, fmt.Errorf("mark game %q turn %d resolved: %w", gameCode, turn, err)
	}
	if conn.Changes() != 1 {
		return Result{}, nil, fmt.Errorf("game %q turn %d changed while it was resolving", gameCode, turn)
	}
	return result, outcomes, nil
}

// execute binds a stored order against the world as the turn found it and
// applies it.
//
// Binding again is not a second implementation of the rules but the same one:
// what Bind settles cannot change during a turn, so an order that bound at
// submission binds again here. An order that does not is one the world moved
// out from under -- a drive disassembled, a map edited -- and it fails without
// stopping the turn.
func execute(conn *sqlite.Conn, loaded *world.World, turn int, order storedOrder) (outcome, error) {
	// An order that acts on no entity -- naming a stellium, say -- has no
	// actor to find.
	actorID := order.params.Actor()
	var at world.Location
	if actorID != 0 {
		actor := loaded.Entity(actorID)
		if actor == nil {
			return outcome{}, fmt.Errorf("%s order faction %d sequence %d references missing entity %d",
				order.verb, order.factionID, order.sequence, actorID)
		}
		at = actor.Location
	}
	item := outcome{
		orderType: order.verb, factionID: order.factionID, sequence: order.sequence,
		line: order.line, actorID: actorID, input: order.input, movement: order.movement,
		status: orders.StatusSucceeded,
	}
	bounds, err := order.params.Bind(&orders.Binder{World: loaded, FactionID: order.factionID})
	switch {
	case err != nil:
		item.status, item.message = orders.StatusFailed, orders.Problem(err)
		item.start, item.final = at, at
	case len(bounds) != 1:
		return outcome{}, fmt.Errorf("%s order faction %d sequence %d bound to %d orders; want exactly one",
			order.verb, order.factionID, order.sequence, len(bounds))
	default:
		applied, err := bounds[0].Apply(&orders.Turn{
			World: loaded, Number: turn, FactionID: order.factionID, Sequence: order.sequence,
		})
		if err != nil {
			return outcome{}, err
		}
		item.status, item.message = applied.Status, applied.Message
		item.start, item.final = applied.Start, applied.Final
		item.fuelSpent, item.survey, item.note = applied.FuelSpent, applied.Survey, applied.Note
	}
	if err := updateOutcome(conn, loaded.Game().ID, turn, item); err != nil {
		return outcome{}, err
	}
	return item, nil
}

// OpenNextTurn advances a resolved game to its next open turn. It retains the
// most recently resolved turn's orders and purges older order history.
func OpenNextTurn(ctx context.Context, conn *sqlite.Conn, gameCode string, resolvedTurn int) (result OpenResult, err error) {
	if gameCode == "" {
		return OpenResult{}, fmt.Errorf("game is required")
	}
	if resolvedTurn < 0 {
		return OpenResult{}, fmt.Errorf("turn must be nonnegative")
	}
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return OpenResult{}, fmt.Errorf("begin open turn transaction: %w", err)
	}
	defer end(&err)

	loaded, err := requireGameTurn(conn, gameCode, resolvedTurn, "resolved")
	if err != nil {
		return OpenResult{}, err
	}
	gameID := loaded.Game().ID
	// What an order recorded goes before the order itself, because the record
	// points at it.
	for _, table := range []string{"order_movement", "order_survey", "game_order",
		"probe_contact", "probe_deposit", "sensor_survey", "sensor_contact"} {
		if err := sqlitex.ExecuteTransient(conn,
			"DELETE FROM "+table+" WHERE game_id = ? AND turn < ?;",
			&sqlitex.ExecOptions{Args: []any{gameID, resolvedTurn}}); err != nil {
			return OpenResult{}, fmt.Errorf("purge old %s rows: %w", table, err)
		}
	}
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET turn = turn + 1, turn_state = 'open'
		WHERE id = ? AND turn = ? AND turn_state = 'resolved';`, &sqlitex.ExecOptions{
		Args: []any{gameID, resolvedTurn},
	}); err != nil {
		return OpenResult{}, fmt.Errorf("open game %q turn %d: %w", gameCode, resolvedTurn+1, err)
	}
	if conn.Changes() != 1 {
		return OpenResult{}, fmt.Errorf("game %q turn %d changed while it was opening", gameCode, resolvedTurn)
	}
	return OpenResult{GameCode: gameCode, Turn: resolvedTurn + 1}, nil
}

// requireGameTurn loads a game and insists it is on the turn and in the state
// the caller expects.
func requireGameTurn(conn *sqlite.Conn, code string, turn int, state string) (*world.World, error) {
	loaded, found, err := world.Load(conn, code)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("game %q does not exist", code)
	}
	game := loaded.Game()
	if game.Turn != turn {
		return nil, fmt.Errorf("game %q is on turn %d, not turn %d", code, game.Turn, turn)
	}
	if game.State != state {
		return nil, fmt.Errorf("game %q turn %d is %s, not %s", code, turn, game.State, state)
	}
	return loaded, nil
}

// loadOrders reads a turn's submitted orders, grouped by the phase they
// resolve in. Every order is a row of one table whatever its verb, so this is
// one query, and the verb is what says which order to rebuild.
func loadOrders(conn *sqlite.Conn, gameID int64, turn int) (map[*orders.Phase][]storedOrder, int, error) {
	byPhase := make(map[*orders.Phase][]storedOrder)
	count := 0
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, verb, actor_entity_id, input, params, status
		FROM game_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			factionID, sequence := stmt.ColumnInt64(0), stmt.ColumnInt(1)
			if status := stmt.ColumnText(7); status != "pending" {
				return fmt.Errorf("order for faction %d sequence %d is already %s", factionID, sequence, status)
			}
			verb := stmt.ColumnText(3)
			spec, known := orders.Lookup(verb)
			if !known {
				return fmt.Errorf("order for faction %d sequence %d is a %q, which this game does not have",
					factionID, sequence, verb)
			}
			params, err := spec.Decode(stmt.ColumnInt64(4), stmt.ColumnText(6))
			if err != nil {
				return fmt.Errorf("read the %s order for faction %d sequence %d: %w", verb, factionID, sequence, err)
			}
			order := storedOrder{
				verb: verb, factionID: factionID, sequence: sequence, line: stmt.ColumnInt(2),
				input: stmt.ColumnText(5), movement: spec.Movement, params: params,
			}
			byPhase[spec.Phase] = append(byPhase[spec.Phase], order)
			count++
			return nil
		},
	}); err != nil {
		return nil, 0, fmt.Errorf("load orders: %w", err)
	}
	return byPhase, count, nil
}

// updateOutcome records what an order did: its status on the row it was
// submitted as, and then, for the orders that have them, the place it left its
// actor and the planet it read.
//
// The status is written first because the guard that keeps a failed order from
// having moved reads it.
func updateOutcome(conn *sqlite.Conn, gameID int64, turn int, item outcome) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game_order SET status = ?, error_message = ?, fuel_spent = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ? AND status = 'pending';`,
		&sqlitex.ExecOptions{Args: []any{
			item.status, nullableMessage(item.message), item.fuelSpent,
			gameID, turn, item.factionID, item.sequence,
		}}); err != nil {
		return fmt.Errorf("record %s order faction %d sequence %d outcome: %w",
			item.orderType, item.factionID, item.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("record %s order faction %d sequence %d outcome: order changed",
			item.orderType, item.factionID, item.sequence)
	}
	// An order given to a ship that is nowhere -- one crossing between stellia
	// -- records no movement, because there is no place to record. It fails to
	// bind, so it never went anywhere either; the failure and its reason are on
	// the order itself, where a report will find them.
	if item.movement && item.start.StelliumID != 0 {
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO order_movement (
				game_id, turn, faction_id, sequence,
				start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
				final_stellium_id, final_system_id, final_planet_id, final_planet_ring
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
			Args: []any{gameID, turn, item.factionID, item.sequence,
				item.start.StelliumID, nullableID(item.start.SystemID), nullableID(item.start.PlanetID), nullableRing(item.start),
				item.final.StelliumID, nullableID(item.final.SystemID), nullableID(item.final.PlanetID), nullableRing(item.final)},
		}); err != nil {
			return fmt.Errorf("record where %s order faction %d sequence %d went: %w",
				item.orderType, item.factionID, item.sequence, err)
		}
	}
	if item.survey != nil {
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO order_survey (
				game_id, turn, faction_id, sequence, stellium_id, system_id, planet_id, habitability
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
			Args: []any{gameID, turn, item.factionID, item.sequence,
				item.survey.StelliumID, item.survey.SystemID, item.survey.PlanetID, item.survey.Habitability},
		}); err != nil {
			return fmt.Errorf("record what %s order faction %d sequence %d read: %w",
				item.orderType, item.factionID, item.sequence, err)
		}
	}
	return nil
}

func nullableText(stmt *sqlite.Stmt, column int) string {
	if stmt.ColumnIsNull(column) {
		return ""
	}
	return stmt.ColumnText(column)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableRing(at world.Location) any {
	if at.SystemID == 0 {
		return nil
	}
	return at.Ring
}

func nullableMessage(message string) any {
	if message == "" {
		return nil
	}
	return message
}

// actorAttr names the entity an order acted on. Moves and jumps are always
// ship orders; a probe may be issued by a colony; an order that acts on no
// entity says nothing.
func actorAttr(item outcome) slog.Attr {
	switch {
	case item.actorID == 0:
		return slog.Attr{}
	case item.orderType == "move" || item.orderType == "jump":
		return slog.Int64("ship_id", item.actorID)
	default:
		return slog.Int64("entity_id", item.actorID)
	}
}

// locationAttr says where an order happened. An order with no actor happened
// nowhere in particular and reports no location.
func locationAttr(name string, at world.Location) slog.Attr {
	if at.StelliumID == 0 {
		return slog.Attr{}
	}
	values := []any{"stellium_id", at.StelliumID}
	if at.SystemID != 0 {
		values = append(values, "system_id", at.SystemID, "planet_id", at.PlanetID, "ring", at.Ring)
	}
	return slog.Group(name, values...)
}
