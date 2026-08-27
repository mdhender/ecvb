// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cadre

import "testing"

// The example docs/accepted-orders.md gives: pooling matters because the
// rounding up is per pool rather than per line.
func TestWorkersAreReckonedFromOneTotalRatherThanLineByLine(t *testing.T) {
	if got := WorkersFor(15120 + 100); got != 31 {
		t.Errorf("pooled = %d workers; want 31", got)
	}
	if got := WorkersFor(15120) + WorkersFor(100); got != 32 {
		t.Errorf("line by line = %d workers; want 32, which is what pooling saves one of", got)
	}
}

func TestWorkersForRoundsUpAndCostsNothingForNothing(t *testing.T) {
	for _, item := range []struct {
		work int64
		want int64
	}{{0, 0}, {-1, 0}, {1, 1}, {500, 1}, {501, 2}, {1000, 2}, {1001, 3}} {
		if got := WorkersFor(item.work); got != item.want {
			t.Errorf("WorkersFor(%d) = %d; want %d", item.work, got, item.want)
		}
	}
}

// Assembly and unassembly draw on the same cadre and never pool with each
// other, so an entity doing 100 MU of each needs two workers and not one.
func TestTheTwoPoolsRoundUpSeparately(t *testing.T) {
	// One worker, and 100 MU of unassembly already done: the whole worker is
	// spoken for, so there is nothing left to assemble with.
	if got := WorkAllowed(1, 0, 100); got != 0 {
		t.Errorf("allowed = %d MU; want 0, the one worker being busy", got)
	}
	// Two workers and the same 100 MU of unassembly leaves a whole second
	// worker, not 900 MU of the first.
	if got := WorkAllowed(2, 0, 100); got != WorkPerUnit {
		t.Errorf("allowed = %d MU; want %d", got, WorkPerUnit)
	}
}

func TestWorkAlreadyDoneComesOffThePoolItWasDoneIn(t *testing.T) {
	if got := WorkAllowed(5, 0, 0); got != 2500 {
		t.Errorf("allowed = %d MU; want 2500", got)
	}
	if got := WorkAllowed(5, 1200, 0); got != 1300 {
		t.Errorf("allowed after 1200 MU = %d MU; want 1300", got)
	}
	if got := WorkAllowed(5, 2500, 0); got != 0 {
		t.Errorf("allowed after the pool is spent = %d MU; want 0", got)
	}
	// More done than the cadre could have done never reads as a negative
	// allowance, whatever put it there.
	if got := WorkAllowed(5, 9999, 0); got != 0 {
		t.Errorf("allowed = %d MU; want 0", got)
	}
}

func TestAnEntityWithNoCadreDoesNoWork(t *testing.T) {
	if got := WorkAllowed(0, 0, 0); got != 0 {
		t.Errorf("allowed = %d MU; want 0", got)
	}
}
