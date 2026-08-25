// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package jumpdrive

import "testing"

func TestDriveRangeUsesLowestTechLevelAndCapacitySumsEveryUnit(t *testing.T) {
	// The documented example: 10 HDRV-1 and 3 HDRV-2 jump 1 unit and propel
	// 10*1045*1 + 3*1045*2 MU.
	drive := Drive{}.Add(1, 10).Add(2, 3)
	if drive.Range != 1 {
		t.Errorf("range = %d; want 1", drive.Range)
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
	if drive := (Drive{}).Add(3, 1).Add(9, 0); drive.Range != 3 {
		t.Errorf("range = %d; want an empty stack to leave the range alone", drive.Range)
	}
}

func TestReachesMatchesRoundedUpDistance(t *testing.T) {
	// (1,2,3) from the origin is sqrt(14), which rounds up to 4.
	if got := Distance(0, 0, 0, 1, 2, 3); got != 4 {
		t.Fatalf("distance = %d; want 4", got)
	}
	squared := SquaredDistance(0, 0, 0, 1, 2, 3)
	if (Drive{Units: 1, Range: 3}).Reaches(squared) {
		t.Error("range 3 reaches a distance of 4")
	}
	if !(Drive{Units: 1, Range: 4}).Reaches(squared) {
		t.Error("range 4 does not reach a distance of 4")
	}
	// An exact integer distance is reachable at exactly that range.
	if !(Drive{Units: 1, Range: 3}).Reaches(SquaredDistance(0, 0, 0, 3, 0, 0)) {
		t.Error("range 3 does not reach a distance of exactly 3")
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
