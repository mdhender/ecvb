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

	if err := run(context.Background(), []string{"db", "verify"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run with default db-path: %v", err)
	}

	if err := os.Setenv("EC_DB_PATH", "beta"); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"db", "verify"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run with EC_DB_PATH: %v", err)
	}

	if err := run(context.Background(), []string{"--db-path", "db", "db", "verify"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run with command-line db-path: %v", err)
	}
}

func TestRunDBVerifyOutput(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "game")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)

	var stderr bytes.Buffer
	if err := run(context.Background(), []string{"--db-path", directory, "db", "verify"}, &stderr); err != nil {
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
	err := run(context.Background(), []string{"--db-path", directory, "db", "verify"}, &stderr)
	if err == nil {
		t.Fatal("run db verify succeeded; want error")
	}
	want := fmt.Sprintf("database path: %s\ndatabase name: %s\n", directory, database.Filename)
	if got := stderr.String(); got != want {
		t.Fatalf("stderr = %q; want %q", got, want)
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
