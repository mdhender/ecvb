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
	// request is how the order is echoed back in the engine log. It is built
	// from the stored row rather than from the bound order, so an order that
	// fails to bind still says what it asked for.
	request string
	params  orders.Params
}

type outcome struct {
	orderType string
	factionID int64
	sequence  int
	line      int
	actorID   int64
	request   string
	status    string
	message   string
	start     world.Location
	final     world.Location
	// fuelSpent is the FUEL the drive burned. An order that failed burned
	// none. Probes do not burn fuel.
	fuelSpent int64
	// result is what the order recorded beyond its status. Only a probe has
	// one today.
	result any
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
			slog.String("request", item.request),
			slog.String("status", item.status),
			locationAttr("start", item.start),
			locationAttr("final", item.final),
		}
		if item.orderType != "probe" {
			attrs = append(attrs, slog.Int64("fuel_spent", item.fuelSpent))
		}
		if item.message != "" {
			attrs = append(attrs, slog.String("error", item.message))
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
	actorID := order.params.Actor()
	actor := loaded.Entity(actorID)
	if actor == nil {
		return outcome{}, fmt.Errorf("%s order faction %d sequence %d references missing entity %d",
			order.verb, order.factionID, order.sequence, actorID)
	}
	item := outcome{
		orderType: order.verb, factionID: order.factionID, sequence: order.sequence,
		line: order.line, actorID: actorID, request: order.request, status: orders.StatusSucceeded,
	}
	bounds, err := order.params.Bind(&orders.Binder{World: loaded, FactionID: order.factionID})
	switch {
	case err != nil:
		item.status, item.message = orders.StatusFailed, orders.Problem(err)
		item.start, item.final = actor.Location, actor.Location
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
		item.fuelSpent, item.result = applied.FuelSpent, applied.Result
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
	for _, table := range []string{"move_order", "jump_order", "probe_order", "probe_contact", "probe_deposit",
		"sensor_survey", "sensor_contact"} {
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
// resolve in. Step 4 of the order-pipeline rework replaces the three tables
// and these three queries with one.
func loadOrders(conn *sqlite.Conn, gameID int64, turn int) (map[*orders.Phase][]storedOrder, int, error) {
	byPhase := make(map[*orders.Phase][]storedOrder)
	count := 0
	add := func(order storedOrder) {
		phase := orders.PhaseOf(order.verb)
		byPhase[phase] = append(byPhase[phase], order)
		count++
	}
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, entity_id, requested_system, requested_orbit, status
		FROM probe_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if err := requirePending(stmt, "probe", 6); err != nil {
				return err
			}
			system, orbit := nullableText(stmt, 4), stmt.ColumnInt(5)
			// The stored order carries no "ship" or "colony": which word the
			// player wrote was settled when they wrote it, and a probe is the
			// one order either may be given.
			add(storedOrder{
				verb: "probe", factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2),
				request: probeRequest(system, orbit),
				params: orders.ProbeParams{
					EntityID: stmt.ColumnInt64(3), System: system, Orbits: []int{orbit},
				},
			})
			return nil
		},
	}); err != nil {
		return nil, 0, fmt.Errorf("load probe orders: %w", err)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, ship_id, requested_system, requested_orbit, status
		FROM move_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if err := requirePending(stmt, "move", 6); err != nil {
				return err
			}
			system, orbit := nullableText(stmt, 4), stmt.ColumnInt(5)
			add(storedOrder{
				verb: "move", factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2),
				request: fmt.Sprintf("system %s orbit %d", displaySystem(system), orbit),
				params:  orders.MoveParams{ShipID: stmt.ColumnInt64(3), System: system, Orbit: orbit},
			})
			return nil
		},
	}); err != nil {
		return nil, 0, fmt.Errorf("load move orders: %w", err)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, status
		FROM jump_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if err := requirePending(stmt, "jump", 7); err != nil {
				return err
			}
			x, y, z := stmt.ColumnInt(4), stmt.ColumnInt(5), stmt.ColumnInt(6)
			add(storedOrder{
				verb: "jump", factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2),
				request: fmt.Sprintf("(%d,%d,%d)", x, y, z),
				params:  orders.JumpParams{ShipID: stmt.ColumnInt64(3), X: x, Y: y, Z: z},
			})
			return nil
		},
	}); err != nil {
		return nil, 0, fmt.Errorf("load jump orders: %w", err)
	}
	return byPhase, count, nil
}

