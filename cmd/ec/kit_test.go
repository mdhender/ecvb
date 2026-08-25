// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestReadHomePlanetKit(t *testing.T) {
	kit, err := readKit(filepath.Join("..", "..", "games", "beta", "home-planet-seed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if kit.name != "Lean Start" || len(kit.entities) != 4 {
		t.Fatalf("kit = (%q, %d entities); want (Lean Start, 4 entities)", kit.name, len(kit.entities))
	}
	want := map[string]struct {
		mass             int64
		enclosedVolume   int64
		controlled       bool
		inventoryEntries int
	}{
		"COPN": {mass: 5499, enclosedVolume: 20000, controlled: true, inventoryEntries: 6},
		"CSFC": {mass: 5594, enclosedVolume: 20000, controlled: true, inventoryEntries: 7},
		"CORB": {mass: 71607, enclosedVolume: 350000, controlled: false, inventoryEntries: 6},
		// The ship carries 20 HDRV-3 and the 3500 FUEL they burn. Bulk
		// resources mass 1 MU each, so a full tank is cheap to carry and the
		// drive can stay at the range the map asks for.
		"SHIP": {mass: 55892, enclosedVolume: 200000, controlled: true, inventoryEntries: 13},
	}
	for _, entity := range kit.entities {
		properties, ok := want[entity.kind]
		if !ok {
			t.Errorf("unexpected entity kind %q", entity.kind)
			continue
		}
		if entity.mass != properties.mass || entity.enclosedVolume != properties.enclosedVolume || entity.controlled != properties.controlled || len(entity.inventory) != properties.inventoryEntries {
			t.Errorf("%s = (mass %d, volume %d, controlled %t, inventory %d); want (%d, %d, %t, %d)",
				entity.kind, entity.mass, entity.enclosedVolume, entity.controlled, len(entity.inventory),
				properties.mass, properties.enclosedVolume, properties.controlled, properties.inventoryEntries)
		}
	}
}

func TestReadKitRejectsUnknownFieldsAndInsufficientSpace(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown field",
			content: `{"kit-name":"test","unknown":true}`,
			want:    "unknown field",
		},
		{
			name: "insufficient space",
			content: `{
				"kit-name":"test",
				"entities":{"entity":{"kind":"COPN","tech-level":1,"population":{"USK":1},"components":{"STRC-1":2}}}
			}`,
			want: "need at least 3 VU",
		},
		{
			name: "invalid entity kind",
			content: `{
				"kit-name":"test",
				"entities":{"entity":{"kind":"BASE","tech-level":1}}
			}`,
			want: "invalid kind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "kit.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := readKit(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readKit error = %v; want containing %q", err, tt.want)
			}
		})
	}
}

