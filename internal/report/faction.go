// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package report

import (
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// faction is the player a report is written for. It holds both handles: the id
// is what the report's own queries join on, and the number is what the report
// prints and what the caller named the faction by.
type faction struct {
	id         int64
	number     int64
	turn       int
	controller string
}

// findFaction finds the faction a report is for, by the player's email or by
// the number the game knows the faction as. The number is per game, so the
// caller's `--faction 3` means the same faction whatever else the database
// holds.
func findFaction(conn *sqlite.Conn, gameCode, email string, factionNumber int64) (faction, error) {
	var found faction
	ok := false
	query := `
		SELECT f.id, f.number, u.email, g.turn
		FROM faction AS f
		JOIN game AS g ON g.id = f.game_id
		JOIN users AS u ON u.id = f.user_id
		WHERE g.code = ? AND u.email = ?;`
	args := []any{gameCode, email}
	if factionNumber != 0 {
		query = `
			SELECT f.id, f.number,
				CASE
					WHEN u.id IS NOT NULL THEN u.email
					ELSE 'agent:' || COALESCE(a.code, a.description)
				END,
				g.turn
			FROM faction AS f
			JOIN game AS g ON g.id = f.game_id
			LEFT JOIN users AS u ON u.id = f.user_id
			LEFT JOIN agent AS a ON a.id = f.agent_id
			WHERE g.code = ? AND f.number = ?;`
		args = []any{gameCode, factionNumber}
	}
	if err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found.id = stmt.ColumnInt64(0)
			found.number = stmt.ColumnInt64(1)
			found.controller = stmt.ColumnText(2)
			found.turn = stmt.ColumnInt(3)
			ok = true
			return nil
		},
	}); err != nil {
		return faction{}, fmt.Errorf("find player in game %q: %w", gameCode, err)
	}
	if !ok {
		if email != "" {
			return faction{}, fmt.Errorf("player %q does not exist in game %q", email, gameCode)
		}
		return faction{}, fmt.Errorf("player faction %d does not exist in game %q", factionNumber, gameCode)
	}
	return found, nil
}
