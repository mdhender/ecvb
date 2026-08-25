// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"

	"github.com/mdhender/ecvb/internal/report"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func ordersReport(ctx context.Context, directory, gameCode, email string, factionID int64, turn int) (rpt *report.Report, err error) {
	conn, err := openDatabase(ctx, directory)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	faction, err := findReportFaction(conn, gameCode, email, factionID)
	if err != nil {
		return nil, err
	}
	if turn == -1 {
		turn = faction.turn
	}

	rpt = report.New("ORDERS REPORT")
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

func addOrders(rpt *report.Report, conn *sqlite.Conn, gameCode string, turn int, factionID int64, heading string) error {
	table := rpt.Table(heading, "SEQUENCE", "LINE", "ENTITY", "VERB", "INPUT", "FUEL", "STATUS", "START", "FINAL", "ERROR")
	// Fuel is the one number that prices a move or a jump: what the order
	// would burn while it is pending, and what it did burn once it resolves.
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT sequence, source_line, ship_id, verb, input, fuel_spent, status, error_message,
			start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
			final_stellium_id, final_system_id, final_planet_id, final_planet_ring
		FROM (
			SELECT sequence, source_line, ship_id, 'move' AS verb,
				CASE WHEN requested_system IS NULL
					THEN printf('orbit %d', requested_orbit)
					ELSE printf('system %s orbit %d', requested_system, requested_orbit)
				END AS input,
				fuel_spent,
				status, error_message,
				start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
				final_stellium_id, final_system_id, final_planet_id, final_planet_ring
			FROM move_order
			WHERE game_id = (SELECT id FROM game WHERE code = ?) AND turn = ? AND faction_id = ?
			UNION ALL
			SELECT sequence, source_line, ship_id, 'jump' AS verb,
				printf('(%d,%d,%d)', destination_x, destination_y, destination_z) AS input,
				fuel_spent,
				status, error_message,
				start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
				final_stellium_id, final_system_id, final_planet_id, final_planet_ring
			FROM jump_order
			WHERE game_id = (SELECT id FROM game WHERE code = ?) AND turn = ? AND faction_id = ?
		)
		ORDER BY sequence, verb;`, &sqlitex.ExecOptions{
		Args: []any{gameCode, turn, factionID, gameCode, turn, factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			table.Row(
				stmt.ColumnInt(0), stmt.ColumnInt(1), stmt.ColumnInt64(2), stmt.ColumnText(3),
				stmt.ColumnText(4), stmt.ColumnInt64(5), stmt.ColumnText(6),
				orderLocation(stmt, 8), orderLocation(stmt, 12), nullableText(stmt, 7))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query submitted orders: %w", err)
	}
	return nil
}

// addProbes reports probe orders. A probe does not move its ship, so it has no
// start and final location; it names the planet it read instead.
func addProbes(rpt *report.Report, conn *sqlite.Conn, gameCode string, turn int, factionID int64) error {
	table := rpt.Table("PROBES", "SEQUENCE", "LINE", "ENTITY", "INPUT", "STATUS", "SYSTEM", "PLANET", "HABITABILITY", "ERROR")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT sequence, source_line, entity_id,
			CASE WHEN requested_system IS NULL
				THEN printf('orbit %d', requested_orbit)
				ELSE printf('system %s orbit %d', requested_system, requested_orbit)
			END AS input,
			status, stellium_id, system_id, planet_id, habitability, error_message
		FROM probe_order
		WHERE game_id = (SELECT id FROM game WHERE code = ?) AND turn = ? AND faction_id = ?
		ORDER BY sequence;`, &sqlitex.ExecOptions{
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

func orderLocation(stmt *sqlite.Stmt, column int) string {
	if stmt.ColumnIsNull(column) {
		return "-"
	}
	if stmt.ColumnIsNull(column + 1) {
		return fmt.Sprintf("%d/-/-/-", stmt.ColumnInt64(column))
	}
	return fmt.Sprintf("%d/%d/%d/%d", stmt.ColumnInt64(column), stmt.ColumnInt64(column+1), stmt.ColumnInt64(column+2), stmt.ColumnInt(column+3))
}
