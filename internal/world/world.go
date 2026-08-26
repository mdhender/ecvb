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
}

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

// Move puts an entity at a location, in the database and in the loaded copy.
func (w *World) Move(entity *Entity, to Location) error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		UPDATE entity SET stellium_id = ?, system_id = ?, planet_id = ?, planet_ring = ? WHERE id = ?;`,
		&sqlitex.ExecOptions{
			Args: []any{to.StelliumID, nullableID(to.SystemID), nullableID(to.PlanetID), nullableRing(to), entity.ID},
		}); err != nil {
		return fmt.Errorf("update entity %d location: %w", entity.ID, err)
	}
	if w.conn.Changes() != 1 {
		return fmt.Errorf("update entity %d location: entity does not exist", entity.ID)
	}
	entity.Location = to
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
