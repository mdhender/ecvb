// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package report

import (
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Orders reports the orders a faction has submitted for a turn.
func Orders(conn *sqlite.Conn, gameCode, email string, factionID int64, turn int) (*Report, error) {
	faction, err := findFaction(conn, gameCode, email, factionID)
	if err != nil {
		return nil, err
	}
	if turn == -1 {
		turn = faction.turn
	}

	rpt := New("ORDERS REPORT")
	rpt.Table("", "GAME", "TURN", "FACTION", "CONTROLLER").
		Row(gameCode, turn, faction.id, faction.controller)
	if err := addOrders(rpt, conn, gameCode, turn, faction.id, "ORDERS"); err != nil {
		return nil, err
	}
	if err := addProbes(rpt, conn, gameCode, turn, faction.id); err != nil {
		return nil, err
	}
	return rpt, nil
}

// addOrders reports every order but the probes, which get a section of their
// own below because what a probe records is what it read rather than where it
// went. Which orders belong in which section is a question about the shape of
// the report now, not about the shape of the schema: they are all one table.
//
// NOTE is what an order that succeeded still wanted to say -- that it did less
// than it was asked for, or that a create put a new entity on the board. It is
// deliberately not the ERROR column: a shortage is a rate rather than a
// failure, so it belongs beside a succeeded status rather than in place of one.
func addOrders(rpt *Report, conn *sqlite.Conn, gameCode string, turn int, factionID int64, heading string) error {
	table := rpt.Table(heading, "SEQUENCE", "LINE", "ENTITY", "VERB", "INPUT", "FUEL", "STATUS", "START", "FINAL", "ERROR", "NOTE")
	// Fuel is the one number that prices a move or a jump: what the order
	// would burn while it is pending, and what it did burn once it resolves.
	// An order that has not resolved has no movement row, and reads as "-".
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT o.sequence, o.source_line, o.actor_entity_id, o.verb, o.input, o.fuel_spent, o.status, o.error_message,
			o.note,
			m.start_stellium_id, m.start_system_id, m.start_planet_id, m.start_planet_ring,
			m.final_stellium_id, m.final_system_id, m.final_planet_id, m.final_planet_ring
		FROM game_order AS o
		LEFT JOIN order_movement AS m
			ON m.game_id = o.game_id AND m.turn = o.turn
			AND m.faction_id = o.faction_id AND m.sequence = o.sequence
		WHERE o.game_id = (SELECT id FROM game WHERE code = ?) AND o.turn = ? AND o.faction_id = ?
			AND o.verb <> 'probe'
		ORDER BY o.sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameCode, turn, factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			table.Row(
				stmt.ColumnInt(0), stmt.ColumnInt(1), actor(stmt, 2), stmt.ColumnText(3),
				stmt.ColumnText(4), stmt.ColumnInt64(5), stmt.ColumnText(6),
				orderLocation(stmt, 9), orderLocation(stmt, 13), nullableText(stmt, 7),
				nullableText(stmt, 8))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query submitted orders: %w", err)
	}
	return nil
}

// addProbes reports probe orders. A probe does not move its ship, so it has no
// start and final location; it names the planet it read instead.
func addProbes(rpt *Report, conn *sqlite.Conn, gameCode string, turn int, factionID int64) error {
	table := rpt.Table("PROBES", "SEQUENCE", "LINE", "ENTITY", "INPUT", "STATUS", "SYSTEM", "PLANET", "HABITABILITY", "ERROR")
	// A probe reads a planet or it does not; there is no partial answer to
	// spend a note on, so the probe section has no NOTE column.
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT o.sequence, o.source_line, o.actor_entity_id, o.input, o.status,
			s.stellium_id, s.system_id, s.planet_id, s.habitability, o.error_message
		FROM game_order AS o
		LEFT JOIN order_survey AS s
			ON s.game_id = o.game_id AND s.turn = o.turn
			AND s.faction_id = o.faction_id AND s.sequence = o.sequence
		WHERE o.game_id = (SELECT id FROM game WHERE code = ?) AND o.turn = ? AND o.faction_id = ?
			AND o.verb = 'probe'
		ORDER BY o.sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameCode, turn, factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			system, planet, habitability := "-", "-", "-"
			if !stmt.ColumnIsNull(6) {
				system = fmt.Sprintf("%d/%d", stmt.ColumnInt64(5), stmt.ColumnInt64(6))
			}
			if !stmt.ColumnIsNull(7) {
				planet = fmt.Sprintf("%d", stmt.ColumnInt64(7))
			}
			if !stmt.ColumnIsNull(8) {
				habitability = fmt.Sprintf("%d", stmt.ColumnInt(8))
			}
			table.Row(
				stmt.ColumnInt(0), stmt.ColumnInt(1), stmt.ColumnInt64(2), stmt.ColumnText(3),
				stmt.ColumnText(4), system, planet, habitability, nullableText(stmt, 9))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query probe orders: %w", err)
	}
	return nil
}

// actor is the entity an order acted on. An order that acted on none -- naming
// a stellium, say -- reads as a dash.
func actor(stmt *sqlite.Stmt, column int) any {
	if stmt.ColumnIsNull(column) {
		return "-"
	}
	return stmt.ColumnInt64(column)
}

func orderLocation(stmt *sqlite.Stmt, column int) string {
	if stmt.ColumnIsNull(column) {
		return "-"
	}
	if stmt.ColumnIsNull(column + 1) {
		return fmt.Sprintf("%d/-/-/-", stmt.ColumnInt64(column))
	}
	return fmt.Sprintf("%d/%d/%d/%d", stmt.ColumnInt64(column), stmt.ColumnInt64(column+1), stmt.ColumnInt64(column+2), stmt.ColumnInt(column+3))
}
