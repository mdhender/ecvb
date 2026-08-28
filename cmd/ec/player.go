// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"fmt"
	"net/mail"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mdhender/ecvb/internal/prng"
	"github.com/mdhender/ecvb/internal/world"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type homeCandidate struct {
	stelliumID int64
	systemID   int64
	planetID   int64
	x          int
	y          int
	z          int
}

// addPlayer adds a player and returns the number their faction is known by in
// this game -- what a report prints and what "ecrpt --faction" takes, never the
// row id.
func addPlayer(ctx context.Context, directory, gameCode, email string) (factionNumber int64, err error) {
	return addPlayerWithKit(ctx, directory, gameCode, email, filepath.Join(directory, "home-planet-seed.json"))
}

func addPlayerWithKit(ctx context.Context, directory, gameCode, email, kitPath string) (factionNumber int64, err error) {
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

	// The kit is read, the uncontrolled faction is settled, and this player's
	// faction number is taken BEFORE a home is chosen, because the number is
	// what the choice is addressed by. Nothing here is lost if the choice then
	// fails: it is all one transaction, so the counter bump rolls back with
	// everything else.
	kit, err := readKit(kitPath)
	if err != nil {
		return 0, err
	}

	// The uncontrolled faction is made before the first player rather than
	// after, so that a game's faction numbers read as the player expects: 1 is
	// the agent that holds the derelicts, and the players count from 2. It is
	// still made only when a kit actually hands it something.
	uncontrolledFactionID := int64(0)
	for _, entity := range kit.entities {
		if !entity.controlled {
			uncontrolledFactionID, err = ensureUncontrolledFaction(conn, gameID)
			if err != nil {
				return 0, err
			}
			break
		}
	}
	factionNumber, err = world.NextFactionNumber(conn, gameID)
	if err != nil {
		return 0, err
	}

	candidates, err := homeCandidates(conn, gameID)
	if err != nil {
		return 0, err
	}
	homes, err := existingHomes(conn, gameID)
	if err != nil {
		return 0, err
	}
	selected, found := selectHomeCandidate(candidates, homes, prng.New(uint64(seedHigh), uint64(seedLow)), factionNumber)
	if !found {
		return 0, fmt.Errorf("game %q has no eligible home planet", gameCode)
	}
	if err := sqlitex.ExecuteTransient(conn, "INSERT INTO faction (game_id, number, user_id) VALUES (?, ?, ?);", &sqlitex.ExecOptions{
		Args: []any{gameID, factionNumber, userID},
	}); err != nil {
		return 0, fmt.Errorf("create faction for %q in game %q: %w", email, gameCode, err)
	}
	factionID := conn.LastInsertRowID()
	if err := sqlitex.ExecuteTransient(conn, "UPDATE planet SET faction_id = ? WHERE id = ? AND faction_id IS NULL;", &sqlitex.ExecOptions{
		Args: []any{factionID, selected.planetID},
	}); err != nil {
		return 0, fmt.Errorf("assign home planet: %w", err)
	}
	if conn.Changes() != 1 {
		return 0, fmt.Errorf("assign home planet: planet is no longer available")
	}
	if err := insertKit(conn, kit, selected, gameID, factionID, uncontrolledFactionID); err != nil {
		return 0, fmt.Errorf("load kit %q: %w", kit.name, err)
	}
	return factionNumber, nil
}

func homeCandidates(conn *sqlite.Conn, gameID int64) ([]homeCandidate, error) {
	var candidates []homeCandidate
	err := sqlitex.ExecuteTransient(conn, `
		SELECT st.id, sy.id, p.id, st.x, st.y, st.z
		FROM stellium AS st
		JOIN system AS sy ON sy.stellium_id = st.id
		JOIN planet AS p ON p.system_id = sy.id AND p.orbit = 4 AND p.habitability = 25
		WHERE st.game_id = ?
		  AND p.faction_id IS NULL
		  AND (SELECT count(*) FROM system AS counted WHERE counted.stellium_id = st.id) = 1;`, &sqlitex.ExecOptions{
		Args: []any{gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			candidates = append(candidates, homeCandidate{
				stelliumID: stmt.ColumnInt64(0),
				systemID:   stmt.ColumnInt64(1),
				planetID:   stmt.ColumnInt64(2),
				x:          stmt.ColumnInt(3),
				y:          stmt.ColumnInt(4),
				z:          stmt.ColumnInt(5),
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

// selectHomeCandidate picks the planet a new faction starts on.
//
// The draw is addressed rather than sequenced: internal/prng hashes the game's
// seeds together with the faction's own number, so which planet a faction is
// given depends on who they are and not on when they joined. It used to seed a
// bare PCG from the raw game seeds with no domain tag at all -- which meant
// every faction shuffled the identical permutation, and the only thing telling
// them apart was that the earlier ones had already taken the head of the list.
// It also meant the draw shared a stream with anything else that seeded a PCG
// the same way.
//
// The faction is named by the number the game gave it, never by its row id: a
// row id comes from a sequence shared with every other game in the database.
//
// The sort before the shuffle is not decoration. SQLite promises no row order,
// so it is what makes the permutation a function of the candidate SET rather
// than of the query plan.
func selectHomeCandidate(candidates, homes []homeCandidate, seeds prng.Seeds, factionNumber int64) (homeCandidate, bool) {
	slices.SortFunc(candidates, compareHomeCandidates)
	roller := seeds.Roller(prng.TagFaction, prng.Key(factionNumber))
	roller.Shuffle(len(candidates), func(i, j int) {
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
