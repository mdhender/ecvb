// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package engine

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/testdb"
	"github.com/mdhender/ecvb/internal/world"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestResolveExecutesMovesBeforeJumpsAndRecordsOutcomes(t *testing.T) {
	conn := openEngineTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 5, 'move', 40, 'orbit 11', '{"orbit":11}'),
			(1, 3, 1, 2, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}'),
			(1, 3, 1, 3, 6, 'jump', 41, '(1,2,3)', '{"x":1,"y":2,"z":3}');
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

	// The shortest crossing still costs a turn: the jump departs in the last
	// stage of this turn and lands in the last stage of the next, so the ship
	// is nowhere at all when this one is over.
	if where := locationOf(t, conn, 40); where != "nowhere" {
		t.Fatalf("ship final location = %s; want it crossing to stellium 11", where)
	}
	if to := crossingTo(t, conn, 40); to != 11 {
		t.Fatalf("ship is crossing to stellium %d; want 11", to)
	}
	if due := arrivalTurn(t, conn, 40); due != 4 {
		t.Fatalf("ship is due on turn %d; want 4", due)
	}

	// The ship began the turn at planet 30 and the move took it to the stellium
	// orbit. The jump records that as its start, which is the MOVE phase having
	// gone first -- and a jump from planet 30 would not have bound at all.
	var jumpStartStellium int64
	var jumpStartedAtNoPlanet bool
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT start_stellium_id, start_planet_id IS NULL FROM order_movement
		WHERE game_id = 1 AND turn = 3 AND faction_id = 1 AND sequence = 2;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		jumpStartStellium = stmt.ColumnInt64(0)
		jumpStartedAtNoPlanet = stmt.ColumnInt(1) != 0
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if jumpStartStellium != 10 || !jumpStartedAtNoPlanet {
		t.Fatalf("jump started at stellium %d, planet null %t; want the stellium orbit of 10 after the MOVE phase",
			jumpStartStellium, jumpStartedAtNoPlanet)
	}

	var status, message string
	var startPlanet, finalPlanet int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT o.status, o.error_message, m.start_planet_id, m.final_planet_id
		FROM game_order AS o JOIN order_movement AS m
			ON m.game_id = o.game_id AND m.turn = o.turn
			AND m.faction_id = o.faction_id AND m.sequence = o.sequence
		WHERE o.game_id = 1 AND o.turn = 3 AND o.faction_id = 1 AND o.sequence = 3;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
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
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, status
		) VALUES
			(1, 2, 1, 1, 3, 'jump', 40, '(0,0,0)', '{"x":0,"y":0,"z":0}', 'succeeded'),
			(1, 3, 1, 1, 3, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}', 'succeeded');
		INSERT INTO order_movement (
			game_id, turn, faction_id, sequence,
			start_stellium_id, start_system_id, start_planet_id, start_planet_ring,
			final_stellium_id, final_system_id, final_planet_id, final_planet_ring
		) VALUES
			(1, 2, 1, 1, 10, 20, 30, 64, 10, NULL, NULL, NULL),
			(1, 3, 1, 1, 10, 20, 30, 64, 11, NULL, NULL, NULL);
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
		SELECT (SELECT count(*) FROM game_order WHERE game_id = 1), turn, turn_state
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
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
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
			(40, 'cargo', 'FUEL', 0, 500),
			(41, 'component', 'SNSR', 1, 1);
		INSERT INTO deposit (id, planet_id, sequence, resource, quality, initial_qty, current_qty)
			VALUES (50, 30, 1, 'gold', 40, 9000, 8000);
		UPDATE entity SET mass = 1234 WHERE id = 41;
		-- 500 of the ship's 3000 MU is the fuel it carries, at 1 MU each, so
		-- burning fuel takes mass off a ship that was carrying it.
		UPDATE entity SET mass = 3000 WHERE id = 40;
	`)
	return conn
}

func TestResolveFailsJumpsTheDriveCannotMake(t *testing.T) {
	// Stellium 11 is (1,2,3) from stellium 10, a distance of 4. Ship 40 starts
	// at a planet, so every case puts it in the stellium orbit first: a jump
	// from a planet fails for that reason before the drive is measured.
	const inTheStelliumOrbit = `
		UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`
	for _, tc := range []struct {
		name    string
		setup   string
		message string
	}{
		{
			// Technology level no longer caps the distance. A drive that the
			// old range rule refused makes this jump, and only its fuel grows.
			name:    "at a planet rather than the stellium orbit",
			setup:   `UPDATE entity SET system_id = 20, planet_id = 30, planet_ring = 64 WHERE id = 40;`,
			message: "ship 40 is at a planet and a jump begins from the stellium orbit; move it to orbit 11 first",
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
			if err := sqlitex.ExecuteScript(conn, inTheStelliumOrbit+tc.setup+`
				INSERT INTO game_order (
					game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
				) VALUES (1, 3, 1, 1, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}');
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
				SELECT o.status, o.error_message, e.stellium_id
				FROM game_order AS o JOIN entity AS e ON e.id = o.actor_entity_id
				WHERE o.actor_entity_id = 40 AND o.verb = 'jump';`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
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
	// The drive propels exactly the mass, and the ship is in the stellium
	// orbit, which is the only thing left that a jump has to be.
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE entity SET mass = 4180, system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES (1, 3, 1, 1, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}');
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
	// The probe reads planet 30, the ship's starting planet, before the move
	// and the jump carry the ship out of the system, so the finding has to
	// survive the ship leaving.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 4, 'probe', 40, 'orbit 4', '{"orbits":[4]}'),
			(1, 3, 1, 2, 5, 'move', 40, 'orbit 11', '{"orbit":11}'),
			(1, 3, 1, 3, 6, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}');
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 3 || result.Succeeded != 3 {
		t.Fatalf("result = %+v; want all three orders to succeed", result)
	}

	var status string
	var planetID int64
	var habitability int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT o.status, s.planet_id, s.habitability
		FROM game_order AS o JOIN order_survey AS s
			ON s.game_id = o.game_id AND s.turn = o.turn
			AND s.faction_id = o.faction_id AND s.sequence = o.sequence
		WHERE o.sequence = 1;`, &sqlitex.ExecOptions{
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
	// Ship 40 starts at planet 30 in system 20 of stellium 10, moves out to the
	// stellium orbit and jumps to stellium 11. Its sensors fire first, so this
	// turn's survey is of stellium 10; stellium 11 is only surveyed next turn.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 4, 'move', 40, 'orbit 11', '{"orbit":11}'),
			(1, 3, 1, 2, 5, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}');
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
	if endedAt != 0 {
		t.Errorf("ship ended at stellium %d; want it nowhere, crossing away", endedAt)
	}
	if to := crossingTo(t, conn, 40); to != 11 {
		t.Errorf("ship is crossing to stellium %d; want it to have jumped for 11", to)
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
			message: "ship 40 has only 2 probes this turn",
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
			message: "ship 40 has no assembled SNSR and cannot probe",
		},
		{
			name:    "no system",
			setup:   `UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`,
			orbits:  []int{4},
			message: "ship 40 is orbiting the stellium; name a system to probe",
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
					INSERT INTO game_order (
						game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
					) VALUES (1, 3, 1, ?, 4, 'probe', 40, printf('orbit %d', ?), printf('{"orbits":[%d]}', ?));`,
					&sqlitex.ExecOptions{Args: []any{i + 1, orbit, orbit}}); err != nil {
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
				SELECT error_message FROM game_order WHERE status = 'failed';`, &sqlitex.ExecOptions{
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
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 4, 'probe', 40, 'system A orbit 4', '{"system":"A","orbits":[4]}'),
			(1, 3, 1, 2, 5, 'probe', 40, 'system C orbit 4', '{"system":"C","orbits":[4]}');
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
		SELECT (SELECT planet_id FROM order_survey WHERE sequence = 1),
			(SELECT system_id FROM order_survey WHERE sequence = 1),
			(SELECT error_message FROM game_order WHERE sequence = 2);`, &sqlitex.ExecOptions{
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
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 4, 'probe', 41, 'orbit 6', '{"orbits":[6]}'),
			(1, 3, 1, 2, 5, 'probe', 41, 'orbit 4', '{"orbits":[4]}');
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
		SELECT (SELECT planet_id FROM order_survey WHERE sequence = 1),
			(SELECT error_message FROM game_order WHERE sequence = 2);`, &sqlitex.ExecOptions{
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
	if want := "colony 41 has only 1 probes this turn"; message != want {
		t.Errorf("message = %q; want %q", message, want)
	}
}

func TestResolveMovesAShipToTheStelliumOrbit(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Ships 40 and 43 both start at planet 30 in system A. 40 crosses to planet
	// 31 of the same system; 43 leaves the planets for the stellium orbit. Each
	// gets one move, because that is all a ship gets in a turn.
	//
	// 43 is added here rather than to the fixture because another ship at the
	// home planet is another contact, and the probe and sensor tests count
	// those.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO entity (
			id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
			faction_id, enclosed_volume, mass
		) VALUES (43, 'SHIP', 1, 10, 20, 30, 64, 1, 100, 3000);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(43, 'component', 'HDRV', 4, 1), (43, 'cargo', 'FUEL', 0, 500);
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 4, 'move', 40, 'orbit 6', '{"orbit":6}'),
			(1, 3, 1, 2, 5, 'move', 43, 'orbit 11', '{"orbit":11}');
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v; want both moves to succeed", result)
	}

	var rows []string
	var arrivalRing int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%s|%d|%s', o.sequence, o.status, o.fuel_spent,
			coalesce(m.final_system_id, '-')), m.final_planet_ring
		FROM game_order AS o JOIN order_movement AS m
			ON m.game_id = o.game_id AND m.turn = o.turn
			AND m.faction_id = o.faction_id AND m.sequence = o.sequence
		WHERE o.verb = 'move' ORDER BY o.sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			rows = append(rows, stmt.ColumnText(0))
			if !stmt.ColumnIsNull(1) {
				arrivalRing = stmt.ColumnInt(1)
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"1|succeeded|4|20", "2|succeeded|4|-"}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("move orders = %q; want %q", rows, want)
	}
	// The ring the ship settles into is drawn, not fixed.
	if arrivalRing < world.MinShipRing || arrivalRing > world.MaxShipRing {
		t.Errorf("arrival ring = %d; want it between %d and %d", arrivalRing, world.MinShipRing, world.MaxShipRing)
	}

	var location string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%s|%s|%s', stellium_id, coalesce(system_id, '-'),
			coalesce(planet_id, '-'), coalesce(planet_ring, '-'))
		FROM entity WHERE id = 43;`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		location = stmt.ColumnText(0)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if location != "10|-|-|-" {
		t.Errorf("ship location = %q; want it orbiting stellium 10", location)
	}
}

func TestResolveFailsMovesTheDriveCannotMake(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   string
		message string
	}{
		{
			name:    "too massive",
			setup:   `UPDATE entity SET mass = 4181 WHERE id = 40;`,
			message: "ship 40 masses 4181 MU and its drive propels 4180 MU",
		},
		{
			name:    "no drive",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'HDRV';`,
			message: "ship 40 has no assembled HDRV and cannot move",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openEngineTestDatabase(t)
			if err := sqlitex.ExecuteScript(conn, tc.setup+`
				INSERT INTO game_order (
					game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, fuel_spent
				) VALUES (1, 3, 1, 1, 4, 'move', 40, 'orbit 6', '{"orbit":6}', 4);
			`, nil); err != nil {
				t.Fatal(err)
			}
			result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
			if err != nil {
				t.Fatal(err)
			}
			if result.Failed != 1 || result.Succeeded != 0 {
				t.Fatalf("result = %+v; want the move to fail", result)
			}
			// A move that never happened burns no fuel and leaves the ship at
			// the planet it started from.
			var row string
			if err := sqlitex.ExecuteTransient(conn, `
				SELECT printf('%s|%s|%d|%d', o.status, o.error_message, o.fuel_spent, e.planet_id)
				FROM game_order AS o JOIN entity AS e ON e.id = o.actor_entity_id
				WHERE o.actor_entity_id = 40 AND o.verb = 'move';`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
				row = stmt.ColumnText(0)
				return nil
			}}); err != nil {
				t.Fatal(err)
			}
			if want := "failed|" + tc.message + "|0|30"; row != want {
				t.Errorf("move order = %q; want %q", row, want)
			}
		})
	}
}

