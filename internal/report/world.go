// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package report

import (
	"fmt"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Stellium reports a stellium and the systems and planets it holds.
func Stellium(conn *sqlite.Conn, id int64) (*Report, error) {
	rpt := New("STELLIUM")
	header := rpt.Table("", "ID", "GAME", "X", "Y", "Z")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT s.id, g.code, s.x, s.y, s.z
		FROM stellium AS s
		JOIN game AS g ON g.id = s.game_id
		WHERE s.id = ?;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			header.Row(stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), stmt.ColumnInt(3), stmt.ColumnInt(4))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("query stellium %d: %w", id, err)
	}
	if len(header.Rows) == 0 {
		return nil, fmt.Errorf("stellium %d does not exist", id)
	}

	systems := rpt.Table("SYSTEMS", "ID", "SEQUENCE", "PLANETS", "DEPOSITS")
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
			systems.Row(stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), stmt.ColumnInt(3))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("query systems in stellium %d: %w", id, err)
	}

	planets := rpt.Table("PLANETS", "SYSTEM", "ID", "ORBIT", "KIND", "HABITABILITY", "DEPOSITS")
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
			planets.Row(stmt.ColumnText(0), stmt.ColumnInt64(1), stmt.ColumnInt(2), stmt.ColumnText(3), stmt.ColumnInt(4), stmt.ColumnInt(5))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("query planets in stellium %d: %w", id, err)
	}
	return rpt, nil
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

// System reports a system and its planets.
func System(conn *sqlite.Conn, id int64, showDeposits bool) (*Report, error) {
	rpt := New("SYSTEM")
	header := rpt.Table("", "ID", "STELLIUM", "SEQUENCE")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT sy.id, sy.stellium_id, sy.sequence
		FROM system AS sy
		WHERE sy.id = ?;`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			header.Row(stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnText(2))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("query system %d: %w", id, err)
	}
	if len(header.Rows) == 0 {
		return nil, fmt.Errorf("system %d does not exist", id)
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
		return nil, fmt.Errorf("query planets in system %d: %w", id, err)
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
		return nil, fmt.Errorf("summarize deposits in system %d: %w", id, err)
	}

	planetTable := rpt.Table("PLANETS", "ID", "ORBIT", "KIND", "HABITABILITY", "DEPOSITS (CURRENT QUANTITY)")
	for _, planet := range planets {
		summary := "none"
		if len(planet.summary) != 0 {
			parts := make([]string, len(planet.summary))
			for i, item := range planet.summary {
				parts[i] = fmt.Sprintf("%s=%d", item.resource, item.quantity)
			}
			summary = strings.Join(parts, ", ")
		}
		planetTable.Row(planet.id, planet.orbit, planet.kind, planet.habitability, summary)
	}

	if showDeposits {
		deposits := rpt.Table("DEPOSITS", "PLANET", "ORBIT", "SEQUENCE", "RESOURCE", "QUALITY", "INITIAL QUANTITY", "CURRENT QUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT p.id, p.orbit, d.sequence, d.resource, d.quality, d.initial_qty, d.current_qty
			FROM planet AS p
			JOIN deposit AS d ON d.planet_id = p.id
			WHERE p.system_id = ?
			ORDER BY p.orbit, d.sequence;`, &sqlitex.ExecOptions{
			Args: []any{id},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				deposits.Row(
					stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnInt(2), stmt.ColumnText(3),
					stmt.ColumnInt(4), stmt.ColumnInt64(5), stmt.ColumnInt64(6))
				return nil
			},
		}); err != nil {
			return nil, fmt.Errorf("query deposits in system %d: %w", id, err)
		}
	}
	return rpt, nil
}
