// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package engine resolves game turns.
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Result summarizes a resolved turn.
type Result struct {
	GameCode  string
	Turn      int
	Orders    int
	Succeeded int
	Failed    int
}

// OpenResult summarizes opening the next turn.
type OpenResult struct {
	GameCode string
	Turn     int
}

type location struct {
	stelliumID int64
	systemID   int64
	planetID   int64
	ring       int
}

type entity struct {
	unit      string
	factionID int64
	location  location
	mass      int64
	drive     jumpdrive.Drive
	sensors   sensors.Array
}

type point struct {
	x, y, z int
}

type outcome struct {
	orderType string
	factionID int64
	sequence  int
	line      int
	shipID    int64
	request   string
	status    string
	message   string
	start     location
	final     location
}

type moveOrder struct {
	factionID             int64
	sequence              int
	line                  int
	shipID                int64
	requestedSystem       string
	requestedOrbit        int
	destinationStelliumID int64
	destinationSystemID   int64
	destinationPlanetID   int64
}

type probeOrder struct {
	factionID       int64
	sequence        int
	line            int
	entityID        int64
	requestedSystem string
	orbit           int
}

type jumpOrder struct {
	factionID             int64
	sequence              int
	line                  int
	shipID                int64
	x                     int
	y                     int
	z                     int
	destinationStelliumID int64
}

// Resolve resolves an open turn. All MOVE orders resolve before any JUMP
// order. Expected game-rule failures are recorded on the order and do not stop
// the turn; database and state errors roll back the entire turn.
func Resolve(ctx context.Context, logger *slog.Logger, conn *sqlite.Conn, gameCode string, turn int) (Result, error) {
	if logger == nil {
		return Result{}, fmt.Errorf("logger is required")
	}
	result, outcomes, err := resolve(ctx, conn, gameCode, turn)
	if err != nil {
		logger.ErrorContext(ctx, "resolve turn", "game", gameCode, "turn", turn, "error", err)
		return Result{}, err
	}
	for _, item := range outcomes {
		attrs := []slog.Attr{
			slog.String("game", gameCode),
			slog.Int("turn", turn),
			slog.String("order_type", item.orderType),
			slog.Int64("faction_id", item.factionID),
			slog.Int("sequence", item.sequence),
			slog.Int("source_line", item.line),
			actorAttr(item),
			slog.String("request", item.request),
			slog.String("status", item.status),
			locationAttr("start", item.start),
			locationAttr("final", item.final),
		}
		if item.message != "" {
			attrs = append(attrs, slog.String("error", item.message))
		}
		logger.LogAttrs(ctx, slog.LevelInfo, "order resolved", attrs...)
	}
	return result, nil
}

