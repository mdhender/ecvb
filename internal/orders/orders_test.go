// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/testdb"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const validOrders = `game "TEST" turn 3
id player " PLAYER@EXAMPLE.COM "

move ship 40 to orbit 6
jump ship 40 to (1,2,3)
move ship 40 to system b orbit 4
`

func TestCheckValidatesSequentialOrdersWithoutWriting(t *testing.T) {
	conn := openOrderTestDatabase(t)
	result, err := Check(context.Background(), conn, strings.NewReader(validOrders))
	if err != nil {
		t.Fatal(err)
	}
	if result.GameCode != "TEST" || result.Turn != 3 || result.FactionID != 1 || result.Orders != 3 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", result.Warnings)
	}
	if got := orderCount(t, conn); got != 1 {
		t.Fatalf("order count after check = %d; want 1", got)
	}
}

// Checking a file runs the turn against the database and rolls it back, so the
// thing to get wrong is the rolling back. Nothing a turn would change may
// survive a check: not where a ship is, not the fuel it holds, not the mass
// that fuel was part of, not what a probe read, and not what a phase's sweep
// wrote when nobody ordered it.
func TestCheckPutsTheWorldBackTheWayItFoundIt(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

probe ship 40 orbit 4
move ship 40 to orbit 6
jump ship 40 to (1,2,3)
`
	before := worldSnapshot(t, conn)
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if after := worldSnapshot(t, conn); after != before {
		t.Errorf("check changed the world:\n before %s\n  after %s", before, after)
	}
}

