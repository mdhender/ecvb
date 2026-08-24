// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
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
	showFlags := ff.NewFlagSet("show")
	showOutput := showFlags.StringLong("output", "", "write the report to a file instead of standard output")
	show := &ff.Command{
		Name:      "show",
		Usage:     "ecrpt show [--output PATH] SUBCOMMAND",
		ShortHelp: "show database data",
		Flags:     showFlags,
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a show subcommand is required (orders, stellium, system, or turn)")
		},
	}
	systemFlags := ff.NewFlagSet("show system")
	showDeposits := systemFlags.BoolLong("show-deposits", "show every deposit on each planet")
	ordersFlags := ff.NewFlagSet("show orders")
	ordersGame := ordersFlags.StringLong("game", "", "code of the game")
	ordersEmail := ordersFlags.StringLong("email", "", "email address of the player")
	ordersFaction := ordersFlags.Int64Long("faction", 0, "id of the player's faction")
	ordersTurn := ordersFlags.IntLong("turn", -1, "turn to report; defaults to the game's current turn")
	turnFlags := ff.NewFlagSet("show turn")
	turnGame := turnFlags.StringLong("game", "", "code of the game")
	turnEmail := turnFlags.StringLong("email", "", "email address of the player")
	turnFaction := turnFlags.Int64Long("faction", 0, "id of the player's faction")
	turnShowDeposits := turnFlags.BoolLong("show-deposits", "show deposits on controlled planets")
	turnSummarizeResources := turnFlags.BoolLong("summarize-resources", "summarize resources across the player's inventory")
	turnWorkGroups := turnFlags.BoolLong("work-groups", "show the player's work groups")
	show.Subcommands = []*ff.Command{
		{
			Name:      "orders",
			Usage:     "ecrpt show orders --game CODE (--email EMAIL | --faction ID) [--turn NUMBER]",
			ShortHelp: "show a faction's submitted orders",
			Flags:     ordersFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				email, err := normalizeFactionSelector(*ordersGame, *ordersEmail, *ordersFaction)
				if err != nil {
					return err
				}
				if *ordersTurn < -1 {
					return fmt.Errorf("turn must be nonnegative")
				}
				return writeReport(*showOutput, stdout, func(output io.Writer) error {
					return showOrdersReport(ctx, *dbPath, *ordersGame, email, *ordersFaction, *ordersTurn, output)
				})
			},
		},
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
				return writeReport(*showOutput, stdout, func(output io.Writer) error {
					return showStellium(ctx, *dbPath, id, output)
				})
			},
		},
		{
			Name:      "system",
			Usage:     "ecrpt show system [--show-deposits] <id>",
			ShortHelp: "show a system and its planets",
			Flags:     systemFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one system id")
				}
				id, err := strconv.ParseInt(args[0], 10, 64)
				if err != nil || id < 1 {
					return fmt.Errorf("invalid system id %q", args[0])
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				return writeReport(*showOutput, stdout, func(output io.Writer) error {
					return showSystem(ctx, *dbPath, id, *showDeposits, output)
				})
			},
		},
		{
			Name:      "turn",
			Usage:     "ecrpt show turn --game CODE (--email EMAIL | --faction ID) [--show-deposits] [--summarize-resources] [--work-groups]",
			ShortHelp: "show a player's turn report",
			Flags:     turnFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				email, err := normalizeFactionSelector(*turnGame, *turnEmail, *turnFaction)
				if err != nil {
					return err
				}
				options := turnReportOptions{
					showDeposits:       *turnShowDeposits,
					summarizeResources: *turnSummarizeResources,
					showWorkGroups:     *turnWorkGroups,
				}
				return writeReport(*showOutput, stdout, func(output io.Writer) error {
					return showTurnReport(ctx, *dbPath, *turnGame, email, *turnFaction, options, output)
				})
			},
		},
	}
	root.Subcommands = []*ff.Command{show}
	return root.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("EC"))
}

