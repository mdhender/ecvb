// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package transport

import (
	"slices"
	"testing"
)

// The example docs/units.md gives: the fuel is reckoned over every hull at
// once, which is what keeps it in whole numbers.
func TestFuelIsReckonedOverEveryHullAtOnce(t *testing.T) {
	mixed := []Hulls{{TechLevel: 2, Count: 5}, {TechLevel: 1, Count: 10}}
	if got := Fuel(Squares(mixed)); got != 3 {
		t.Errorf("ten TRAN-1 and five TRAN-2 cost %d FUEL; want 3", got)
	}
	if got := Fuel(Squares([]Hulls{{TechLevel: 1, Count: 1}})); got != 1 {
		t.Errorf("a single TRAN-1 costs %d FUEL; want 1", got)
	}
	if got := Fuel(0); got != 0 {
		t.Errorf("no transports cost %d FUEL; want 0", got)
	}
	// One hull at a time would round each up on its own and charge ten.
	perHull := int64(0)
	for range 10 {
		perHull += Fuel(Squares([]Hulls{{TechLevel: 1, Count: 1}}))
	}
	if perHull != 10 {
		t.Fatalf("per-hull total = %d; the test's own arithmetic is wrong", perHull)
	}
	if Fuel(Squares([]Hulls{{TechLevel: 1, Count: 10}})) != 1 {
		t.Error("ten TRAN-1 reckoned at once should cost 1 FUEL, not 10")
	}
}

// Fuel per MU moved does not depend on the technology level: capacity is 20t
// squared and fuel is t squared over ten, so the ratio is flat. A better
// transport buys fewer hulls and fewer crew, not cheaper freight.
func TestFuelPerMassMovedIsFlatAcrossTechnologyLevels(t *testing.T) {
	for level := 1; level <= 10; level++ {
		hulls := []Hulls{{TechLevel: level, Count: 10}}
		capacity := Capacity(hulls)
		if got := capacity.Mass / Squares(hulls); got != MassPerSquare {
			t.Errorf("TRAN-%d carries %d MU per unit of fuel-reckoning; want %d",
				level, got, MassPerSquare)
		}
	}
}

func TestBothLimitsHold(t *testing.T) {
	hulls := []Hulls{{TechLevel: 1, Count: 1}}
	if got := Capacity(hulls); got.Mass != 20 || got.Volume != 60 {
		t.Fatalf("one TRAN-1 carries %+v; want 20 MU and 60 VU", got)
	}
	if !Carries(hulls, Load{Mass: 20, Volume: 60}) {
		t.Error("a TRAN-1 refused a load exactly its size")
	}
	if Carries(hulls, Load{Mass: 21, Volume: 0}) {
		t.Error("a TRAN-1 took a load over its mass limit")
	}
	if Carries(hulls, Load{Mass: 0, Volume: 61}) {
		t.Error("a TRAN-1 took a load over its volume limit")
	}
}

// A load takes the fewest hulls that carry it, highest technology level first,
// because the fuel is reckoned over the hulls used: a load that fits in one
// TRAN-2 must not be charged for ten TRAN-1 standing idle beside it.
func TestPackTakesTheFewestHullsHighestLevelFirst(t *testing.T) {
	free := []Hulls{{TechLevel: 2, Count: 3}, {TechLevel: 1, Count: 10}}
	// A TRAN-2 carries 80 MU, so 80 MU is one hull and nothing else.
	if got := Pack(free, Load{Mass: 80}); !slices.Equal(got, []Hulls{{TechLevel: 2, Count: 1}}) {
		t.Errorf("Pack(80 MU) = %+v; want one TRAN-2", got)
	}
	// 260 MU is the three TRAN-2 (240) and one TRAN-1 (20) on top.
	want := []Hulls{{TechLevel: 2, Count: 3}, {TechLevel: 1, Count: 1}}
	if got := Pack(free, Load{Mass: 260}); !slices.Equal(got, want) {
		t.Errorf("Pack(260 MU) = %+v; want %+v", got, want)
	}
	// A load nothing can carry packs everything, which is what a transfer
	// filled partway is packed into.
	if got := Pack(free, Load{Mass: 1_000_000}); !slices.Equal(got, free) {
		t.Errorf("Pack(more than they carry) = %+v; want every hull", got)
	}
	if got := Pack(free, Load{}); len(got) != 0 {
		t.Errorf("Pack(nothing) = %+v; want no hulls", got)
	}
}

func TestPackTakesWhicheverLimitBinds(t *testing.T) {
	free := []Hulls{{TechLevel: 1, Count: 10}}
	// 100 VU needs two hulls by volume and one by mass; the volume binds.
	if got := Pack(free, Load{Mass: 1, Volume: 100}); !slices.Equal(got, []Hulls{{TechLevel: 1, Count: 2}}) {
		t.Errorf("Pack(1 MU, 100 VU) = %+v; want two TRAN-1", got)
	}
}

// A hull at technology level zero carries nothing, so it is never worth taking
// and never worth its share of the fuel.
func TestPackSkipsHullsThatCarryNothing(t *testing.T) {
	free := []Hulls{{TechLevel: 1, Count: 1}, {TechLevel: 0, Count: 99}}
	if got := Pack(free, Load{Mass: 1000}); !slices.Equal(got, []Hulls{{TechLevel: 1, Count: 1}}) {
		t.Errorf("Pack = %+v; want only the one hull that carries anything", got)
	}
}

// One SKW unit operates up to ten transports in a turn.
func TestTheCrewCapsTheHullsAndTakesTheBestFirst(t *testing.T) {
	free := []Hulls{{TechLevel: 2, Count: 8}, {TechLevel: 1, Count: 20}}
	if got := CrewedHulls(1); got != 10 {
		t.Errorf("one SKW crews %d hulls; want %d", got, CrewPerUnit)
	}
	want := []Hulls{{TechLevel: 2, Count: 8}, {TechLevel: 1, Count: 2}}
	if got := Limit(free, CrewedHulls(1)); !slices.Equal(got, want) {
		t.Errorf("Limit = %+v; want %+v, the best eight and two of the rest", got, want)
	}
	if got := Limit(free, 0); len(got) != 0 {
		t.Errorf("Limit with no crew = %+v; want nothing", got)
	}
}
