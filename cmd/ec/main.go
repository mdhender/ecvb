// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "ec: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := ff.NewFlagSet("ec")
	dbPath := flags.StringLong("db-path", "db", "path to the database directory")
	root := &ff.Command{
		Name:      "ec",
		Usage:     "ec [--db-path PATH] SUBCOMMAND",
		ShortHelp: "manage an ECVB game",
		Flags:     flags,
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a subcommand is required (add, db, game, load, orders, or turn)")
		},
	}
	db := &ff.Command{
		Name:      "db",
		Usage:     "ec db SUBCOMMAND",
		ShortHelp: "inspect the ECVB database",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a database subcommand is required (verify)")
		},
	}
	db.Subcommands = []*ff.Command{
		{
			Name:      "verify",
			Usage:     "ec db verify",
			ShortHelp: "verify the ECVB database",
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				if _, err := fmt.Fprintf(stderr, "database path: %s\ndatabase name: %s\n", *dbPath, database.Filename); err != nil {
					return fmt.Errorf("write database details: %w", err)
				}
				version, err := verifyDatabase(ctx, *dbPath)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(stderr, "database version: %d\n", version); err != nil {
					return fmt.Errorf("write database version: %w", err)
				}
				return nil
			},
		},
	}
	game := &ff.Command{
		Name:      "game",
		Usage:     "ec game SUBCOMMAND",
		ShortHelp: "manage games",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a game subcommand is required (create)")
		},
	}
	load := &ff.Command{
		Name:      "load",
		Usage:     "ec load SUBCOMMAND",
		ShortHelp: "load seed data",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a load subcommand is required (game)")
		},
	}
	load.Subcommands = []*ff.Command{
		{
			Name:      "game",
			Usage:     "ec load game <code>",
			ShortHelp: "load a game's seed files",
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one game code")
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				return loadGame(ctx, *dbPath, args[0])
			},
		},
	}
	createFlags := ff.NewFlagSet("game create")
	gameSeed := createFlags.StringLong("game-seed", "db/game-seed.json", "path to the game seed JSON file")
	seedHigh := createFlags.Int64Long("seed-high", 19, "high half of the game's PCG seed")
	seedLow := createFlags.Int64Long("seed-low", 12, "low half of the game's PCG seed")
	game.Subcommands = []*ff.Command{
		{
			Name:      "create",
			Usage:     "ec game create [--game-seed PATH] [--seed-high N] [--seed-low N]",
			ShortHelp: "create a game from seed metadata",
			Flags:     createFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				if *gameSeed == "" {
					return fmt.Errorf("game-seed is required")
				}
				if *seedHigh < 0 || *seedLow < 0 {
					return fmt.Errorf("seed-high and seed-low must be nonnegative")
				}
				return createGame(ctx, *dbPath, *gameSeed, *seedHigh, *seedLow)
			},
		},
	}
	add := &ff.Command{
		Name:      "add",
		Usage:     "ec add SUBCOMMAND",
		ShortHelp: "add objects to a game",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("an add subcommand is required (player)")
		},
	}
	playerFlags := ff.NewFlagSet("add player")
	playerGame := playerFlags.StringLong("game", "", "code of the game")
	playerEmail := playerFlags.StringLong("email", "", "email address of the player")
	playerKit := playerFlags.StringLong("kit", "", "path to the starting kit JSON file")
	add.Subcommands = []*ff.Command{
		{
			Name:      "player",
			Usage:     "ec add player --game CODE --email EMAIL [--kit PATH]",
			ShortHelp: "add a player to a game",
			Flags:     playerFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				kitPath := *playerKit
				if kitPath == "" {
					kitPath = filepath.Join(*dbPath, "home-planet-seed.json")
				}
				factionID, err := addPlayerWithKit(ctx, *dbPath, *playerGame, *playerEmail, kitPath)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintln(stdout, factionID); err != nil {
					return fmt.Errorf("write faction id: %w", err)
				}
				return nil
			},
		},
	}
	orders := &ff.Command{
		Name:      "orders",
		Usage:     "ec orders SUBCOMMAND",
		ShortHelp: "check or submit player orders",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("an orders subcommand is required (check or submit)")
		},
	}
	orders.Subcommands = []*ff.Command{
		{
			Name:      "check",
			Usage:     "ec orders check FILE",
			ShortHelp: "check an order file without changing the database",
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one orders file")
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				result, err := processOrderFile(ctx, *dbPath, args[0], false)
				if err != nil {
					return err
				}
				return writeOrderResult(stdout, "checked", result)
			},
		},
		{
			Name:      "submit",
			Usage:     "ec orders submit FILE",
			ShortHelp: "validate and submit an order file",
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 1 {
					return fmt.Errorf("expected exactly one orders file")
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				result, err := processOrderFile(ctx, *dbPath, args[0], true)
				if err != nil {
					return err
				}
				return writeOrderResult(stdout, "submitted", result)
			},
		},
	}
	turn := &ff.Command{
		Name:      "turn",
		Usage:     "ec turn SUBCOMMAND",
		ShortHelp: "resolve a turn or open the next turn",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a turn subcommand is required (open or resolve)")
		},
	}
	resolveTurnFlags := ff.NewFlagSet("turn resolve")
	resolveTurnGame := resolveTurnFlags.StringLong("game", "", "code of the game")
	resolveTurnNumber := resolveTurnFlags.IntLong("turn", -1, "turn number to resolve")
	resolveNoLogTime := resolveTurnFlags.BoolLong("no-log-timestamps", "omit wall-clock timestamps from the engine log, so the same turn logs the same bytes")
	openTurnFlags := ff.NewFlagSet("turn open")
	openTurnGame := openTurnFlags.StringLong("game", "", "code of the game")
	openResolvedTurn := openTurnFlags.IntLong("turn", -1, "resolved turn after which to open the next turn")
	turn.Subcommands = []*ff.Command{
		{
			Name:      "resolve",
			Usage:     "ec turn resolve --game CODE --turn NUMBER [--no-log-timestamps]",
			ShortHelp: "resolve all orders for an open turn",
			Flags:     resolveTurnFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				result, err := resolveGameTurn(ctx, *dbPath, *resolveTurnGame, *resolveTurnNumber, stderr, !*resolveNoLogTime)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(stdout, "resolved game %s turn %d: %d orders, %d succeeded, %d failed\n",
					result.GameCode, result.Turn, result.Orders, result.Succeeded, result.Failed)
				return err
			},
		},
		{
			Name:      "open",
			Usage:     "ec turn open --game CODE --turn RESOLVED-TURN",
			ShortHelp: "open the turn following a resolved turn",
			Flags:     openTurnFlags,
			Exec: func(ctx context.Context, args []string) error {
				if len(args) != 0 {
					return fmt.Errorf("unexpected arguments: %v", args)
				}
				if *dbPath == "" {
					return fmt.Errorf("db-path is required")
				}
				result, err := openGameTurn(ctx, *dbPath, *openTurnGame, *openResolvedTurn)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(stdout, "opened game %s turn %d\n", result.GameCode, result.Turn)
				return err
			},
		},
	}
	root.Subcommands = []*ff.Command{add, db, game, load, orders, turn}

	return root.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("EC"))
}

