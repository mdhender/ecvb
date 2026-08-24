// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"fmt"
	"net/mail"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type reportFaction struct {
	id         int64
	turn       int
	controller string
}

func normalizeFactionSelector(gameCode, email string, factionID int64) (string, error) {
	if gameCode == "" {
		return "", fmt.Errorf("game is required")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			return "", fmt.Errorf("invalid email address %q", email)
		}
	}
	if (email == "") == (factionID == 0) {
		return "", fmt.Errorf("exactly one of email or faction is required")
	}
	if factionID < 0 {
		return "", fmt.Errorf("invalid faction id %d", factionID)
	}
	return email, nil
}

func findReportFaction(conn *sqlite.Conn, gameCode, email string, factionID int64) (reportFaction, error) {
	var faction reportFaction
	found := false
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
			faction.id = stmt.ColumnInt64(0)
			faction.controller = stmt.ColumnText(1)
			faction.turn = stmt.ColumnInt(2)
			found = true
			return nil
		},
	}); err != nil {
		return reportFaction{}, fmt.Errorf("find player in game %q: %w", gameCode, err)
	}
	if !found {
		if email != "" {
			return reportFaction{}, fmt.Errorf("player %q does not exist in game %q", email, gameCode)
		}
		return reportFaction{}, fmt.Errorf("player faction %d does not exist in game %q", factionID, gameCode)
	}
	return faction, nil
}
