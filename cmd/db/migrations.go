// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitemigration"
)

var migrationSchema = sqlitemigration.Schema{
	AppID:      applicationID,
	Migrations: database.Migrations(),
}

func latestSchemaVersion() int {
	return len(migrationSchema.Migrations)
}

func applyMigrations(ctx context.Context, conn *sqlite.Conn) error {
	if err := sqlitemigration.Migrate(ctx, conn, migrationSchema); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
