// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package units

import "testing"

func TestBulkResourcesMassAndOccupyOneUnit(t *testing.T) {
	// The four raw resources are measured, not manufactured: 1 MU and 1 VU
	// each, in every section, with none of the multipliers that installing a
	// manufactured unit costs.
	for _, unit := range []string{"FUEL", "GOLD", "METL", "MNRL"} {
		got := MetricsFor(unit, 0, false)
		if want := (Metrics{Mass: 1, CargoVolume: 1, UnassembledVolume: 1,
			OperationalVolume: 1, ComponentVolume: 1}); got != want {
			t.Errorf("%s metrics = %+v; want %+v", unit, got, want)
		}
	}
	// Another unit without a technology level keeps the general rule.
	got := MetricsFor("CNGD", 0, false)
	if want := (Metrics{Mass: 6, CargoVolume: 6, UnassembledVolume: 6,
		OperationalVolume: 12, ComponentVolume: 24}); got != want {
		t.Errorf("CNGD metrics = %+v; want %+v", got, want)
	}
}

// Nothing an order writes chooses the section: which one a unit works in is a
// property of the unit code.
func TestAssembledSectionIsSettledByTheUnitCode(t *testing.T) {
	// The six that docs/units.md says are freight in any other section.
	for _, unit := range []string{"HDRV", "SNSR", "SDRV", "LFSU", "STRC", "STRL"} {
		section, assemblable := AssembledSection(unit)
		if !assemblable || section != SectionComponent {
			t.Errorf("%s assembles into (%q, %t); want component", unit, section, assemblable)
		}
	}
	for _, unit := range []string{"MINE", "FARM", "FACT", "TRAN", "CNGD", "FOOD"} {
		section, assemblable := AssembledSection(unit)
		if !assemblable || section != SectionOperational {
			t.Errorf("%s assembles into (%q, %t); want operational", unit, section, assemblable)
		}
	}
	// Three kinds of thing are never assembled: resources are measured rather
	// than made, and population and cadres are not inventory at all.
	for _, unit := range []string{"GOLD", "FUEL", "METL", "MNRL", "USK", "SKW", "SOL", "NAS",
		"CWKR", "PLCF", "SPCF", "TRNE"} {
		if _, assemblable := AssembledSection(unit); assemblable {
			t.Errorf("%s reads as assemblable; want it refused", unit)
		}
	}
}

// Assembled structure creates enclosed space rather than filling it, and a
// unit at technology level t encloses t squared VU.
func TestOnlyAssembledStructureEnclosesAnythingAndOccupiesNothing(t *testing.T) {
	if got := EnclosedVolumePerUnit(SectionComponent, "STRC", 10); got != 100 {
		t.Errorf("assembled STRC-10 encloses %d VU; want 100", got)
	}
	if got := EnclosedVolumePerUnit(SectionComponent, "STRL", 3); got != 9 {
		t.Errorf("assembled STRL-3 encloses %d VU; want 9", got)
	}
	if got := EnclosedVolumePerUnit(SectionUnassembled, "STRC", 10); got != 0 {
		t.Errorf("unassembled STRC-10 encloses %d VU; want 0", got)
	}
	if got := EnclosedVolumePerUnit(SectionComponent, "HDRV", 10); got != 0 {
		t.Errorf("assembled HDRV-10 encloses %d VU; want 0", got)
	}
	if got := OccupiedVolumePerUnit("SHIP", SectionComponent, "STRC", 10, true); got != 0 {
		t.Errorf("assembled STRC-10 occupies %d VU; want 0", got)
	}
	// In any other section it consumes space like anything else.
	if got := OccupiedVolumePerUnit("SHIP", SectionUnassembled, "STRC", 10, true); got != 20 {
		t.Errorf("unassembled STRC-10 occupies %d VU; want 20", got)
	}
}