func resolve(ctx context.Context, conn *sqlite.Conn, gameCode string, turn int) (result Result, outcomes []outcome, err error) {
	if gameCode == "" {
		return Result{}, nil, fmt.Errorf("game is required")
	}
	if turn < 0 {
		return Result{}, nil, fmt.Errorf("turn must be nonnegative")
	}
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return Result{}, nil, fmt.Errorf("begin resolve turn transaction: %w", err)
	}
	defer end(&err)

	gameID, err := requireGameTurn(conn, gameCode, turn, "open")
	if err != nil {
		return Result{}, nil, err
	}
	entities, err := loadEntities(conn, gameID)
	if err != nil {
		return Result{}, nil, err
	}
	stellia, err := loadStellia(conn, gameID)
	if err != nil {
		return Result{}, nil, err
	}
	moves, err := loadMoveOrders(conn, gameID, turn)
	if err != nil {
		return Result{}, nil, err
	}
	probes, err := loadProbeOrders(conn, gameID, turn)
	if err != nil {
		return Result{}, nil, err
	}
	jumps, err := loadJumpOrders(conn, gameID, turn)
	if err != nil {
		return Result{}, nil, err
	}

	result = Result{GameCode: gameCode, Turn: turn, Orders: len(moves) + len(probes) + len(jumps)}
	// Probes and passive sensors both read the turn's starting positions, so
	// they resolve before anything moves. A ship that jumps this turn reports
	// its new stellium in the next turn's report, not this one's.
	spent := make(map[int64]int64)
	for _, order := range probes {
		item, err := executeProbe(conn, gameID, turn, entities, spent, order)
		if err != nil {
			return Result{}, nil, err
		}
		outcomes = append(outcomes, item)
		countOutcome(&result, item)
	}
	if err := recordPassiveSensors(conn, gameID, turn, entities); err != nil {
		return Result{}, nil, err
	}
	for _, order := range moves {
		item, err := executeMove(conn, gameID, turn, entities, order)
		if err != nil {
			return Result{}, nil, err
		}
		outcomes = append(outcomes, item)
		countOutcome(&result, item)
	}
	for _, order := range jumps {
		item, err := executeJump(conn, gameID, turn, entities, stellia, order)
		if err != nil {
			return Result{}, nil, err
		}
		outcomes = append(outcomes, item)
		countOutcome(&result, item)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET turn_state = 'resolved'
		WHERE id = ? AND turn = ? AND turn_state = 'open';`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
	}); err != nil {
		return Result{}, nil, fmt.Errorf("mark game %q turn %d resolved: %w", gameCode, turn, err)
	}
	if conn.Changes() != 1 {
		return Result{}, nil, fmt.Errorf("game %q turn %d changed while it was resolving", gameCode, turn)
	}
	return result, outcomes, nil
}

// OpenNextTurn advances a resolved game to its next open turn. It retains the
// most recently resolved turn's orders and purges older order history.
func OpenNextTurn(ctx context.Context, conn *sqlite.Conn, gameCode string, resolvedTurn int) (result OpenResult, err error) {
	if gameCode == "" {
		return OpenResult{}, fmt.Errorf("game is required")
	}
	if resolvedTurn < 0 {
		return OpenResult{}, fmt.Errorf("turn must be nonnegative")
	}
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return OpenResult{}, fmt.Errorf("begin open turn transaction: %w", err)
	}
	defer end(&err)

	gameID, err := requireGameTurn(conn, gameCode, resolvedTurn, "resolved")
	if err != nil {
		return OpenResult{}, err
	}
	for _, table := range []string{"move_order", "jump_order", "probe_order", "probe_contact", "probe_deposit",
		"sensor_survey", "sensor_contact"} {
		if err := sqlitex.ExecuteTransient(conn,
			"DELETE FROM "+table+" WHERE game_id = ? AND turn < ?;",
			&sqlitex.ExecOptions{Args: []any{gameID, resolvedTurn}}); err != nil {
			return OpenResult{}, fmt.Errorf("purge old %s rows: %w", table, err)
		}
	}
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET turn = turn + 1, turn_state = 'open'
		WHERE id = ? AND turn = ? AND turn_state = 'resolved';`, &sqlitex.ExecOptions{
		Args: []any{gameID, resolvedTurn},
	}); err != nil {
		return OpenResult{}, fmt.Errorf("open game %q turn %d: %w", gameCode, resolvedTurn+1, err)
	}
	if conn.Changes() != 1 {
		return OpenResult{}, fmt.Errorf("game %q turn %d changed while it was opening", gameCode, resolvedTurn)
	}
	return OpenResult{GameCode: gameCode, Turn: resolvedTurn + 1}, nil
}

