// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mdhender/ecvb/internal/mapkey"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// The four seed files a map is made of. Every record is keyed by its own
// address -- a stellium by its coordinates, a system by its stellium and
// sequence, a planet by its system and orbit, a deposit by its planet and its
// number on that planet. These are the same tuples internal/prng addresses a
// draw by, so what names a thing in the file and what names it in a draw are
// one fact rather than two that could disagree.
//
// There are no UUIDs. They were doing one job that has to be replaced: because
// a UUID was unique to one generated map, feeding in a file from a DIFFERENT
// map failed loudly. Coordinates are shared vocabulary, so a mismatched set
// would now join cleanly and build a wrong-but-valid game. The seed block is
// what catches that instead -- see checkOneMap.

type seedPair struct {
	High string `json:"high"`
	Lo   string `json:"lo"`
}

type stelliaSeed struct {
	Seed    seedPair       `json:"seed"`
	Stellia []seedStellium `json:"stellia"`
}

type seedStellium struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

type systemsSeed struct {
	Seed    seedPair     `json:"seed"`
	Systems []seedSystem `json:"systems"`
}

type seedSystem struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Z        int    `json:"z"`
	Sequence string `json:"sequence"`
}

type planetsSeed struct {
	Seed    seedPair     `json:"seed"`
	Planets []seedPlanet `json:"planets"`
}

type seedPlanet struct {
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Z            int    `json:"z"`
	Sequence     string `json:"sequence"`
	Orbit        int    `json:"orbit"`
	Type         string `json:"type"`
	Habitability int    `json:"habitability"`
}

type depositsSeed struct {
	Seed     seedPair      `json:"seed"`
	Deposits []seedDeposit `json:"deposits"`
}

// A deposit's ordinal on its planet is "deposit-no" in the file, because
// "sequence" is the system's letter and both appear in one record. The column
// is still deposit.sequence; this is the one place the two names meet.
type seedDeposit struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Z         int    `json:"z"`
	Sequence  string `json:"sequence"`
	Orbit     int    `json:"orbit"`
	DepositNo int    `json:"deposit-no"`
	Resource  string `json:"resource"`
	Quantity  int    `json:"quantity"`
	Quality   int    `json:"quality"`
}

// The address tuples the four files join on. They are comparable, so they are
// map keys directly, and they print as the player would say them.
type stelliumKey struct{ x, y, z int }
type systemKey struct {
	stelliumKey
	sequence string
}
type planetKey struct {
	systemKey
	orbit int
}

func (k stelliumKey) String() string { return fmt.Sprintf("(%d,%d,%d)", k.x, k.y, k.z) }
func (k systemKey) String() string   { return fmt.Sprintf("system %s at %s", k.sequence, k.stelliumKey) }
func (k planetKey) String() string   { return fmt.Sprintf("orbit %d of %s", k.orbit, k.systemKey) }

func loadGame(ctx context.Context, directory, code string) (err error) {
	if code == "" {
		return fmt.Errorf("game code is required")
	}

	var stellia stelliaSeed
	var systems systemsSeed
	var planets planetsSeed
	var deposits depositsSeed
	for _, seed := range []struct {
		name string
		data any
	}{
		{name: "stellia-seed.json", data: &stellia},
		{name: "systems-seed.json", data: &systems},
		{name: "planets-seed.json", data: &planets},
		{name: "deposits-seed.json", data: &deposits},
	} {
		if err := readSeedFile(filepath.Join(directory, seed.name), seed.data); err != nil {
			return err
		}
	}

	conn, _, err := openVerifiedDatabase(ctx, directory)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	end, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return fmt.Errorf("begin load transaction: %w", err)
	}
	defer end(&err)

	gameID, found, err := findGameID(conn, code)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("game code %q does not exist", code)
	}
	loaded, err := gameHasData(conn, gameID)
	if err != nil {
		return err
	}
	if loaded {
		return fmt.Errorf("game code %q already has data loaded", code)
	}
	if err := insertGameSeeds(conn, gameID, stellia, systems, planets, deposits); err != nil {
		return fmt.Errorf("load game %q: %w", code, err)
	}
	return nil
}

func readSeedFile(path string, dst any) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("required seed file %s does not exist", path)
		}
		return fmt.Errorf("stat seed file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("required seed file %s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open seed file %s: %w", path, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("parse seed file %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected additional JSON value")
		}
		return fmt.Errorf("parse seed file %s: %w", path, err)
	}
	return nil
}

func findGameID(conn *sqlite.Conn, code string) (id int64, found bool, err error) {
	err = sqlitex.ExecuteTransient(conn, "SELECT id FROM game WHERE code = ?;", &sqlitex.ExecOptions{
		Args: []any{code},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, found = stmt.ColumnInt64(0), true
			return nil
		},
	})
	if err != nil {
		return 0, false, fmt.Errorf("find game %q: %w", code, err)
	}
	return id, found, nil
}

// gameHasData reports whether a game has been loaded already. Every order a
// player has ever written is one table, so this asks about all of them rather
// than about the ones somebody remembered to list.
func gameHasData(conn *sqlite.Conn, gameID int64) (loaded bool, err error) {
	err = sqlitex.ExecuteTransient(conn, `
		SELECT EXISTS (
			SELECT 1 FROM stellium WHERE game_id = ?
			UNION ALL SELECT 1 FROM faction WHERE game_id = ?
			UNION ALL SELECT 1 FROM game_order WHERE game_id = ?
		);`, &sqlitex.ExecOptions{
		Args: []any{gameID, gameID, gameID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			loaded = stmt.ColumnInt(0) != 0
			return nil
		},
	})
	if err != nil {
		return false, fmt.Errorf("check game data: %w", err)
	}
	return loaded, nil
}

