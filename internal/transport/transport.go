// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Package transport implements the TRAN rules that decide how much an entity
// can hand to another entity at the same place in one turn, and what the
// hulls it used cost it in FUEL.
//
// A transport goes there and comes back, so a turn's capacity covers the round
// trip and is charged once. See docs/units.md.
package transport

// Unit is the inventory unit code of a transport. Only assembled units carry
// anything; TRAN held anywhere else is freight.
//
// What a transport itself masses and occupies is not here. docs/units.md gives
// it a flat 4 MU and 4t VU, but internal/units still carries the provisional
// table for every unit, TRAN included, and correcting one unit of it would
// change every kit built from it. What a transport *carries* is this package's,
// and that is what the transfer rules read.
const Unit = "TRAN"

// CrewPerUnit is how many transports one SKW population unit operates in a
// turn. The engine allocates the crew; no order names it.
const CrewPerUnit = 10

// The capacity of one transport in a turn, per t squared. Both limits hold at
// once, and with today's unit table it is nearly always the mass that binds.
const (
	MassPerSquare   = 20
	VolumePerSquare = 60
)

// FuelPerTenSquares is the divisor in the fuel rule: an entity's transports
// burn ceil(sum of t squared / 10) FUEL in a turn.
const FuelPerTenSquares = 10

// Load is what a transport carries: its mass in MU and the room it takes in VU.
// Both are limits, so a load is carried only when the hulls cover both.
type Load struct {
	Mass   int64
	Volume int64
}

// Hulls is a count of transports at one technology level.
type Hulls struct {
	TechLevel int
	Count     int64
}

// Squares is the sum of t squared over a set of hulls, which is the only thing
// the fuel rule reads.
func Squares(hulls []Hulls) int64 {
	total := int64(0)
	for _, hull := range hulls {
		total += hull.Count * int64(hull.TechLevel) * int64(hull.TechLevel)
	}
	return total
}

// Fuel is the FUEL a set of transports burns in a turn, reckoned over all of
// them at once rather than one hull at a time, which is what keeps it in whole
// numbers. Ten TRAN-1 and five TRAN-2 cost 3 FUEL; a single TRAN-1 costs 1.
//
// It takes the total of t squared rather than the hulls themselves, because
// what an entity owes for a turn is a function of every hull it used and the
// rounding happens once, at the end.
func Fuel(squares int64) int64 {
	if squares <= 0 {
		return 0
	}
	return (squares + FuelPerTenSquares - 1) / FuelPerTenSquares
}

// Capacity is what a set of hulls carries in a turn. Fungible cargo divides
// across the hulls however it likes, so the set's capacity is the sum of the
// hulls'.
func Capacity(hulls []Hulls) Load {
	var total Load
	for _, hull := range hulls {
		square := hull.Count * int64(hull.TechLevel) * int64(hull.TechLevel)
		total.Mass += square * MassPerSquare
		total.Volume += square * VolumePerSquare
	}
	return total
}

// Carries reports whether a set of hulls takes a load. Both limits hold.
func Carries(hulls []Hulls, load Load) bool {
	capacity := Capacity(hulls)
	return load.Mass <= capacity.Mass && load.Volume <= capacity.Volume
}

// CrewedHulls is how many transports a number of skilled workers operates in a
// turn. The engine allocates the crew; no order names it.
func CrewedHulls(skilledWorkers int64) int64 { return skilledWorkers * CrewPerUnit }

// Limit trims a set of hulls to the most that may be used, taken from the
// highest technology level down, because a better hull carries more for the
// same crew and the same fuel. free is in descending order of technology
// level.
func Limit(free []Hulls, most int64) []Hulls {
	var allowed []Hulls
	for _, hull := range free {
		if most <= 0 {
			break
		}
		count := min(hull.Count, most)
		allowed = append(allowed, Hulls{TechLevel: hull.TechLevel, Count: count})
		most -= count
	}
	return allowed
}

// Pack is the fewest hulls that carry a load, taken from the highest
// technology level down. It returns every hull when the load is more than they
// carry, which is what a partly filled transfer is packed into.
//
// free is in descending order of technology level. Fewest hulls matters
// because the fuel is reckoned over the hulls used: a load that fits in one
// TRAN-2 should not be charged for ten TRAN-1 standing idle beside it.
func Pack(free []Hulls, load Load) []Hulls {
	var used []Hulls
	var carried Load
	for _, hull := range free {
		if load.Mass <= carried.Mass && load.Volume <= carried.Volume {
			break
		}
		// Take only as many hulls of this level as the load still needs. A
		// hull at technology level zero carries nothing, so it is never worth
		// taking and never worth its share of the fuel.
		square := int64(hull.TechLevel) * int64(hull.TechLevel)
		if square == 0 {
			continue
		}
		byMass := ceilDiv(load.Mass-carried.Mass, square*MassPerSquare)
		byVolume := ceilDiv(load.Volume-carried.Volume, square*VolumePerSquare)
		needed := min(hull.Count, max(byMass, byVolume))
		if needed <= 0 {
			continue
		}
		used = append(used, Hulls{TechLevel: hull.TechLevel, Count: needed})
		carried.Mass += needed * square * MassPerSquare
		carried.Volume += needed * square * VolumePerSquare
	}
	return used
}

func ceilDiv(value, divisor int64) int64 {
	if value <= 0 || divisor <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}
