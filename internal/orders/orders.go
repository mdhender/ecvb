// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Result summarizes a checked or submitted order file.
type Result struct {
	GameCode  string
	Turn      int
	FactionID int64
	Orders    int
}

type storedMove struct {
	sequence              int
	line                  int
	shipID                int64
	requestedSystem       string
	requestedOrbit        int
	destinationStelliumID int64
	destinationSystemID   int64
	destinationPlanetID   int64
}

type storedJump struct {
	sequence              int
	line                  int
	shipID                int64
	x                     int
	y                     int
	z                     int
	destinationStelliumID int64
}

type shipLocation struct {
	stelliumID int64
	x, y, z    int
	systemID   int64
	planetID   int64
	mass       int64
	drive      jumpdrive.Drive
	sensors    sensors.Array
}

type storedProbe struct {
	sequence        int
	line            int
	shipID          int64
	requestedSystem string
	orbit           int
}

type validatedSubmission struct {
	result Result
	gameID int64
	moves  []storedMove
	probes []storedProbe
	jumps  []storedJump
}

// Check parses and validates an order file without changing the database.
func Check(ctx context.Context, conn *sqlite.Conn, r io.Reader) (result Result, err error) {
	submission, err := Parse(r)
	if err != nil {
		return Result{}, err
	}
	end := sqlitex.Transaction(conn)
	defer end(&err)

	validated, err := validate(ctx, conn, submission)
	if err != nil {
		return Result{}, err
	}
	return validated.result, nil
}

// Submit parses and validates an order file, then atomically replaces the
// faction's submitted orders.
func Submit(ctx context.Context, conn *sqlite.Conn, r io.Reader) (result Result, err error) {
	submission, err := Parse(r)
	if err != nil {
		return Result{}, err
	}
	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return Result{}, fmt.Errorf("begin submit orders transaction: %w", err)
	}
	defer end(&err)

	validated, err := validate(ctx, conn, submission)
	if err != nil {
		return Result{}, err
	}
	if err := sqlitex.ExecuteTransient(conn, "DELETE FROM move_order WHERE game_id = ? AND turn = ? AND faction_id = ?;", &sqlitex.ExecOptions{
		Args: []any{validated.gameID, validated.result.Turn, validated.result.FactionID},
	}); err != nil {
		return Result{}, fmt.Errorf("delete previous move orders: %w", err)
	}
	if err := sqlitex.ExecuteTransient(conn, "DELETE FROM jump_order WHERE game_id = ? AND turn = ? AND faction_id = ?;", &sqlitex.ExecOptions{
		Args: []any{validated.gameID, validated.result.Turn, validated.result.FactionID},
	}); err != nil {
		return Result{}, fmt.Errorf("delete previous jump orders: %w", err)
	}
	for _, table := range []string{"probe_contact", "probe_deposit", "probe_order"} {
		if err := sqlitex.ExecuteTransient(conn, fmt.Sprintf("DELETE FROM %s WHERE game_id = ? AND turn = ? AND faction_id = ?;", table), &sqlitex.ExecOptions{
			Args: []any{validated.gameID, validated.result.Turn, validated.result.FactionID},
		}); err != nil {
			return Result{}, fmt.Errorf("delete previous probe orders: %w", err)
		}
	}
	for _, order := range validated.probes {
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO probe_order (
				game_id, turn, faction_id, sequence, source_line, entity_id, requested_system, requested_orbit
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
			Args: []any{validated.gameID, submission.Turn, validated.result.FactionID,
				order.sequence, order.line, order.shipID, nullableString(order.requestedSystem), order.orbit},
		}); err != nil {
			return Result{}, fmt.Errorf("insert probe order from line %d: %w", order.line, err)
		}
	}
	for _, order := range validated.moves {
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO move_order (
				game_id, turn, faction_id, sequence, source_line, ship_id,
				requested_system, requested_orbit,
				destination_stellium_id, destination_system_id, destination_planet_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
			Args: []any{validated.gameID, submission.Turn, validated.result.FactionID,
				order.sequence, order.line, order.shipID, nullableString(order.requestedSystem), order.requestedOrbit,
				order.destinationStelliumID, order.destinationSystemID, order.destinationPlanetID},
		}); err != nil {
			return Result{}, fmt.Errorf("insert move order from line %d: %w", order.line, err)
		}
	}
	for _, order := range validated.jumps {
		if err := sqlitex.ExecuteTransient(conn, `
			INSERT INTO jump_order (
				game_id, turn, faction_id, sequence, source_line, ship_id,
				destination_x, destination_y, destination_z, destination_stellium_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`, &sqlitex.ExecOptions{
			Args: []any{validated.gameID, submission.Turn, validated.result.FactionID,
				order.sequence, order.line, order.shipID, order.x, order.y, order.z, order.destinationStelliumID},
		}); err != nil {
			return Result{}, fmt.Errorf("insert jump order from line %d: %w", order.line, err)
		}
	}
	return validated.result, nil
}

