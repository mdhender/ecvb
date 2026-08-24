// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func showOrdersReport(ctx context.Context, directory, gameCode, email string, factionID int64, output io.Writer) (err error) {
	conn, err := openDatabase(ctx, directory)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	faction, err := findReportFaction(conn, gameCode, email, factionID)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ORDERS REPORT")
	fmt.Fprintln(w, "GAME\tTURN\tFACTION\tCONTROLLER")
	fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", gameCode, faction.turn, faction.id, faction.controller)
	if err := writeOrders(w, conn, gameCode, faction.id, "ORDERS"); err != nil {
		return err
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("write orders report: %w", err)
	}
	return nil
}

func writeOrders(w io.Writer, conn *sqlite.Conn, gameCode string, factionID int64, heading string) error {
	fmt.Fprintf(w, "\n%s\n", heading)
	fmt.Fprintln(w, "SEQUENCE\tENTITY\tVERB\tTARGET\tSUPPORT\tPARAMETERS")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT sequence, entity_id, verb, target_entity_id, support_entity_id, parameters
		FROM order_entry
		WHERE game_id = (SELECT id FROM game WHERE code = ?) AND faction_id = ?
		ORDER BY sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameCode, factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%s\n",
				stmt.ColumnInt(0), stmt.ColumnInt64(1), stmt.ColumnText(2),
				nullableInt(stmt, 3), nullableInt(stmt, 4), stmt.ColumnText(5))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query submitted orders: %w", err)
	}
	return nil
}
