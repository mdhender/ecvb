// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import "testing"

func TestRingsAreDrawnFromTheGameSeedAndRepeat(t *testing.T) {
	game := Seed{High: 19, Low: 12}
	// The same game, turn, faction, and order always reach the same ring, so
	// re-resolving a turn puts the ship back where it was.
	first := game.RingFor(3, 1, 2)
	if again := game.RingFor(3, 1, 2); again != first {
		t.Errorf("ring = %d then %d; want the draw to repeat", first, again)
	}
	// Every draw lands in a ring a ship may occupy, and consecutive orders do
	// not share a stream: 400 draws spread across the range rather than
	// clustering, which a poorly mixed seed would produce.
	seen := make(map[int]bool)
	for sequence := 1; sequence <= 400; sequence++ {
		ring := game.RingFor(3, 1, sequence)
		if ring < MinShipRing || ring > MaxShipRing {
			t.Fatalf("ring = %d; want it between %d and %d", ring, MinShipRing, MaxShipRing)
		}
		seen[ring] = true
	}
	if len(seen) < 50 {
		t.Errorf("400 draws covered %d rings; want them spread across the range", len(seen))
	}
}
