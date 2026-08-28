// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"github.com/mdhender/ecvb/internal/testdb"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestECRptShowStellium(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{"--db-path", directory, "show", "stellium", "79"}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"STELLIUM", "79  BETA-001  9  13  -5",
		"SYSTEMS", "88  A         2        4",
		"PLANETS", "A       871  1      rocky     8             3",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestRunShowStelliumRejectsInvalidAndMissingIDs(t *testing.T) {
	directory := createTestDatabase(t)
	for _, args := range [][]string{
		{"--db-path", directory, "show", "stellium", "nope"},
		{"--db-path", directory, "show", "stellium", "0"},
	} {
		if err := run(context.Background(), args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "invalid stellium id") {
			t.Errorf("run(%v) error = %v; want invalid id", args, err)
		}
	}
	if err := run(context.Background(), []string{"--db-path", directory, "show", "stellium", "80"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing stellium error = %v; want not found", err)
	}
}

func TestECRptShowSystem(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{"--db-path", directory, "show", "system", "88"}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"SYSTEM", "88  79        A",
		"PLANETS", "871  1      rocky", "fuel=30, metals=40", "872  2      asteroid", "gold=15",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "\nDEPOSITS\n") {
		t.Errorf("default output unexpectedly contains deposit details:\n%s", output.String())
	}
}

func TestECRptShowSystemWithDeposits(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{"--db-path", directory, "show", "system", "--show-deposits", "88"}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"DEPOSITS", "PLANET  ORBIT  SEQUENCE  RESOURCE", "871     1      1         fuel", "871     1      2         metals", "872     2      1         gold",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestRunShowSystemRejectsInvalidAndMissingIDs(t *testing.T) {
	directory := createTestDatabase(t)
	if err := run(context.Background(), []string{"--db-path", directory, "show", "system", "nope"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "invalid system id") {
		t.Fatalf("invalid system error = %v; want invalid id", err)
	}
	if err := run(context.Background(), []string{"--db-path", directory, "show", "system", "89"}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing system error = %v; want not found", err)
	}
}

func TestECRptShowTurnByEmail(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"--db-path", directory, "show", "turn", "--game", "BETA-001", "--email", " PLAYER@EXAMPLE.COM ",
	}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"TURN REPORT", "BETA-001  3     1        player@example.com",
		"CONTROLLED PLANETS", "871  79        9,13,-5",
		"ENTITIES", "100501  SHIP  2", "100502  COPN  1",
		"CENSUS", "100501  SKW    1200",
		"INVENTORY", "100501  cargo        FUEL", "100502  cargo        GOLD",
		"ORDERS", "1         4     100501  move", "2         3     100501  jump",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
	for _, absent := range []string{"\nDEPOSITS\n", "\nRESOURCE SUMMARY\n", "\nWORK GROUPS\n"} {
		if strings.Contains(output.String(), absent) {
			t.Errorf("default output unexpectedly contains %q:\n%s", absent, output.String())
		}
	}
}

func TestECRptShowOrdersByEmail(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"--db-path", directory, "show", "orders", "--game", "BETA-001", "--email", " PLAYER@EXAMPLE.COM ",
	}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"ORDERS REPORT", "BETA-001  3     1        player@example.com",
		"SEQUENCE  LINE  ENTITY  VERB", "1         4     100501  move", "2         3     100501  jump",
		"orbit 1", "pending",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestECRptShowOrdersForAgentFaction(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"--db-path", directory, "show", "orders", "--game", "BETA-001", "--faction", "2",
	}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "BETA-001  3     2        agent:uncontrolled"; !strings.Contains(output.String(), want) {
		t.Errorf("output does not contain %q:\n%s", want, output.String())
	}
	if strings.Contains(output.String(), "100501") {
		t.Errorf("agent report contains another faction's orders:\n%s", output.String())
	}
}

func TestECRptShowTurnByFactionWithOptionalSections(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"--db-path", directory, "show", "turn", "--game", "BETA-001", "--faction", "1",
		"--show-deposits", "--summarize-resources", "--work-groups",
	}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{
		"DEPOSITS", "871     1         fuel",
		"RESOURCE SUMMARY", "FUEL      30", "GOLD      7", "METL      9",
		"WORK GROUPS", "100502  MINE  1         1        1     4",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestECRptShowTurnForAgentFaction(t *testing.T) {
	directory := createTestDatabase(t)
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"--db-path", directory, "show", "turn", "--game", "BETA-001", "--faction", "2",
	}, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "BETA-001  3     2        agent:uncontrolled"; !strings.Contains(output.String(), want) {
		t.Errorf("output does not contain %q:\n%s", want, output.String())
	}
}

func TestECRptShowWritesOutputFile(t *testing.T) {
	directory := createTestDatabase(t)
	tests := []struct {
		name    string
		args    []string
		content string
	}{
		{name: "orders", args: []string{"orders", "--game", "BETA-001", "--faction", "1"}, content: "ORDERS REPORT"},
		{name: "stellium", args: []string{"stellium", "79"}, content: "STELLIUM"},
		{name: "system", args: []string{"system", "88"}, content: "SYSTEM"},
		{name: "turn", args: []string{"turn", "--game", "BETA-001", "--faction", "1"}, content: "TURN REPORT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "report.txt")
			var stdout bytes.Buffer
			args := append([]string{"--db-path", directory, "show", "--output", outputPath}, test.args...)
			if err := run(context.Background(), args, &stdout); err != nil {
				t.Fatalf("run: %v", err)
			}
			if stdout.Len() != 0 {
				t.Errorf("standard output = %q; want empty", stdout.String())
			}
			report, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if !strings.Contains(string(report), test.content) {
				t.Errorf("output file does not contain %q:\n%s", test.content, report)
			}
		})
	}
}