func validate(ctx context.Context, conn *sqlite.Conn, submission Submission) (validatedSubmission, error) {
	if err := ctx.Err(); err != nil {
		return validatedSubmission{}, err
	}
	gameID, currentTurn, turnState, found, err := findGame(conn, submission.GameCode)
	if err != nil {
		return validatedSubmission{}, err
	}
	if !found {
		return validatedSubmission{}, problems{{1, fmt.Sprintf("game %q does not exist", submission.GameCode)}}
	}
	var foundProblems problems
	if currentTurn != submission.Turn {
		foundProblems = append(foundProblems, problem{1, fmt.Sprintf("game %q is on turn %d, not turn %d", submission.GameCode, currentTurn, submission.Turn)})
	}
	if turnState != "open" {
		foundProblems = append(foundProblems, problem{1, fmt.Sprintf("game %q turn %d is resolved and not accepting orders", submission.GameCode, currentTurn)})
	}
	factionID, identityProblems, err := resolveFaction(conn, gameID, submission.Identity)
	if err != nil {
		return validatedSubmission{}, err
	}
	foundProblems = append(foundProblems, identityProblems...)
	if factionID == 0 {
		return validatedSubmission{}, foundProblems
	}

	locations := make(map[int64]shipLocation)
	moves := make([]storedMove, 0, len(submission.Orders))
	probes := make([]storedProbe, 0, len(submission.Orders))
	jumps := make([]storedJump, 0, len(submission.Orders))
	resolutionSequence := 0
	spent := make(map[int64]int64)
	for _, order := range submission.Orders {
		if order.Verb != "probe" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return validatedSubmission{}, err
		}
		location, ok := locations[order.ShipID]
		if !ok {
			var orderProblems problems
			location, orderProblems, err = findEntity(conn, gameID, factionID, order)
			if err != nil {
				return validatedSubmission{}, err
			}
			if len(orderProblems) != 0 {
				foundProblems = append(foundProblems, orderProblems...)
				continue
			}
			locations[order.ShipID] = location
		}
		if !location.sensors.Installed() {
			foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("%s %d has no assembled %s and cannot probe", order.Actor, order.ShipID, sensors.Unit)})
			continue
		}
		// A probe that names a system reads any system of the ship's stellium.
		// A probe that does not reads the system the ship is in, which is why
		// a ship orbiting the stellium has to name one.
		systemID := location.systemID
		if order.System != "" {
			var exists bool
			systemID, exists, err = findSystem(conn, location.stelliumID, order.System)
			if err != nil {
				return validatedSubmission{}, err
			}
			if !exists {
				foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("current stellium has no system %s", order.System)})
				continue
			}
		} else if systemID == 0 {
			foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("%s %d is orbiting the stellium; name a system to probe", order.Actor, order.ShipID)})
			continue
		}
		for _, orbit := range order.Orbits {
			if spent[order.ShipID] == location.sensors.Probes {
				foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("%s %d has only %d probes this turn", order.Actor, order.ShipID, location.sensors.Probes)})
				break
			}
			if orbit < 1 || orbit > 10 {
				foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("orbit %d is not between 1 and 10", orbit)})
				continue
			}
			if _, exists, err := findPlanet(conn, systemID, orbit); err != nil {
				return validatedSubmission{}, err
			} else if !exists {
				system := order.System
				if system == "" {
					system = "current"
				}
				foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("system %s has no planet in orbit %d", system, orbit)})
				continue
			}
			spent[order.ShipID]++
			resolutionSequence++
			probes = append(probes, storedProbe{sequence: resolutionSequence, line: order.Line, shipID: order.ShipID,
				requestedSystem: order.System, orbit: orbit})
		}
	}
	for _, order := range submission.Orders {
		if order.Verb != "move" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return validatedSubmission{}, err
		}
		location, ok := locations[order.ShipID]
		if !ok {
			var orderProblems problems
			location, orderProblems, err = findEntity(conn, gameID, factionID, order)
			if err != nil {
				return validatedSubmission{}, err
			}
			if len(orderProblems) != 0 {
				foundProblems = append(foundProblems, orderProblems...)
				continue
			}
			locations[order.ShipID] = location
		}

		systemID := location.systemID
		if order.System != "" {
			var exists bool
			systemID, exists, err = findSystem(conn, location.stelliumID, order.System)
			if err != nil {
				return validatedSubmission{}, err
			}
			if !exists {
				foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("current stellium has no system %s", order.System)})
				continue
			}
		} else if systemID == 0 {
			foundProblems = append(foundProblems, problem{order.Line, "ship has no current system; specify a destination system"})
			continue
		}
		planetID, exists, err := findPlanet(conn, systemID, order.Orbit)
		if err != nil {
			return validatedSubmission{}, err
		}
		if !exists {
			system := order.System
			if system == "" {
				system = "current"
			}
			foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("system %s has no planet in orbit %d", system, order.Orbit)})
			continue
		}
		location.systemID, location.planetID = systemID, planetID
		locations[order.ShipID] = location
		resolutionSequence++
		moves = append(moves, storedMove{
			sequence: resolutionSequence, line: order.Line, shipID: order.ShipID,
			requestedSystem: order.System, requestedOrbit: order.Orbit,
			destinationStelliumID: location.stelliumID, destinationSystemID: systemID, destinationPlanetID: planetID,
		})
	}
	for _, order := range submission.Orders {
		if order.Verb != "jump" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return validatedSubmission{}, err
		}
		location, ok := locations[order.ShipID]
		if !ok {
			var orderProblems problems
			location, orderProblems, err = findEntity(conn, gameID, factionID, order)
			if err != nil {
				return validatedSubmission{}, err
			}
			if len(orderProblems) != 0 {
				foundProblems = append(foundProblems, orderProblems...)
				continue
			}
			locations[order.ShipID] = location
		}
		destinationID, exists, err := findStellium(conn, gameID, order.X, order.Y, order.Z)
		if err != nil {
			return validatedSubmission{}, err
		}
		if !exists {
			foundProblems = append(foundProblems, problem{order.Line, fmt.Sprintf("game %q has no stellium at (%d,%d,%d)", submission.GameCode, order.X, order.Y, order.Z)})
			continue
		}
		if message := jumpProblem(location, order); message != "" {
			foundProblems = append(foundProblems, problem{order.Line, message})
			continue
		}
		location.stelliumID, location.x, location.y, location.z = destinationID, order.X, order.Y, order.Z
		location.systemID, location.planetID = 0, 0
		locations[order.ShipID] = location
		resolutionSequence++
		jumps = append(jumps, storedJump{
			sequence: resolutionSequence, line: order.Line, shipID: order.ShipID,
			x: order.X, y: order.Y, z: order.Z, destinationStelliumID: destinationID,
		})
	}
	if len(foundProblems) != 0 {
		return validatedSubmission{}, foundProblems
	}
	return validatedSubmission{
		result: Result{GameCode: submission.GameCode, Turn: submission.Turn, FactionID: factionID, Orders: len(moves) + len(probes) + len(jumps)},
		gameID: gameID,
		moves:  moves,
		probes: probes,
		jumps:  jumps,
	}, nil
}