type gameMetadata struct {
	Code string `json:"code"`
}

func createGame(ctx context.Context, directory, seedPath string, seedHigh, seedLow int64) (err error) {
	if seedHigh < 0 || seedLow < 0 {
		return fmt.Errorf("seed-high and seed-low must be nonnegative")
	}
	metadata, err := readGameMetadata(seedPath)
	if err != nil {
		return err
	}
	if metadata.Code == "" || metadata.Code != strings.ToUpper(metadata.Code) {
		return fmt.Errorf("invalid game code %q: code must be uppercase", metadata.Code)
	}

	conn, _, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO game (code, seed_high, seed_low) VALUES (?, ?, ?);", &sqlitex.ExecOptions{
		Args: []any{metadata.Code, seedHigh, seedLow},
	}); err != nil {
		if sqlite.ErrCode(err) == sqlite.ResultConstraintUnique {
			return fmt.Errorf("game code %q already exists", metadata.Code)
		}
		return fmt.Errorf("create game %q: %w", metadata.Code, err)
	}
	return nil
}

func readGameMetadata(path string) (gameMetadata, error) {
	if path == "" {
		return gameMetadata{}, fmt.Errorf("game-seed is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return gameMetadata{}, fmt.Errorf("stat game seed %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return gameMetadata{}, fmt.Errorf("game seed %s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return gameMetadata{}, fmt.Errorf("read game seed %s: %w", path, err)
	}
	var metadata gameMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return gameMetadata{}, fmt.Errorf("parse game seed %s: %w", path, err)
	}
	return metadata, nil
}

func verifyDatabase(ctx context.Context, directory string) (version int, err error) {
	conn, version, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return 0, err
	}
	if closeErr := conn.Close(); closeErr != nil {
		return 0, fmt.Errorf("close database: %w", closeErr)
	}
	return version, nil
}

func openVerifiedDatabase(ctx context.Context, directory string) (conn *sqlite.Conn, version int, err error) {
	path := filepath.Join(directory, database.Filename)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, fmt.Errorf("database %s does not exist", path)
		}
		return nil, 0, fmt.Errorf("stat database %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("database %s is not a regular file", path)
	}

	conn, err = sqlite.OpenConn(path, sqlite.OpenReadWrite)
	if err != nil {
		return nil, 0, fmt.Errorf("open database %s: %w", path, err)
	}
	conn.SetInterrupt(ctx.Done())
	defer func() {
		if err != nil {
			_ = conn.Close()
			conn = nil
		}
	}()

	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		return nil, 0, fmt.Errorf("enable foreign keys: %w", err)
	}

	var applicationID int
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA application_id;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			applicationID = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		return nil, 0, fmt.Errorf("read application id: %w", err)
	}
	if applicationID != database.ApplicationID {
		return nil, 0, fmt.Errorf("invalid application id: got %#x, want %#x", applicationID, database.ApplicationID)
	}

	if err := sqlitex.ExecuteTransient(conn, "PRAGMA user_version;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			version = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		return nil, 0, fmt.Errorf("read database version: %w", err)
	}
	if version != database.SchemaVersion {
		return nil, 0, fmt.Errorf("invalid database version: got %d, want %d", version, database.SchemaVersion)
	}

	return conn, version, nil
}
