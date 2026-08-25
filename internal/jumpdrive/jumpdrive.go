// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package jumpdrive implements the HDRV rules that limit how a ship moves
// inside a stellium, how far it jumps between stellia, and how much mass it
// can carry either way.
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

// MoveKind classifies a move inside a stellium. Distance inside a stellium is
// not measured: it takes one of three fixed values, and naming them keeps the
// fractions out of the code entirely.
type MoveKind int

const (
	// MoveNowhere is the one move that costs nothing: a ship already orbiting
	// the stellium ordered back to the stellium orbit. There is nowhere in the
	// stellium orbit to go, so the ship does not stir.
	MoveNowhere MoveKind = iota
	// MoveHop crosses 0.1 LY: between the stellium orbit and any planet of
	// the stellium, or between two planets of one system.
	MoveHop
	// MoveCrossSystem crosses 0.2 LY, between planets of different systems of
	// one stellium. It is two hops because the ship crosses the stellium orbit
	// on the way.
	MoveCrossSystem
)

// KindOfMove classifies a move from one place inside a stellium to another. A
// system of zero is the stellium orbit, which every system is one hop away
// from.
//
// A ship ordered to the planet it is already at still makes a hop. It has to
// break orbit and settle again, which costs the same as crossing to any other
// planet of the system; only a ship going nowhere in the stellium orbit stays
// put. That is why the planets do not enter into this: whether the endpoints
// are the same planet or different ones, the cost is the same.
func KindOfMove(startSystemID, endSystemID int64) MoveKind {
	switch {
	case startSystemID == 0 && endSystemID == 0:
		return MoveNowhere
	case startSystemID != 0 && endSystemID != 0 && startSystemID != endSystemID:
		return MoveCrossSystem
	default:
		return MoveHop
	}
}

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

// Distance returns the distance between two stellia in light years: their
// Euclidean distance rounded up to the next whole light year. Range tests use
// SquaredDistance; this reports a distance to a player and prices a jump.
func Distance(x1, y1, z1, x2, y2, z2 int) int {
	return int(math.Ceil(math.Sqrt(float64(SquaredDistance(x1, y1, z1, x2, y2, z2)))))
}

// FUEL costs are per assembled HDRV unit. Every unit draws: a ship cannot idle
// part of its drive to save fuel, so a large drive is expensive to run even on
// a short hop.
const (
	// FuelPerLightYear is the FUEL one unit burns per light year of a jump.
	FuelPerLightYear = 40
	// FuelPerHop is the FUEL one unit burns on a hop inside a stellium. A hop
	// is a tenth of a light year, so this is FuelPerLightYear divided by ten.
	FuelPerHop = FuelPerLightYear / 10
	// FuelPerCrossSystem is the FUEL one unit burns crossing between systems
	// of one stellium, which is two hops.
	FuelPerCrossSystem = 2 * FuelPerHop
)

// FuelForMove returns the FUEL the drive burns on one move inside a stellium.
func (d Drive) FuelForMove(kind MoveKind) int64 {
	switch kind {
	case MoveHop:
		return d.Units * FuelPerHop
	case MoveCrossSystem:
		return d.Units * FuelPerCrossSystem
	default:
		return 0
	}
}

// FuelForJump returns the FUEL the drive burns crossing lightYears between
// stellia. A jump distance is always a whole number of light years, because it
// is a Euclidean distance rounded up.
func (d Drive) FuelForJump(lightYears int) int64 {
	return d.Units * int64(lightYears) * FuelPerLightYear
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
