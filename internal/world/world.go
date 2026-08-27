// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package world holds one game's live state: where its entities are, what
// they carry, and the map they move across.
//
// It exists so that a rule is written once. A player's order file is measured
// against a World and the turn is resolved against a World, so "this ship is
// too heavy for its drive" is one function rather than one in the submission
// path and a second in the engine. Every mutation writes through to SQLite and
// keeps the loaded copy in step, which is what lets the second order of a turn
// measure a ship as the first order left it.
//
// A World is loaded for one operation and thrown away. It is not a cache.
package world

import (
	"fmt"
	"slices"

	"github.com/mdhender/ecvb/internal/fuel"
	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Location is where an entity stands. A system of zero is the stellium orbit,
// which has no planet and no ring.
type Location struct {
	StelliumID int64
	SystemID   int64
	PlanetID   int64
	Ring       int
}

// Point is a stellium's place in the game's space.
type Point struct {
	X, Y, Z int
}

// Entity is one ship, colony, or station, with everything the rules weigh.
type Entity struct {
	ID        int64
	Unit      string
	FactionID int64
	Location  Location
	Mass      int64
	Fuel      int64
	Drive     jumpdrive.Drive
	Sensors   sensors.Array
	// Transit is the crossing this ship is making, or nil when it is on the
	// board. A ship in transit has no Location at all.
	Transit *Transit
}

// Transit is a crossing between stellia in progress: where the ship is bound
// and the turn it is due. It is what outlives the jump order that began it.
type Transit struct {
	DestinationID int64
	ArrivalTurn   int
}

// InTransit reports whether the entity is crossing between stellia, and so is
// nowhere: not probeable, not on a sensor sweep, and not able to be given an
// order until it arrives.
func (e *Entity) InTransit() bool { return e.Transit != nil }

// Planet is a planet of a system, as much of it as the rules read.
type Planet struct {
	ID           int64
	Habitability int
}

// Game is the game a World belongs to.
type Game struct {
	ID    int64
	Code  string
	Turn  int
	State string
	Seed  Seed
}

// World is one game's state.
type World struct {
	conn     *sqlite.Conn
	game     Game
	entities map[int64]*Entity
	stellia  map[int64]Point
	atPoint  map[Point]int64
	// probes counts the probes each entity has launched this turn. It is turn
	// state rather than stored state: the budget refills every turn.
	probes map[int64]int64
}

// Load reads a game into memory. found is false when no game has that code,
// which is a player's mistake in a file header rather than a failure.
func Load(conn *sqlite.Conn, gameCode string) (w *World, found bool, err error) {
	loaded := &World{
		conn:     conn,
		entities: make(map[int64]*Entity),
		stellia:  make(map[int64]Point),
		atPoint:  make(map[Point]int64),
		probes:   make(map[int64]int64),
	}
	loaded.game.Code = gameCode
	if err := sqlitex.ExecuteTransient(conn,
		"SELECT id, turn, turn_state, seed_high, seed_low FROM game WHERE code = ?;", &sqlitex.ExecOptions{
			Args: []any{gameCode},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				loaded.game.ID, loaded.game.Turn, loaded.game.State = stmt.ColumnInt64(0), stmt.ColumnInt(1), stmt.ColumnText(2)
				loaded.game.Seed = Seed{High: stmt.ColumnInt64(3), Low: stmt.ColumnInt64(4)}
				found = true
				return nil
			},
		}); err != nil {
		return nil, false, fmt.Errorf("find game %q: %w", gameCode, err)
	}
	if !found {
		return nil, false, nil
	}
	if err := loaded.loadStellia(); err != nil {
		return nil, false, err
	}
	if err := loaded.loadEntities(); err != nil {
		return nil, false, err
	}
	return loaded, true, nil
}

// Game is the game this World holds.
func (w *World) Game() Game { return w.game }

// Entity is the entity with an id, or nil when the game holds none. An entity
// of another game is not in this world and reads as absent.
func (w *World) Entity(id int64) *Entity { return w.entities[id] }

