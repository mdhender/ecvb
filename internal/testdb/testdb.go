// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package testdb builds databases for tests.
//
// Every database it returns is created in the test's own temporary directory
// and migrated with database.Migrations(), so a test always runs against the
// same schema the commands run against. Tests that stub the schema by hand
// drift from the migrations without anything failing to say so.
package testdb

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// New returns a connection to a freshly migrated, empty database. Foreign keys
// are enforced, and the connection closes when the test finishes.
func New(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn, err := sqlite.OpenConn(filepath.Join(t.TempDir(), database.Filename), sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	for i, migration := range database.Migrations() {
		if err := sqlitex.ExecuteScript(conn, migration, nil); err != nil {
			t.Fatalf("apply migration %d: %v", i+1, err)
		}
	}
	return conn
}

// Exec runs a SQL script, failing the test if any statement does. Use it to
// load fixture rows into a database from New.
func Exec(t *testing.T, conn *sqlite.Conn, script string) {
	t.Helper()
	if err := sqlitex.ExecuteScript(conn, script, nil); err != nil {
		t.Fatalf("execute fixture script: %v", err)
	}
}

// Tables returns every table the migrations create, in alphabetical order and
// excluding SQLite's own bookkeeping tables. Tests assert against this rather
// than against a hand-maintained list, so adding a table does not fail an
// unrelated test.
func Tables(t *testing.T, conn *sqlite.Conn) []string {
	t.Helper()
	var names []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT name FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			names = append(names, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return names
}

// NewAt migrates a database at path, stamping the application ID and schema
// version the commands verify, and returns the directory holding it. Use it
// when the code under test opens a database by path rather than taking a
// connection.
func NewAt(t *testing.T, path string) {
	t.Helper()
	conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite|sqlite.OpenCreate)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	}()
	if err := sqlitex.ExecuteScript(conn, fmt.Sprintf(
		"PRAGMA application_id = %d;\nPRAGMA user_version = %d;",
		database.ApplicationID, database.SchemaVersion), nil); err != nil {
		t.Fatalf("stamp test database: %v", err)
	}
	for i, migration := range database.Migrations() {
		if err := sqlitex.ExecuteScript(conn, migration, nil); err != nil {
			t.Fatalf("apply migration %d: %v", i+1, err)
		}
	}
}