// requirePending refuses an order that has already been resolved, so a turn
// cannot be resolved twice.
func requirePending(stmt *sqlite.Stmt, kind string, column int) error {
	if status := stmt.ColumnText(column); status != "pending" {
		return fmt.Errorf("%s order for faction %d sequence %d is already %s",
			kind, stmt.ColumnInt64(0), stmt.ColumnInt(1), status)
	}
	return nil
}

// updateOutcome records what an order did on the row it was submitted as.
// Step 4 replaces the three tables with one and these three statements with
// one.
func updateOutcome(conn *sqlite.Conn, gameID int64, turn int, item outcome) error {
	if item.orderType == "probe" {
		return updateProbeOutcome(conn, gameID, turn, item)
	}
	table := "move_order"
	if item.orderType == "jump" {
		table = "jump_order"
	}
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE `+table+` SET status = ?, error_message = ?,
			start_stellium_id = ?, start_system_id = ?, start_planet_id = ?, start_planet_ring = ?,
			final_stellium_id = ?, final_system_id = ?, final_planet_id = ?, final_planet_ring = ?,
			fuel_spent = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ? AND status = 'pending';`,
		&sqlitex.ExecOptions{Args: []any{
			item.status, nullableMessage(item.message),
			item.start.StelliumID, nullableID(item.start.SystemID), nullableID(item.start.PlanetID), nullableRing(item.start),
			item.final.StelliumID, nullableID(item.final.SystemID), nullableID(item.final.PlanetID), nullableRing(item.final),
			item.fuelSpent, gameID, turn, item.factionID, item.sequence,
		}}); err != nil {
		return fmt.Errorf("record %s order faction %d sequence %d outcome: %w",
			item.orderType, item.factionID, item.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("record %s order faction %d sequence %d outcome: order changed",
			item.orderType, item.factionID, item.sequence)
	}
	return nil
}

func updateProbeOutcome(conn *sqlite.Conn, gameID int64, turn int, item outcome) error {
	args := []any{item.status, nullableMessage(item.message)}
	if read, ok := item.result.(orders.ProbeResult); ok {
		args = append(args, read.StelliumID, read.SystemID, read.PlanetID, read.Habitability)
	} else {
		args = append(args, nil, nil, nil, nil)
	}
	args = append(args, gameID, turn, item.factionID, item.sequence)
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE probe_order
		SET status = ?, error_message = ?, stellium_id = ?, system_id = ?, planet_id = ?, habitability = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ?;`,
		&sqlitex.ExecOptions{Args: args}); err != nil {
		return fmt.Errorf("update probe order faction %d sequence %d: %w", item.factionID, item.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("probe order faction %d sequence %d changed while it was resolving",
			item.factionID, item.sequence)
	}
	return nil
}

func probeRequest(system string, orbit int) string {
	if system == "" {
		return fmt.Sprintf("orbit %d", orbit)
	}
	return fmt.Sprintf("system %s orbit %d", system, orbit)
}

func displaySystem(system string) string {
	if system == "" {
		return "current"
	}
	return system
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
// ship orders; a probe may be issued by a colony.
func actorAttr(item outcome) slog.Attr {
	if item.orderType == "probe" {
		return slog.Int64("entity_id", item.actorID)
	}
	return slog.Int64("ship_id", item.actorID)
}

func locationAttr(name string, at world.Location) slog.Attr {
	values := []any{"stellium_id", at.StelliumID}
	if at.SystemID != 0 {
		values = append(values, "system_id", at.SystemID, "planet_id", at.PlanetID, "ring", at.Ring)
	}
	return slog.Group(name, values...)
}
