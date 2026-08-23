// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/peterbourgon/ff/v4"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "db: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	root := &ff.Command{
		Name:      "db",
		Usage:     "db SUBCOMMAND <path>",
		ShortHelp: "create, migrate, and seed an ECVB database",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a subcommand is required (create, migrate, or seed)")
		},
	}
	migrate := &ff.Command{
		Name:      "migrate",
		Usage:     "db migrate SUBCOMMAND <path>",
		ShortHelp: "migrate an existing database",
		Exec: func(context.Context, []string) error {
			return fmt.Errorf("a migration subcommand is required (up)")
		},
	}
	migrate.Subcommands = []*ff.Command{
		{
			Name:      "up",
			Usage:     "db migrate up <path>",
			ShortHelp: "apply all pending migrations",
			Exec: func(ctx context.Context, args []string) error {
				path, err := onlyPath(args)
				if err != nil {
					return err
				}
				return migrateUp(ctx, path)
			},
		},
	}
	root.Subcommands = []*ff.Command{
		{
			Name:      "create",
			Usage:     "db create <path>",
			ShortHelp: "create and migrate a new database in an existing directory",
			Exec: func(ctx context.Context, args []string) error {
				path, err := onlyPath(args)
				if err != nil {
					return err
				}
				return createDatabase(ctx, path)
			},
		},
		migrate,
		{
			Name:      "seed",
			Usage:     "db seed <path>",
			ShortHelp: "seed an existing database in an existing directory",
			Exec: func(ctx context.Context, args []string) error {
				path, err := onlyPath(args)
				if err != nil {
					return err
				}
				return seedDatabase(ctx, path)
			},
		},
	}

	return root.ParseAndRun(ctx, args)
}

func onlyPath(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("expected exactly one directory path")
	}
	return args[0], nil
}