// Bulk resources in the cargo of a COPN or a CORB sit in external depots
// outside the hull: they contribute mass but consume no enclosed space.
func TestBulkCargoSitsInExternalDepotsOnTheTwoColoniesThatHaveThem(t *testing.T) {
	for _, kind := range []string{"COPN", "CORB"} {
		if got := OccupiedVolumePerUnit(kind, SectionCargo, "GOLD", 0, false); got != 0 {
			t.Errorf("%s cargo GOLD occupies %d VU; want 0", kind, got)
		}
	}
	for _, kind := range []string{"CSFC", "SHIP"} {
		if got := OccupiedVolumePerUnit(kind, SectionCargo, "GOLD", 0, false); got != 1 {
			t.Errorf("%s cargo GOLD occupies %d VU; want 1", kind, got)
		}
	}
	// Only cargo, and only the bulk resources.
	if got := OccupiedVolumePerUnit("COPN", SectionUnassembled, "GOLD", 0, false); got != 1 {
		t.Errorf("COPN unassembled GOLD occupies %d VU; want 1", got)
	}
	if got := OccupiedVolumePerUnit("COPN", SectionCargo, "CNGD", 0, false); got != 6 {
		t.Errorf("COPN cargo CNGD occupies %d VU; want 6", got)
	}
}

// An unknown kind is an error rather than a panic: order code calls this with
// a unit code read off a database row.
func TestUsableEnclosedSpaceAppliesTheEfficiencyAndRefusesAnUnknownKind(t *testing.T) {
	for _, item := range []struct {
		kind string
		want int64
	}{{"COPN", 1000}, {"CSFC", 200}, {"CORB", 100}, {"SHIP", 100}} {
		got, err := UsableEnclosedSpace(item.kind, 1000)
		if err != nil || got != item.want {
			t.Errorf("%s = (%d, %v); want %d", item.kind, got, err, item.want)
		}
	}
	if _, err := UsableEnclosedSpace("BASE", 1000); err == nil {
		t.Error("an unknown entity kind was accepted; want an error")
	}
}

// A row is written with the level ParseTag read, and a unit with no level is
// written as zero, so zero is what "no level" reads as coming back.
func TestMetricsForStoredReadsAZeroLevelAsNoLevel(t *testing.T) {
	if got := MetricsForStored("CNGD", 0); got != MetricsFor("CNGD", 0, false) {
		t.Errorf("stored CNGD = %+v; want the no-level metrics", got)
	}
	if got := MetricsForStored("SNSR", 1); got != MetricsFor("SNSR", 1, true) {
		t.Errorf("stored SNSR-1 = %+v; want the level-1 metrics", got)
	}
}

// AUTO is the one unit whose cargo volume differs from its unassembled
// volume: it packs down for carrying and does not fold up any smaller sitting
// idle in the hold. It is why Metrics carries four volumes rather than three.
func TestAutomationPacksDownForCarryingAndNowhereElse(t *testing.T) {
	got := MetricsFor("AUTO", 2, true)
	if want := (Metrics{Mass: 8, CargoVolume: 4, UnassembledVolume: 8,
		OperationalVolume: 8, ComponentVolume: 8}); got != want {
		t.Errorf("AUTO-2 metrics = %+v; want %+v", got, want)
	}
	if got := got.VolumeIn(SectionCargo); got != 4 {
		t.Errorf("AUTO-2 cargo volume = %d; want 4, half what it takes unassembled", got)
	}
	if got := got.VolumeIn(SectionUnassembled); got != 8 {
		t.Errorf("AUTO-2 unassembled volume = %d; want 8", got)
	}
	// Automation does unskilled work rather than propelling or sensing, so it
	// is assembled into operational inventory like everything else that is not
	// one of the six.
	if section, assemblable := AssembledSection("AUTO"); !assemblable || section != SectionOperational {
		t.Errorf("AUTO assembles into (%q, %v); want operational", section, assemblable)
	}
	// A bare AUTO has no technology level to be worth anything at, so it falls
	// back to the general rule rather than to a level-zero unit that would
	// mass nothing.
	if got := MetricsFor("AUTO", 0, false); got.Mass != 6 {
		t.Errorf("AUTO with no level masses %d; want the general 6", got.Mass)
	}
}

func TestVolumeInNamesTheSectionsMultiplier(t *testing.T) {
	metrics := MetricsFor("CNGD", 0, false)
	for _, item := range []struct {
		section string
		want    int64
	}{
		{SectionCargo, 6},
		{SectionUnassembled, 6},
		{SectionOperational, 12},
		{SectionComponent, 24},
	} {
		if got := metrics.VolumeIn(item.section); got != item.want {
			t.Errorf("%s volume = %d; want %d", item.section, got, item.want)
		}
	}
}
