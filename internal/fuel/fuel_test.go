// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package fuel

import (
	"slices"
	"testing"

	"github.com/mdhender/ecvb/internal/units"
)

// Spending fuel is internal/world's, because world is the only thing that
// writes an inventory row; what is left here is the rule world spends by.
// TestBurnFuelEmptiesOperationalThenUnassembledThenCargo is where the draw is
// exercised end to end.
func TestDrawOrderEmptiesTheWorkingSupplyBeforeTheHold(t *testing.T) {
	want := []string{units.SectionOperational, units.SectionUnassembled, units.SectionCargo}
	if got := DrawOrder(); !slices.Equal(got, want) {
		t.Errorf("DrawOrder() = %v; want %v", got, want)
	}
	// Fuel is burned rather than assembled, so there is no component fuel to
	// draw on and the draw must never look for any.
	if slices.Contains(DrawOrder(), units.SectionComponent) {
		t.Error("DrawOrder() reaches component inventory; fuel is never assembled")
	}
}

func TestFuelIsABulkResource(t *testing.T) {
	if !units.IsBulkResource(Unit) {
		t.Errorf("%s is not a bulk resource; it masses %d MU because it is one", Unit, UnitMass)
	}
	if got := units.MetricsForStored(Unit, 0).Mass; got != UnitMass {
		t.Errorf("unit mass = %d; want %d", got, UnitMass)
	}
}
