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

func createTestDatabase(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, database.Filename)
	conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`
		PRAGMA application_id = %d;
		PRAGMA user_version = %d;
		CREATE TABLE game (id INTEGER PRIMARY KEY, code TEXT NOT NULL);
		CREATE TABLE stellium (id INTEGER PRIMARY KEY, game_id INTEGER NOT NULL, x INTEGER NOT NULL, y INTEGER NOT NULL, z INTEGER NOT NULL);
		CREATE TABLE system (id INTEGER PRIMARY KEY, stellium_id INTEGER NOT NULL, sequence TEXT NOT NULL);
		CREATE TABLE planet (id INTEGER PRIMARY KEY, system_id INTEGER NOT NULL, orbit INTEGER NOT NULL, kind TEXT NOT NULL, habitability INTEGER NOT NULL);
		CREATE TABLE deposit (id INTEGER PRIMARY KEY, planet_id INTEGER NOT NULL, sequence INTEGER NOT NULL, resource TEXT NOT NULL, quality INTEGER NOT NULL, initial_qty INTEGER NOT NULL, current_qty INTEGER NOT NULL);
		INSERT INTO game (id, code) VALUES (1, 'BETA-001');
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (79, 1, 9, 13, -5);
		INSERT INTO system (id, stellium_id, sequence) VALUES (88, 79, 'A');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES (871, 88, 1, 'rocky', 8), (872, 88, 2, 'asteroid', 0);
		INSERT INTO deposit (planet_id, sequence, resource, quality, initial_qty, current_qty) VALUES
			(871, 1, 'fuel', 5, 30, 25),
			(871, 2, 'metals', 10, 50, 40),
			(871, 3, 'fuel', 15, 10, 5),
			(872, 1, 'gold', 5, 20, 15);
	`, database.ApplicationID, database.SchemaVersion)
	if err := sqlitex.ExecuteScript(conn, script, nil); err != nil {
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
