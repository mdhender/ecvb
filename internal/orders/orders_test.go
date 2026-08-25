// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
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
	if result != (Result{GameCode: "TEST", Turn: 3, FactionID: 1, Orders: 3}) {
		t.Fatalf("result = %+v", result)
	}
	if got := orderCount(t, conn); got != 1 {
		t.Fatalf("order count after check = %d; want 1", got)
	}
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
			(40, 'component', 'HDRV', 4, 1), (42, 'component', 'HDRV', 4, 1);
		INSERT INTO jump_order (
			game_id, turn, faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id
		) VALUES (1, 3, 1, 1, 3, 40, 0, 0, 0, 10);
	`, nil); err != nil {
		t.Fatal(err)
	}
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
