// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package units

import "testing"

func TestBulkResourcesMassAndOccupyOneUnit(t *testing.T) {
	// The four raw resources are measured, not manufactured: 1 MU and 1 VU
	// each, in every section, with none of the multipliers that installing a
	// manufactured unit costs.
	for _, unit := range []string{"FUEL", "GOLD", "METL", "MNRL"} {
		got := MetricsFor(unit, 0, false)
		if want := (Metrics{Mass: 1, CargoVolume: 1, OperationalVolume: 1, ComponentVolume: 1}); got != want {
			t.Errorf("%s metrics = %+v; want %+v", unit, got, want)
		}
	}
	// Another unit without a technology level keeps the general rule.
	got := MetricsFor("CNGD", 0, false)
	if want := (Metrics{Mass: 6, CargoVolume: 6, OperationalVolume: 12, ComponentVolume: 24}); got != want {
		t.Errorf("CNGD metrics = %+v; want %+v", got, want)
	}
}