// worldSnapshot is everything an order may touch, in one string.
func worldSnapshot(t *testing.T, conn *sqlite.Conn) string {
	t.Helper()
	var parts []string
	for _, query := range []string{
		`SELECT group_concat(printf('%d@%d/%s/%s/%s:%d', id, stellium_id, coalesce(system_id, '-'),
			coalesce(planet_id, '-'), coalesce(planet_ring, '-'), mass), ' ')
			FROM (SELECT * FROM entity ORDER BY id)`,
		`SELECT group_concat(printf('%d/%s/%d', entity_id, section, quantity), ' ')
			FROM (SELECT * FROM inventory ORDER BY entity_id, section, unit, tech_level)`,
		`SELECT (SELECT count(*) FROM probe_contact) || '/' || (SELECT count(*) FROM probe_deposit)
			|| '/' || (SELECT count(*) FROM sensor_survey) || '/' || (SELECT count(*) FROM sensor_contact)`,
	} {
		if err := sqlitex.ExecuteTransient(conn, query+";", &sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				parts = append(parts, stmt.ColumnText(0))
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return strings.Join(parts, " | ")
}

func TestCheckValidatesMovesBeforeJumpsRegardlessOfFileOrder(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1
jump ship 40 to (1,2,3)
move ship 40 to orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckReportsInvalidTargets(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 4
id faction 1
jump ship 999 to (1,2,3)
move ship 41 to orbit 6
jump ship 42 to (1,2,3)
jump ship 40 to (9,9,9)
`
	_, err := Check(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatal("Check succeeded; want errors")
	}
	message := err.Error()
	for _, want := range []string{
		"line 1: game \"TEST\" is on turn 3, not turn 4",
		"line 3: ship 999 does not exist",
		"line 4: entity 41 is a COPN, not a ship",
		"line 5: ship 42 does not belong to faction 1",
		"line 6: game \"TEST\" has no stellium at (9,9,9)",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}

func TestSubmitAtomicallyReplacesOrders(t *testing.T) {
	conn := openOrderTestDatabase(t)
	result, err := Submit(context.Background(), conn, strings.NewReader(validOrders))
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 3 {
		t.Fatalf("submitted orders = %d; want 3", result.Orders)
	}

	var rows []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%d|%s|%s', sequence, ship_id, verb, input)
		FROM (
			SELECT sequence, ship_id, 'move' AS verb,
				CASE WHEN requested_system IS NULL THEN printf('orbit %d', requested_orbit)
				ELSE printf('system %s orbit %d', requested_system, requested_orbit) END AS input
			FROM move_order WHERE faction_id = 1 AND turn = 3
			UNION ALL
			SELECT sequence, ship_id, 'jump', printf('(%d,%d,%d)', destination_x, destination_y, destination_z)
			FROM jump_order WHERE faction_id = 1 AND turn = 3
		) ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			rows = append(rows, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1|40|move|orbit 6",
		"2|40|move|system B orbit 4",
		"3|40|jump|(1,2,3)",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("orders = %q; want %q", rows, want)
	}

	invalid := strings.Replace(validOrders, "(1,2,3)", "(9,9,9)", 1)
	if _, err := Submit(context.Background(), conn, strings.NewReader(invalid)); err == nil {
		t.Fatal("invalid Submit succeeded")
	}
	if got := orderCount(t, conn); got != 3 {
		t.Fatalf("order count after rejected submit = %d; want 3", got)
	}
}

func TestSubmitRejectsResolvedTurn(t *testing.T) {
	conn := openOrderTestDatabase(t)
	if err := sqlitex.ExecuteTransient(conn, "UPDATE game SET turn_state = 'resolved' WHERE id = 1;", nil); err != nil {
		t.Fatal(err)
	}
	_, err := Submit(context.Background(), conn, strings.NewReader(validOrders))
	if err == nil || !strings.Contains(err.Error(), "resolved and not accepting orders") {
		t.Fatalf("Submit error = %v; want resolved turn error", err)
	}
	if got := orderCount(t, conn); got != 1 {
		t.Fatalf("order count after rejected submit = %d; want 1", got)
	}
}

func TestSpecializedOrderForeignKeysEnforceOwnershipAndDestinationGame(t *testing.T) {
	conn := openOrderTestDatabase(t)
	for name, script := range map[string]string{
		"ship ownership": `
			INSERT INTO jump_order (
				game_id, turn, faction_id, sequence, source_line, ship_id,
				destination_x, destination_y, destination_z, destination_stellium_id
			) VALUES (1, 3, 1, 2, 4, 42, 1, 2, 3, 11);`,
		"destination game": `
			INSERT INTO jump_order (
				game_id, turn, faction_id, sequence, source_line, ship_id,
				destination_x, destination_y, destination_z, destination_stellium_id
			) VALUES (1, 3, 1, 2, 4, 40, 1, 2, 3, 12);`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := sqlitex.ExecuteTransient(conn, script, nil); err == nil {
				t.Fatal("insert succeeded; want foreign key error")
			}
		})
	}
}

func openOrderTestDatabase(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
		INSERT INTO users (id, email, role) VALUES (1, 'player@example.com', 'non-administrator');
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 3), (2, 'OTHER', 3);
		INSERT INTO agent (id, code, description) VALUES (1, 'uncontrolled', 'Uncontrolled');
		INSERT INTO faction (id, game_id, user_id) VALUES (1, 1, 1);
		INSERT INTO faction (id, game_id, agent_id) VALUES (2, 1, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES
			(10, 1, 0, 0, 0), (11, 1, 1, 2, 3), (12, 2, 1, 2, 3), (13, 1, 2, 4, 6);
		INSERT INTO system (id, stellium_id, sequence) VALUES
			(20, 10, 'A'), (21, 11, 'A'), (22, 11, 'B'), (23, 10, 'B');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES
			(30, 20, 4, 'rocky', 10), (31, 20, 6, 'rocky', 10),
			(32, 22, 4, 'rocky', 10), (33, 23, 4, 'rocky', 10);
		INSERT INTO entity (id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume) VALUES
			(40, 'SHIP', 1, 10, 20, 30, 64, 1, 100),
			(41, 'COPN', 1, 10, 20, 30, 0, 1, 100),
			(42, 'SHIP', 1, 10, 20, 30, 64, 2, 100);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(40, 'component', 'HDRV', 4, 1), (42, 'component', 'HDRV', 4, 1),
			(40, 'component', 'SNSR', 2, 1), (41, 'component', 'SNSR', 1, 1),
			(40, 'cargo', 'FUEL', 0, 500), (42, 'cargo', 'FUEL', 0, 500);
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id
		) VALUES (1, 3, 1, 1, 3, 40, 0, 0, 0, 10);
		-- Checking a file runs the orders for real and rolls them back, so a
		-- ship has to mass at least the fuel it is about to burn. 500 of each
		-- ship's 3,000 MU is the fuel it carries, at 1 MU each.
		UPDATE entity SET mass = 3000 WHERE id IN (40, 42);
		UPDATE entity SET mass = 1234 WHERE id = 41;
	`)
	return conn
}

func orderCount(t *testing.T, conn *sqlite.Conn) int {
	t.Helper()
	count := 0
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT (SELECT count(*) FROM move_order WHERE faction_id = 1 AND turn = 3)
			+ (SELECT count(*) FROM jump_order WHERE faction_id = 1 AND turn = 3);`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCheckRejectsJumpsTheDriveCannotMake(t *testing.T) {
	// Stellium 11 is (1,2,3) from stellium 10, a distance of 4.
	input := `game "TEST" turn 3
id faction 1

jump ship 40 to (1,2,3)
`
	for _, tc := range []struct {
		name    string
		setup   string
		problem string
	}{
		{
			name:    "out of range",
			setup:   `UPDATE inventory SET tech_level = 3 WHERE entity_id = 40 AND unit = 'HDRV';`,
			problem: "jump of 4 units exceeds ship 40 jump range of 3 units",
		},
		{
			name:    "too massive",
			setup:   `UPDATE entity SET mass = 4181 WHERE id = 40;`,
			problem: "ship 40 masses 4181 MU and its jump drive propels 4180 MU",
		},
		{
			name:    "no drive",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'HDRV';`,
			problem: "ship 40 has no assembled HDRV and cannot jump",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			if err := sqlitex.ExecuteScript(conn, tc.setup, nil); err != nil {
				t.Fatal(err)
			}
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
			if !strings.Contains(err.Error(), "line 4") {
				t.Errorf("error = %v; want it to name the source line", err)
			}
		})
	}
}

