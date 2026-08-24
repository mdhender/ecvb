// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package engine resolves game turns.
package engine

import (
	"context"
	"fmt"
	"log/slog"

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

type location struct {
	stelliumID int64
	systemID   int64
	planetID   int64
	ring       int
}

type entity struct {
	unit      string
	factionID int64
	location  location
}

type outcome struct {
	orderType string
	factionID int64
	sequence  int
	line      int
	shipID    int64
	request   string
	status    string
	message   string
	start     location
	final     location
}

type moveOrder struct {
	factionID             int64
	sequence              int
	line                  int
	shipID                int64
	requestedSystem       string
	requestedOrbit        int
	destinationStelliumID int64
	destinationSystemID   int64
	destinationPlanetID   int64
}

type jumpOrder struct {
	factionID             int64
	sequence              int
	line                  int
	shipID                int64
	x                     int
	y                     int
	z                     int
	destinationStelliumID int64
}

// Resolve resolves an open turn. All MOVE orders resolve before any JUMP
// order. Expected game-rule failures are recorded on the order and do not stop
// the turn; database and state errors roll back the entire turn.
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
			slog.Int64("ship_id", item.shipID),
			slog.String("request", item.request),
			slog.String("status", item.status),
			locationAttr("start", item.start),
			locationAttr("final", item.final),
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

	gameID, err := requireGameTurn(conn, gameCode, turn, "open")
	if err != nil {
		return Result{}, nil, err
	}
	entities, err := loadEntities(conn, gameID)
	if err != nil {
		return Result{}, nil, err
	}
	moves, err := loadMoveOrders(conn, gameID, turn)
	if err != nil {
		return Result{}, nil, err
	}
	jumps, err := loadJumpOrders(conn, gameID, turn)
	if err != nil {
		return Result{}, nil, err
	}

	result = Result{GameCode: gameCode, Turn: turn, Orders: len(moves) + len(jumps)}
	for _, order := range moves {
		item, err := executeMove(conn, gameID, turn, entities, order)
		if err != nil {
			return Result{}, nil, err
		}
		outcomes = append(outcomes, item)
		countOutcome(&result, item)
	}
	for _, order := range jumps {
		item, err := executeJump(conn, gameID, turn, entities, order)
		if err != nil {
			return Result{}, nil, err
		}
		outcomes = append(outcomes, item)
		countOutcome(&result, item)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET turn_state = 'resolved'
		WHERE id = ? AND turn = ? AND turn_state = 'open';`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
	}); err != nil {
		return Result{}, nil, fmt.Errorf("mark game %q turn %d resolved: %w", gameCode, turn, err)
	}
	if conn.Changes() != 1 {
		return Result{}, nil, fmt.Errorf("game %q turn %d changed while it was resolving", gameCode, turn)
	}
	return result, outcomes, nil
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

	gameID, err := requireGameTurn(conn, gameCode, resolvedTurn, "resolved")
	if err != nil {
		return OpenResult{}, err
	}
	for _, table := range []string{"move_order", "jump_order"} {
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

func requireGameTurn(conn *sqlite.Conn, code string, turn int, state string) (int64, error) {
	var gameID int64
	var currentTurn int
	var currentState string
	found := false
	if err := sqlitex.ExecuteTransient(conn, "SELECT id, turn, turn_state FROM game WHERE code = ?;", &sqlitex.ExecOptions{
		Args: []any{code},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			gameID, currentTurn, currentState = stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnText(2)
			found = true
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find game %q: %w", code, err)
	}
	if !found {
		return 0, fmt.Errorf("game %q does not exist", code)
	}
	if currentTurn != turn {
		return 0, fmt.Errorf("game %q is on turn %d, not turn %d", code, currentTurn, turn)
	}
	if currentState != state {
		return 0, fmt.Errorf("game %q turn %d is %s, not %s", code, turn, currentState, state)
	}
	return gameID, nil
}

func loadEntities(conn *sqlite.Conn, gameID int64) (map[int64]*entity, error) {
	entities := make(map[int64]*entity)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT e.id, e.unit, e.faction_id, e.stellium_id, e.system_id, e.planet_id, e.planet_ring
		FROM entity AS e
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			entities[stmt.ColumnInt64(0)] = &entity{
				unit: stmt.ColumnText(1), factionID: stmt.ColumnInt64(2),
				location: readLocation(stmt, 3),
			}
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}
	return entities, nil
}

func loadMoveOrders(conn *sqlite.Conn, gameID int64, turn int) ([]moveOrder, error) {
	var orders []moveOrder
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, ship_id, requested_system, requested_orbit,
			destination_stellium_id, destination_system_id, destination_planet_id, status
		FROM move_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if status := stmt.ColumnText(9); status != "pending" {
				return fmt.Errorf("move order for faction %d sequence %d is already %s", stmt.ColumnInt64(0), stmt.ColumnInt(1), status)
			}
			orders = append(orders, moveOrder{
				factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2), shipID: stmt.ColumnInt64(3),
				requestedSystem: nullableText(stmt, 4), requestedOrbit: stmt.ColumnInt(5),
				destinationStelliumID: stmt.ColumnInt64(6), destinationSystemID: stmt.ColumnInt64(7), destinationPlanetID: stmt.ColumnInt64(8),
			})
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load move orders: %w", err)
	}
	return orders, nil
}

func loadJumpOrders(conn *sqlite.Conn, gameID int64, turn int) ([]jumpOrder, error) {
	var orders []jumpOrder
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id, status
		FROM jump_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if status := stmt.ColumnText(8); status != "pending" {
				return fmt.Errorf("jump order for faction %d sequence %d is already %s", stmt.ColumnInt64(0), stmt.ColumnInt(1), status)
			}
			orders = append(orders, jumpOrder{
				factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2), shipID: stmt.ColumnInt64(3),
				x: stmt.ColumnInt(4), y: stmt.ColumnInt(5), z: stmt.ColumnInt(6), destinationStelliumID: stmt.ColumnInt64(7),
			})
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load jump orders: %w", err)
	}
	return orders, nil
}

func executeMove(conn *sqlite.Conn, gameID int64, turn int, entities map[int64]*entity, order moveOrder) (outcome, error) {
	item := outcome{
		orderType: "move", factionID: order.factionID, sequence: order.sequence,
		line: order.line, shipID: order.shipID, request: fmt.Sprintf("system %s orbit %d", displaySystem(order.requestedSystem), order.requestedOrbit),
		status: "succeeded",
	}
	ship := entities[order.shipID]
	if ship == nil {
		return outcome{}, fmt.Errorf("move order faction %d sequence %d references missing ship %d", order.factionID, order.sequence, order.shipID)
	}
	item.start = ship.location
	item.final = location{stelliumID: order.destinationStelliumID, systemID: order.destinationSystemID, planetID: order.destinationPlanetID, ring: 99}
	if ship.factionID != order.factionID {
		item.status, item.message, item.final = "failed", fmt.Sprintf("ship %d does not belong to faction %d", order.shipID, order.factionID), item.start
	} else if ship.unit != "SHIP" {
		item.status, item.message, item.final = "failed", fmt.Sprintf("entity %d is a %s, not a ship", order.shipID, ship.unit), item.start
	} else if ship.location.stelliumID != order.destinationStelliumID {
		item.status, item.message, item.final = "failed", "destination system is not in the ship's current stellium", item.start
	}
	if item.status == "succeeded" {
		ship.location = item.final
		if err := updateEntityLocation(conn, order.shipID, item.final); err != nil {
			return outcome{}, err
		}
	}
	if err := updateMoveOutcome(conn, gameID, turn, order, item); err != nil {
		return outcome{}, err
	}
	return item, nil
}

func executeJump(conn *sqlite.Conn, gameID int64, turn int, entities map[int64]*entity, order jumpOrder) (outcome, error) {
	item := outcome{
		orderType: "jump", factionID: order.factionID, sequence: order.sequence,
		line: order.line, shipID: order.shipID, request: fmt.Sprintf("(%d,%d,%d)", order.x, order.y, order.z),
		status: "succeeded",
	}
	ship := entities[order.shipID]
	if ship == nil {
		return outcome{}, fmt.Errorf("jump order faction %d sequence %d references missing ship %d", order.factionID, order.sequence, order.shipID)
	}
	item.start = ship.location
	item.final = location{stelliumID: order.destinationStelliumID}
	if ship.factionID != order.factionID {
		item.status, item.message, item.final = "failed", fmt.Sprintf("ship %d does not belong to faction %d", order.shipID, order.factionID), item.start
	} else if ship.unit != "SHIP" {
		item.status, item.message, item.final = "failed", fmt.Sprintf("entity %d is a %s, not a ship", order.shipID, ship.unit), item.start
	}
	if item.status == "succeeded" {
		ship.location = item.final
		if err := updateEntityLocation(conn, order.shipID, item.final); err != nil {
			return outcome{}, err
		}
	}
	if err := updateJumpOutcome(conn, gameID, turn, order, item); err != nil {
		return outcome{}, err
	}
	return item, nil
}

func updateEntityLocation(conn *sqlite.Conn, entityID int64, loc location) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE entity SET stellium_id = ?, system_id = ?, planet_id = ?, planet_ring = ? WHERE id = ?;`, &sqlitex.ExecOptions{
		Args: []any{loc.stelliumID, nullableID(loc.systemID), nullableID(loc.planetID), nullableRing(loc), entityID},
	}); err != nil {
		return fmt.Errorf("update entity %d location: %w", entityID, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("update entity %d location: entity does not exist", entityID)
	}
	return nil
}

func updateMoveOutcome(conn *sqlite.Conn, gameID int64, turn int, order moveOrder, item outcome) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE move_order SET status = ?, error_message = ?,
			start_stellium_id = ?, start_system_id = ?, start_planet_id = ?, start_planet_ring = ?,
			final_stellium_id = ?, final_system_id = ?, final_planet_id = ?, final_planet_ring = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ? AND status = 'pending';`, &sqlitex.ExecOptions{
		Args: append(outcomeArgs(item), gameID, turn, order.factionID, order.sequence),
	}); err != nil {
		return fmt.Errorf("record move order faction %d sequence %d outcome: %w", order.factionID, order.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("record move order faction %d sequence %d outcome: order changed", order.factionID, order.sequence)
	}
	return nil
}

func updateJumpOutcome(conn *sqlite.Conn, gameID int64, turn int, order jumpOrder, item outcome) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE jump_order SET status = ?, error_message = ?,
			start_stellium_id = ?, start_system_id = ?, start_planet_id = ?, start_planet_ring = ?,
			final_stellium_id = ?, final_system_id = ?, final_planet_id = ?, final_planet_ring = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ? AND status = 'pending';`, &sqlitex.ExecOptions{
		Args: append(outcomeArgs(item), gameID, turn, order.factionID, order.sequence),
	}); err != nil {
		return fmt.Errorf("record jump order faction %d sequence %d outcome: %w", order.factionID, order.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("record jump order faction %d sequence %d outcome: order changed", order.factionID, order.sequence)
	}
	return nil
}