func requireGameTurn(conn *sqlite.Conn, code string, turn int, state string) (int64, error) {
	var gameID int64
	var currentTurn int
	var currentState string
	found := false
	if err := sqlitex.ExecuteTransient(conn, "SELECT id, turn, turn_state FROM game WHERE code = ?;", &sqlitex.ExecOptions{
		Args: []any{code},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			gameID, currentTurn, currentState = stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnText(2)
			found = true
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find game %q: %w", code, err)
	}
	if !found {
		return 0, fmt.Errorf("game %q does not exist", code)
	}
	if currentTurn != turn {
		return 0, fmt.Errorf("game %q is on turn %d, not turn %d", code, currentTurn, turn)
	}
	if currentState != state {
		return 0, fmt.Errorf("game %q turn %d is %s, not %s", code, turn, currentState, state)
	}
	return gameID, nil
}

func loadEntities(conn *sqlite.Conn, gameID int64) (map[int64]*entity, error) {
	entities := make(map[int64]*entity)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT e.id, e.unit, e.faction_id, e.stellium_id, e.system_id, e.planet_id, e.planet_ring, e.mass
		FROM entity AS e
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			entities[stmt.ColumnInt64(0)] = &entity{
				unit: stmt.ColumnText(1), factionID: stmt.ColumnInt64(2),
				location: readLocation(stmt, 3), mass: stmt.ColumnInt64(7),
			}
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}
	drives, err := jumpdrive.LoadAll(conn, gameID)
	if err != nil {
		return nil, err
	}
	for id, drive := range drives {
		if item, ok := entities[id]; ok {
			item.drive = drive
		}
	}
	arrays, err := sensors.LoadAll(conn, gameID)
	if err != nil {
		return nil, err
	}
	for id, array := range arrays {
		if item, ok := entities[id]; ok {
			item.sensors = array
		}
	}
	return entities, nil
}

func loadStellia(conn *sqlite.Conn, gameID int64) (map[int64]point, error) {
	stellia := make(map[int64]point)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT id, x, y, z FROM stellium WHERE game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			stellia[stmt.ColumnInt64(0)] = point{x: stmt.ColumnInt(1), y: stmt.ColumnInt(2), z: stmt.ColumnInt(3)}
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load stellia: %w", err)
	}
	return stellia, nil
}

func loadMoveOrders(conn *sqlite.Conn, gameID int64, turn int) ([]moveOrder, error) {
	var orders []moveOrder
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, ship_id, requested_system, requested_orbit,
			destination_stellium_id, destination_system_id, destination_planet_id, status
		FROM move_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if status := stmt.ColumnText(9); status != "pending" {
				return fmt.Errorf("move order for faction %d sequence %d is already %s", stmt.ColumnInt64(0), stmt.ColumnInt(1), status)
			}
			orders = append(orders, moveOrder{
				factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2), shipID: stmt.ColumnInt64(3),
				requestedSystem: nullableText(stmt, 4), requestedOrbit: stmt.ColumnInt(5),
				destinationStelliumID: stmt.ColumnInt64(6), destinationSystemID: stmt.ColumnInt64(7), destinationPlanetID: stmt.ColumnInt64(8),
			})
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load move orders: %w", err)
	}
	return orders, nil
}

func loadProbeOrders(conn *sqlite.Conn, gameID int64, turn int) ([]probeOrder, error) {
	var orders []probeOrder
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, entity_id, requested_system, requested_orbit, status
		FROM probe_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if status := stmt.ColumnText(6); status != "pending" {
				return fmt.Errorf("probe order for faction %d sequence %d is already %s", stmt.ColumnInt64(0), stmt.ColumnInt(1), status)
			}
			orders = append(orders, probeOrder{
				factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2),
				entityID: stmt.ColumnInt64(3), requestedSystem: nullableText(stmt, 4), orbit: stmt.ColumnInt(5),
			})
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load probe orders: %w", err)
	}
	return orders, nil
}

func loadJumpOrders(conn *sqlite.Conn, gameID int64, turn int) ([]jumpOrder, error) {
	var orders []jumpOrder
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT faction_id, sequence, source_line, ship_id,
			destination_x, destination_y, destination_z, destination_stellium_id, status
		FROM jump_order WHERE game_id = ? AND turn = ?
		ORDER BY faction_id, sequence;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if status := stmt.ColumnText(8); status != "pending" {
				return fmt.Errorf("jump order for faction %d sequence %d is already %s", stmt.ColumnInt64(0), stmt.ColumnInt(1), status)
			}
			orders = append(orders, jumpOrder{
				factionID: stmt.ColumnInt64(0), sequence: stmt.ColumnInt(1), line: stmt.ColumnInt(2), shipID: stmt.ColumnInt64(3),
				x: stmt.ColumnInt(4), y: stmt.ColumnInt(5), z: stmt.ColumnInt(6), destinationStelliumID: stmt.ColumnInt64(7),
			})
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load jump orders: %w", err)
	}
	return orders, nil
}