// jumpProblem returns the reason a ship cannot make a jump, or an empty string
// when the jump is within its drive's range and capacity. The engine applies the
// same rules when it resolves the turn.
func jumpProblem(location shipLocation, order Order) string {
	if !location.drive.Installed() {
		return fmt.Sprintf("ship %d has no assembled %s and cannot jump", order.ShipID, jumpdrive.Unit)
	}
	if !location.drive.CanPropel(location.mass) {
		return fmt.Sprintf("ship %d masses %d MU and its jump drive propels %d MU",
			order.ShipID, location.mass, location.drive.Capacity)
	}
	if !location.drive.Reaches(jumpdrive.SquaredDistance(location.x, location.y, location.z, order.X, order.Y, order.Z)) {
		return fmt.Sprintf("jump of %d units exceeds ship %d jump range of %d units",
			jumpdrive.Distance(location.x, location.y, location.z, order.X, order.Y, order.Z), order.ShipID, location.drive.Range)
	}
	return ""
}

func findGame(conn *sqlite.Conn, code string) (id int64, turn int, state string, found bool, err error) {
	err = sqlitex.ExecuteTransient(conn, "SELECT id, turn, turn_state FROM game WHERE code = ?;", &sqlitex.ExecOptions{
		Args: []any{code},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, turn, state, found = stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnText(2), true
			return nil
		},
	})
	if err != nil {
		err = fmt.Errorf("find game %q: %w", code, err)
	}
	return
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func resolveFaction(conn *sqlite.Conn, gameID int64, identity Identity) (int64, problems, error) {
	if identity.PlayerEmail != "" {
		email := strings.ToLower(strings.TrimSpace(identity.PlayerEmail))
		address, parseErr := mail.ParseAddress(email)
		if parseErr != nil || address.Address != email {
			return 0, problems{{2, fmt.Sprintf("invalid player email %q", email)}}, nil
		}
		var factionID int64
		err := sqlitex.ExecuteTransient(conn, `
			SELECT f.id FROM faction AS f JOIN users AS u ON u.id = f.user_id
			WHERE f.game_id = ? AND u.email = ?;`, &sqlitex.ExecOptions{
			Args: []any{gameID, email},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				factionID = stmt.ColumnInt64(0)
				return nil
			},
		})
		if err != nil {
			return 0, nil, fmt.Errorf("find player %q: %w", email, err)
		}
		if factionID == 0 {
			return 0, problems{{2, fmt.Sprintf("player %q does not belong to this game", email)}}, nil
		}
		return factionID, nil, nil
	}
	var belongs bool
	err := sqlitex.ExecuteTransient(conn, "SELECT EXISTS (SELECT 1 FROM faction WHERE id = ? AND game_id = ?);", &sqlitex.ExecOptions{
		Args: []any{identity.FactionID, gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			belongs = stmt.ColumnInt(0) != 0
			return nil
		},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("find faction %d: %w", identity.FactionID, err)
	}
	if !belongs {
		return 0, problems{{2, fmt.Sprintf("faction %d does not belong to this game", identity.FactionID)}}, nil
	}
	return identity.FactionID, nil, nil
}