// Entities lists every entity of the game, ordered by id so that a turn step
// walking all of them reaches them in the same order every time.
func (w *World) Entities() []*Entity {
	ids := make([]int64, 0, len(w.entities))
	for id := range w.entities {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	list := make([]*Entity, 0, len(ids))
	for _, id := range ids {
		list = append(list, w.entities[id])
	}
	return list
}

// Coordinates is where a stellium sits. An unknown stellium reads as the
// origin, which no caller reaches: every entity stands in a stellium of its
// own game.
func (w *World) Coordinates(stelliumID int64) Point { return w.stellia[stelliumID] }

// StelliumAt is the stellium at a point, or 0 when that point is empty space.
func (w *World) StelliumAt(x, y, z int) int64 { return w.atPoint[Point{X: x, Y: y, Z: z}] }

// System is the system with a letter in a stellium, or 0 when the stellium has
// no such system.
func (w *World) System(stelliumID int64, letter string) (int64, error) {
	var id int64
	if err := sqlitex.ExecuteTransient(w.conn,
		"SELECT id FROM system WHERE stellium_id = ? AND sequence = ?;", &sqlitex.ExecOptions{
			Args: []any{stelliumID, letter},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				id = stmt.ColumnInt64(0)
				return nil
			},
		}); err != nil {
		return 0, fmt.Errorf("find system %s: %w", letter, err)
	}
	return id, nil
}

// Planet is the planet in an orbit of a system. found is false when the orbit
// is empty.
func (w *World) Planet(systemID int64, orbit int) (planet Planet, found bool, err error) {
	if err := sqlitex.ExecuteTransient(w.conn,
		"SELECT id, habitability FROM planet WHERE system_id = ? AND orbit = ?;", &sqlitex.ExecOptions{
			Args: []any{systemID, orbit},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				planet, found = Planet{ID: stmt.ColumnInt64(0), Habitability: stmt.ColumnInt(1)}, true
				return nil
			},
		}); err != nil {
		return Planet{}, false, fmt.Errorf("find planet in orbit %d: %w", orbit, err)
	}
	return planet, found, nil
}

// Move puts an entity at a location, in the database and in the loaded copy. A
// zero stellium is nowhere at all, which only a ship crossing between stellia
// may be; Depart is what puts one there.
func (w *World) Move(entity *Entity, to Location) error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		UPDATE entity SET stellium_id = ?, system_id = ?, planet_id = ?, planet_ring = ? WHERE id = ?;`,
		&sqlitex.ExecOptions{
			Args: []any{nullableID(to.StelliumID), nullableID(to.SystemID), nullableID(to.PlanetID), nullableRing(to), entity.ID},
		}); err != nil {
		return fmt.Errorf("update entity %d location: %w", entity.ID, err)
	}
	if w.conn.Changes() != 1 {
		return fmt.Errorf("update entity %d location: entity does not exist", entity.ID)
	}
	entity.Location = to
	return nil
}

// Depart sends a ship on a crossing between stellia: it takes the ship off the
// board and records where it is bound and when it is due.
//
// The crossing outlives the order that began it, which is why it is a row of
// its own. The ship is nowhere while it stands -- no stellium, no system, no
// planet -- so nothing can see it and no order can reach it. A crossing that
// takes one turn is written here and consumed by the same turn's arrival step,
// which is what every jump did before a crossing could span turns.
func (w *World) Depart(entity *Entity, destinationID int64, arrivalTurn int) error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		INSERT INTO in_transit (game_id, entity_id, destination_stellium_id, arrival_turn)
		VALUES (?, ?, ?, ?);`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID, entity.ID, destinationID, arrivalTurn},
	}); err != nil {
		return fmt.Errorf("send entity %d in transit: %w", entity.ID, err)
	}
	if err := w.Move(entity, Location{}); err != nil {
		return err
	}
	entity.Transit = &Transit{DestinationID: destinationID, ArrivalTurn: arrivalTurn}
	return nil
}