func executeMove(conn *sqlite.Conn, gameID int64, turn int, entities map[int64]*entity, order moveOrder) (outcome, error) {
	item := outcome{
		orderType: "move", factionID: order.factionID, sequence: order.sequence,
		line: order.line, shipID: order.shipID, request: fmt.Sprintf("system %s orbit %d", displaySystem(order.requestedSystem), order.requestedOrbit),
		status: "succeeded",
	}
	ship := entities[order.shipID]
	if ship == nil {
		return outcome{}, fmt.Errorf("move order faction %d sequence %d references missing ship %d", order.factionID, order.sequence, order.shipID)
	}
	item.start = ship.location
	item.final = location{stelliumID: order.destinationStelliumID, systemID: order.destinationSystemID, planetID: order.destinationPlanetID, ring: 99}
	if ship.factionID != order.factionID {
		item.status, item.message, item.final = "failed", fmt.Sprintf("ship %d does not belong to faction %d", order.shipID, order.factionID), item.start
	} else if ship.unit != "SHIP" {
		item.status, item.message, item.final = "failed", fmt.Sprintf("entity %d is a %s, not a ship", order.shipID, ship.unit), item.start
	} else if ship.location.stelliumID != order.destinationStelliumID {
		item.status, item.message, item.final = "failed", "destination system is not in the ship's current stellium", item.start
	}
	if item.status == "succeeded" {
		ship.location = item.final
		if err := updateEntityLocation(conn, order.shipID, item.final); err != nil {
			return outcome{}, err
		}
	}
	if err := updateMoveOutcome(conn, gameID, turn, order, item); err != nil {
		return outcome{}, err
	}
	return item, nil
}

func executeJump(conn *sqlite.Conn, gameID int64, turn int, entities map[int64]*entity, stellia map[int64]point, order jumpOrder) (outcome, error) {
	item := outcome{
		orderType: "jump", factionID: order.factionID, sequence: order.sequence,
		line: order.line, shipID: order.shipID, request: fmt.Sprintf("(%d,%d,%d)", order.x, order.y, order.z),
		status: "succeeded",
	}
	ship := entities[order.shipID]
	if ship == nil {
		return outcome{}, fmt.Errorf("jump order faction %d sequence %d references missing ship %d", order.factionID, order.sequence, order.shipID)
	}
	item.start = ship.location
	item.final = location{stelliumID: order.destinationStelliumID}
	if ship.factionID != order.factionID {
		item.status, item.message, item.final = "failed", fmt.Sprintf("ship %d does not belong to faction %d", order.shipID, order.factionID), item.start
	} else if ship.unit != "SHIP" {
		item.status, item.message, item.final = "failed", fmt.Sprintf("entity %d is a %s, not a ship", order.shipID, ship.unit), item.start
	} else if message := checkDrive(ship, stellia[ship.location.stelliumID], order); message != "" {
		item.status, item.message, item.final = "failed", message, item.start
	}
	if item.status == "succeeded" {
		ship.location = item.final
		if err := updateEntityLocation(conn, order.shipID, item.final); err != nil {
			return outcome{}, err
		}
	}
	if err := updateJumpOutcome(conn, gameID, turn, order, item); err != nil {
		return outcome{}, err
	}
	return item, nil
}

// checkDrive returns the reason a ship cannot make a jump, or an empty string
// when the jump is within its drive's range and capacity.
func checkDrive(ship *entity, start point, order jumpOrder) string {
	if !ship.drive.Installed() {
		return fmt.Sprintf("ship %d has no assembled %s and cannot jump", order.shipID, jumpdrive.Unit)
	}
	if !ship.drive.CanPropel(ship.mass) {
		return fmt.Sprintf("ship %d masses %d MU and its jump drive propels %d MU",
			order.shipID, ship.mass, ship.drive.Capacity)
	}
	if !ship.drive.Reaches(jumpdrive.SquaredDistance(start.x, start.y, start.z, order.x, order.y, order.z)) {
		return fmt.Sprintf("jump of %d units exceeds ship %d jump range of %d units",
			jumpdrive.Distance(start.x, start.y, start.z, order.x, order.y, order.z), order.shipID, ship.drive.Range)
	}
	return ""
}