// findEntity locates the entity an order names and checks that its unit
// matches the keyword used. Only a probe may name a colony.
func findEntity(conn *sqlite.Conn, gameID, factionID int64, order Order) (shipLocation, problems, error) {
	var location shipLocation
	var unit string
	var ownerID, factionGameID, stelliumGameID int64
	found := false
	err := sqlitex.ExecuteTransient(conn, `
		SELECT e.unit, e.faction_id, e.stellium_id, e.system_id, e.planet_id, f.game_id, st.game_id,
		       st.x, st.y, st.z, e.mass
		FROM entity AS e
		JOIN faction AS f ON f.id = e.faction_id
		JOIN stellium AS st ON st.id = e.stellium_id
		WHERE e.id = ?;`, &sqlitex.ExecOptions{
		Args: []any{order.ShipID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			unit = stmt.ColumnText(0)
			ownerID = stmt.ColumnInt64(1)
			location.stelliumID = stmt.ColumnInt64(2)
			if !stmt.ColumnIsNull(3) {
				location.systemID = stmt.ColumnInt64(3)
			}
			if !stmt.ColumnIsNull(4) {
				location.planetID = stmt.ColumnInt64(4)
			}
			factionGameID, stelliumGameID = stmt.ColumnInt64(5), stmt.ColumnInt64(6)
			location.x, location.y, location.z = stmt.ColumnInt(7), stmt.ColumnInt(8), stmt.ColumnInt(9)
			location.mass = stmt.ColumnInt64(10)
			found = true
			return nil
		},
	})
	if err != nil {
		return shipLocation{}, nil, fmt.Errorf("find ship %d: %w", order.ShipID, err)
	}
	if !found {
		return shipLocation{}, problems{{order.Line, fmt.Sprintf("%s %d does not exist", order.Actor, order.ShipID)}}, nil
	}
	if order.Actor == "colony" {
		if unit == "SHIP" {
			return shipLocation{}, problems{{order.Line, fmt.Sprintf("entity %d is a ship, not a colony", order.ShipID)}}, nil
		}
	} else if unit != "SHIP" {
		return shipLocation{}, problems{{order.Line, fmt.Sprintf("entity %d is a %s, not a ship", order.ShipID, unit)}}, nil
	}
	if ownerID != factionID {
		return shipLocation{}, problems{{order.Line, fmt.Sprintf("%s %d does not belong to faction %d", order.Actor, order.ShipID, factionID)}}, nil
	}
	if factionGameID != gameID || stelliumGameID != gameID {
		return shipLocation{}, problems{{order.Line, fmt.Sprintf("%s %d does not belong to this game", order.Actor, order.ShipID)}}, nil
	}
	if location.drive, err = jumpdrive.Load(conn, order.ShipID); err != nil {
		return shipLocation{}, nil, err
	}
	if location.sensors, err = sensors.Load(conn, order.ShipID); err != nil {
		return shipLocation{}, nil, err
	}
	return location, nil, nil
}

