// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestRunDatabasePathSources(t *testing.T) {
	root := t.TempDir()
	createTestDatabase(t, filepath.Join(root, "db"), database.ApplicationID, database.SchemaVersion)
	createTestDatabase(t, filepath.Join(root, "beta"), database.ApplicationID, database.SchemaVersion)

	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	oldDBPath, hadDBPath := os.LookupEnv("EC_DB_PATH")
	if err := os.Unsetenv("EC_DB_PATH"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadDBPath {
			_ = os.Setenv("EC_DB_PATH", oldDBPath)
		} else {
			_ = os.Unsetenv("EC_DB_PATH")
		}
	})

	if err := run(context.Background(), []string{"db", "verify"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run with default db-path: %v", err)
	}

	if err := os.Setenv("EC_DB_PATH", "beta"); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"db", "verify"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run with EC_DB_PATH: %v", err)
	}

	if err := run(context.Background(), []string{"--db-path", "db", "db", "verify"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run with command-line db-path: %v", err)
	}
}

func TestRunDBVerifyOutput(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "game")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)

	var stderr bytes.Buffer
	if err := run(context.Background(), []string{"--db-path", directory, "db", "verify"}, &bytes.Buffer{}, &stderr); err != nil {
		t.Fatalf("run db verify: %v", err)
	}
	want := fmt.Sprintf("database path: %s\ndatabase name: %s\ndatabase version: %d\n",
		directory, database.Filename, database.SchemaVersion)
	if got := stderr.String(); got != want {
		t.Fatalf("stderr = %q; want %q", got, want)
	}
}

func TestRunDBVerifyInvalidDatabaseOmitsVersion(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "game")
	createTestDatabase(t, directory, 0, database.SchemaVersion)

	var stderr bytes.Buffer
	err := run(context.Background(), []string{"--db-path", directory, "db", "verify"}, &bytes.Buffer{}, &stderr)
	if err == nil {
		t.Fatal("run db verify succeeded; want error")
	}
	want := fmt.Sprintf("database path: %s\ndatabase name: %s\n", directory, database.Filename)
	if got := stderr.String(); got != want {
		t.Fatalf("stderr = %q; want %q", got, want)
	}
}