func TestCheckMeasuresASecondJumpFromTheFirstDestination(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 starts at the origin with a range of 4. Stellium 13 at (2,4,6)
	// is 4 units from stellium 11 at (1,2,3) but 8 units from the origin, so
	// the second jump is only legal when it is measured from the first jump's
	// destination.
	input := `game "TEST" turn 3
id faction 1

jump ship 40 to (1,2,3)
jump ship 40 to (2,4,6)
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want 2", result.Orders)
	}
}

func TestCheckRejectsASecondJumpBeyondRangeOfTheFirstDestination(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Reversing the order puts stellium 13 first, 8 units from the origin.
	input := `game "TEST" turn 3
id faction 1

jump ship 40 to (2,4,6)
`
	_, err := Check(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatal("Check succeeded; want the jump rejected")
	}
	if want := "line 4: jump of 8 units exceeds ship 40 jump range of 4 units"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v; want %q", err, want)
	}
}

func TestCheckAcceptsProbesWithinTheSensorBudget(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 carries one SNSR-2, so it launches two probes. System 20 has
	// planets in orbits 4 and 6.
	input := `game "TEST" turn 3
id faction 1

probe ship 40 orbit 4 6
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want one order per probed orbit", result.Orders)
	}
}

func TestCheckRejectsProbesTheSensorsCannotSupport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   string
		input   string
		problem string
	}{
		{
			name:    "over budget",
			input:   "probe ship 40 orbit 4 6 4",
			problem: "ship 40 has only 2 probes this turn",
		},
		{
			name:    "empty orbit",
			input:   "probe ship 40 orbit 5",
			problem: "system current has no planet in orbit 5",
		},
		{
			name:    "no sensors",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'SNSR';`,
			input:   "probe ship 40 orbit 4",
			problem: "ship 40 has no assembled SNSR and cannot probe",
		},
		{
			name:    "no system",
			setup:   `UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`,
			input:   "probe ship 40 orbit 4",
			problem: "ship 40 is orbiting the stellium; name a system to probe",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			if tc.setup != "" {
				if err := sqlitex.ExecuteScript(conn, tc.setup, nil); err != nil {
					t.Fatal(err)
				}
			}
			input := "game \"TEST\" turn 3\nid faction 1\n\n" + tc.input + "\n"
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
		})
	}
}

func TestCheckProbesTheSystemTheShipStartsIn(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 starts in system 20, which has planets in orbits 4 and 6.
	// Moving to system B puts it in system 23, which has no orbit 6. Probes
	// resolve before moves, so the probe reads system 20 and succeeds.
	input := `game "TEST" turn 3
id faction 1

probe ship 40 orbit 6
move ship 40 to system b orbit 4
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want 2", result.Orders)
	}
}