func writeReport(outputPath string, stdout io.Writer, render func(io.Writer) error) error {
	if outputPath == "" {
		return render(stdout)
	}
	var report bytes.Buffer
	if err := render(&report); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, report.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write report to %s: %w", outputPath, err)
	}
	return nil
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

type systemPlanet struct {
	id           int64
	orbit        int
	kind         string
	habitability int
	summary      []depositSummary
}

type depositSummary struct {
	resource string
	quantity int64
}

func showSystem(ctx context.Context, directory string, id int64, showDeposits bool, output io.Writer) (err error) {
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
		SELECT sy.id, sy.stellium_id, sy.sequence
		FROM system AS sy
		WHERE sy.id = ?;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			fmt.Fprintln(w, "SYSTEM")
			fmt.Fprintln(w, "ID\tSTELLIUM\tSEQUENCE")
			fmt.Fprintf(w, "%d\t%d\t%s\n", stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnText(2))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query system %d: %w", id, err)
	}
	if !found {
		return fmt.Errorf("system %d does not exist", id)
	}

	var planets []systemPlanet
	planetIndexByID := make(map[int64]int)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT id, orbit, kind, habitability
		FROM planet
		WHERE system_id = ?
		ORDER BY orbit;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planets = append(planets, systemPlanet{
				id:           stmt.ColumnInt64(0),
				orbit:        stmt.ColumnInt(1),
				kind:         stmt.ColumnText(2),
				habitability: stmt.ColumnInt(3),
			})
			planetIndexByID[planets[len(planets)-1].id] = len(planets) - 1
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query planets in system %d: %w", id, err)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT d.planet_id, d.resource, SUM(d.current_qty)
		FROM deposit AS d
		JOIN planet AS p ON p.id = d.planet_id
		WHERE p.system_id = ?
		GROUP BY d.planet_id, d.resource
		ORDER BY d.planet_id, d.resource;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planetIndex := planetIndexByID[stmt.ColumnInt64(0)]
			planets[planetIndex].summary = append(planets[planetIndex].summary, depositSummary{
				resource: stmt.ColumnText(1),
				quantity: stmt.ColumnInt64(2),
			})
			return nil
		},
	}); err != nil {
		return fmt.Errorf("summarize deposits in system %d: %w", id, err)
	}

	fmt.Fprintln(w, "\nPLANETS")
	fmt.Fprintln(w, "ID\tORBIT\tKIND\tHABITABILITY\tDEPOSITS (CURRENT QUANTITY)")
	for _, planet := range planets {
		fmt.Fprintf(w, "%d\t%d\t%s\t%d\t", planet.id, planet.orbit, planet.kind, planet.habitability)
		if len(planet.summary) == 0 {
			fmt.Fprintln(w, "none")
			continue
		}
		for i, summary := range planet.summary {
			if i != 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%s=%d", summary.resource, summary.quantity)
		}
		fmt.Fprintln(w)
	}

	if showDeposits {
		fmt.Fprintln(w, "\nDEPOSITS")
		fmt.Fprintln(w, "PLANET\tORBIT\tSEQUENCE\tRESOURCE\tQUALITY\tINITIAL QUANTITY\tCURRENT QUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT p.id, p.orbit, d.sequence, d.resource, d.quality, d.initial_qty, d.current_qty
			FROM planet AS p
			JOIN deposit AS d ON d.planet_id = p.id
			WHERE p.system_id = ?
			ORDER BY p.orbit, d.sequence;`, &sqlitex.ExecOptions{
			Args: []any{id},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				fmt.Fprintf(w, "%d\t%d\t%d\t%s\t%d\t%d\t%d\n",
					stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnInt(2), stmt.ColumnText(3),
					stmt.ColumnInt(4), stmt.ColumnInt64(5), stmt.ColumnInt64(6))
				return nil
			},
		}); err != nil {
			return fmt.Errorf("query deposits in system %d: %w", id, err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("write system: %w", err)
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