func outcomeArgs(item outcome) []any {
	return []any{
		item.status, nullableMessage(item.message),
		item.start.stelliumID, nullableID(item.start.systemID), nullableID(item.start.planetID), nullableRing(item.start),
		item.final.stelliumID, nullableID(item.final.systemID), nullableID(item.final.planetID), nullableRing(item.final),
	}
}

func readLocation(stmt *sqlite.Stmt, column int) location {
	loc := location{stelliumID: stmt.ColumnInt64(column)}
	if !stmt.ColumnIsNull(column + 1) {
		loc.systemID = stmt.ColumnInt64(column + 1)
		loc.planetID = stmt.ColumnInt64(column + 2)
		loc.ring = stmt.ColumnInt(column + 3)
	}
	return loc
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

func nullableRing(loc location) any {
	if loc.systemID == 0 {
		return nil
	}
	return loc.ring
}

func nullableMessage(message string) any {
	if message == "" {
		return nil
	}
	return message
}

func displaySystem(system string) string {
	if system == "" {
		return "current"
	}
	return system
}

func countOutcome(result *Result, item outcome) {
	if item.status == "succeeded" {
		result.Succeeded++
	} else {
		result.Failed++
	}
}

func locationAttr(name string, loc location) slog.Attr {
	values := []any{"stellium_id", loc.stelliumID}
	if loc.systemID != 0 {
		values = append(values, "system_id", loc.systemID, "planet_id", loc.planetID, "ring", loc.ring)
	}
	return slog.Group(name, values...)
}
