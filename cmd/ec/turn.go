// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/mdhender/ecvb/internal/engine"
	"github.com/mdhender/ecvb/internal/logging"
)

func resolveGameTurn(ctx context.Context, directory, gameCode string, turn int, logOutput io.Writer, logTimestamps bool) (result engine.Result, err error) {
	conn, _, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return engine.Result{}, err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()
	logger := logging.NewLogger(logOutput, logTimestamps)
	return engine.Resolve(ctx, logger, conn, gameCode, turn)
}

func openGameTurn(ctx context.Context, directory, gameCode string, resolvedTurn int) (result engine.OpenResult, err error) {
	conn, _, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return engine.OpenResult{}, err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()
	return engine.OpenNextTurn(ctx, conn, gameCode, resolvedTurn)
}
