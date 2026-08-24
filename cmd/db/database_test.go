// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
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

func TestSchemaVersion(t *testing.T) {
	if got := latestSchemaVersion(); got != database.SchemaVersion {
		t.Fatalf("migration count = %d; database schema version = %d", got, database.SchemaVersion)
	}
}

func TestCreateDatabase(t *testing.T) {
	directory := t.TempDir()
	if err := createDatabase(context.Background(), directory); err != nil {
		t.Fatalf("createDatabase: %v", err)
	}

	path := filepath.Join(directory, databaseName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("database mode = %v; want regular file", info.Mode())
	}

	conn := openTestDatabase(t, path)
	if err := verifyDatabase(conn); err != nil {
		t.Errorf("verifyDatabase: %v", err)
	}

	wantTables := []string{
		"agent", "deposit", "entity", "faction", "game", "inventory",
		"order_entry", "planet", "stellium", "system", "users", "work_group",
		"work_group_units",
	}
	var gotTables []string
	if err := sqlitex.ExecuteTransient(conn, strings.TrimSpace(`
		SELECT name FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name;
	`), &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			gotTables = append(gotTables, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if strings.Join(gotTables, ",") != strings.Join(wantTables, ",") {
		t.Errorf("tables = %v; want %v", gotTables, wantTables)
	}

	var users int
	if err := sqlitex.ExecuteTransient(conn, "SELECT count(*) FROM users;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			users = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 0 {
		t.Errorf("users after create = %d; want 0", users)
	}

	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO game (code) VALUES ('DEFAULT-SEED');", nil); err != nil {
		t.Fatalf("insert game with default seed: %v", err)
	}
	var seedHigh, seedLow int64
	if err := sqlitex.ExecuteTransient(conn, "SELECT seed_high, seed_low FROM game WHERE code = 'DEFAULT-SEED';", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			seedHigh, seedLow = stmt.ColumnInt64(0), stmt.ColumnInt64(1)
			return nil
		},
	}); err != nil {
		t.Fatalf("read default game seed: %v", err)
	}
	if seedHigh != 19 || seedLow != 12 {
		t.Errorf("default game seed = (%d, %d); want (19, 12)", seedHigh, seedLow)
	}

	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO users (email, role) VALUES ('player@example.com', 'non-administrator');", nil); err != nil {
		t.Fatalf("insert player: %v", err)
	}
	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO faction (game_id, user_id) VALUES (1, 1);", nil); err != nil {
		t.Fatalf("insert player faction: %v", err)
	}
	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO faction (game_id, user_id) VALUES (1, 1);", nil); err == nil {
		t.Fatal("insert duplicate player faction succeeded; want unique constraint error")
	}
}

func TestCreateDatabaseRejectsInvalidPaths(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "missing")
		if err := createDatabase(context.Background(), directory); err == nil {
			t.Fatal("createDatabase succeeded; want error")
		}
		if _, err := os.Stat(directory); !os.IsNotExist(err) {
			t.Fatalf("missing directory was created or stat failed: %v", err)
		}
	})

	t.Run("path is file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := createDatabase(context.Background(), path); err == nil {
			t.Fatal("createDatabase succeeded; want error")
		}
	})

	t.Run("database exists", func(t *testing.T) {
		directory := t.TempDir()
		if err := createDatabase(context.Background(), directory); err != nil {
			t.Fatal(err)
		}
		if err := createDatabase(context.Background(), directory); err == nil {
			t.Fatal("second createDatabase succeeded; want error")
		}
	})
}

