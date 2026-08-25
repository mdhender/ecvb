// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fuel

import (
	"path/filepath"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestSpendEmptiesOperationalThenUnassembledThenCargo(t *testing.T) {
	conn := openFuelTestDatabase(t)
	if got, err := Available(conn, 40); err != nil || got != 60 {
		t.Fatalf("available = (%d, %v); want 60", got, err)
	}
	// 45 FUEL empties the 10 operational and 15 unassembled units and takes
	// the remaining 20 out of the 35 held as cargo.
	if err := Spend(conn, 40, 45); err != nil {
		t.Fatal(err)
	}
	if got := held(t, conn, 40); got != "cargo=15" {
		t.Errorf("inventory = %q; want only 15 units of cargo left", got)
	}
	if got, err := Available(conn, 40); err != nil || got != 15 {
		t.Fatalf("available = (%d, %v); want 15", got, err)
	}
	// The burned fuel left the ship, so 45 * 6 MU came off its mass.
	if got := mass(t, conn, 40); got != 1000-45*UnitMass {
		t.Errorf("mass = %d; want %d", got, 1000-45*UnitMass)
	}
}

func TestSpendLeavesOtherEntitiesAndReportsAShortfall(t *testing.T) {
	conn := openFuelTestDatabase(t)
	if err := Spend(conn, 40, 61); err == nil {
		t.Fatal("Spend succeeded; want a shortfall error")
	}
	if err := Spend(conn, 40, 60); err != nil {
		t.Fatal(err)
	}
	if got := held(t, conn, 40); got != "" {
		t.Errorf("inventory = %q; want every stack emptied", got)
	}
	// Entity 41's fuel is untouched, and spending nothing is not an error.
	if got, err := Available(conn, 41); err != nil || got != 7 {
		t.Fatalf("other entity available = (%d, %v); want 7", got, err)
	}
	if err := Spend(conn, 41, 0); err != nil {
		t.Fatal(err)
	}
	if got := mass(t, conn, 41); got != 500 {
		t.Errorf("other entity mass = %d; want 500", got)
	}
}

func TestAvailableAllReportsEveryEntityHoldingFuel(t *testing.T) {
	conn := openFuelTestDatabase(t)
	quantities, err := AvailableAll(conn, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(quantities) != 2 || quantities[40] != 60 || quantities[41] != 7 {
		t.Fatalf("quantities = %v; want 40 to hold 60 and 41 to hold 7", quantities)
	}
	// An entity holding no fuel is absent, which reads as zero.
	if quantities[42] != 0 {
		t.Errorf("dry entity = %d; want 0", quantities[42])
	}
}

func openFuelTestDatabase(t *testing.T) *sqlite.Conn {
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
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 0);
		INSERT INTO faction (id, game_id, user_id) VALUES (1, 1, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (10, 1, 0, 0, 0);
		INSERT INTO entity (id, unit, tech_level, stellium_id, faction_id, enclosed_volume, mass) VALUES
			(40, 'SHIP', 1, 10, 1, 100, 1000),
			(41, 'SHIP', 1, 10, 1, 100, 500),
			(42, 'SHIP', 1, 10, 1, 100, 500);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(40, 'cargo', 'FUEL', 0, 35), (40, 'operational', 'FUEL', 0, 10),
			(40, 'unassembled', 'FUEL', 0, 15), (40, 'cargo', 'METL', 0, 99),
			(41, 'cargo', 'FUEL', 0, 7),
			(42, 'cargo', 'METL', 0, 3);
	`, nil); err != nil {
		t.Fatal(err)
	}
	return conn
}

// held renders the entity's remaining fuel stacks in draw order, so a test can
// assert which sections were emptied.
func held(t *testing.T, conn *sqlite.Conn, entityID int64) string {
	t.Helper()
	var text string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT coalesce(group_concat(printf('%s=%d', section, quantity), ' '), '')
		FROM (SELECT section, quantity FROM inventory
			WHERE entity_id = ? AND unit = 'FUEL' ORDER BY section);`, &sqlitex.ExecOptions{
		Args: []any{entityID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			text = stmt.ColumnText(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return text
}

func mass(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	var value int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT mass FROM entity WHERE id = ?;", &sqlitex.ExecOptions{
		Args: []any{entityID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			value = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return value
}
