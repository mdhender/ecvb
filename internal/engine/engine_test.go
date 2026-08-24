// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package engine

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestResolveExecutesMovesBeforeJumpsAndRecordsOutcomes(t *testing.T) {
	conn := openEngineTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO move_order (
			game_id, turn, faction_id, sequence, source_line, ship_id, requested_orbit,
			destination_stellium_id, destination_system_id, destination_planet_id
		) VALUES (1, 3, 1, 1, 5, 40, 6, 10, 20, 31);
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id
		) VALUES
			(1, 3, 1, 2, 4, 40, 1, 2, 3, 11),
			(1, 3, 1, 3, 6, 41, 1, 2, 3, 11);
	`, nil); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return attr
	}}))
	result, err := Resolve(context.Background(), logger, conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 3 || result.Succeeded != 2 || result.Failed != 1 {
		t.Fatalf("result = %+v; want 3 orders, 2 succeeded, 1 failed", result)
	}

	var stelliumID int64
	var systemIsNull, planetIsNull, ringIsNull bool
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT stellium_id, system_id IS NULL, planet_id IS NULL, planet_ring IS NULL
		FROM entity WHERE id = 40;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		stelliumID = stmt.ColumnInt64(0)
		systemIsNull = stmt.ColumnInt(1) != 0
		planetIsNull = stmt.ColumnInt(2) != 0
		ringIsNull = stmt.ColumnInt(3) != 0
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if stelliumID != 11 || !systemIsNull || !planetIsNull || !ringIsNull {
		t.Fatalf("ship final location = (%d, system null %t, planet null %t, ring null %t)", stelliumID, systemIsNull, planetIsNull, ringIsNull)
	}

	var jumpStartPlanet int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT start_planet_id FROM jump_order
		WHERE game_id = 1 AND turn = 3 AND faction_id = 1 AND sequence = 2;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		jumpStartPlanet = stmt.ColumnInt64(0)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if jumpStartPlanet != 31 {
		t.Fatalf("jump start planet = %d; want 31 after MOVE phase", jumpStartPlanet)
	}

	var status, message string
	var startPlanet, finalPlanet int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT status, error_message, start_planet_id, final_planet_id
		FROM jump_order WHERE game_id = 1 AND turn = 3 AND faction_id = 1 AND sequence = 3;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		status, message = stmt.ColumnText(0), stmt.ColumnText(1)
		startPlanet, finalPlanet = stmt.ColumnInt64(2), stmt.ColumnInt64(3)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || !strings.Contains(message, "not a ship") || startPlanet != finalPlanet {
		t.Fatalf("failed outcome = %q, %q, %d → %d", status, message, startPlanet, finalPlanet)
	}
	if !strings.Contains(logs.String(), "order_type=move") || !strings.Contains(logs.String(), "status=failed") {
		t.Fatalf("logs do not contain order outcomes:\n%s", logs.String())
	}

	var turnState string
	if err := sqlitex.ExecuteTransient(conn, "SELECT turn_state FROM game WHERE id = 1;", &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		turnState = stmt.ColumnText(0)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if turnState != "resolved" {
		t.Fatalf("turn state = %q; want resolved", turnState)
	}
}

func TestOpenNextTurnRetainsLatestResolvedOrdersAndPurgesOlderOrders(t *testing.T) {
	conn := openEngineTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE game SET turn_state = 'resolved' WHERE id = 1;
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id,
			status, start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
			final_stellium_id, final_system_id, final_planet_id, final_planet_ring
		) VALUES
			(1, 2, 1, 1, 3, 40, 0, 0, 0, 10, 'succeeded', 10, 20, 30, 64, 10, NULL, NULL, NULL),
			(1, 3, 1, 1, 3, 40, 1, 2, 3, 11, 'succeeded', 10, 20, 30, 64, 11, NULL, NULL, NULL);
	`, nil); err != nil {
		t.Fatal(err)
	}

	result, err := OpenNextTurn(context.Background(), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Turn != 4 {
		t.Fatalf("opened turn = %d; want 4", result.Turn)
	}
	var rows, turn int
	var state string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT (SELECT count(*) FROM jump_order WHERE game_id = 1), turn, turn_state
		FROM game WHERE id = 1;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		rows, turn, state = stmt.ColumnInt(0), stmt.ColumnInt(1), stmt.ColumnText(2)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || turn != 4 || state != "open" {
		t.Fatalf("after open: rows=%d turn=%d state=%q; want 1, 4, open", rows, turn, state)
	}
}

func openEngineTestDatabase(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn, err := sqlite.OpenConn(filepath.Join(t.TempDir(), database.Filename), sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		t.Fatal(err)
	}
	for _, migration := range database.Migrations() {
		if err := sqlitex.ExecuteScript(conn, migration, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (id, email, role) VALUES (1, 'player@example.com', 'non-administrator');
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 3);
		INSERT INTO faction (id, game_id, user_id) VALUES (1, 1, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (10, 1, 0, 0, 0), (11, 1, 1, 2, 3);
		INSERT INTO system (id, stellium_id, sequence) VALUES (20, 10, 'A'), (21, 11, 'A');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES
			(30, 20, 4, 'rocky', 10), (31, 20, 6, 'rocky', 10), (32, 21, 4, 'rocky', 10);
		INSERT INTO entity (
			id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume
		) VALUES
			(40, 'SHIP', 1, 10, 20, 30, 64, 1, 100),
			(41, 'COPN', 1, 10, 20, 30, 0, 1, 100);
	`, nil); err != nil {
		t.Fatal(err)
	}
	return conn
}
