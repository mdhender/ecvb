// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"fmt"

	"github.com/mdhender/ecvb/internal/entityid"
	"github.com/mdhender/ecvb/internal/prng"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// NextEntityNumber hands out the next entity number of a game and is the only
// thing that does. It is a package function rather than a method because the
// starting kit mints numbers before any World is loaded, and there must not be
// two implementations of what a number is.
//
// The counter and the number are not the same value: the counter is the
// ordinal, and the number is what internal/entityid permutes it into. Bumping
// the counter and reading the seeds in one statement is what keeps the ordinal
// from being handed out twice; the caller's transaction is what makes it and
// the entity row land together or not at all.
func NextEntityNumber(conn *sqlite.Conn, gameID int64) (int64, error) {
	var ordinal int64
	var seeds prng.Seeds
	found := false
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET next_entity_ordinal = next_entity_ordinal + 1
		WHERE id = ?
		RETURNING next_entity_ordinal - 1, seed_high, seed_low;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			ordinal = stmt.ColumnInt64(0)
			seeds = prng.New(uint64(stmt.ColumnInt64(1)), uint64(stmt.ColumnInt64(2)))
			found = true
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("take an entity number in game %d: %w", gameID, err)
	}
	if !found {
		return 0, fmt.Errorf("take an entity number: game %d does not exist", gameID)
	}
	number, err := entityid.Number(seeds, ordinal)
	if err != nil {
		return 0, fmt.Errorf("take an entity number in game %d: %w", gameID, err)
	}
	return number, nil
}

// NextFactionNumber hands out the next faction number of a game. It is the
// counter itself: a player already knows how many factions a game has, so there
// is nothing here for a permutation to hide.
func NextFactionNumber(conn *sqlite.Conn, gameID int64) (int64, error) {
	var number int64
	if err := sqlitex.ExecuteTransient(conn, `
		UPDATE game SET next_faction_number = next_faction_number + 1
		WHERE id = ?
		RETURNING next_faction_number;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			number = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("take a faction number in game %d: %w", gameID, err)
	}
	if number == 0 {
		return 0, fmt.Errorf("take a faction number: game %d does not exist", gameID)
	}
	return number, nil
}
