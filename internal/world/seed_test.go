// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"testing"

	"github.com/mdhender/ecvb/internal/testdb"
)

// A ring is addressed, not sequenced. What it is drawn from is the planet, the
// turn, the faction and the ship -- and nothing about the order that moved the
// ship, which is the point: the ring used to be drawn from the order's sequence
// number, and a sequence number is a position in the phase table, so every
// reordering of a turn silently redrew every ring in every game.
func TestRingsAreAddressedAndRepeat(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	// A second system and two more planets, so a test can tell that the planet
	// is part of the address rather than merely along for the ride.
	testdb.Exec(t, conn, `
		INSERT INTO system (id, stellium_id, sequence) VALUES (21, 10, 'B');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES
			(31, 20, 7, 'rocky', 3),
			(32, 21, 4, 'rocky', 3);
	`)
	loaded := loadWorld(t, conn)
	at := Location{StelliumID: 10, SystemID: 20, PlanetID: 30}

	// The same ship settling at the same planet on the same turn reaches the
	// same ring however the turn is resolved.
	first := drawRing(t, loaded, at, 3, 1, 40)
	if again := drawRing(t, loaded, at, 3, 1, 40); again != first {
		t.Errorf("ring = %d then %d; want the draw to repeat", first, again)
	}

	// Every element of the address separates one draw from another: two ships
	// settling at one planet in one turn, the same ship on a later turn -- which
	// is what makes a hop to the planet a ship is already at draw a fresh ring
	// -- two factions, and the same orbit of a different system.
	differs := map[string]int{
		"another ship":    drawRing(t, loaded, at, 3, 1, 41),
		"a later turn":    drawRing(t, loaded, at, 4, 1, 40),
		"another orbit":   drawRing(t, loaded, Location{StelliumID: 10, SystemID: 20, PlanetID: 31}, 3, 1, 40),
		"another system":  drawRing(t, loaded, Location{StelliumID: 10, SystemID: 21, PlanetID: 32}, 3, 1, 40),
		"another faction": drawRing(t, loaded, at, 3, 2, 40),
	}
	for what, ring := range differs {
		if ring == first {
			t.Errorf("%s drew ring %d, the same as the first draw; want the address to separate them", what, ring)
		}
	}

	// Every draw lands in a ring a ship may occupy, and the addresses do not
	// cluster: 400 ships at one planet spread across the range rather than
	// bunching, which a poorly mixed address would produce.
	seen := make(map[int]bool)
	for entityID := int64(1); entityID <= 400; entityID++ {
		ring := drawRing(t, loaded, at, 3, 1, entityID)
		if ring < MinShipRing || ring > MaxShipRing {
			t.Fatalf("ring = %d; want it between %d and %d", ring, MinShipRing, MaxShipRing)
		}
		seen[ring] = true
	}
	if len(seen) < 50 {
		t.Errorf("400 draws covered %d rings; want them spread across the range", len(seen))
	}
}

// A ring belongs to a planet, so asking for one anywhere else is a failure
// rather than a number.
func TestRingsAreOnlyDrawnAtAPlanet(t *testing.T) {
	loaded := loadWorld(t, openInventoryTestDatabase(t))
	if _, err := loaded.DrawRing(Location{StelliumID: 10}, 3, 1, 40); err == nil {
		t.Error("drawing a ring in the stellium orbit succeeded; want an error")
	}
}

func drawRing(t *testing.T, w *World, at Location, turn int, factionID, entityID int64) int {
	t.Helper()
	ring, err := w.DrawRing(at, turn, factionID, entityID)
	if err != nil {
		t.Fatal(err)
	}
	return ring
}