// executeProbe resolves one probe of one orbit. A probe reads the planet in
// that orbit of the ship's current system and records what it finds, so the
// finding survives the ship jumping away later in the same turn.
func executeProbe(conn *sqlite.Conn, gameID int64, turn int, entities map[int64]*entity, spent map[int64]int64, order probeOrder) (outcome, error) {
	item := outcome{
		orderType: "probe", factionID: order.factionID, sequence: order.sequence,
		line: order.line, shipID: order.entityID, request: probeRequest(order),
		status: "succeeded",
	}
	ship := entities[order.entityID]
	if ship == nil {
		return outcome{}, fmt.Errorf("probe order faction %d sequence %d references missing entity %d", order.factionID, order.sequence, order.entityID)
	}
	item.start, item.final = ship.location, ship.location

	probedSystemID, planetID := int64(0), int64(0)
	habitability := 0
	switch {
	case ship.factionID != order.factionID:
		item.status, item.message = "failed", fmt.Sprintf("entity %d does not belong to faction %d", order.entityID, order.factionID)
	case !ship.sensors.Installed():
		item.status, item.message = "failed", fmt.Sprintf("entity %d has no assembled %s and cannot probe", order.entityID, sensors.Unit)
	case spent[order.entityID] >= ship.sensors.Probes:
		item.status, item.message = "failed", fmt.Sprintf("entity %d has only %d probes this turn", order.entityID, ship.sensors.Probes)
	default:
		// A probe that names a system reads any system of the ship's stellium.
		// A probe that does not reads the system the ship is in.
		systemID := ship.location.systemID
		if order.requestedSystem != "" {
			found, err := findSystemInStellium(conn, ship.location.stelliumID, order.requestedSystem)
			if err != nil {
				return outcome{}, err
			}
			if found == 0 {
				item.status, item.message = "failed", fmt.Sprintf("current stellium has no system %s", order.requestedSystem)
				break
			}
			systemID = found
		} else if systemID == 0 {
			item.status, item.message = "failed", fmt.Sprintf("entity %d is orbiting the stellium; name a system to probe", order.entityID)
			break
		}
		found, hab, err := findPlanetInOrbit(conn, systemID, order.orbit)
		if err != nil {
			return outcome{}, err
		}
		if found == 0 {
			item.status, item.message = "failed", fmt.Sprintf("system %s has no planet in orbit %d", displaySystem(order.requestedSystem), order.orbit)
			break
		}
		probedSystemID, planetID, habitability = systemID, found, hab
		spent[order.entityID]++
	}

	if item.status == "succeeded" {
		if err := recordProbeFindings(conn, gameID, turn, order.factionID, planetID); err != nil {
			return outcome{}, err
		}
	}
	if err := updateProbeOutcome(conn, gameID, turn, order, item, probedSystemID, planetID, habitability); err != nil {
		return outcome{}, err
	}
	return item, nil
}

// recordPassiveSensors snapshots what every sensor-equipped entity reads from
// where it stands at the start of the turn. The reading is stored rather than
// derived at report time because the entity may move or jump later in the turn.
func recordPassiveSensors(conn *sqlite.Conn, gameID int64, turn int, entities map[int64]*entity) error {
	ids := make([]int64, 0, len(entities))
	for id, item := range entities {
		if item.sensors.Installed() {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		item := entities[id]
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT OR REPLACE INTO sensor_survey (game_id, turn, faction_id, entity_id, stellium_id, system_id, systems)
			VALUES (?, ?, ?, ?, ?, ?, (SELECT count(*) FROM system WHERE stellium_id = ?));`, &sqlitex.ExecOptions{
			Args: []any{gameID, turn, item.factionID, id, item.location.stelliumID,
				nullableID(item.location.systemID), item.location.stelliumID},
		}); err != nil {
			return fmt.Errorf("record sensor survey for entity %d: %w", id, err)
		}
		if item.location.systemID == 0 {
			continue
		}
		// At a planet the sensors also read every ship and orbital colony
		// around every planet of that system.
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT OR REPLACE INTO sensor_contact (game_id, turn, faction_id, entity_id, planet_id, contact_id, unit, planet_ring, mass)
			SELECT ?, ?, ?, ?, c.planet_id, c.id, c.unit, c.planet_ring, c.mass
			FROM entity AS c
			JOIN planet AS p ON p.id = c.planet_id
			WHERE p.system_id = ? AND c.unit IN ('SHIP', 'CORB');`, &sqlitex.ExecOptions{
			Args: []any{gameID, turn, item.factionID, id, item.location.systemID},
		}); err != nil {
			return fmt.Errorf("record sensor contacts for entity %d: %w", id, err)
		}
	}
	return nil
}

