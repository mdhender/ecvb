// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/mail"
	"slices"
	"strings"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type homeCandidate struct {
	planetID int64
	x        int
	y        int
	z        int
}

func addPlayer(ctx context.Context, directory, gameCode, email string) (factionID int64, err error) {
	if gameCode == "" {
		return 0, fmt.Errorf("game is required")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	address, parseErr := mail.ParseAddress(email)
	if parseErr != nil || address.Address != email {
		return 0, fmt.Errorf("invalid email address %q", email)
	}

	conn, _, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return 0, fmt.Errorf("begin add player transaction: %w", err)
	}
	defer end(&err)

	var gameID, seedHigh, seedLow int64
	var turn int
	gameFound := false
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT id, turn, seed_high, seed_low
		FROM game
		WHERE code = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameCode},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			gameID = stmt.ColumnInt64(0)
			turn = stmt.ColumnInt(1)
			seedHigh = stmt.ColumnInt64(2)
			seedLow = stmt.ColumnInt64(3)
			gameFound = true
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find game %q: %w", gameCode, err)
	}
	if !gameFound {
		return 0, fmt.Errorf("game code %q does not exist", gameCode)
	}
	if turn != 0 {
		return 0, fmt.Errorf("game code %q is on turn %d; players can only be added on turn 0", gameCode, turn)
	}

	var userID int64
	userFound := false
	if err := sqlitex.ExecuteTransient(conn, "SELECT id FROM users WHERE email = ?;", &sqlitex.ExecOptions{
		Args: []any{email},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			userID = stmt.ColumnInt64(0)
			userFound = true
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("find user %q: %w", email, err)
	}
	if !userFound {
		return 0, fmt.Errorf("user %q does not exist", email)
	}

	alreadyAdded := false
	if err := sqlitex.ExecuteTransient(conn, "SELECT EXISTS (SELECT 1 FROM faction WHERE game_id = ? AND user_id = ?);", &sqlitex.ExecOptions{
		Args: []any{gameID, userID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			alreadyAdded = stmt.ColumnInt(0) != 0
			return nil
		},
	}); err != nil {
		return 0, fmt.Errorf("check player membership: %w", err)
	}
	if alreadyAdded {
		return 0, fmt.Errorf("user %q is already in game %q", email, gameCode)
	}

	candidates, err := homeCandidates(conn, gameID)
	if err != nil {
		return 0, err
	}
	homes, err := existingHomes(conn, gameID)
	if err != nil {
		return 0, err
	}
	selected, found := selectHomeCandidate(candidates, homes, seedHigh, seedLow)
	if !found {
		return 0, fmt.Errorf("game %q has no eligible home planet", gameCode)
	}

	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO faction (game_id, user_id) VALUES (?, ?);", &sqlitex.ExecOptions{
		Args: []any{gameID, userID},
	}); err != nil {
		return 0, fmt.Errorf("create faction for %q in game %q: %w", email, gameCode, err)
	}
	factionID = conn.LastInsertRowID()
	if err := sqlitex.ExecuteTransient(conn, "UPDATE planet SET faction_id = ? WHERE id = ? AND faction_id IS NULL;", &sqlitex.ExecOptions{
		Args: []any{factionID, selected.planetID},
	}); err != nil {
		return 0, fmt.Errorf("assign home planet: %w", err)
	}
	if conn.Changes() != 1 {
		return 0, fmt.Errorf("assign home planet: planet is no longer available")
	}
	return factionID, nil
}

func homeCandidates(conn *sqlite.Conn, gameID int64) ([]homeCandidate, error) {
	var candidates []homeCandidate
	err := sqlitex.ExecuteTransient(conn, `
		SELECT p.id, st.x, st.y, st.z
		FROM stellium AS st
		JOIN system AS sy ON sy.stellium_id = st.id
		JOIN planet AS p ON p.system_id = sy.id AND p.orbit = 4 AND p.habitability = 25
		WHERE st.game_id = ?
		  AND p.faction_id IS NULL
		  AND (SELECT count(*) FROM system AS counted WHERE counted.stellium_id = st.id) = 1;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			candidates = append(candidates, homeCandidate{
				planetID: stmt.ColumnInt64(0),
				x:        stmt.ColumnInt(1),
				y:        stmt.ColumnInt(2),
				z:        stmt.ColumnInt(3),
			})
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("find potential home planets: %w", err)
	}
	return candidates, nil
}

func existingHomes(conn *sqlite.Conn, gameID int64) ([]homeCandidate, error) {
	var homes []homeCandidate
	err := sqlitex.ExecuteTransient(conn, `
		SELECT st.x, st.y, st.z
		FROM faction AS f
		JOIN planet AS p ON p.faction_id = f.id AND p.orbit = 4 AND p.habitability = 25
		JOIN system AS sy ON sy.id = p.system_id
		JOIN stellium AS st ON st.id = sy.stellium_id
		WHERE f.game_id = ? AND st.game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{gameID, gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			homes = append(homes, homeCandidate{
				x: stmt.ColumnInt(0),
				y: stmt.ColumnInt(1),
				z: stmt.ColumnInt(2),
			})
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("find existing home planets: %w", err)
	}
	return homes, nil
}

func compareHomeCandidates(a, b homeCandidate) int {
	if n := a.x - b.x; n != 0 {
		return n
	}
	if n := a.y - b.y; n != 0 {
		return n
	}
	return a.z - b.z
}

func selectHomeCandidate(candidates, homes []homeCandidate, seedHigh, seedLow int64) (homeCandidate, bool) {
	slices.SortFunc(candidates, compareHomeCandidates)
	rng := rand.New(rand.NewPCG(uint64(seedHigh), uint64(seedLow)))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	for _, candidate := range candidates {
		if separatedFromHomes(candidate, homes) {
			return candidate, true
		}
	}
	return homeCandidate{}, false
}

func separatedFromHomes(candidate homeCandidate, homes []homeCandidate) bool {
	for _, home := range homes {
		dx, dy, dz := candidate.x-home.x, candidate.y-home.y, candidate.z-home.z
		if dx*dx+dy*dy+dz*dz <= 36 {
			return false
		}
	}
	return true
}