// checkOneMap refuses four seed files that did not come from one map.
//
// The UUIDs this replaced made that check free: they were unique to a generated
// map, so a file from another one referenced parents that were not there.
// Coordinates are shared vocabulary -- every map has a (3,-7,12) -- so a
// mismatched set would join cleanly and quietly build the wrong game. The seed
// block is the replacement, and it is stronger: it catches a mismatch even when
// the two maps happen to agree about which coordinates exist.
func checkOneMap(stellia stelliaSeed, systems systemsSeed, planets planetsSeed, deposits depositsSeed) error {
	if stellia.Seed == (seedPair{}) {
		return fmt.Errorf("stellia-seed.json has no seed block; it was not written by ecgen")
	}
	for _, other := range []struct {
		name string
		seed seedPair
	}{
		{"systems-seed.json", systems.Seed},
		{"planets-seed.json", planets.Seed},
		{"deposits-seed.json", deposits.Seed},
	} {
		if other.seed != stellia.Seed {
			return fmt.Errorf("%s was generated from seed (%s, %s) and stellia-seed.json from (%s, %s); the four seed files must come from one map",
				other.name, other.seed.High, other.seed.Lo, stellia.Seed.High, stellia.Seed.Lo)
		}
	}
	return nil
}

func insertGameSeeds(conn *sqlite.Conn, gameID int64, stellia stelliaSeed, systems systemsSeed, planets planetsSeed, deposits depositsSeed) error {
	if err := checkOneMap(stellia, systems, planets, deposits); err != nil {
		return err
	}

	stelliumIDs := make(map[stelliumKey]int64, len(stellia.Stellia))
	for i, stellium := range stellia.Stellia {
		key := stelliumKey{stellium.X, stellium.Y, stellium.Z}
		if _, ok := stelliumIDs[key]; ok {
			return fmt.Errorf("stellia[%d] repeats the stellium at %s", i, key)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO stellium (game_id, x, y, z) VALUES (?, ?, ?, ?);", &sqlitex.ExecOptions{Args: []any{gameID, stellium.X, stellium.Y, stellium.Z}}); err != nil {
			return fmt.Errorf("insert stellia[%d]: %w", i, err)
		}
		stelliumIDs[key] = conn.LastInsertRowID()
	}

	systemIDs := make(map[systemKey]int64, len(systems.Systems))
	for i, system := range systems.Systems {
		key := systemKey{stelliumKey{system.X, system.Y, system.Z}, system.Sequence}
		if _, err := mapkey.Sequence(system.Sequence); err != nil {
			return fmt.Errorf("systems[%d] %w", i, err)
		}
		if _, ok := systemIDs[key]; ok {
			return fmt.Errorf("systems[%d] repeats %s", i, key)
		}
		stelliumID, ok := stelliumIDs[key.stelliumKey]
		if !ok {
			return fmt.Errorf("systems[%d] names no stellium at %s", i, key.stelliumKey)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO system (stellium_id, sequence) VALUES (?, ?);", &sqlitex.ExecOptions{Args: []any{stelliumID, system.Sequence}}); err != nil {
			return fmt.Errorf("insert systems[%d]: %w", i, err)
		}
		systemIDs[key] = conn.LastInsertRowID()
	}

	planetIDs := make(map[planetKey]int64, len(planets.Planets))
	for i, planet := range planets.Planets {
		key := planetKey{systemKey{stelliumKey{planet.X, planet.Y, planet.Z}, planet.Sequence}, planet.Orbit}
		if _, ok := planetIDs[key]; ok {
			return fmt.Errorf("planets[%d] repeats %s", i, key)
		}
		systemID, ok := systemIDs[key.systemKey]
		if !ok {
			return fmt.Errorf("planets[%d] names no %s", i, key.systemKey)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO planet (system_id, orbit, kind, habitability) VALUES (?, ?, ?, ?);", &sqlitex.ExecOptions{Args: []any{systemID, planet.Orbit, planet.Type, planet.Habitability}}); err != nil {
			return fmt.Errorf("insert planets[%d]: %w", i, err)
		}
		planetIDs[key] = conn.LastInsertRowID()
	}

	seenDeposits := make(map[planetKey]map[int]bool, len(planets.Planets))
	for i, deposit := range deposits.Deposits {
		key := planetKey{systemKey{stelliumKey{deposit.X, deposit.Y, deposit.Z}, deposit.Sequence}, deposit.Orbit}
		planetID, ok := planetIDs[key]
		if !ok {
			return fmt.Errorf("deposits[%d] names no planet in %s", i, key)
		}
		if seenDeposits[key] == nil {
			seenDeposits[key] = make(map[int]bool)
		}
		if seenDeposits[key][deposit.DepositNo] {
			return fmt.Errorf("deposits[%d] repeats deposit %d of %s", i, deposit.DepositNo, key)
		}
		seenDeposits[key][deposit.DepositNo] = true
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO deposit (planet_id, sequence, resource, quality, initial_qty, current_qty) VALUES (?, ?, ?, ?, ?, ?);", &sqlitex.ExecOptions{Args: []any{planetID, deposit.DepositNo, deposit.Resource, deposit.Quality, deposit.Quantity, deposit.Quantity}}); err != nil {
			return fmt.Errorf("insert deposits[%d]: %w", i, err)
		}
	}
	return nil
}