func findPlanetInOrbit(conn *sqlite.Conn, systemID int64, orbit int) (planetID int64, habitability int, err error) {
	if err := sqlitex.ExecuteTransient(conn, "SELECT id, habitability FROM planet WHERE system_id = ? AND orbit = ?;", &sqlitex.ExecOptions{
		Args: []any{systemID, orbit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			planetID, habitability = stmt.ColumnInt64(0), stmt.ColumnInt(1)
			return nil
		},
	}); err != nil {
		return 0, 0, fmt.Errorf("find planet in orbit %d: %w", orbit, err)
	}
	return planetID, habitability, nil
}

// recordProbeFindings snapshots everything at a planet. Probing the same planet
// twice in one turn re-reads it rather than failing on the recorded finding.
func recordProbeFindings(conn *sqlite.Conn, gameID int64, turn int, factionID, planetID int64) error {
	if err := sqlitex.ExecuteTransient(conn, `
		INSERT OR REPLACE INTO probe_contact (game_id, turn, faction_id, planet_id, entity_id, unit, planet_ring, mass)
		SELECT ?, ?, ?, ?, e.id, e.unit, e.planet_ring, e.mass
		FROM entity AS e WHERE e.planet_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn, factionID, planetID, planetID},
	}); err != nil {
		return fmt.Errorf("record probe contacts at planet %d: %w", planetID, err)
	}
	if err := sqlitex.ExecuteTransient(conn, `
		INSERT OR REPLACE INTO probe_deposit (game_id, turn, faction_id, planet_id, deposit_id, resource, quantity)
		SELECT ?, ?, ?, ?, d.id, d.resource, d.current_qty
		FROM deposit AS d WHERE d.planet_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameID, turn, factionID, planetID, planetID},
	}); err != nil {
		return fmt.Errorf("record probe deposits at planet %d: %w", planetID, err)
	}
	return nil
}

func probeRequest(order probeOrder) string {
	if order.requestedSystem == "" {
		return fmt.Sprintf("orbit %d", order.orbit)
	}
	return fmt.Sprintf("system %s orbit %d", order.requestedSystem, order.orbit)
}

