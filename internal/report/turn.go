// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package report

import (
	"fmt"

	"github.com/mdhender/ecvb/internal/sensors"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// TurnOptions selects the report's optional sections.
type TurnOptions struct {
	ShowDeposits       bool
	SummarizeResources bool
	ShowWorkGroups     bool
}

// Turn reports a faction's whole position at the current turn.
func Turn(conn *sqlite.Conn, gameCode, email string, factionID int64, options TurnOptions) (*Report, error) {
	faction, err := findFaction(conn, gameCode, email, factionID)
	if err != nil {
		return nil, err
	}
	factionID = faction.id

	rpt := New("TURN REPORT")
	rpt.Table("", "GAME", "TURN", "FACTION", "CONTROLLER").
		Row(gameCode, faction.turn, factionID, faction.controller)

	planets := rpt.Table("CONTROLLED PLANETS", "ID", "STELLIUM", "COORDINATES", "SYSTEM", "ORBIT", "KIND", "HABITABILITY")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT p.id, st.id, st.x, st.y, st.z, sy.sequence, p.orbit, p.kind, p.habitability
		FROM planet AS p
		JOIN system AS sy ON sy.id = p.system_id
		JOIN stellium AS st ON st.id = sy.stellium_id
		WHERE p.faction_id = ?
		ORDER BY st.x, st.y, st.z, sy.sequence, p.orbit;`, &sqlitex.ExecOptions{
		Args: []any{factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planets.Row(
				stmt.ColumnInt64(0), stmt.ColumnInt64(1),
				fmt.Sprintf("%d,%d,%d", stmt.ColumnInt(2), stmt.ColumnInt(3), stmt.ColumnInt(4)),
				stmt.ColumnText(5), stmt.ColumnInt(6), stmt.ColumnText(7), stmt.ColumnInt(8))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("query controlled planets: %w", err)
	}

	if options.ShowDeposits {
		deposits := rpt.Table("DEPOSITS", "PLANET", "SEQUENCE", "RESOURCE", "QUALITY", "INITIAL QUANTITY", "CURRENT QUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT d.planet_id, d.sequence, d.resource, d.quality, d.initial_qty, d.current_qty
			FROM deposit AS d
			JOIN planet AS p ON p.id = d.planet_id
			WHERE p.faction_id = ?
			ORDER BY d.planet_id, d.sequence;`, &sqlitex.ExecOptions{
			Args: []any{factionID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				deposits.Row(stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnText(2), stmt.ColumnInt(3), stmt.ColumnInt64(4), stmt.ColumnInt64(5))
				return nil
			},
		}); err != nil {
			return nil, fmt.Errorf("query deposits on controlled planets: %w", err)
		}
	}

	entities := rpt.Table("ENTITIES", "ID", "UNIT", "TECH", "STELLIUM", "SYSTEM", "PLANET", "RING", "MASS", "ENCLOSED VOLUME")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT e.id, e.unit, e.tech_level, e.stellium_id, sy.sequence, e.planet_id, e.planet_ring, e.mass, e.enclosed_volume
		FROM entity AS e
		LEFT JOIN system AS sy ON sy.id = e.system_id
		WHERE e.faction_id = ?
		ORDER BY e.id;`, &sqlitex.ExecOptions{
		Args: []any{factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			entities.Row(
				stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), stmt.ColumnInt64(3),
				nullableText(stmt, 4), nullableInt(stmt, 5), nullableInt(stmt, 6), stmt.ColumnInt64(7), stmt.ColumnInt64(8))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("query entities: %w", err)
	}

	// What this faction calls things. A name is its own: nobody else's report
	// shows it, and a place may be named without ever being visited.
	names := rpt.Table("NAMES", "SUBJECT", "ID", "NAME")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT CASE
				WHEN stellium_id IS NOT NULL THEN 'stellium'
				WHEN system_id IS NOT NULL THEN 'system'
				WHEN planet_id IS NOT NULL THEN 'planet'
				ELSE 'entity'
			END AS subject,
			coalesce(stellium_id, system_id, planet_id, entity_id) AS id,
			name
		FROM faction_name WHERE faction_id = ?
		ORDER BY subject, id;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
		names.Row(stmt.ColumnText(0), stmt.ColumnInt64(1), stmt.ColumnText(2))
	})); err != nil {
		return nil, fmt.Errorf("query names: %w", err)
	}

	census := rpt.Table("CENSUS", "ENTITY", "CLASS", "PEOPLE")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT ep.entity_id, ep.class, ep.quantity * 100
		FROM entity_population AS ep
		JOIN entity AS e ON e.id = ep.entity_id
		WHERE e.faction_id = ?
		ORDER BY ep.entity_id, ep.class;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
		census.Row(stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt64(2))
	})); err != nil {
		return nil, fmt.Errorf("query census: %w", err)
	}

	inventory := rpt.Table("INVENTORY", "ENTITY", "SECTION", "UNIT", "TECH", "QUANTITY")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT i.entity_id, i.section, i.unit, i.tech_level, i.quantity
		FROM inventory AS i
		JOIN entity AS e ON e.id = i.entity_id
		WHERE e.faction_id = ?
		ORDER BY i.entity_id, i.section, i.unit, i.tech_level;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
		inventory.Row(stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnText(2), stmt.ColumnInt(3), stmt.ColumnInt64(4))
	})); err != nil {
		return nil, fmt.Errorf("query inventory: %w", err)
	}

	if options.SummarizeResources {
		resources := rpt.Table("RESOURCE SUMMARY", "RESOURCE", "QUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT i.unit, SUM(i.quantity)
			FROM inventory AS i
			JOIN entity AS e ON e.id = i.entity_id
			WHERE e.faction_id = ? AND i.unit IN ('FUEL', 'GOLD', 'METL', 'MNRL')
			GROUP BY i.unit
			ORDER BY i.unit;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
			resources.Row(stmt.ColumnText(0), stmt.ColumnInt64(1))
		})); err != nil {
			return nil, fmt.Errorf("summarize resources: %w", err)
		}
	}

	if options.ShowWorkGroups {
		groups := rpt.Table("WORK GROUPS", "ENTITY", "UNIT", "SEQUENCE", "DEPOSIT", "TECH", "QUANTITY")
		if err := sqlitex.ExecuteTransient(conn, `
			SELECT wg.entity_id, wg.unit, wg.sequence, wg.deposit_id, wgu.tech_level, wgu.quantity
			FROM work_group AS wg
			JOIN entity AS e ON e.id = wg.entity_id
			LEFT JOIN work_group_units AS wgu ON wgu.work_group_id = wg.id
			WHERE e.faction_id = ?
			ORDER BY wg.entity_id, wg.unit, wg.sequence, wgu.tech_level;`, reportRows(factionID, func(stmt *sqlite.Stmt) {
			groups.Row(stmt.ColumnInt64(0), stmt.ColumnText(1), stmt.ColumnInt(2), nullableInt(stmt, 3), nullableInt(stmt, 4), nullableInt(stmt, 5))
		})); err != nil {
			return nil, fmt.Errorf("query work groups: %w", err)
		}
	}

	if err := addSensorReport(rpt, conn, gameCode, faction.turn, factionID); err != nil {
		return nil, err
	}
	if err := addProbeFindings(rpt, conn, gameCode, faction.turn, factionID); err != nil {
		return nil, err
	}
	if err := addOrders(rpt, conn, gameCode, faction.turn, factionID, "ORDERS"); err != nil {
		return nil, err
	}
	if err := addProbes(rpt, conn, gameCode, faction.turn, factionID); err != nil {
		return nil, err
	}
	return rpt, nil
}

// addSensorReport reports the passive sensor reading taken at the start of the
// turn, before anything moved. A ship that jumped this turn reads its new
// stellium in the next turn's report, not this one.
func addSensorReport(rpt *Report, conn *sqlite.Conn, gameCode string, turn int, factionID int64) error {
	args := []any{gameCode, turn, factionID}

	survey := rpt.Table("SENSOR SURVEY", "ENTITY", "STELLIUM", "COORDINATES", "SYSTEM", "SYSTEMS")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT s.entity_id, s.stellium_id, st.x, st.y, st.z, s.system_id, s.systems
		FROM sensor_survey AS s
		JOIN stellium AS st ON st.id = s.stellium_id
		WHERE s.game_id = (SELECT id FROM game WHERE code = ?) AND s.turn = ? AND s.faction_id = ?
		ORDER BY s.entity_id;`, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			survey.Row(
				stmt.ColumnInt64(0), stmt.ColumnInt64(1),
				fmt.Sprintf("%d,%d,%d", stmt.ColumnInt(2), stmt.ColumnInt(3), stmt.ColumnInt(4)),
				nullableInt(stmt, 5), stmt.ColumnInt(6))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query sensor survey: %w", err)
	}

	planets := rpt.Table("SENSOR PLANETS", "ENTITY", "STELLIUM", "SYSTEM", "ORBIT", "KIND")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT s.entity_id, s.stellium_id, sy.sequence, p.orbit, p.kind
		FROM sensor_survey AS s
		JOIN system AS sy ON sy.stellium_id = s.stellium_id
		JOIN planet AS p ON p.system_id = sy.id
		WHERE s.game_id = (SELECT id FROM game WHERE code = ?) AND s.turn = ? AND s.faction_id = ?
		ORDER BY s.entity_id, sy.sequence, p.orbit;`, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planets.Row(stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnText(2), stmt.ColumnInt(3), stmt.ColumnText(4))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query sensor planets: %w", err)
	}

	contacts := rpt.Table("SYSTEM CONTACTS", "ENTITY", "PLANET", "ORBIT", "CONTACT UNIT", "RING", "APPROXIMATE MASS")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT c.entity_id, c.planet_id, p.orbit, c.unit, c.planet_ring, c.mass
		FROM sensor_contact AS c
		JOIN planet AS p ON p.id = c.planet_id
		WHERE c.game_id = (SELECT id FROM game WHERE code = ?) AND c.turn = ? AND c.faction_id = ?
		ORDER BY c.entity_id, p.orbit, c.unit, c.contact_id;`, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			contacts.Row(
				stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnInt(2), stmt.ColumnText(3),
				stmt.ColumnInt(4), sensors.ApproximateMass(stmt.ColumnInt64(5)))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query system contacts: %w", err)
	}
	return nil
}