func TestResolveBurnsFuelAndFailsAnOrderTheShipCannotPayFor(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Ship 40 has one HDRV-4 and 500 FUEL. The move out to the stellium orbit
	// burns 1 * 0.1 * 40 and the 4-light-year jump burns 1 * 4 * 40, leaving
	// 336. That is a whole turn's travel for one ship: one move and one jump.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, fuel_spent
		) VALUES
			(1, 3, 1, 1, 4, 'move', 40, 'orbit 11', '{"orbit":11}', 4),
			(1, 3, 1, 2, 5, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}', 160);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v; want every order to succeed", result)
	}
	if got := shipFuel(t, conn, 40); got != 336 {
		t.Errorf("fuel left = %d; want 336", got)
	}
	// The ship started at 3000 MU and burned 164 units of fuel at 1 MU each.
	if got := shipMass(t, conn, 40); got != 3000-164 {
		t.Errorf("mass = %d; want %d", got, 3000-164)
	}
}

func TestResolveFailsAMoveTheShipCannotFuel(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// One HDRV-4 burns 4 FUEL crossing a system. Three units is not enough.
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE inventory SET quantity = 3 WHERE entity_id = 40 AND unit = 'FUEL';
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, fuel_spent
		) VALUES (1, 3, 1, 1, 4, 'move', 40, 'orbit 6', '{"orbit":6}', 4);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || result.Succeeded != 0 {
		t.Fatalf("result = %+v; want the move to fail", result)
	}
	// A failed order burns nothing, leaves the fuel alone, and leaves the ship
	// where it started.
	var row string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%s|%s|%d|%d', o.status, o.error_message, o.fuel_spent, e.planet_id)
		FROM game_order AS o JOIN entity AS e ON e.id = o.actor_entity_id
		WHERE o.actor_entity_id = 40 AND o.verb = 'move';`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		row = stmt.ColumnText(0)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if want := "failed|ship 40 needs 4 FUEL to move and holds 3|0|30"; row != want {
		t.Errorf("move order = %q; want %q", row, want)
	}
	if got := shipFuel(t, conn, 40); got != 3 {
		t.Errorf("fuel left = %d; want the 3 units untouched", got)
	}
}

func shipFuel(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	var quantity int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT coalesce(sum(quantity), 0) FROM inventory WHERE entity_id = ? AND unit = 'FUEL';`,
		&sqlitex.ExecOptions{Args: []any{entityID}, ResultFunc: func(stmt *sqlite.Stmt) error {
			quantity = stmt.ColumnInt64(0)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	return quantity
}

func shipMass(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	var value int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT mass FROM entity WHERE id = ?;",
		&sqlitex.ExecOptions{Args: []any{entityID}, ResultFunc: func(stmt *sqlite.Stmt) error {
			value = stmt.ColumnInt64(0)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestResolveChargesAMoveToTheSamePlanetAndRerollsTheRing(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Ship 40 is at planet 30 in ring 64. Ordering it to the orbit it is
	// already in is not free: it breaks orbit and settles again.
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE entity SET planet_ring = 64 WHERE id = 40;
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, fuel_spent
		) VALUES (1, 3, 1, 1, 4, 'move', 40, 'orbit 4', '{"orbit":4}', 4);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("result = %+v; want the move to succeed", result)
	}
	var planetID int64
	var ring int
	if err := sqlitex.ExecuteTransient(conn, "SELECT planet_id, planet_ring FROM entity WHERE id = 40;",
		&sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
			planetID, ring = stmt.ColumnInt64(0), stmt.ColumnInt(1)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	if planetID != 30 {
		t.Errorf("planet = %d; want the ship still at planet 30", planetID)
	}
	if ring == 64 || ring < world.MinShipRing || ring > world.MaxShipRing {
		t.Errorf("ring = %d; want a fresh draw between %d and %d", ring, world.MinShipRing, world.MaxShipRing)
	}
	// The hop cost 4 FUEL, the same as crossing to any other planet of the
	// system, and took its mass with it.
	if got := shipFuel(t, conn, 40); got != 496 {
		t.Errorf("fuel left = %d; want 496", got)
	}
	if got := shipMass(t, conn, 40); got != 3000-4 {
		t.Errorf("mass = %d; want %d", got, 3000-4)
	}
}

func TestResolveLeavesAShipAlreadyInTheStelliumOrbitUntouched(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Ship 40 is already in the stellium orbit and is ordered there again. The
	// move has nowhere to go: no fuel, no change. It is put there rather than
	// moved there, because a ship moves once a turn and this is that move.
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, fuel_spent
		) VALUES (1, 3, 1, 1, 4, 'move', 40, 'orbit 11', '{"orbit":11}', 0);
	`, nil); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve(context.Background(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("result = %+v; want the move to succeed", result)
	}
	var rows []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%s|%d', sequence, status, fuel_spent)
		FROM game_order WHERE actor_entity_id = 40 AND verb = 'move' ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			rows = append(rows, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"1|succeeded|0"}; strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("move orders = %q; want %q", rows, want)
	}
	// A ship with nowhere to go burns nothing getting there.
	if got := shipFuel(t, conn, 40); got != 500 {
		t.Errorf("fuel left = %d; want 500", got)
	}
}

// A crossing longer than the drive covers in a turn spans turns. The jump
// order itself departs and succeeds in the turn it was written: it draws the
// whole fuel bill and takes the ship off the board. What continues is the
// crossing, which is a row of its own, and the arrival step lands the ship on
// the turn it is due.
//
// The turn count is the whole of what a technology level buys. Nothing here
// refuses the distance; a slower drive just spends longer nowhere.
func TestResolveCarriesALongCrossingAcrossSeveralTurns(t *testing.T) {
	conn := openEngineTestDatabase(t)
	// Stellium 12 at (6,6,7) is exactly 11 light years from the origin. Ship
	// 40's drive is an HDRV-4, so the crossing is ceil(11/4) = 3 turns: it
	// departs in the last stage of turn 3 and is due in the last stage of turn
	// 6, which costs the ship the three turns the crossing takes.
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (12, 1, 6, 6, 7);
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES
			(1, 3, 1, 1, 4, 'move', 40, 'orbit 11', '{"orbit":11}'),
			(1, 3, 1, 2, 5, 'jump', 40, '(6,6,7)', '{"x":6,"y":6,"z":7}');
	`, nil); err != nil {
		t.Fatal(err)
	}

	result, err := Resolve(context.Background(), discardLogger(), conn, "TEST", 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v; want both orders to succeed", result)
	}

	// The order records the destination as where it sent the ship, and the
	// whole 11 light years of fuel: 1 HDRV unit at 40 per light year.
	var finalStellium, fuelSpent int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT m.final_stellium_id, o.fuel_spent
		FROM game_order AS o JOIN order_movement AS m
			ON m.game_id = o.game_id AND m.turn = o.turn
			AND m.faction_id = o.faction_id AND m.sequence = o.sequence
		WHERE o.game_id = 1 AND o.turn = 3 AND o.sequence = 2;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			finalStellium, fuelSpent = stmt.ColumnInt64(0), stmt.ColumnInt64(1)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	if finalStellium != 12 || fuelSpent != 440 {
		t.Fatalf("jump recorded final stellium %d and %d FUEL; want 12 and 440", finalStellium, fuelSpent)
	}

	// Three turns of crossing, in which the ship is nowhere at all.
	for turn := 3; turn <= 5; turn++ {
		if where := locationOf(t, conn, 40); where != "nowhere" {
			t.Fatalf("after turn %d the ship is at %s; want it still crossing", turn, where)
		}
		if due := arrivalTurn(t, conn, 40); due != 6 {
			t.Fatalf("after turn %d the ship is due on turn %d; want 6", turn, due)
		}
		openAndResolve(t, conn, turn)
	}

	// The arrival step of turn 6 lands it in the destination's stellium orbit
	// and the crossing is over.
	if where := locationOf(t, conn, 40); where != "12/-/-" {
		t.Fatalf("after turn 6 the ship is at %s; want the stellium orbit of 12", where)
	}
	if crossings := countRows(t, conn, "in_transit"); crossings != 0 {
		t.Fatalf("in_transit holds %d rows after the ship landed; want none", crossings)
	}
}

// A ship in the middle of a crossing is not somewhere an order can reach. The
// order binds against a ship that is nowhere, which fails, and a stored order
// that fails to bind is a failed order rather than a stopped turn.
func TestResolveRefusesOrdersToAShipInTransit(t *testing.T) {
	conn := openEngineTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (12, 1, 6, 6, 7);
		UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES (1, 3, 1, 1, 4, 'jump', 40, '(6,6,7)', '{"x":6,"y":6,"z":7}');
	`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(context.Background(), discardLogger(), conn, "TEST", 3); err != nil {
		t.Fatal(err)
	}
	openAndResolveWithOrders(t, conn, 3, `
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES (1, 4, 1, 1, 4, 'move', 40, 'orbit 4', '{"orbit":4}');`)

	var status, message string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT status, coalesce(error_message, '') FROM game_order
		WHERE game_id = 1 AND turn = 4 AND sequence = 1;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			status, message = stmt.ColumnText(0), stmt.ColumnText(1)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	const want = "ship 40 is in transit; it arrives on turn 6 and can be given orders from turn 7"
	if status != "failed" || message != want {
		t.Fatalf("outcome = %q, %q; want failed with %q", status, message, want)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// locationOf renders where an entity stands, or "nowhere" for a ship crossing
// between stellia.
func locationOf(t *testing.T, conn *sqlite.Conn, entityID int64) string {
	t.Helper()
	where := "nowhere"
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d/%s/%s', stellium_id, coalesce(system_id, '-'), coalesce(planet_id, '-'))
		FROM entity WHERE id = ? AND stellium_id IS NOT NULL;`, &sqlitex.ExecOptions{
		Args: []any{entityID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			where = stmt.ColumnText(0)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	return where
}

// crossingTo is the stellium a ship is bound for, or 0 when it is not crossing.
func crossingTo(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	var destination int64
	if err := sqlitex.ExecuteTransient(conn,
		"SELECT destination_stellium_id FROM in_transit WHERE entity_id = ?;", &sqlitex.ExecOptions{
			Args: []any{entityID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				destination = stmt.ColumnInt64(0)
				return nil
			}}); err != nil {
		t.Fatal(err)
	}
	return destination
}

// arrivalTurn is the turn a crossing is due, or 0 when the ship is not on one.
func arrivalTurn(t *testing.T, conn *sqlite.Conn, entityID int64) int {
	t.Helper()
	due := 0
	if err := sqlitex.ExecuteTransient(conn,
		"SELECT arrival_turn FROM in_transit WHERE entity_id = ?;", &sqlitex.ExecOptions{
			Args: []any{entityID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				due = stmt.ColumnInt(0)
				return nil
			}}); err != nil {
		t.Fatal(err)
	}
	return due
}

func countRows(t *testing.T, conn *sqlite.Conn, table string) int {
	t.Helper()
	count := 0
	if err := sqlitex.ExecuteTransient(conn, "SELECT count(*) FROM "+table+";", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		}}); err != nil {
		t.Fatal(err)
	}
	return count
}

// openAndResolve advances past a resolved turn and resolves the next one, which
// carries no orders at all: the only thing that happens in it is the sweeps.
func openAndResolve(t *testing.T, conn *sqlite.Conn, resolvedTurn int) {
	t.Helper()
	openAndResolveWithOrders(t, conn, resolvedTurn, "")
}

func openAndResolveWithOrders(t *testing.T, conn *sqlite.Conn, resolvedTurn int, submit string) {
	t.Helper()
	if _, err := OpenNextTurn(context.Background(), conn, "TEST", resolvedTurn); err != nil {
		t.Fatal(err)
	}
	if submit != "" {
		if err := sqlitex.ExecuteScript(conn, submit, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Resolve(context.Background(), discardLogger(), conn, "TEST", resolvedTurn+1); err != nil {
		t.Fatal(err)
	}
}

// A transfer moves no entity and still runs its transports, so its fuel is
// worth logging even though it records no movement; and a partly filled order
// succeeded, so what it has to say is a note rather than an error.
func TestResolveRecordsATransfersFuelAndItsShortfall(t *testing.T) {
	conn := openEngineTestDatabase(t)
	testdb.Exec(t, conn, `
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(41, 'operational', 'TRAN', 1, 20), (41, 'cargo', 'GOLD', 0, 1000),
			(41, 'cargo', 'FUEL', 0, 100);
		INSERT INTO entity_population (entity_id, class, quantity) VALUES (41, 'SKW', 50);
		UPDATE entity SET mass = 5000 WHERE id = 41;
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES (1, 3, 1, 1, 4, 'transfer', 41, '1,000 GOLD to ship 40',
			'{"units":[{"quantity":1000,"unit":"GOLD"}],"to":40,"to_kind":"ship"}');
	`)
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
	if result.Succeeded != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v; want the order to succeed: a shortage is a rate, not a failure", result)
	}
	// Twenty TRAN-1 carry 400 MU of the 1,000 GOLD, and the twenty hulls burn
	// ceil(20/10) = 2 FUEL.
	var status, message string
	var fuelSpent int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT status, coalesce(error_message, ''), fuel_spent FROM game_order
		WHERE game_id = 1 AND turn = 3 AND faction_id = 1 AND sequence = 1;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			status, message, fuelSpent = stmt.ColumnText(0), stmt.ColumnText(1), stmt.ColumnInt64(2)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || message != "" || fuelSpent != 2 {
		t.Errorf("order = (%q, %q, %d FUEL); want succeeded with no message and 2 FUEL", status, message, fuelSpent)
	}
	if !strings.Contains(logs.String(), `note="colony 41 transferred 400 of 1,000 GOLD`) {
		t.Errorf("log does not carry the shortfall as a note:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), "fuel_spent=2") {
		t.Errorf("log does not report the transports' fuel:\n%s", logs.String())
	}
}
