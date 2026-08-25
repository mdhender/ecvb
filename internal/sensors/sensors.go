// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package sensors implements the SNSR rules that tell a faction what its ships
// can see and how many probes they can launch.
package sensors

import (
	"fmt"
	"math"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Unit is the inventory unit code of a sensor. Only assembled units in the
// component section see anything; SNSR held anywhere else is freight.
const Unit = "SNSR"

// MassPerTechLevel is the mass in MU of one SNSR unit per technology level.
const MassPerTechLevel = 40

// UnitMass returns the mass in MU of one SNSR unit at techLevel.
func UnitMass(techLevel int) int64 { return MassPerTechLevel * int64(techLevel) }

// Array is the sensor array assembled from an entity's component SNSR units.
// The zero Array is an entity that senses nothing and cannot probe.
type Array struct {
	Units  int64 // assembled SNSR units
	Probes int64 // probes the array launches in one turn
}

// Add installs quantity SNSR units at techLevel. Each unit contributes one
// probe per technology level, so 5 SNSR-1 and 3 SNSR-2 launch 11 probes.
func (a Array) Add(techLevel int, quantity int64) Array {
	if quantity <= 0 {
		return a
	}
	a.Units += quantity
	a.Probes += quantity * int64(techLevel)
	return a
}

// Installed reports whether the entity carries sensors.
func (a Array) Installed() bool { return a.Units > 0 }

// ApproximateMass reports a mass as its order of magnitude in base 10, which is
// the precision a passive sensor reading carries. A mass of 1999 MU reads as 3.
func ApproximateMass(mass int64) int {
	if mass < 1 {
		return 0
	}
	return int(math.Floor(math.Log10(float64(mass))))
}

// Load returns the sensor array assembled on one entity.
func Load(conn *sqlite.Conn, entityID int64) (Array, error) {
	var array Array
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT tech_level, quantity
		FROM inventory
		WHERE entity_id = ? AND section = 'component' AND unit = ? AND quantity > 0;`, &sqlitex.ExecOptions{
		Args: []any{entityID, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			array = array.Add(stmt.ColumnInt(0), stmt.ColumnInt64(1))
			return nil
		},
	}); err != nil {
		return Array{}, fmt.Errorf("load sensors for entity %d: %w", entityID, err)
	}
	return array, nil
}

// LoadAll returns the sensor array assembled on every entity in a game.
func LoadAll(conn *sqlite.Conn, gameID int64) (map[int64]Array, error) {
	arrays := make(map[int64]Array)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT i.entity_id, i.tech_level, i.quantity
		FROM inventory AS i
		JOIN entity AS e ON e.id = i.entity_id
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ? AND i.section = 'component' AND i.unit = ? AND i.quantity > 0;`, &sqlitex.ExecOptions{
		Args: []any{gameID, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id := stmt.ColumnInt64(0)
			arrays[id] = arrays[id].Add(stmt.ColumnInt(1), stmt.ColumnInt64(2))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load sensors: %w", err)
	}
	return arrays, nil
}