// addProbeFindings reports what this turn's probes read. A probe reads exact
// masses and identities, unlike a passive sensor reading.
func addProbeFindings(rpt *Report, conn *sqlite.Conn, gameCode string, turn int, factionID int64) error {
	contacts := rpt.Table("PROBE CONTACTS", "PLANET", "ENTITY", "UNIT", "RING", "MASS")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT planet_id, entity_id, unit, planet_ring, mass
		FROM probe_contact
		WHERE game_id = (SELECT id FROM game WHERE code = ?) AND turn = ? AND faction_id = ?
		ORDER BY planet_id, planet_ring, entity_id;`, &sqlitex.ExecOptions{
		Args: []any{gameCode, turn, factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			contacts.Row(stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnText(2), stmt.ColumnInt(3), stmt.ColumnInt64(4))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query probe contacts: %w", err)
	}

	deposits := rpt.Table("PROBE DEPOSITS", "PLANET", "DEPOSIT", "RESOURCE", "APPROXIMATE QUANTITY")
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT planet_id, deposit_id, resource, quantity
		FROM probe_deposit
		WHERE game_id = (SELECT id FROM game WHERE code = ?) AND turn = ? AND faction_id = ?
		ORDER BY planet_id, deposit_id;`, &sqlitex.ExecOptions{
		Args: []any{gameCode, turn, factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			deposits.Row(stmt.ColumnInt64(0), stmt.ColumnInt64(1), stmt.ColumnText(2), sensors.ApproximateMass(stmt.ColumnInt64(3)))
			return nil
		},
	}); err != nil {
		return fmt.Errorf("query probe deposits: %w", err)
	}
	return nil
}

func reportRows(factionID int64, addRow func(*sqlite.Stmt)) *sqlitex.ExecOptions {
	return &sqlitex.ExecOptions{
		Args: []any{factionID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			addRow(stmt)
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
