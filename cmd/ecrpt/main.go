// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"

	"github.com/mdhender/ecvb/internal/database"
	"github.com/peterbourgon/ff/v4"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ecrpt: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	flags := ff.NewFlagSet("ecrpt")
	dbPath := flags.StringLong("db-path", "db", "path to the database directory")
	root := &ff.Command{
		Name:      "ecrpt",
		Usage:     "ecrpt [--db-path PATH] SUBCOMMAND",
		ShortHelp: "print ECVB database data",
		Flags:     flags,
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a subcommand is required (show)")
		},
	}
	show := &ff.Command{
		Name:      "show",
		Usage:     "ecrpt show SUBCOMMAND",
		ShortHelp: "show database data",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a show subcommand is required (stellium)")
		},
	}
	show.Subcommands = []*ff.Command{
		{
			Name:      "stellium",
			Usage:     "ecrpt show stellium <id>",
			ShortHelp: "show a stellium and its contents",
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one stellium id")
				}
				id, err := strconv.ParseInt(args[0], 10, 64)
				if err != nil || id < 1 {
					return fmt.Errorf("invalid stellium id %q", args[0])
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				return showStellium(ctx, *dbPath, id, stdout)
			},
		},
	}
	root.Subcommands = []*ff.Command{show}
	return root.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("EC"))
}

func showStellium(ctx context.Context, directory string, id int64, output io.Writer) (err error) {
	conn, err := openDatabase(ctx, directory)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	w := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	found := false
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT s.id, g.code, s.x, s.y, s.z
		FROM stellium AS s
		JOIN game AS g ON g.id = s.game_id
		WHERE s.id = ?;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			fmt.Fprintln(w, "STELLIUM")
			fmt.Fprintln(w, "ID\tGAME\tX\tY\tZ")
			fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%d\n", stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), stmt.ColumnInt(3), stmt.ColumnInt(4))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query stellium %d: %w", id, err)
	}
	if !found {
		return fmt.Errorf("stellium %d does not exist", id)
	}

	fmt.Fprintln(w, "\nSYSTEMS")
	fmt.Fprintln(w, "ID\tSEQUENCE\tPLANETS\tDEPOSITS")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT sy.id, sy.sequence, COUNT(DISTINCT p.id), COUNT(d.id)
		FROM system AS sy
		LEFT JOIN planet AS p ON p.system_id = sy.id
		LEFT JOIN deposit AS d ON d.planet_id = p.id
		WHERE sy.stellium_id = ?
		GROUP BY sy.id, sy.sequence
		ORDER BY sy.sequence;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			fmt.Fprintf(w, "%d\t%s\t%d\t%d\n", stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), stmt.ColumnInt(3))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query systems in stellium %d: %w", id, err)
	}

	fmt.Fprintln(w, "\nPLANETS")
	fmt.Fprintln(w, "SYSTEM\tID\tORBIT\tKIND\tHABITABILITY\tDEPOSITS")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT sy.sequence, p.id, p.orbit, p.kind, p.habitability, COUNT(d.id)
		FROM system AS sy
		JOIN planet AS p ON p.system_id = sy.id
		LEFT JOIN deposit AS d ON d.planet_id = p.id
		WHERE sy.stellium_id = ?
		GROUP BY sy.sequence, p.id, p.orbit, p.kind, p.habitability
		ORDER BY sy.sequence, p.orbit;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%d\t%d\n", stmt.ColumnText(0), stmt.ColumnInt64(1), stmt.ColumnInt(2), stmt.ColumnText(3), stmt.ColumnInt(4), stmt.ColumnInt(5))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query planets in stellium %d: %w", id, err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("write stellium: %w", err)
	}
	return nil
}

func openDatabase(ctx context.Context, directory string) (*sqlite.Conn, error) {
	path := filepath.Join(directory, database.Filename)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("database %s does not exist", path)
		}
		return nil, fmt.Errorf("stat database %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database %s is not a regular file", path)
	}
	conn, err := sqlite.OpenConn(path, sqlite.OpenReadOnly)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	conn.SetInterrupt(ctx.Done())
	return conn, nil
}
