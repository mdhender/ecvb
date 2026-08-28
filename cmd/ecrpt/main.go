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

	"github.com/mdhender/ecvb/internal/database"
	"github.com/mdhender/ecvb/internal/report"
	"github.com/peterbourgon/ff/v4"
	"zombiezen.com/go/sqlite"
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
	showFormat := showFlags.StringLong("format", "text", "output format: text or json")
	show := &ff.Command{
		Name:      "show",
		Usage:     "ecrpt show [--output PATH] [--format text|json] SUBCOMMAND",
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
	ordersFaction := ordersFlags.Int64Long("faction", 0, "number of the player's faction in the game")
	ordersTurn := ordersFlags.IntLong("turn", -1, "turn to report; defaults to the game's current turn")
	turnFlags := ff.NewFlagSet("show turn")
	turnGame := turnFlags.StringLong("game", "", "code of the game")
	turnEmail := turnFlags.StringLong("email", "", "email address of the player")
	turnFaction := turnFlags.Int64Long("faction", 0, "number of the player's faction in the game")
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
				return writeReport(ctx, *dbPath, *showOutput, *showFormat, stdout, func(conn *sqlite.Conn) (*report.Report, error) {
					return report.Orders(conn, *ordersGame, email, *ordersFaction, *ordersTurn)
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
				return writeReport(ctx, *dbPath, *showOutput, *showFormat, stdout, func(conn *sqlite.Conn) (*report.Report, error) {
					return report.Stellium(conn, id)
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
				return writeReport(ctx, *dbPath, *showOutput, *showFormat, stdout, func(conn *sqlite.Conn) (*report.Report, error) {
					return report.System(conn, id, *showDeposits)
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
				options := report.TurnOptions{
					ShowDeposits:       *turnShowDeposits,
					SummarizeResources: *turnSummarizeResources,
					ShowWorkGroups:     *turnWorkGroups,
				}
				return writeReport(ctx, *dbPath, *showOutput, *showFormat, stdout, func(conn *sqlite.Conn) (*report.Report, error) {
					return report.Turn(conn, *turnGame, email, *turnFaction, options)
				})
			},
		},
	}
	root.Subcommands = []*ff.Command{show}
	return root.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("EC"))
}

// writeReport opens the database, builds a report from it, and renders it in
// the requested format to standard output or to a file. The report is built
// before anything is written, so a query that fails leaves no half-written
// file behind.
func writeReport(ctx context.Context, directory, outputPath, format string, stdout io.Writer, build func(*sqlite.Conn) (*report.Report, error)) (err error) {
	chosen, err := report.ParseFormat(format)
	if err != nil {
		return err
	}
	conn, err := openDatabase(ctx, directory)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()
	rpt, err := build(conn)
	if err != nil {
		return err
	}
	if outputPath == "" {
		return report.Write(stdout, rpt, chosen)
	}
	var rendered bytes.Buffer
	if err := report.Write(&rendered, rpt, chosen); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, rendered.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write report to %s: %w", outputPath, err)
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
