// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package jumpdrive implements the HDRV rules that limit how a ship moves
// inside a stellium, how far it jumps between stellia, and how much mass it
// can carry either way.
package jumpdrive

import "math"

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
	Units int64 // assembled HDRV units
	// TechLevel is the lowest technology level installed, which is the level
	// the whole drive runs at: every unit has to make the same jump. It no
	// longer caps how far a drive goes -- nothing does but the fuel -- and it
	// is what will divide a jump's distance to give the turns it takes.
	TechLevel int
	Capacity  int64 // mass in MU the drive propels through one jump
}

// Add installs quantity HDRV units at techLevel.
//
// The lowest technology level installed is the level the whole drive runs at,
// because every unit has to make the same jump. Capacity is the sum over the
// units of their own technology levels, so a mixed drive still carries the mass
// its high-technology units can propel.
func (d Drive) Add(techLevel int, quantity int64) Drive {
	if quantity <= 0 {
		return d
	}
	if d.Units == 0 || techLevel < d.TechLevel {
		d.TechLevel = techLevel
	}
	d.Units += quantity
	d.Capacity += quantity * PropulsionPerTechLevel * int64(techLevel)
	return d
}

// Installed reports whether the ship has a jump drive.
func (d Drive) Installed() bool { return d.Units > 0 }

// CanPropel reports whether the drive moves mass through a jump.
func (d Drive) CanPropel(mass int64) bool { return mass <= d.Capacity }

// SquaredDistance returns the squared distance between two stellia.
func SquaredDistance(x1, y1, z1, x2, y2, z2 int) int {
	dx, dy, dz := x1-x2, y1-y2, z1-z2
	return dx*dx + dy*dy + dz*dz
}

// Distance returns the distance between two stellia in light years: their
// Euclidean distance rounded up to the next whole light year. It is what a
// report shows a player and what prices a jump.
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
//
// The whole bill is drawn on departure, however many turns the crossing takes,
// so a ship that cannot pay for the whole crossing never leaves.
func (d Drive) FuelForJump(lightYears int) int64 {
	return d.Units * int64(lightYears) * FuelPerLightYear
}

// TurnsForJump returns how many turns the drive takes to cross lightYears.
//
// This is the whole of what a technology level buys. It does not lengthen a
// drive's reach -- every drive reaches every stellium in the game, and the FUEL
// is what makes a long jump expensive -- it divides the distance, so a better
// drive spends fewer turns off the board.
//
// Never less than one turn: a crossing that takes one turn arrives in the turn
// it departed, which is what every jump did before a crossing could span turns.
func (d Drive) TurnsForJump(lightYears int) int {
	if lightYears <= 0 || d.TechLevel <= 0 {
		return 1
	}
	turns := (lightYears + d.TechLevel - 1) / d.TechLevel
	if turns < 1 {
		return 1
	}
	return turns
}

// Reading the inventory table is internal/world's, and only world's, so
// that what an entity holds and what it can do cannot drift apart. A jump drive
// is what the entity's component section adds up to, and world adds it up
// again every time it changes one.
