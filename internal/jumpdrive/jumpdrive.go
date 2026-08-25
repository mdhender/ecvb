// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package jumpdrive implements the HDRV rules that limit how far a ship jumps
// and how much mass it can carry through a jump.
package jumpdrive

import (
	"fmt"
	"math"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Unit is the inventory unit code of a jump drive. Only assembled units in the
// component section propel a ship; HDRV held anywhere else is freight.
const Unit = "HDRV"

const (
	// MassPerTechLevel is the mass in MU of one HDRV unit per technology level.
	MassPerTechLevel = 45
	// PropulsionPerTechLevel is the mass in MU that one HDRV unit can propel
	// through one jump, per technology level.
	PropulsionPerTechLevel = 1045
)

// UnitMass returns the mass in MU of one HDRV unit at techLevel.
func UnitMass(techLevel int) int64 { return MassPerTechLevel * int64(techLevel) }

// Drive is the jump drive assembled from a ship's component HDRV units. The
// zero Drive is a ship that cannot jump at all.
type Drive struct {
	Units    int64 // assembled HDRV units
	Range    int   // longest jump in units of distance
	Capacity int64 // mass in MU the drive propels through one jump
}

// Add installs quantity HDRV units at techLevel.
//
// The lowest technology level installed limits the range of the whole drive,
// because every unit has to make the same jump. Capacity is the sum over the
// units of their own technology levels, so a mixed drive still carries the mass
// its high-technology units can propel.
func (d Drive) Add(techLevel int, quantity int64) Drive {
	if quantity <= 0 {
		return d
	}
	if d.Units == 0 || techLevel < d.Range {
		d.Range = techLevel
	}
	d.Units += quantity
	d.Capacity += quantity * PropulsionPerTechLevel * int64(techLevel)
	return d
}

// Installed reports whether the ship has a jump drive.
func (d Drive) Installed() bool { return d.Units > 0 }

// Reaches reports whether the drive crosses a jump of the given squared
// distance. The test stays in integer arithmetic: a jump distance is a
// Euclidean distance rounded up, so it is within an integer range exactly when
// the squared distance is within the squared range.
func (d Drive) Reaches(squaredDistance int) bool { return squaredDistance <= d.Range*d.Range }

// CanPropel reports whether the drive moves mass through a jump.
func (d Drive) CanPropel(mass int64) bool { return mass <= d.Capacity }

// SquaredDistance returns the squared distance between two stellia.
func SquaredDistance(x1, y1, z1, x2, y2, z2 int) int {
	dx, dy, dz := x1-x2, y1-y2, z1-z2
	return dx*dx + dy*dy + dz*dz
}

// Distance returns the distance between two stellia: their Euclidean distance
// rounded up to the next integer. Range tests use SquaredDistance; this reports
// a distance to a player.
func Distance(x1, y1, z1, x2, y2, z2 int) int {
	return int(math.Ceil(math.Sqrt(float64(SquaredDistance(x1, y1, z1, x2, y2, z2)))))
}

// Load returns the jump drive assembled on one entity.
func Load(conn *sqlite.Conn, entityID int64) (Drive, error) {
	var drive Drive
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT tech_level, quantity
		FROM inventory
		WHERE entity_id = ? AND section = 'component' AND unit = ? AND quantity > 0;`, &sqlitex.ExecOptions{
		Args: []any{entityID, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			drive = drive.Add(stmt.ColumnInt(0), stmt.ColumnInt64(1))
			return nil
		},
	}); err != nil {
		return Drive{}, fmt.Errorf("load jump drive for entity %d: %w", entityID, err)
	}
	return drive, nil
}

// LoadAll returns the jump drive assembled on every entity in a game.
func LoadAll(conn *sqlite.Conn, gameID int64) (map[int64]Drive, error) {
	drives := make(map[int64]Drive)
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT i.entity_id, i.tech_level, i.quantity
		FROM inventory AS i
		JOIN entity AS e ON e.id = i.entity_id
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ? AND i.section = 'component' AND i.unit = ? AND i.quantity > 0;`, &sqlitex.ExecOptions{
		Args: []any{gameID, Unit},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id := stmt.ColumnInt64(0)
			drives[id] = drives[id].Add(stmt.ColumnInt(1), stmt.ColumnInt64(2))
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("load jump drives: %w", err)
	}
	return drives, nil
}