// LandArrivals is the arrival step of ship movement: every ship whose crossing
// finished lands in its destination's stellium orbit and its crossing is over.
// A ship reaches a planet from there under its own power, with a MOVE, which
// is next turn's business.
//
// It is a sweep rather than an order because no player writes it: what the
// jump order left behind is what the step reads. Ships land in id order so the
// step does the same thing every time it is run.
func (w *World) LandArrivals(turn int) error {
	for _, entity := range w.Entities() {
		if entity.Transit == nil || entity.Transit.ArrivalTurn > turn {
			continue
		}
		destination := entity.Transit.DestinationID
		if err := sqlitex.ExecuteTransient(w.conn,
			"DELETE FROM in_transit WHERE entity_id = ?;", &sqlitex.ExecOptions{
				Args: []any{entity.ID},
			}); err != nil {
			return fmt.Errorf("land entity %d: %w", entity.ID, err)
		}
		entity.Transit = nil
		if err := w.Move(entity, Location{StelliumID: destination}); err != nil {
			return err
		}
	}
	return nil
}

// BurnFuel draws an order's fuel. Burned fuel leaves the entity, so its mass
// falls with it and a later order in the same turn measures the drive against
// the lighter ship.
func (w *World) BurnFuel(entity *Entity, quantity int64) error {
	if err := fuel.Spend(w.conn, entity.ID, quantity); err != nil {
		return err
	}
	entity.Fuel -= quantity
	entity.Mass -= quantity * fuel.UnitMass
	return nil
}

// ProbesSpent is how many probes an entity has launched this turn.
func (w *World) ProbesSpent(entityID int64) int64 { return w.probes[entityID] }

// SpendProbe charges one probe against an entity's budget for the turn.
func (w *World) SpendProbe(entityID int64) { w.probes[entityID]++ }

// RecordProbe snapshots everything at a planet for a faction. Probing the same
// planet twice in one turn re-reads it rather than failing on the finding
// already recorded.
func (w *World) RecordProbe(turn int, factionID, planetID int64) error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		INSERT OR REPLACE INTO probe_contact (game_id, turn, faction_id, planet_id, entity_id, unit, planet_ring, mass)
		SELECT ?, ?, ?, ?, e.id, e.unit, e.planet_ring, e.mass
		FROM entity AS e WHERE e.planet_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID, turn, factionID, planetID, planetID},
	}); err != nil {
		return fmt.Errorf("record probe contacts at planet %d: %w", planetID, err)
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		INSERT OR REPLACE INTO probe_deposit (game_id, turn, faction_id, planet_id, deposit_id, resource, quantity)
		SELECT ?, ?, ?, ?, d.id, d.resource, d.current_qty
		FROM deposit AS d WHERE d.planet_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID, turn, factionID, planetID, planetID},
	}); err != nil {
		return fmt.Errorf("record probe deposits at planet %d: %w", planetID, err)
	}
	return nil
}

// Subject is the kind of thing a faction can give a name to.
type Subject string

// The things a faction can name.
const (
	NamedStellium Subject = "stellium"
	NamedSystem   Subject = "system"
	NamedPlanet   Subject = "planet"
	NamedEntity   Subject = "entity"
)

// column is where a subject's id is stored.
func (s Subject) column() string { return string(s) + "_id" }

// SetName is what a faction calls something, replacing whatever it called it
// before. The name is the faction's own: naming a ship does not change what
// anybody else's report calls it, and naming a stellium takes no visit.
func (w *World) SetName(factionID int64, of Subject, id int64, name string) error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		INSERT INTO faction_name (game_id, faction_id, `+of.column()+`, name) VALUES (?, ?, ?, ?)
		ON CONFLICT (faction_id, `+of.column()+`) WHERE `+of.column()+` IS NOT NULL
		DO UPDATE SET name = excluded.name;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID, factionID, id, name},
	}); err != nil {
		return fmt.Errorf("name %s %d: %w", of, id, err)
	}
	return nil
}

