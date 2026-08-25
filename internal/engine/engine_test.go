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
		INSERT INTO system (id, stellium_id, sequence) VALUES (20, 10, 'A'), (21, 11, 'A'), (23, 10, 'B');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES
			(30, 20, 4, 'rocky', 10), (31, 20, 6, 'rocky', 10), (32, 21, 4, 'rocky', 10);
		INSERT INTO entity (
			id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume
		) VALUES
			(40, 'SHIP', 1, 10, 20, 30, 64, 1, 100),
			(41, 'COPN', 1, 10, 20, 30, 0, 1, 100);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(40, 'component', 'HDRV', 4, 1), (40, 'component', 'SNSR', 2, 1),
			(41, 'component', 'SNSR', 1, 1);
		INSERT INTO deposit (id, planet_id, sequence, resource, quality, initial_qty, current_qty)
			VALUES (50, 30, 1, 'gold', 40, 9000, 8000);
		UPDATE entity SET mass = 1234 WHERE id = 41;
	`, nil); err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestResolveFailsJumpsBeyondDriveRangeAndCapacity(t *testing.T) {
	// Stellium 11 is (1,2,3) from stellium 10, a distance of 4.
	for _, tc := range []struct {
		name    string
		setup   string
		message string
	}{
		{
			name:    "out of range",
			setup:   `UPDATE inventory SET tech_level = 3 WHERE entity_id = 40 AND unit = 'HDRV';`,
			message: "jump of 4 units exceeds ship 40 jump range of 3 units",
		},
		{
			name:    "too massive",
			setup:   `UPDATE entity SET mass = 4181 WHERE id = 40;`,
			message: "ship 40 masses 4181 MU and its jump drive propels 4180 MU",
		},
		{
			name:    "no drive",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'HDRV';`,
			message: "ship 40 has no assembled HDRV and cannot jump",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openEngineTestDatabase(t)
			if err := sqlitex.ExecuteScript(conn, tc.setup+`
				INSERT INTO jump_order (
					game_id, turn, faction_id, sequence, source_line, ship_id,
					destination_x, destination_y, destination_z, destination_stellium_id
				) VALUES (1, 3, 1, 1, 4, 40, 1, 2, 3, 11);
			`, nil); err != nil {
				t.Fatal(err)
			}
			result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
			if err != nil {
				t.Fatal(err)
			}
			if result.Failed != 1 || result.Succeeded != 0 {
				t.Fatalf("result = %+v; want the jump to fail", result)
			}
			var status, message string
			var stelliumID int64
			if err := sqlitex.ExecuteTransient(conn, `
				SELECT j.status, j.error_message, e.stellium_id
				FROM jump_order AS j JOIN entity AS e ON e.id = j.ship_id
				WHERE j.ship_id = 40;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
				status, message, stelliumID = stmt.ColumnText(0), stmt.ColumnText(1), stmt.ColumnInt64(2)
				return nil
			}}); err != nil {
				t.Fatal(err)
			}
			if status != "failed" || message != tc.message {
				t.Errorf("outcome = (%s, %q); want (failed, %q)", status, message, tc.message)
			}
			if stelliumID != 10 {
				t.Errorf("ship stellium = %d; want it to stay at 10", stelliumID)
			}
		})
	}
}

func TestResolveAllowsJumpExactlyAtDriveLimits(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Range 4 reaches the distance of 4, and the drive propels exactly the mass.
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE entity SET mass = 4180 WHERE id = 40;
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id
		) VALUES (1, 3, 1, 1, 4, 40, 1, 2, 3, 11);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v; want the jump to succeed", result)
	}
}

func TestResolveProbesBeforeMovesAndRecordsFindings(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// The probe reads planet 30, the ship's starting planet, before the jump
	// carries the ship out of the system, so the finding has to survive the
	// ship leaving.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO probe_order (game_id, turn, faction_id, sequence, source_line, entity_id, requested_orbit)
			VALUES (1, 3, 1, 1, 4, 40, 4);
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id
		) VALUES (1, 3, 1, 2, 5, 40, 1, 2, 3, 11);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 2 || result.Succeeded != 2 {
		t.Fatalf("result = %+v; want both orders to succeed", result)
	}

	var status string
	var planetID int64
	var habitability int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT status, planet_id, habitability FROM probe_order WHERE sequence = 1;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			status, planetID, habitability = stmt.ColumnText(0), stmt.ColumnInt64(1), stmt.ColumnInt(2)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || planetID != 30 || habitability != 10 {
		t.Errorf("probe = (%s, planet %d, habitability %d); want (succeeded, 30, 10)", status, planetID, habitability)
	}

	var contacts, deposits int
	var contactMass int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT (SELECT count(*) FROM probe_contact WHERE turn = 3 AND faction_id = 1),
			(SELECT count(*) FROM probe_deposit WHERE turn = 3 AND faction_id = 1),
			(SELECT mass FROM probe_contact WHERE entity_id = 41);`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			contacts, deposits, contactMass = stmt.ColumnInt(0), stmt.ColumnInt(1), stmt.ColumnInt64(2)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if contacts != 2 || deposits != 1 || contactMass != 1234 {
		t.Errorf("findings = (%d contacts, %d deposits, colony mass %d); want (2, 1, 1234)", contacts, deposits, contactMass)
	}
}

