// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package sensors

import "testing"

func TestArrayProbesSumEveryUnitAtItsOwnTechLevel(t *testing.T) {
	// The documented example: 5 SNSR-1 and 3 SNSR-2 launch 5*1 + 3*2 probes.
	array := Array{}.Add(1, 5).Add(2, 3)
	if array.Probes != 11 {
		t.Errorf("probes = %d; want 11", array.Probes)
	}
	if array.Units != 8 {
		t.Errorf("units = %d; want 8", array.Units)
	}
	if reversed := (Array{}).Add(2, 3).Add(1, 5); reversed != array {
		t.Errorf("reversed = %+v; want %+v", reversed, array)
	}
}

func TestArrayWithoutUnitsSensesNothing(t *testing.T) {
	if (Array{}).Installed() {
		t.Error("zero array reports installed")
	}
	if array := (Array{}).Add(4, 0); array.Installed() || array.Probes != 0 {
		t.Errorf("array = %+v; want nothing from an empty stack", array)
	}
	// A technology level 0 sensor sees, but it cannot launch a probe.
	if array := (Array{}).Add(0, 3); !array.Installed() || array.Probes != 0 {
		t.Errorf("array = %+v; want an installed array with no probes", array)
	}
}

func TestApproximateMassIsTheBaseTenOrderOfMagnitude(t *testing.T) {
	for _, tc := range []struct {
		mass int64
		want int
	}{
		{mass: 1999, want: 3}, // the documented example
		{mass: 0, want: 0},
		{mass: 1, want: 0},
		{mass: 9, want: 0},
		{mass: 10, want: 1},
		{mass: 999, want: 2},
		{mass: 1000, want: 3},
		{mass: 55264, want: 4},
	} {
		if got := ApproximateMass(tc.mass); got != tc.want {
			t.Errorf("ApproximateMass(%d) = %d; want %d", tc.mass, got, tc.want)
		}
	}
}

func TestUnitMass(t *testing.T) {
	if got := UnitMass(3); got != 120 {
		t.Errorf("UnitMass(3) = %d; want 120", got)
	}
	if got := UnitMass(0); got != 0 {
		t.Errorf("UnitMass(0) = %d; want 0", got)
	}
}
