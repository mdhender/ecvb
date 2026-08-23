// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const databaseName = database.Filename

const applicationID = database.ApplicationID

func createDatabase(ctx context.Context, directory string) (err error) {
	if err := requireDirectory(directory); err != nil {
		return err
	}

	path := filepath.Join(directory, databaseName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create database: %s already exists", path)
		}
		return fmt.Errorf("create database %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("create database %s: %w", path, err)
	}

	created := false
	defer func() {
		if !created {
			_ = os.Remove(path)
			_ = os.Remove(path + "-journal")
			_ = os.Remove(path + "-shm")
			_ = os.Remove(path + "-wal")
		}
	}()

	conn, err := openDatabase(ctx, path)
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	defer func() {
		closeErr := conn.Close()
		if err == nil {
			if closeErr != nil {
				err = fmt.Errorf("close database: %w", closeErr)
			} else {
				created = true
			}
		}
	}()

	if err := applyMigrations(ctx, conn); err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	return nil
}

func seedDatabase(ctx context.Context, directory string) (err error) {
	path, err := existingDatabasePath(directory)
	if err != nil {
		return fmt.Errorf("seed database: %w", err)
	}

	conn, err := openDatabase(ctx, path)
	if err != nil {
		return fmt.Errorf("seed database: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	if err := verifyDatabase(conn); err != nil {
		return fmt.Errorf("seed database: %w", err)
	}
	return seedUsers(conn)
}

func migrateUp(ctx context.Context, directory string) (err error) {
	path, err := existingDatabasePath(directory)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	conn, err := openDatabase(ctx, path)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	if err := verifyApplicationMarker(conn); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	current, err := databaseVersion(conn)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	want := latestSchemaVersion()
	if current > want {
		return fmt.Errorf("migrate database: schema version %d is newer than supported version %d", current, want)
	}
	if current == want {
		return nil
	}
	if err := applyMigrations(ctx, conn); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	return nil
}

func existingDatabasePath(directory string) (string, error) {
	if err := requireDirectory(directory); err != nil {
		return "", err
	}

	path := filepath.Join(directory, databaseName)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s does not exist", path)
		}
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", path)
	}
	return path, nil
}

func requireDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database directory %s does not exist", path)
		}
		return fmt.Errorf("stat database directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("database directory %s is not a directory", path)
	}
	return nil
}

func openDatabase(ctx context.Context, path string) (*sqlite.Conn, error) {
	conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	conn.SetInterrupt(ctx.Done())
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	return conn, nil
}

func verifyDatabase(conn *sqlite.Conn) error {
	if err := verifyApplicationMarker(conn); err != nil {
		return err
	}

	version, err := databaseVersion(conn)
	if err != nil {
		return err
	}
	if version != latestSchemaVersion() {
		return fmt.Errorf("database is not fully migrated")
	}
	return nil
}

func verifyApplicationMarker(conn *sqlite.Conn) error {
	var id int64
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA application_id;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return fmt.Errorf("read application marker: %w", err)
	}
	if id != applicationID {
		return fmt.Errorf("invalid application marker")
	}
	return nil
}

func databaseVersion(conn *sqlite.Conn) (int, error) {
	var version int
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA user_version;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			version = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("read database version: %w", err)
	}
	return version, nil
}

func seedUsers(conn *sqlite.Conn) (err error) {
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer end(&err)

	users := []struct {
		email string
		role  string
	}{
		{email: "admin@example.com", role: "administrator"},
	}
	for n := 1; n <= 10; n++ {
		users = append(users, struct {
			email string
			role  string
		}{
			email: fmt.Sprintf("user%02d@example.com", n),
			role:  "non-administrator",
		})
	}

	for _, user := range users {
		if err := sqlitex.ExecuteTransient(conn, strings.TrimSpace(`
			INSERT INTO users (email, role) VALUES (?, ?)
			ON CONFLICT (email) DO UPDATE SET role = excluded.role;
		`), &sqlitex.ExecOptions{Args: []any{user.email, user.role}}); err != nil {
			return fmt.Errorf("seed user %s: %w", user.email, err)
		}
	}
	return nil
}
