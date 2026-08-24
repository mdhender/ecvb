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

type turnReportOptions struct {
	showDeposits       bool
	summarizeResources bool
	showWorkGroups     bool
}

func showTurnReport(ctx context.Context, directory, gameCode, email string, factionID int64, options turnReportOptions, output io.Writer) (err error) {
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
	factionID = faction.id

	w := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TURN REPORT")
	fmt.Fprintln(w, "GAME\tTURN\tFACTION\tCONTROLLER")
	fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", gameCode, faction.turn, factionID, faction.controller)

	fmt.Fprintln(w, "\nCONTROLLED PLANETS")
	fmt.Fprintln(w, "ID\tSTELLIUM\tCOORDINATES\tSYSTEM\tORBIT\tKIND\tHABITABILITY")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT p.id, st.id, st.x, st.y, st.z, sy.sequence, p.orbit, p.kind, p.habitability
		FROM planet AS p
		JOIN system AS sy ON sy.id = p.system_id
		JOIN stellium AS st ON st.id = sy.stellium_id
		WHERE p.faction_id = ?
		ORDER BY st.x, st.y, st.z, sy.sequence, p.orbit;`, &sqlitex.ExecOptions{
		Args: []any{factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			fmt.Fprintf(w, "%d\t%d\t%d,%d,%d\t%s\t%d\t%s\t%d\n",
				stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnInt(2), stmt.ColumnInt(3), stmt.ColumnInt(4),
				stmt.ColumnText(5), stmt.ColumnInt(6), stmt.ColumnText(7), stmt.ColumnInt(8))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query controlled planets: %w", err)
	}

	if options.showDeposits {
		fmt.Fprintln(w, "\nDEPOSITS")
		fmt.Fprintln(w, "PLANET\tSEQUENCE\tRESOURCE\tQUALITY\tINITIAL QUANTITY\tCURRENT QUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT d.planet_id, d.sequence, d.resource, d.quality, d.initial_qty, d.current_qty
			FROM deposit AS d
			JOIN planet AS p ON p.id = d.planet_id
			WHERE p.faction_id = ?
			ORDER BY d.planet_id, d.sequence;`, &sqlitex.ExecOptions{
			Args: []any{factionID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				fmt.Fprintf(w, "%d\t%d\t%s\t%d\t%d\t%d\n", stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnText(2), stmt.ColumnInt(3), stmt.ColumnInt64(4), stmt.ColumnInt64(5))
				return nil
			},
		}); err != nil {
			return fmt.Errorf("query deposits on controlled planets: %w", err)
		}
	}

	fmt.Fprintln(w, "\nENTITIES")
	fmt.Fprintln(w, "ID\tUNIT\tTECH\tSTELLIUM\tSYSTEM\tPLANET\tRING\tMASS\tENCLOSED VOLUME")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT e.id, e.unit, e.tech_level, e.stellium_id, sy.sequence, e.planet_id, e.planet_ring, e.mass, e.enclosed_volume
		FROM entity AS e
		LEFT JOIN system AS sy ON sy.id = e.system_id
		WHERE e.faction_id = ?
		ORDER BY e.id;`, &sqlitex.ExecOptions{
		Args: []any{factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\t%s\t%s\t%d\t%d\n",
				stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), stmt.ColumnInt64(3),
				nullableText(stmt, 4), nullableInt(stmt, 5), nullableInt(stmt, 6), stmt.ColumnInt64(7), stmt.ColumnInt64(8))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query entities: %w", err)
	}

	fmt.Fprintln(w, "\nCENSUS")
	fmt.Fprintln(w, "ENTITY\tCLASS\tPEOPLE")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT ep.entity_id, ep.class, ep.quantity * 100
		FROM entity_population AS ep
		JOIN entity AS e ON e.id = ep.entity_id
		WHERE e.faction_id = ?
		ORDER BY ep.entity_id, ep.class;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
		fmt.Fprintf(w, "%d\t%s\t%d\n", stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt64(2))
	})); err != nil {
		return fmt.Errorf("query census: %w", err)
	}

	fmt.Fprintln(w, "\nINVENTORY")
	fmt.Fprintln(w, "ENTITY\tSECTION\tUNIT\tTECH\tQUANTITY")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT i.entity_id, i.section, i.unit, i.tech_level, i.quantity
		FROM inventory AS i
		JOIN entity AS e ON e.id = i.entity_id
		WHERE e.faction_id = ?
		ORDER BY i.entity_id, i.section, i.unit, i.tech_level;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%d\n", stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnText(2), stmt.ColumnInt(3), stmt.ColumnInt64(4))
	})); err != nil {
		return fmt.Errorf("query inventory: %w", err)
	}

	if options.summarizeResources {
		fmt.Fprintln(w, "\nRESOURCE SUMMARY")
		fmt.Fprintln(w, "RESOURCE\tQUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT i.unit, SUM(i.quantity)
			FROM inventory AS i
			JOIN entity AS e ON e.id = i.entity_id
			WHERE e.faction_id = ? AND i.unit IN ('FUEL', 'GOLD', 'METL', 'MNRL')
			GROUP BY i.unit
			ORDER BY i.unit;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
			fmt.Fprintf(w, "%s\t%d\n", stmt.ColumnText(0), stmt.ColumnInt64(1))
		})); err != nil {
			return fmt.Errorf("summarize resources: %w", err)
		}
	}

	if options.showWorkGroups {
		fmt.Fprintln(w, "\nWORK GROUPS")
		fmt.Fprintln(w, "ENTITY\tUNIT\tSEQUENCE\tDEPOSIT\tTECH\tQUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT wg.entity_id, wg.unit, wg.sequence, wg.deposit_id, wgu.tech_level, wgu.quantity
			FROM work_group AS wg
			JOIN entity AS e ON e.id = wg.entity_id
			LEFT JOIN work_group_units AS wgu ON wgu.work_group_id = wg.id
			WHERE e.faction_id = ?
			ORDER BY wg.entity_id, wg.unit, wg.sequence, wgu.tech_level;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
			fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\t%s\n", stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), nullableInt(stmt, 3), nullableInt(stmt, 4), nullableInt(stmt, 5))
		})); err != nil {
			return fmt.Errorf("query work groups: %w", err)
		}
	}

	if err := writeOrders(w, conn, gameCode, factionID, "SUBMITTED ORDERS"); err != nil {
		return err
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("write turn report: %w", err)
	}
	return nil
}

func reportRows(factionID int64, printRow func(*sqlite.Stmt)) *sqlitex.ExecOptions {
	return &sqlitex.ExecOptions{
		Args: []any{factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			printRow(stmt)
			return nil
		},
	}
}

func nullableInt(stmt *sqlite.Stmt, column int) string {
	if stmt.ColumnIsNull(column) {
		return "-"
	}
	return fmt.Sprintf("%d", stmt.ColumnInt64(column))
}

func nullableText(stmt *sqlite.Stmt, column int) string {
	if stmt.ColumnIsNull(column) {
		return "-"
	}
	return stmt.ColumnText(column)
}