// RecordSensors snapshots what every sensor-equipped entity reads from where it
// stands. The reading is stored rather than derived at report time because the
// entity may move or jump later in the turn, and what it saw is what it saw.
func (w *World) RecordSensors(turn int) error {
	for _, entity := range w.Entities() {
		if !entity.Sensors.Installed() {
			continue
		}
		// A ship crossing between stellia is nowhere, so its sensors have
		// nothing to read and nothing reads them.
		if entity.InTransit() {
			continue
		}
		if err := sqlitex.ExecuteTransient(w.conn, `
			INSERT OR REPLACE INTO sensor_survey (game_id, turn, faction_id, entity_id, stellium_id, system_id, systems)
			VALUES (?, ?, ?, ?, ?, ?, (SELECT count(*) FROM system WHERE stellium_id = ?));`, &sqlitex.ExecOptions{
			Args: []any{w.game.ID, turn, entity.FactionID, entity.ID, entity.Location.StelliumID,
				nullableID(entity.Location.SystemID), entity.Location.StelliumID},
		}); err != nil {
			return fmt.Errorf("record sensor survey for entity %d: %w", entity.ID, err)
		}
		if entity.Location.SystemID == 0 {
			continue
		}
		// At a planet the sensors also read every ship and orbital colony
		// around every planet of that system.
		if err := sqlitex.ExecuteTransient(w.conn, `
			INSERT OR REPLACE INTO sensor_contact (game_id, turn, faction_id, entity_id, planet_id, contact_id, unit, planet_ring, mass)
			SELECT ?, ?, ?, ?, c.planet_id, c.id, c.unit, c.planet_ring, c.mass
			FROM entity AS c
			JOIN planet AS p ON p.id = c.planet_id
			WHERE p.system_id = ? AND c.unit IN ('SHIP', 'CORB');`, &sqlitex.ExecOptions{
			Args: []any{w.game.ID, turn, entity.FactionID, entity.ID, entity.Location.SystemID},
		}); err != nil {
			return fmt.Errorf("record sensor contacts for entity %d: %w", entity.ID, err)
		}
	}
	return nil
}

func (w *World) loadStellia() error {
	if err := sqlitex.ExecuteTransient(w.conn,
		"SELECT id, x, y, z FROM stellium WHERE game_id = ?;", &sqlitex.ExecOptions{
			Args: []any{w.game.ID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				id := stmt.ColumnInt64(0)
				at := Point{X: stmt.ColumnInt(1), Y: stmt.ColumnInt(2), Z: stmt.ColumnInt(3)}
				w.stellia[id], w.atPoint[at] = at, id
				return nil
			},
		}); err != nil {
		return fmt.Errorf("load stellia: %w", err)
	}
	return nil
}

func (w *World) loadEntities() error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT e.id, e.unit, e.faction_id, e.stellium_id, e.system_id, e.planet_id, e.planet_ring, e.mass
		FROM entity AS e
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id := stmt.ColumnInt64(0)
			w.entities[id] = &Entity{
				ID: id, Unit: stmt.ColumnText(1), FactionID: stmt.ColumnInt64(2),
				Location: readLocation(stmt, 3), Mass: stmt.ColumnInt64(7),
			}
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load entities: %w", err)
	}
	drives, err := jumpdrive.LoadAll(w.conn, w.game.ID)
	if err != nil {
		return err
	}
	for id, drive := range drives {
		if entity, ok := w.entities[id]; ok {
			entity.Drive = drive
		}
	}
	arrays, err := sensors.LoadAll(w.conn, w.game.ID)
	if err != nil {
		return err
	}
	for id, array := range arrays {
		if entity, ok := w.entities[id]; ok {
			entity.Sensors = array
		}
	}
	quantities, err := fuel.AvailableAll(w.conn, w.game.ID)
	if err != nil {
		return err
	}
	for id, quantity := range quantities {
		if entity, ok := w.entities[id]; ok {
			entity.Fuel = quantity
		}
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT entity_id, destination_stellium_id, arrival_turn
		FROM in_transit WHERE game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if entity, ok := w.entities[stmt.ColumnInt64(0)]; ok {
				entity.Transit = &Transit{DestinationID: stmt.ColumnInt64(1), ArrivalTurn: stmt.ColumnInt(2)}
			}
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load ships in transit: %w", err)
	}
	return nil
}

func readLocation(stmt *sqlite.Stmt, column int) Location {
	at := Location{StelliumID: stmt.ColumnInt64(column)}
	if !stmt.ColumnIsNull(column + 1) {
		at.SystemID = stmt.ColumnInt64(column + 1)
		at.PlanetID = stmt.ColumnInt64(column + 2)
		at.Ring = stmt.ColumnInt(column + 3)
	}
	return at
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableRing(at Location) any {
	if at.SystemID == 0 {
		return nil
	}
	return at.Ring
}
