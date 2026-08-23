// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mdhender/ecvb/internal/database"
	"github.com/mdhender/ecvb/internal/dotenv"
	"github.com/peterbourgon/ff/v4"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func main() {
	// EC_ENV selects which dotenv files load, and scopes the credential file.
	// It is read before flag parsing because those files populate the
	// environment ff then reads.
	env, ok := os.LookupEnv("EC_ENV")
	if !ok {
		env = "development"
	}
	// Load also rejects an unknown environment, which is what lets env be used
	// as a path segment in the credential file without further checking.
	if err := dotenv.Load(env); err != nil {
		fmt.Fprintf(os.Stderr, "ec: %v\n", err)
		os.Exit(1)
	}

	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ec: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := ff.NewFlagSet("ec")
	dbPath := flags.StringLong("db-path", "db", "path to the database directory")
	root := &ff.Command{
		Name:      "ec",
		Usage:     "ec [--db-path PATH]",
		ShortHelp: "verify an ECVB database",
		Flags:     flags,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("unexpected arguments: %v", args)
			}
			if *dbPath == "" {
				return fmt.Errorf("db-path is required")
			}
			return verifyDatabase(ctx, *dbPath)
		},
	}

	return root.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("EC"))
}

func verifyDatabase(ctx context.Context, directory string) (err error) {
	path := filepath.Join(directory, database.Filename)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("database %s does not exist", path)
		}
		return fmt.Errorf("stat database %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database %s is not a regular file", path)
	}

	conn, err := sqlite.OpenConn(path, sqlite.OpenReadWrite)
	if err != nil {
		return fmt.Errorf("open database %s: %w", path, err)
	}
	conn.SetInterrupt(ctx.Done())
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	var applicationID, schemaVersion int
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA application_id;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			applicationID = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		return fmt.Errorf("read application id: %w", err)
	}
	if applicationID != database.ApplicationID {
		return fmt.Errorf("invalid application id: got %#x, want %#x", applicationID, database.ApplicationID)
	}

	if err := sqlitex.ExecuteTransient(conn, "PRAGMA user_version;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			schemaVersion = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		return fmt.Errorf("read database version: %w", err)
	}
	if schemaVersion != database.SchemaVersion {
		return fmt.Errorf("invalid database version: got %d, want %d", schemaVersion, database.SchemaVersion)
	}

	return nil
}