func TestSeedDatabase(t *testing.T) {
	directory := t.TempDir()
	if err := createDatabase(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := seedDatabase(context.Background(), directory); err != nil {
			t.Fatalf("seedDatabase: %v", err)
		}
	}

	conn := openTestDatabase(t, filepath.Join(directory, databaseName))
	var count, administrators int
	if err := sqlitex.ExecuteTransient(conn, strings.TrimSpace(`
		SELECT count(*), sum(role = 'administrator') FROM users;
	`), &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			administrators = stmt.ColumnInt(1)
			return nil
		},
	}); err != nil {
		t.Fatalf("query users: %v", err)
	}
	if count != 11 || administrators != 1 {
		t.Errorf("users = %d, administrators = %d; want 11, 1", count, administrators)
	}

	var adminRole, lastUserRole string
	if err := sqlitex.ExecuteTransient(conn, strings.TrimSpace(`
		SELECT
			(SELECT role FROM users WHERE email = 'admin@example.com'),
			(SELECT role FROM users WHERE email = 'user10@example.com');
	`), &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			adminRole = stmt.ColumnText(0)
			lastUserRole = stmt.ColumnText(1)
			return nil
		},
	}); err != nil {
		t.Fatalf("query seeded roles: %v", err)
	}
	if adminRole != "administrator" || lastUserRole != "non-administrator" {
		t.Errorf("roles = %q, %q", adminRole, lastUserRole)
	}
}

func TestSeedDatabaseRequiresExistingDatabase(t *testing.T) {
	if err := seedDatabase(context.Background(), t.TempDir()); err == nil {
		t.Fatal("seedDatabase succeeded; want error")
	}
}

func TestMigrationsAreRepeatable(t *testing.T) {
	directory := t.TempDir()
	if err := createDatabase(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	conn := openTestDatabase(t, filepath.Join(directory, databaseName))
	if err := applyMigrations(context.Background(), conn); err != nil {
		t.Fatalf("applyMigrations: %v", err)
	}

	version, err := databaseVersion(conn)
	if err != nil {
		t.Fatal(err)
	}
	if version != latestSchemaVersion() {
		t.Errorf("database version = %d; want %d", version, latestSchemaVersion())
	}
}

func TestMigrateUp(t *testing.T) {
	t.Run("current version", func(t *testing.T) {
		directory := t.TempDir()
		if err := createDatabase(context.Background(), directory); err != nil {
			t.Fatal(err)
		}
		if err := run(context.Background(), []string{"migrate", "up", directory}); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
	})

	t.Run("older version", func(t *testing.T) {
		directory := t.TempDir()
		if err := createDatabase(context.Background(), directory); err != nil {
			t.Fatal(err)
		}

		original := migrationSchema.Migrations
		migrationSchema.Migrations = append(migrationSchema.Migrations,
			"CREATE TABLE migration_test (id INTEGER PRIMARY KEY);")
		t.Cleanup(func() { migrationSchema.Migrations = original })

		if err := migrateUp(context.Background(), directory); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		conn := openTestDatabase(t, filepath.Join(directory, databaseName))
		version, err := databaseVersion(conn)
		if err != nil {
			t.Fatal(err)
		}
		if version != latestSchemaVersion() {
			t.Errorf("schema version = %d; want %d", version, latestSchemaVersion())
		}
	})

	t.Run("newer version", func(t *testing.T) {
		directory := t.TempDir()
		if err := createDatabase(context.Background(), directory); err != nil {
			t.Fatal(err)
		}
		conn := openTestDatabase(t, filepath.Join(directory, databaseName))
		if err := sqlitex.ExecuteTransient(conn,
			fmt.Sprintf("PRAGMA user_version = %d;", latestSchemaVersion()+1), nil); err != nil {
			t.Fatal(err)
		}

		err := migrateUp(context.Background(), directory)
		if err == nil {
			t.Fatal("migrate up succeeded; want error")
		}
		if !strings.Contains(err.Error(), "newer than supported") {
			t.Fatalf("migrate up error = %q", err)
		}
	})

	t.Run("invalid marker", func(t *testing.T) {
		directory := t.TempDir()
		if err := createDatabase(context.Background(), directory); err != nil {
			t.Fatal(err)
		}
		conn := openTestDatabase(t, filepath.Join(directory, databaseName))
		if err := sqlitex.ExecuteTransient(conn, "PRAGMA application_id = 0;", nil); err != nil {
			t.Fatal(err)
		}

		err := migrateUp(context.Background(), directory)
		if err == nil {
			t.Fatal("migrate up succeeded; want error")
		}
		if !strings.Contains(err.Error(), "invalid application marker") {
			t.Fatalf("migrate up error = %q", err)
		}
	})
}

func openTestDatabase(t *testing.T, path string) *sqlite.Conn {
	t.Helper()
	conn, err := openDatabase(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return conn
}
