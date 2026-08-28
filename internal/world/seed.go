// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"fmt"

	"github.com/mdhender/ecvb/internal/mapkey"
	"github.com/mdhender/ecvb/internal/prng"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Rings a ship may settle into around a planet. Ring 0 is the surface and ring
// 1 belongs to orbital colonies, so a ship arriving under its own power takes
// one of the rings above them.
const (
	MinShipRing = 2
	MaxShipRing = 99
)

// DrawRing picks the ring a ship settles into when it arrives at a planet.
//
// The draw is addressed rather than sequenced: `internal/prng` hashes the game's
// seeds together with a key path naming the planet by its coordinates, the turn,
// the faction and the ship, so the same ship settling at the same planet on the
// same turn reaches the same ring however the turn is resolved. Nothing in the
// address depends on the order the draws are made in, which is the point --
// the ring used to be drawn from the order's sequence number, and a sequence
// number is a position in the phase table, so every reordering of a turn
// silently redrew every ring in every game.
//
// The faction and the ship are named by the numbers the game gave them, never
// by their row ids. That is the same rule read once more: a row id is drawn
// from a sequence shared with every other game in the database, so addressing
// a draw by one would weld this game's rings to what some other game wrote.
//
// The turn is in the address because a ship ordered to the planet it is
// already at draws a fresh ring, which is the one way to change a ring without
// going anywhere. The faction and the ship are in it because two ships may
// settle at one planet in one turn.
func (w *World) DrawRing(at Location, turn int, ship *Entity) (int, error) {
	if at.PlanetID == 0 {
		return 0, fmt.Errorf("draw a ring for entity %d: it is not at a planet", ship.Number)
	}
	point, ok := w.stellia[at.StelliumID]
	if !ok {
		return 0, fmt.Errorf("draw a ring for entity %d: stellium %d is not in this game", ship.Number, at.StelliumID)
	}
	sequence, orbit, err := w.planetAddress(at.PlanetID)
	if err != nil {
		return 0, err
	}
	roller := w.game.Seed.Roller(prng.TagRing,
		prng.Key(point.X), prng.Key(point.Y), prng.Key(point.Z),
		prng.Key(sequence), prng.Key(orbit),
		prng.Key(turn), prng.Key(ship.FactionNumber), prng.Key(ship.Number))
	return roller.RollRange(MinShipRing, MaxShipRing), nil
}

// planetAddress is how a planet is named in a key path: its system's sequence
// letter as a number, A being 1, and its orbit. Neither depends on a row id, so
// neither depends on the order the map was written in.
//
// The letter-to-number rule is [mapkey.Sequence]'s, and is shared with the map
// generators rather than written twice: it is part of an address, so two copies
// of it could disagree about where a draw lives.
func (w *World) planetAddress(planetID int64) (sequence, orbit int, err error) {
	found := false
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT system.sequence, planet.orbit
		FROM planet JOIN system ON system.id = planet.system_id
		WHERE planet.id = ?;`, &sqlitex.ExecOptions{
		Args: []any{planetID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			key, err := mapkey.Sequence(stmt.ColumnText(0))
			if err != nil {
				return err
			}
			sequence, orbit, found = int(key), stmt.ColumnInt(1), true
			return nil
		},
	}); err != nil {
		return 0, 0, fmt.Errorf("address planet %d: %w", planetID, err)
	}
	if !found {
		return 0, 0, fmt.Errorf("address planet %d: no such planet", planetID)
	}
	return sequence, orbit, nil
}
