// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	orderpkg "github.com/mdhender/ecvb/internal/orders"
)

func processOrderFile(ctx context.Context, directory, path string, submit bool) (result orderpkg.Result, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return orderpkg.Result{}, fmt.Errorf("stat orders file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return orderpkg.Result{}, fmt.Errorf("orders file %s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return orderpkg.Result{}, fmt.Errorf("open orders file %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close orders file %s: %w", path, closeErr)
		}
	}()

	conn, _, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return orderpkg.Result{}, err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	if submit {
		return orderpkg.Submit(ctx, conn, file)
	}
	return orderpkg.Check(ctx, conn, file)
}

func writeOrderResult(w io.Writer, action string, result orderpkg.Result) error {
	_, err := fmt.Fprintf(w, "%s %d orders for game %s turn %d faction %d\n",
		action, result.Orders, result.GameCode, result.Turn, result.FactionID)
	if err != nil {
		return fmt.Errorf("write orders result: %w", err)
	}
	return nil
}