func TestResolveReadsPassiveSensorsBeforeAnythingMoves(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Ship 40 starts at planet 30 in system 20 of stellium 10 and jumps to
	// stellium 11. Its sensors fire first, so this turn's survey is of
	// stellium 10; stellium 11 is only surveyed next turn.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id
		) VALUES (1, 3, 1, 1, 4, 40, 1, 2, 3, 11);
	`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3); err != nil {
		t.Fatal(err)
	}

	var surveyed, systemID, endedAt int64
	var systems int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT s.stellium_id, s.system_id, s.systems, e.stellium_id
		FROM sensor_survey AS s JOIN entity AS e ON e.id = s.entity_id
		WHERE s.turn = 3 AND s.entity_id = 40;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			surveyed, systemID, systems, endedAt = stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnInt(2), stmt.ColumnInt64(3)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if surveyed != 10 || systemID != 20 {
		t.Errorf("survey = stellium %d system %d; want the starting location 10/20", surveyed, systemID)
	}
	if systems != 2 {
		t.Errorf("systems = %d; want the 2 systems of stellium 10", systems)
	}
	if endedAt != 11 {
		t.Errorf("ship ended at stellium %d; want it to have jumped to 11", endedAt)
	}

	// The contacts are the ships and orbital colonies of the starting system.
	var contacts int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT count(*) FROM sensor_contact WHERE turn = 3 AND entity_id = 40;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			contacts = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Ship 40 sees itself; colony 41 is a COPN on the surface and is not a contact.
	if contacts != 1 {
		t.Errorf("contacts = %d; want 1", contacts)
	}
}

func TestResolveFailsProbesTheSensorsCannotSupport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   string
		orbits  []int
		message string
	}{
		{
			name:    "over budget",
			orbits:  []int{4, 6, 4},
			message: "entity 40 has only 2 probes this turn",
		},
		{
			name:    "empty orbit",
			orbits:  []int{5},
			message: "system current has no planet in orbit 5",
		},
		{
			name:    "no sensors",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'SNSR';`,
			orbits:  []int{4},
			message: "entity 40 has no assembled SNSR and cannot probe",
		},
		{
			name:    "no system",
			setup:   `UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`,
			orbits:  []int{4},
			message: "entity 40 is orbiting the stellium; name a system to probe",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openEngineTestDatabase(t)
			if tc.setup != "" {
				if err := sqlitex.ExecuteScript(conn, tc.setup, nil); err != nil {
					t.Fatal(err)
				}
			}
			for i, orbit := range tc.orbits {
				if err := sqlitex.ExecuteTransient(conn, `
					INSERT INTO probe_order (game_id, turn, faction_id, sequence, source_line, entity_id, requested_orbit)
					VALUES (1, 3, 1, ?, 4, 40, ?);`, &sqlitex.ExecOptions{Args: []any{i + 1, orbit}}); err != nil {
					t.Fatal(err)
				}
			}
			result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
			if err != nil {
				t.Fatal(err)
			}
			if result.Failed != 1 {
				t.Fatalf("result = %+v; want exactly one failed probe", result)
			}
			var message string
			if err := sqlitex.ExecuteTransient(conn, `
				SELECT error_message FROM probe_order WHERE status = 'failed';`, &sqlitex.ExecOptions{
				ResultFunc: func(stmt *sqlite.Stmt) error {
					message = stmt.ColumnText(0)
					return nil
				},
			}); err != nil {
				t.Fatal(err)
			}
			if message != tc.message {
				t.Errorf("message = %q; want %q", message, tc.message)
			}
		})
	}
}

func TestResolveProbesANamedSystemFromStelliumOrbit(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// A ship orbiting the stellium has no current system. Naming system A of
	// stellium 10 still reads planet 30 in orbit 4.
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;
		INSERT INTO probe_order (game_id, turn, faction_id, sequence, source_line, entity_id, requested_system, requested_orbit)
			VALUES (1, 3, 1, 1, 4, 40, 'A', 4), (1, 3, 1, 2, 5, 40, 'C', 4);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("result = %+v; want system A to succeed and system C to fail", result)
	}
	var planetID, systemID int64
	var message string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT (SELECT planet_id FROM probe_order WHERE sequence = 1),
			(SELECT system_id FROM probe_order WHERE sequence = 1),
			(SELECT error_message FROM probe_order WHERE sequence = 2);`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planetID, systemID, message = stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnText(2)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if planetID != 30 || systemID != 20 {
		t.Errorf("probe read planet %d of system %d; want planet 30 of system 20", planetID, systemID)
	}
	if want := "current stellium has no system C"; message != want {
		t.Errorf("message = %q; want %q", message, want)
	}
}

func TestResolveProbesFromAColony(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Colony 41 is a COPN on planet 30 with one SNSR-1, so it launches one
	// probe. A colony never leaves its planet, so it always has a system.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO probe_order (game_id, turn, faction_id, sequence, source_line, entity_id, requested_orbit)
			VALUES (1, 3, 1, 1, 4, 41, 6), (1, 3, 1, 2, 5, 41, 4);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("result = %+v; want the first probe to succeed and the second to exhaust the array", result)
	}
	var planetID int64
	var message string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT (SELECT planet_id FROM probe_order WHERE sequence = 1),
			(SELECT error_message FROM probe_order WHERE sequence = 2);`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planetID, message = stmt.ColumnInt64(0), stmt.ColumnText(1)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if planetID != 31 {
		t.Errorf("probe read planet %d; want planet 31 in orbit 6", planetID)
	}
	if want := "entity 41 has only 1 probes this turn"; message != want {
		t.Errorf("message = %q; want %q", message, want)
	}
}