func findSystemInStellium(conn *sqlite.Conn, stelliumID int64, sequence string) (systemID int64, err error) {
	if err := sqlitex.ExecuteTransient(conn, "SELECT id FROM system WHERE stellium_id = ? AND sequence = ?;", &sqlitex.ExecOptions{
		Args: []any{stelliumID, sequence},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			systemID = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find system %s: %w", sequence, err)
	}
	return systemID, nil
}

func updateProbeOutcome(conn *sqlite.Conn, gameID int64, turn int, order probeOrder, item outcome, systemID, planetID int64, habitability int) error {
	args := []any{item.status, nullableMessage(item.message)}
	if item.status == "succeeded" {
		args = append(args, item.start.stelliumID, systemID, planetID, habitability)
	} else {
		args = append(args, nil, nil, nil, nil)
	}
	args = append(args, gameID, turn, order.factionID, order.sequence)
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE probe_order
		SET status = ?, error_message = ?, stellium_id = ?, system_id = ?, planet_id = ?, habitability = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ?;`, &sqlitex.ExecOptions{Args: args}); err != nil {
		return fmt.Errorf("update probe order faction %d sequence %d: %w", order.factionID, order.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("probe order faction %d sequence %d changed while it was resolving", order.factionID, order.sequence)
	}
	return nil
}

func updateEntityLocation(conn *sqlite.Conn, entityID int64, loc location) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE entity SET stellium_id = ?, system_id = ?, planet_id = ?, planet_ring = ? WHERE id = ?;`, &sqlitex.ExecOptions{
		Args: []any{loc.stelliumID, nullableID(loc.systemID), nullableID(loc.planetID), nullableRing(loc), entityID},
	}); err != nil {
		return fmt.Errorf("update entity %d location: %w", entityID, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("update entity %d location: entity does not exist", entityID)
	}
	return nil
}

func updateMoveOutcome(conn *sqlite.Conn, gameID int64, turn int, order moveOrder, item outcome) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE move_order SET status = ?, error_message = ?,
			start_stellium_id = ?, start_system_id = ?, start_planet_id = ?, start_planet_ring = ?,
			final_stellium_id = ?, final_system_id = ?, final_planet_id = ?, final_planet_ring = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ? AND status = 'pending';`, &sqlitex.ExecOptions{
		Args: append(outcomeArgs(item), gameID, turn, order.factionID, order.sequence),
	}); err != nil {
		return fmt.Errorf("record move order faction %d sequence %d outcome: %w", order.factionID, order.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("record move order faction %d sequence %d outcome: order changed", order.factionID, order.sequence)
	}
	return nil
}

func updateJumpOutcome(conn *sqlite.Conn, gameID int64, turn int, order jumpOrder, item outcome) error {
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE jump_order SET status = ?, error_message = ?,
			start_stellium_id = ?, start_system_id = ?, start_planet_id = ?, start_planet_ring = ?,
			final_stellium_id = ?, final_system_id = ?, final_planet_id = ?, final_planet_ring = ?
		WHERE game_id = ? AND turn = ? AND faction_id = ? AND sequence = ? AND status = 'pending';`, &sqlitex.ExecOptions{
		Args: append(outcomeArgs(item), gameID, turn, order.factionID, order.sequence),
	}); err != nil {
		return fmt.Errorf("record jump order faction %d sequence %d outcome: %w", order.factionID, order.sequence, err)
	}
	if conn.Changes() != 1 {
		return fmt.Errorf("record jump order faction %d sequence %d outcome: order changed", order.factionID, order.sequence)
	}
	return nil
}

func outcomeArgs(item outcome) []any {
	return []any{
		item.status, nullableMessage(item.message),
		item.start.stelliumID, nullableID(item.start.systemID), nullableID(item.start.planetID), nullableRing(item.start),
		item.final.stelliumID, nullableID(item.final.systemID), nullableID(item.final.planetID), nullableRing(item.final),
	}
}

func readLocation(stmt *sqlite.Stmt, column int) location {
	loc := location{stelliumID: stmt.ColumnInt64(column)}
	if !stmt.ColumnIsNull(column + 1) {
		loc.systemID = stmt.ColumnInt64(column + 1)
		loc.planetID = stmt.ColumnInt64(column + 2)
		loc.ring = stmt.ColumnInt(column + 3)
	}
	return loc
}

func nullableText(stmt *sqlite.Stmt, column int) string {
	if stmt.ColumnIsNull(column) {
		return ""
	}
	return stmt.ColumnText(column)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableRing(loc location) any {
	if loc.systemID == 0 {
		return nil
	}
	return loc.ring
}

func nullableMessage(message string) any {
	if message == "" {
		return nil
	}
	return message
}

func displaySystem(system string) string {
	if system == "" {
		return "current"
	}
	return system
}

func countOutcome(result *Result, item outcome) {
	if item.status == "succeeded" {
		result.Succeeded++
	} else {
		result.Failed++
	}
}

// actorAttr names the entity an order acted on. Moves and jumps are always
// ship orders; a probe may be issued by a colony.
func actorAttr(item outcome) slog.Attr {
	if item.orderType == "probe" {
		return slog.Int64("entity_id", item.shipID)
	}
	return slog.Int64("ship_id", item.shipID)
}

func locationAttr(name string, loc location) slog.Attr {
	values := []any{"stellium_id", loc.stelliumID}
	if loc.systemID != 0 {
		values = append(values, "system_id", loc.systemID, "planet_id", loc.planetID, "ring", loc.ring)
	}
	return slog.Group(name, values...)
}
