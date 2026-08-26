// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package jumpdrive

import "testing"

func TestDriveRunsAtTheLowestTechLevelAndCapacitySumsEveryUnit(t *testing.T) {
	// The documented example: 10 HDRV-1 and 3 HDRV-2 run at technology level 1
	// and propel 10*1045*1 + 3*1045*2 MU.
	drive := Drive{}.Add(1, 10).Add(2, 3)
	if drive.TechLevel != 1 {
		t.Errorf("tech level = %d; want 1", drive.TechLevel)
	}
	if want := int64(10*1045*1 + 3*1045*2); drive.Capacity != want {
		t.Errorf("capacity = %d; want %d", drive.Capacity, want)
	}
	if drive.Units != 13 {
		t.Errorf("units = %d; want 13", drive.Units)
	}
	// Installation order must not change the result.
	if reversed := (Drive{}).Add(2, 3).Add(1, 10); reversed != drive {
		t.Errorf("reversed = %+v; want %+v", reversed, drive)
	}
}

func TestDriveIgnoresEmptyStacksAndReportsNoDrive(t *testing.T) {
	if (Drive{}).Installed() {
		t.Error("zero drive reports installed")
	}
	if drive := (Drive{}).Add(5, 0); drive.Installed() {
		t.Errorf("drive = %+v; want no drive from an empty stack", drive)
	}
	if drive := (Drive{}).Add(3, 1).Add(9, 0); drive.TechLevel != 3 {
		t.Errorf("tech level = %d; want an empty stack to leave it alone", drive.TechLevel)
	}
}

// TestDistanceRoundsUp covers the one measurement a jump still makes. Nothing
// compares it against the drive any more: technology level stopped capping how
// far a drive goes, so the distance only prices the jump.
func TestDistanceRoundsUp(t *testing.T) {
	// (1,2,3) from the origin is sqrt(14), which rounds up to 4.
	if got := Distance(0, 0, 0, 1, 2, 3); got != 4 {
		t.Errorf("distance = %d; want 4", got)
	}
	// An exact integer distance is not rounded up past itself.
	if got := Distance(0, 0, 0, 3, 0, 0); got != 3 {
		t.Errorf("distance = %d; want 3", got)
	}
	if got := SquaredDistance(0, 0, 0, 1, 2, 3); got != 14 {
		t.Errorf("squared distance = %d; want 14", got)
	}
}

func TestCanPropelAndUnitMass(t *testing.T) {
	drive := Drive{}.Add(8, 7)
	if want := int64(7 * 1045 * 8); drive.Capacity != want {
		t.Fatalf("capacity = %d; want %d", drive.Capacity, want)
	}
	if !drive.CanPropel(drive.Capacity) {
		t.Error("drive cannot propel exactly its capacity")
	}
	if drive.CanPropel(drive.Capacity + 1) {
		t.Error("drive propels more than its capacity")
	}
	if got := UnitMass(8); got != 360 {
		t.Errorf("unit mass = %d; want 360", got)
	}
}

func TestKindOfMoveDistinguishesHopsFromCrossingSystems(t *testing.T) {
	const orbit = 0 // the stellium orbit is the absence of a system
	for _, tc := range []struct {
		name                   string
		startSystem, endSystem int64
		want                   MoveKind
	}{
		{name: "same system", startSystem: 1, endSystem: 1, want: MoveHop},
		{name: "leaving the planets", startSystem: 1, endSystem: orbit, want: MoveHop},
		{name: "arriving from the stellium orbit", startSystem: orbit, endSystem: 2, want: MoveHop},
		{name: "across systems", startSystem: 1, endSystem: 2, want: MoveCrossSystem},
		{name: "already in the stellium orbit", startSystem: orbit, endSystem: orbit, want: MoveNowhere},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := KindOfMove(tc.startSystem, tc.endSystem); got != tc.want {
				t.Errorf("kind = %d; want %d", got, tc.want)
			}
		})
	}
}

func TestAShipOrderedToThePlanetItIsAtStillHops(t *testing.T) {
	// Breaking orbit and settling again costs what crossing to any other
	// planet of the system costs, so the same system is a hop whether or not
	// the planet changes. Only the stellium orbit is free.
	if got := KindOfMove(7, 7); got != MoveHop {
		t.Errorf("same-planet move = %d; want a hop", got)
	}
	if got := (Drive{}).Add(1, 5).FuelForMove(KindOfMove(7, 7)); got != 20 {
		t.Errorf("same-planet fuel = %d; want 20", got)
	}
	if got := (Drive{}).Add(1, 5).FuelForMove(KindOfMove(0, 0)); got != 0 {
		t.Errorf("stellium-orbit fuel = %d; want 0", got)
	}
}

func TestFuelBurnsEveryAssembledUnit(t *testing.T) {
	// 20 HDRV-1 and 1 HDRV-8 are 21 units, and every one of them draws.
	drive := Drive{}.Add(1, 20).Add(8, 1)
	if got := drive.FuelForMove(MoveHop); got != 21*4 {
		t.Errorf("hop fuel = %d; want %d", got, 21*4)
	}
	if got := drive.FuelForMove(MoveCrossSystem); got != 21*8 {
		t.Errorf("cross-system fuel = %d; want %d", got, 21*8)
	}
	if got := drive.FuelForMove(MoveNowhere); got != 0 {
		t.Errorf("fuel for going nowhere = %d; want 0", got)
	}
	// 15 HDRV-2 jumping 2 light years burn 15 * 2 * 40 FUEL.
	if got := (Drive{}).Add(2, 15).FuelForJump(2); got != 1200 {
		t.Errorf("jump fuel = %d; want 1200", got)
	}
	// A ship with no drive burns nothing, and neither does a jump of zero.
	if got := (Drive{}).FuelForJump(9); got != 0 {
		t.Errorf("fuel with no drive = %d; want 0", got)
	}
	if got := drive.FuelForJump(0); got != 0 {
		t.Errorf("fuel for no distance = %d; want 0", got)
	}
}

func TestMoveCostsMatchTheirFractionOfALightYear(t *testing.T) {
	// The integer move costs stand in for 0.1 and 0.2 light years at the jump
	// rate. Keeping that tie in a test means changing the rate cannot silently
	// leave the move costs behind.
	if FuelPerHop*10 != FuelPerLightYear {
		t.Errorf("a hop costs %d; want a tenth of %d", FuelPerHop, FuelPerLightYear)
	}
	if FuelPerCrossSystem*5 != FuelPerLightYear {
		t.Errorf("crossing systems costs %d; want a fifth of %d", FuelPerCrossSystem, FuelPerLightYear)
	}
}