func TestRunShowTurnValidatesPlayerSelector(t *testing.T) {
	directory := createTestDatabase(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing game", args: []string{"--email", "player@example.com"}, want: "game is required"},
		{name: "missing selector", args: []string{"--game", "BETA-001"}, want: "exactly one"},
		{name: "both selectors", args: []string{"--game", "BETA-001", "--email", "player@example.com", "--faction", "1"}, want: "exactly one"},
		{name: "bad email", args: []string{"--game", "BETA-001", "--email", "not-an-email"}, want: "invalid email"},
		{name: "missing player", args: []string{"--game", "BETA-001", "--email", "missing@example.com"}, want: "does not exist"},
		{name: "faction in other game", args: []string{"--game", "OTHER", "--faction", "1"}, want: "does not exist"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--db-path", directory, "show", "turn"}, test.args...)
			if err := run(context.Background(), args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run error = %v; want %q", err, test.want)
			}
		})
	}
}

func TestRunShowOrdersValidatesPlayerSelector(t *testing.T) {
	directory := createTestDatabase(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing game", args: []string{"--email", "player@example.com"}, want: "game is required"},
		{name: "missing selector", args: []string{"--game", "BETA-001"}, want: "exactly one"},
		{name: "both selectors", args: []string{"--game", "BETA-001", "--email", "player@example.com", "--faction", "1"}, want: "exactly one"},
		{name: "bad email", args: []string{"--game", "BETA-001", "--email", "not-an-email"}, want: "invalid email"},
		{name: "missing player", args: []string{"--game", "BETA-001", "--email", "missing@example.com"}, want: "does not exist"},
		{name: "faction in other game", args: []string{"--game", "OTHER", "--faction", "1"}, want: "does not exist"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"--db-path", directory, "show", "orders"}, test.args...)
			if err := run(context.Background(), args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run error = %v; want %q", err, test.want)
			}
		})
	}
}

func createTestDatabase(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	// Migrate rather than stub the schema by hand. A hand-written stub drifts
	// from the migrations silently, so a report query can pass here and fail
	// against a real database.
	testdb.NewAt(t, filepath.Join(directory, database.Filename))
	conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (id, email, role) VALUES (11, 'player@example.com', 'non-administrator');
		INSERT INTO game (id, code, turn) VALUES (1, 'BETA-001', 3), (2, 'OTHER', 0);
		INSERT INTO agent (id, code, description) VALUES (21, 'uncontrolled', 'Uncontrolled faction');
		INSERT INTO faction (id, game_id, number, user_id, agent_id) VALUES (41, 1, 1, 11, NULL), (42, 1, 2, NULL, 21);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (79, 1, 9, 13, -5);
		INSERT INTO system (id, stellium_id, sequence) VALUES (88, 79, 'A');
		INSERT INTO planet (id, system_id, orbit, kind, habitability, faction_id) VALUES (871, 88, 1, 'rocky', 8, 41), (872, 88, 2, 'asteroid', 0, NULL);
		INSERT INTO deposit (planet_id, sequence, resource, quality, initial_qty, current_qty) VALUES
			(871, 1, 'fuel', 5, 30, 25),
			(871, 2, 'metals', 10, 50, 40),
			(871, 3, 'fuel', 15, 10, 5),
			(872, 1, 'gold', 5, 20, 15);
		INSERT INTO entity (id, game_id, number, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume, mass) VALUES
			(501, 1, 100501, 'SHIP', 2, 79, 88, 871, 64, 41, 100, 200),
			(502, 1, 100502, 'COPN', 1, 79, 88, 871, 0, 41, 300, 400);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(501, 'cargo', 'FUEL', 0, 30), (501, 'cargo', 'METL', 0, 9),
			(502, 'cargo', 'GOLD', 0, 7), (502, 'operational', 'MINE', 1, 4);
		INSERT INTO entity_population (entity_id, class, quantity) VALUES (501, 'SKW', 12), (502, 'USK', 20);
		INSERT INTO work_group (id, entity_id, unit, sequence, deposit_id) VALUES (61, 502, 'MINE', 1, 1);
		INSERT INTO work_group_units (work_group_id, tech_level, quantity) VALUES (61, 1, 4);
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_number, input, params, fuel_spent
		) VALUES
			(1, 3, 41, 1, 4, 'move', 100501, 'orbit 1', '{"orbit":1}', 4),
			(1, 3, 41, 2, 3, 'jump', 100501, '(9,13,-5)', '{"x":9,"y":13,"z":-5}', 0);
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	return directory
}

func TestOpenDatabaseRequiresExistingFile(t *testing.T) {
	if _, err := openDatabase(context.Background(), t.TempDir()); err == nil {
		t.Fatal("openDatabase succeeded; want error")
	}
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, database.Filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := openDatabase(context.Background(), directory); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("openDatabase error = %v; want regular file error", err)
	}
}
