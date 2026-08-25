// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package report

import (
	"fmt"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// faction is the player a report is written for.
type faction struct {
	id         int64
	turn       int
	controller string
}

func findFaction(conn *sqlite.Conn, gameCode, email string, factionID int64) (faction, error) {
	var found faction
	ok := false
	query := `
		SELECT f.id, u.email, g.turn
		FROM faction AS f
		JOIN game AS g ON g.id = f.game_id
		JOIN users AS u ON u.id = f.user_id
		WHERE g.code = ? AND u.email = ?;`
	args := []any{gameCode, email}
	if factionID != 0 {
		query = `
			SELECT f.id,
				CASE
					WHEN u.id IS NOT NULL THEN u.email
					ELSE 'agent:' || COALESCE(a.code, a.description)
				END,
				g.turn
			FROM faction AS f
			JOIN game AS g ON g.id = f.game_id
			LEFT JOIN users AS u ON u.id = f.user_id
			LEFT JOIN agent AS a ON a.id = f.agent_id
			WHERE g.code = ? AND f.id = ?;`
		args = []any{gameCode, factionID}
	}
	if err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found.id = stmt.ColumnInt64(0)
			found.controller = stmt.ColumnText(1)
			found.turn = stmt.ColumnInt(2)
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
		return faction{}, fmt.Errorf("player faction %d does not exist in game %q", factionID, gameCode)
	}
	return found, nil
}