func TestAddPlayerLoadsKitAndAssignsUncontrolledEntities(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	writeTestKit(t, directory)
	conn := openPlayerTestDatabase(t, directory)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (email, role) VALUES ('player@example.com', 'non-administrator');
		INSERT INTO game (code) VALUES ('TEST');
		INSERT INTO stellium (game_id, x, y, z) VALUES (1, 4, 5, 6);
		INSERT INTO system (stellium_id, sequence) VALUES (1, 'A');
		INSERT INTO planet (system_id, orbit, kind, habitability)
		VALUES (1, 4, 'rocky', 25);
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	playerFactionID, err := addPlayer(context.Background(), directory, "TEST", "player@example.com")
	if err != nil {
		t.Fatal(err)
	}
	conn = openPlayerTestDatabase(t, directory)
	defer conn.Close()

	var entityCount, inventoryCount, populationCount int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT
			(SELECT count(*) FROM entity),
			(SELECT count(*) FROM inventory),
			(SELECT count(*) FROM entity_population);`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		entityCount = stmt.ColumnInt(0)
		inventoryCount = stmt.ColumnInt(1)
		populationCount = stmt.ColumnInt(2)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if entityCount != 3 || inventoryCount != 2 || populationCount != 2 {
		t.Fatalf("rows = (entities %d, inventory %d, population %d); want (3, 2, 2)", entityCount, inventoryCount, populationCount)
	}

	var controlledFactionID, uncontrolledFactionID int64
	var controlledRing, uncontrolledRing int
	var controlledMass, controlledVolume int64
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, planet_ring, mass, enclosed_volume
		FROM entity WHERE unit = 'COPN';`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		controlledFactionID = stmt.ColumnInt64(0)
		controlledRing = stmt.ColumnInt(1)
		controlledMass = stmt.ColumnInt64(2)
		controlledVolume = stmt.ColumnInt64(3)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if controlledFactionID != playerFactionID || controlledRing != 0 || controlledMass != 8 || controlledVolume != 3 {
		t.Errorf("controlled entity = (faction %d, ring %d, mass %d, volume %d); want (%d, 0, 8, 3)", controlledFactionID, controlledRing, controlledMass, controlledVolume, playerFactionID)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT e.faction_id, e.planet_ring
		FROM entity AS e
		JOIN faction AS f ON f.id = e.faction_id
		JOIN agent AS a ON a.id = f.agent_id
		WHERE e.unit = 'CORB' AND a.code = 'uncontrolled';`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		uncontrolledFactionID = stmt.ColumnInt64(0)
		uncontrolledRing = stmt.ColumnInt(1)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if uncontrolledFactionID == 0 || uncontrolledFactionID == playerFactionID || uncontrolledRing != 1 {
		t.Errorf("uncontrolled entity = (faction %d, ring %d); want non-player faction and ring 1", uncontrolledFactionID, uncontrolledRing)
	}
	var shipFactionID int64
	var shipRing int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, planet_ring FROM entity WHERE unit = 'SHIP';`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		shipFactionID = stmt.ColumnInt64(0)
		shipRing = stmt.ColumnInt(1)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if shipFactionID != playerFactionID || shipRing != 64 {
		t.Errorf("ship = (faction %d, ring %d); want (faction %d, ring 64)", shipFactionID, shipRing, playerFactionID)
	}
}

func TestAddPlayerRollsBackInvalidKit(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	if err := os.WriteFile(filepath.Join(directory, "home-planet-seed.json"), []byte(`{
		"kit-name":"invalid",
		"entities":{"entity":{"kind":"COPN","tech-level":1,"population":{"USK":1}}}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	conn := openPlayerTestDatabase(t, directory)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (email, role) VALUES ('player@example.com', 'non-administrator');
		INSERT INTO game (code) VALUES ('TEST');
		INSERT INTO stellium (game_id, x, y, z) VALUES (1, 4, 5, 6);
		INSERT INTO system (stellium_id, sequence) VALUES (1, 'A');
		INSERT INTO planet (system_id, orbit, kind, habitability)
		VALUES (1, 4, 'rocky', 25);
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := addPlayer(context.Background(), directory, "TEST", "player@example.com"); err == nil || !strings.Contains(err.Error(), "enclosed space") {
		t.Fatalf("addPlayer error = %v; want enclosed space error", err)
	}
	conn = openPlayerTestDatabase(t, directory)
	defer conn.Close()
	var factions, controlledPlanets int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT
			(SELECT count(*) FROM faction),
			(SELECT count(*) FROM planet WHERE faction_id IS NOT NULL);`, &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		factions = stmt.ColumnInt(0)
		controlledPlanets = stmt.ColumnInt(1)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if factions != 0 || controlledPlanets != 0 {
		t.Errorf("after rejected kit: factions = %d, controlled planets = %d; want zero", factions, controlledPlanets)
	}
}

func writeTestKit(t *testing.T, directory string) {
	t.Helper()
	content := `{
		"kit-name":"test",
		"entities":{
			"controlled":{"kind":"COPN","tech-level":1,"population":{"USK":1},"components":{"STRC-1":3}},
			"uncontrolled":{"kind":"CORB","tech-level":1},
			"ship":{"kind":"SHIP","tech-level":1,"population":{"SKW":1},"components":{"STRC-1":30}}
		}
	}`
	if err := os.WriteFile(filepath.Join(directory, "home-planet-seed.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBulkResourcesMassAndOccupyOneUnit(t *testing.T) {
	// The four raw resources are measured, not manufactured: 1 MU and 1 VU
	// each, in every section, with none of the multipliers that installing a
	// manufactured unit costs.
	for _, unit := range []string{"FUEL", "GOLD", "METL", "MNRL"} {
		got := metricsForUnit(unit, 0, false)
		if want := (unitMetrics{mass: 1, cargoVolume: 1, operationalVolume: 1, componentVolume: 1}); got != want {
			t.Errorf("%s metrics = %+v; want %+v", unit, got, want)
		}
	}
	// Another unit without a technology level keeps the general rule.
	got := metricsForUnit("CNGD", 0, false)
	if want := (unitMetrics{mass: 6, cargoVolume: 6, operationalVolume: 12, componentVolume: 24}); got != want {
		t.Errorf("CNGD metrics = %+v; want %+v", got, want)
	}
}