func findStellium(conn *sqlite.Conn, gameID int64, x, y, z int) (id int64, found bool, err error) {
	err = sqlitex.ExecuteTransient(conn, "SELECT id FROM stellium WHERE game_id = ? AND x = ? AND y = ? AND z = ?;", &sqlitex.ExecOptions{
		Args: []any{gameID, x, y, z},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, found = stmt.ColumnInt64(0), true
			return nil
		},
	})
	if err != nil {
		err = fmt.Errorf("find stellium at (%d,%d,%d): %w", x, y, z, err)
	}
	return
}

func findSystem(conn *sqlite.Conn, stelliumID int64, sequence string) (id int64, found bool, err error) {
	err = sqlitex.ExecuteTransient(conn, "SELECT id FROM system WHERE stellium_id = ? AND sequence = ?;", &sqlitex.ExecOptions{
		Args: []any{stelliumID, sequence},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, found = stmt.ColumnInt64(0), true
			return nil
		},
	})
	if err != nil {
		err = fmt.Errorf("find system %s: %w", sequence, err)
	}
	return
}

func findPlanet(conn *sqlite.Conn, systemID int64, orbit int) (id int64, found bool, err error) {
	err = sqlitex.ExecuteTransient(conn, "SELECT id FROM planet WHERE system_id = ? AND orbit = ?;", &sqlitex.ExecOptions{
		Args: []any{systemID, orbit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, found = stmt.ColumnInt64(0), true
			return nil
		},
	})
	if err != nil {
		err = fmt.Errorf("find planet in orbit %d: %w", orbit, err)
	}
	return
}