func TestCheckOrdersProbesAheadOfMovesAndJumps(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// File order is move, jump, probe. Resolution order is probe, move, jump,
	// and the stored sequence has to record the resolution order.
	input := `game "TEST" turn 3
id faction 1

move ship 40 to orbit 6
jump ship 40 to (1,2,3)
probe ship 40 orbit 4
`
	if _, err := Submit(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, query := range []string{
		"SELECT 'probe', sequence FROM probe_order WHERE faction_id = 1 AND turn = 3",
		"SELECT 'move', sequence FROM move_order WHERE faction_id = 1 AND turn = 3",
		"SELECT 'jump', sequence FROM jump_order WHERE faction_id = 1 AND turn = 3",
	} {
		if err := sqlitex.ExecuteTransient(conn, query+";", &sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				order = append(order, fmt.Sprintf("%s=%d", stmt.ColumnText(0), stmt.ColumnInt(1)))
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"probe=1", "move=2", "jump=3"}; !slices.Equal(order, want) {
		t.Errorf("sequences = %v; want %v", order, want)
	}
}

func TestSubmitStoresOneProbeOrderForEachOrbit(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

probe ship 40 orbit 4 6
`
	if _, err := Submit(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var orbits []int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT requested_orbit FROM probe_order
		WHERE faction_id = 1 AND turn = 3 ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			orbits = append(orbits, stmt.ColumnInt(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []int{4, 6}; !slices.Equal(orbits, want) {
		t.Errorf("stored orbits = %v; want %v", orbits, want)
	}
}

func TestCheckProbesANamedSystemOfTheCurrentStellium(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 is in system 20 (A) of stellium 10, which also holds system 23
	// (B). Naming B probes a system the ship is not in.
	input := `game "TEST" turn 3
id faction 1

probe ship 40 system B orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckProbesANamedSystemFromStelliumOrbit(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// A ship orbiting the stellium has no current system, so a bare probe
	// fails. Naming a system of that stellium works.
	if err := sqlitex.ExecuteScript(conn,
		`UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`, nil); err != nil {
		t.Fatal(err)
	}
	input := `game "TEST" turn 3
id faction 1

probe ship 40 system A orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckRejectsAProbeOfASystemTheStelliumDoesNotHold(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

probe ship 40 system C orbit 4
`
	_, err := Check(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatal("Check succeeded; want a problem")
	}
	if want := "current stellium has no system C"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v; want %q", err, want)
	}
}

func TestCheckAcceptsAColonyProbe(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Entity 41 is a COPN at planet 30 in system 20, with one SNSR-1.
	input := `game "TEST" turn 3
id faction 1

probe colony 41 orbit 4
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 1 {
		t.Errorf("orders = %d; want 1", result.Orders)
	}
}

func TestCheckRejectsAProbeThatNamesTheWrongKindOfEntity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		problem string
	}{
		{name: "colony named as a ship", input: "probe ship 41 orbit 4", problem: "entity 41 is a COPN, not a ship"},
		{name: "ship named as a colony", input: "probe colony 40 orbit 4", problem: "entity 40 is a ship, not a colony"},
		// Only a probe may name a colony, so MOVE reports its own syntax
		// rather than every form of every order in the game.
		{name: "colony ordered to move", input: "move colony 41 to orbit 6", problem: "expected move ship SHIP-ID to orbit ORBIT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			input := "game \"TEST\" turn 3\nid faction 1\n\n" + tc.input + "\n"
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
		})
	}
}

func TestSubmitStoresMoveFuelAndTheStelliumOrbit(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 starts at planet 30 in system A of stellium 10. Orbit 6 is
	// another planet of system A, system B orbit 4 is planet 33 of the same
	// stellium, and orbit 11 is the stellium orbit.
	input := `game "TEST" turn 3
id faction 1

move ship 40 to orbit 6
move ship 40 to system B orbit 4
move ship 40 to orbit 11
`
	if _, err := Submit(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var rows []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%d|%d|%s|%s|%d', sequence, requested_orbit,
			fuel_spent, coalesce(destination_system_id, '-'), coalesce(destination_planet_id, '-'),
			destination_stellium_id)
		FROM move_order WHERE faction_id = 1 AND turn = 3 ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			rows = append(rows, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1|6|4|20|31|10", // one hop: planet to planet inside system A
		"2|4|8|23|33|10", // two hops: planet to planet across systems
		"3|11|4|-|-|10",  // one hop: planet to the stellium orbit
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("move orders = %q; want %q", rows, want)
	}
}

func TestCheckRejectsMovesTheDriveCannotMake(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

move ship 40 to orbit 6
`
	for _, tc := range []struct {
		name    string
		setup   string
		problem string
	}{
		{
			name:    "too massive",
			setup:   `UPDATE entity SET mass = 4181 WHERE id = 40;`,
			problem: "ship 40 masses 4181 MU and its drive propels 4180 MU",
		},
		{
			name:    "no drive",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'HDRV';`,
			problem: "ship 40 has no assembled HDRV and cannot move",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			if err := sqlitex.ExecuteScript(conn, tc.setup, nil); err != nil {
				t.Fatal(err)
			}
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
			if !strings.Contains(err.Error(), "line 4") {
				t.Errorf("error = %v; want it to name the source line", err)
			}
		})
	}
}

func TestCheckMovesFromTheStelliumOrbitAndRejectsQualifyingIt(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// A ship in the stellium orbit has no current system, so it has to name
	// one; the stellium orbit itself belongs to no system, so naming one for
	// orbit 11 is an error.
	input := `game "TEST" turn 3
id faction 1

move ship 40 to orbit 11
move ship 40 to orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err == nil ||
		!strings.Contains(err.Error(), "line 5: ship has no current system") {
		t.Fatalf("Check error = %v; want the ship to have left its system", err)
	}
	qualified := `game "TEST" turn 3
id faction 1

move ship 40 to system A orbit 11
`
	if _, err := Check(context.Background(), conn, strings.NewReader(qualified)); err == nil ||
		!strings.Contains(err.Error(), "orbit 11 is the stellium orbit and belongs to no system") {
		t.Fatalf("Check error = %v; want the stellium orbit to reject a system", err)
	}
}

func TestCheckWarnsWhenTheShipCannotPayForItsOrders(t *testing.T) {
	conn := openOrderTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE inventory SET quantity = 200 WHERE entity_id = 40 AND unit = 'FUEL';`, nil); err != nil {
		t.Fatal(err)
	}
	// Ship 40 has one HDRV-4 and 200 FUEL. The move burns 1 * 0.1 * 40 and
	// each 4-unit jump burns 1 * 4 * 40, so the tank covers the move and the
	// first jump but comes up short on the second.
	input := `game "TEST" turn 3
id faction 1

move ship 40 to orbit 6
jump ship 40 to (1,2,3)
jump ship 40 to (0,0,0)
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 3 {
		t.Fatalf("orders = %d; want 3", result.Orders)
	}
	var lines []string
	for _, warning := range result.Warnings {
		lines = append(lines, fmt.Sprintf("%d: %s", warning.Line, warning.Message))
	}
	want := []string{
		"6: ship 40 needs 160 FUEL to jump and holds 36; the order is kept in case that changes before the turn resolves",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("warnings = %q; want %q", lines, want)
	}
}

func TestCheckWarnsOnceTheTankIsEmptyAndSubmitKeepsTheOrders(t *testing.T) {
	conn := openOrderTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'FUEL';`, nil); err != nil {
		t.Fatal(err)
	}
	// A dry ship warns on every order but still submits them: fuel may reach
	// the ship before the turn resolves.
	input := `game "TEST" turn 3
id faction 1

move ship 40 to orbit 6
move ship 40 to orbit 4
`
	result, err := Submit(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings = %+v; want one per order", result.Warnings)
	}
	for _, warning := range result.Warnings {
		if !strings.Contains(warning.Message, "needs 4 FUEL to move and holds 0") {
			t.Errorf("warning = %q; want it to report an empty tank", warning.Message)
		}
	}
	var stored []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%d', sequence, fuel_spent)
		FROM move_order WHERE faction_id = 1 AND turn = 3 ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			stored = append(stored, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"1|4", "2|4"}; strings.Join(stored, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stored moves = %q; want %q", stored, want)
	}
}