func TestRunGameCreate(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	seed := filepath.Join(t.TempDir(), "game.json")
	if err := os.WriteFile(seed, []byte(`{"code":"BETA-001"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(context.Background(), []string{"--db-path", directory, "game", "create", "--game-seed", seed}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run game create: %v", err)
	}

	conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var code string
	var turn int
	var seedHigh, seedLow int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT code, turn, seed_high, seed_low FROM game;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			code = stmt.ColumnText(0)
			turn = stmt.ColumnInt(1)
			seedHigh = stmt.ColumnInt64(2)
			seedLow = stmt.ColumnInt64(3)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if code != "BETA-001" || turn != 0 || seedHigh != 19 || seedLow != 12 {
		t.Fatalf("game = (%q, %d, %d, %d); want (%q, 0, 19, 12)", code, turn, seedHigh, seedLow, "BETA-001")
	}
}

func TestRunGameCreateWithExplicitSeed(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	seed := filepath.Join(t.TempDir(), "game.json")
	if err := os.WriteFile(seed, []byte(`{"code":"SEEDED"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := run(context.Background(), []string{"--db-path", directory, "game", "create", "--game-seed", seed, "--seed-high", "41", "--seed-low", "73"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run game create: %v", err)
	}
	conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var high, low int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT seed_high, seed_low FROM game WHERE code = 'SEEDED';", &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
		high, low = stmt.ColumnInt64(0), stmt.ColumnInt64(1)
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if high != 41 || low != 73 {
		t.Fatalf("seed = (%d, %d); want (41, 73)", high, low)
	}
}

func TestRunGameCreateUsesDefaultSeedPath(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "db")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	if err := os.WriteFile(filepath.Join(directory, "game-seed.json"), []byte(`{"code":"DEFAULT"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })

	if err := run(context.Background(), []string{"game", "create"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run game create with default seed: %v", err)
	}
}

func TestRunResolveAndOpenTurn(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (id, email, role) VALUES (1, 'player@example.com', 'non-administrator');
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 0);
		INSERT INTO faction (id, game_id, user_id) VALUES (1, 1, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (10, 1, 0, 0, 0), (11, 1, 1, 2, 3);
		INSERT INTO system (id, stellium_id, sequence) VALUES (20, 10, 'A');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES (30, 20, 4, 'rocky', 10);
		INSERT INTO entity (
			id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume
		) VALUES (40, 'SHIP', 1, 10, 20, 30, 64, 1, 100);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(40, 'component', 'HDRV', 4, 1), (40, 'cargo', 'FUEL', 0, 500);
		UPDATE entity SET mass = 4000 WHERE id = 40;
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES (1, 0, 1, 1, 3, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}');
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{
		"--db-path", directory, "turn", "resolve", "--game", "TEST", "--turn", "0",
	}, &stdout, &stderr); err != nil {
		t.Fatalf("resolve turn: %v", err)
	}
	if !strings.Contains(stdout.String(), "1 orders, 1 succeeded, 0 failed") {
		t.Fatalf("resolve output = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "order_type=jump") || !strings.Contains(stderr.String(), "status=succeeded") {
		t.Fatalf("resolve log = %q", stderr.String())
	}

	stdout.Reset()
	if err := run(context.Background(), []string{
		"--db-path", directory, "turn", "open", "--game", "TEST", "--turn", "0",
	}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("open turn: %v", err)
	}
	if got := stdout.String(); got != "opened game TEST turn 1\n" {
		t.Fatalf("open output = %q", got)
	}
}

func TestCreateGameRejectsInvalidSeedAndDuplicateCode(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)

	tests := []struct {
		name    string
		content string
		pathDir bool
		want    string
	}{
		{name: "invalid JSON", content: `{`, want: "parse game seed"},
		{name: "missing code", content: `{}`, want: "code must be uppercase"},
		{name: "lowercase code", content: `{"code":"beta"}`, want: "code must be uppercase"},
		{name: "path is directory", pathDir: true, want: "not a regular file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "seed")
			if tt.pathDir {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := createGame(context.Background(), directory, path, 19, 12)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("createGame error = %v; want containing %q", err, tt.want)
			}
		})
	}

	seed := filepath.Join(t.TempDir(), "game.json")
	if err := os.WriteFile(seed, []byte(`{"code":"DUPLICATE"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := createGame(context.Background(), directory, seed, 19, 12); err != nil {
		t.Fatalf("first createGame: %v", err)
	}
	if err := createGame(context.Background(), directory, seed, 19, 12); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate createGame error = %v; want duplicate error", err)
	}
}

func TestRunLoadGame(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	writeTestGameSeeds(t, directory)

	conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO game (code) VALUES ('BETA-001');", nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	if err := run(context.Background(), []string{"--db-path", directory, "load", "game", "BETA-001"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run load game: %v", err)
	}

	conn, err = sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for table, want := range map[string]int{"stellium": 1, "system": 1, "planet": 1, "deposit": 1} {
		var got int
		if err := sqlitex.ExecuteTransient(conn, "SELECT COUNT(*) FROM "+table+";", &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
			got = stmt.ColumnInt(0)
			return nil
		}}); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s count = %d; want %d", table, got, want)
		}
	}
}

func TestLoadGameRejectsMissingFilesUnknownGameAndLoadedGame(t *testing.T) {
	t.Run("missing seed file", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "database")
		createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
		err := loadGame(context.Background(), directory, "BETA-001")
		if err == nil || !strings.Contains(err.Error(), "stellia-seed.json does not exist") {
			t.Fatalf("loadGame error = %v; want missing seed file", err)
		}
	})

	t.Run("unknown and loaded game", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "database")
		createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
		writeTestGameSeeds(t, directory)
		if err := loadGame(context.Background(), directory, "UNKNOWN"); err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("unknown loadGame error = %v; want game not found", err)
		}

		conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadWrite)
		if err != nil {
			t.Fatal(err)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO game (code) VALUES ('LOADED');", nil); err != nil {
			t.Fatal(err)
		}
		gameID := conn.LastInsertRowID()
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO stellium (game_id, x, y, z) VALUES (?, 1, 1, 1);", &sqlitex.ExecOptions{Args: []any{gameID}}); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
		if err := loadGame(context.Background(), directory, "LOADED"); err == nil || !strings.Contains(err.Error(), "already has data loaded") {
			t.Fatalf("loaded loadGame error = %v; want already loaded", err)
		}
	})
}

func writeTestGameSeeds(t *testing.T, directory string) {
	t.Helper()
	files := map[string]string{
		"stellia-seed.json":  `{"stellia":[{"uuid":"st-1","x":1,"y":2,"z":3}]}`,
		"systems-seed.json":  `{"systems":[{"uuid":"sy-1","stellium-uuid":"st-1","sequence":"A"}]}`,
		"planets-seed.json":  `{"planets":[{"uuid":"pl-1","system-uuid":"sy-1","orbit":1,"type":"asteroid","habitability":0}]}`,
		"deposits-seed.json": `{"deposits":[{"planet-uuid":"pl-1","sequence":1,"resource":"metals","quantity":100,"quality":5}]}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerifyDatabase(t *testing.T) {
	tests := []struct {
		name          string
		applicationID int
		version       int
		wantError     string
	}{
		{name: "valid", applicationID: database.ApplicationID, version: database.SchemaVersion},
		{name: "wrong application id", applicationID: 0, version: database.SchemaVersion, wantError: "invalid application id"},
		{name: "wrong version", applicationID: database.ApplicationID, version: database.SchemaVersion + 1, wantError: "invalid database version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "game")
			createTestDatabase(t, directory, tt.applicationID, tt.version)

			_, err := verifyDatabase(context.Background(), directory)
			if tt.wantError == "" && err != nil {
				t.Fatalf("verifyDatabase: %v", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("verifyDatabase error = %v; want containing %q", err, tt.wantError)
			}
		})
	}
}

func TestVerifyDatabaseRequiresExistingFile(t *testing.T) {
	if _, err := verifyDatabase(context.Background(), t.TempDir()); err == nil {
		t.Fatal("verifyDatabase succeeded; want error")
	}
}

func createTestDatabase(t *testing.T, directory string, applicationID, version int) {
	t.Helper()
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, database.Filename)
	conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range database.Migrations() {
		if err := sqlitex.ExecuteScript(conn, migration, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqlitex.ExecuteTransient(conn, fmt.Sprintf("PRAGMA application_id = %d;", applicationID), nil); err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecuteTransient(conn, fmt.Sprintf("PRAGMA user_version = %d;", version), nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}
