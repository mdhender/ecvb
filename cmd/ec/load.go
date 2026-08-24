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

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type stelliaSeed struct {
	Stellia []seedStellium `json:"stellia"`
}

type seedStellium struct {
	UUID string `json:"uuid"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Z    int    `json:"z"`
}

type systemsSeed struct {
	Systems []seedSystem `json:"systems"`
}

type seedSystem struct {
	UUID         string `json:"uuid"`
	StelliumUUID string `json:"stellium-uuid"`
	Sequence     string `json:"sequence"`
}

type planetsSeed struct {
	Planets []seedPlanet `json:"planets"`
}

type seedPlanet struct {
	UUID         string `json:"uuid"`
	SystemUUID   string `json:"system-uuid"`
	Orbit        int    `json:"orbit"`
	Type         string `json:"type"`
	Habitability int    `json:"habitability"`
}

type depositsSeed struct {
	Deposits []seedDeposit `json:"deposits"`
}

type seedDeposit struct {
	PlanetUUID string `json:"planet-uuid"`
	Sequence   int    `json:"sequence"`
	Resource   string `json:"resource"`
	Quantity   int    `json:"quantity"`
	Quality    int    `json:"quality"`
}

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

func gameHasData(conn *sqlite.Conn, gameID int64) (loaded bool, err error) {
	err = sqlitex.ExecuteTransient(conn, `
		SELECT EXISTS (
			SELECT 1 FROM stellium WHERE game_id = ?
			UNION ALL SELECT 1 FROM faction WHERE game_id = ?
			UNION ALL SELECT 1 FROM order_entry WHERE game_id = ?
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

func insertGameSeeds(conn *sqlite.Conn, gameID int64, stellia stelliaSeed, systems systemsSeed, planets planetsSeed, deposits depositsSeed) error {
	stelliumIDs := make(map[string]int64, len(stellia.Stellia))
	for i, stellium := range stellia.Stellia {
		if stellium.UUID == "" || stelliumIDs[stellium.UUID] != 0 {
			return fmt.Errorf("stellia[%d] has missing or duplicate uuid %q", i, stellium.UUID)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO stellium (game_id, x, y, z) VALUES (?, ?, ?, ?);", &sqlitex.ExecOptions{Args: []any{gameID, stellium.X, stellium.Y, stellium.Z}}); err != nil {
			return fmt.Errorf("insert stellia[%d]: %w", i, err)
		}
		stelliumIDs[stellium.UUID] = conn.LastInsertRowID()
	}

	systemIDs := make(map[string]int64, len(systems.Systems))
	for i, system := range systems.Systems {
		stelliumID, ok := stelliumIDs[system.StelliumUUID]
		if system.UUID == "" || systemIDs[system.UUID] != 0 {
			return fmt.Errorf("systems[%d] has missing or duplicate uuid %q", i, system.UUID)
		}
		if !ok {
			return fmt.Errorf("systems[%d] references unknown stellium uuid %q", i, system.StelliumUUID)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO system (stellium_id, sequence) VALUES (?, ?);", &sqlitex.ExecOptions{Args: []any{stelliumID, system.Sequence}}); err != nil {
			return fmt.Errorf("insert systems[%d]: %w", i, err)
		}
		systemIDs[system.UUID] = conn.LastInsertRowID()
	}

	planetIDs := make(map[string]int64, len(planets.Planets))
	for i, planet := range planets.Planets {
		systemID, ok := systemIDs[planet.SystemUUID]
		if planet.UUID == "" || planetIDs[planet.UUID] != 0 {
			return fmt.Errorf("planets[%d] has missing or duplicate uuid %q", i, planet.UUID)
		}
		if !ok {
			return fmt.Errorf("planets[%d] references unknown system uuid %q", i, planet.SystemUUID)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO planet (system_id, orbit, kind, habitability) VALUES (?, ?, ?, ?);", &sqlitex.ExecOptions{Args: []any{systemID, planet.Orbit, planet.Type, planet.Habitability}}); err != nil {
			return fmt.Errorf("insert planets[%d]: %w", i, err)
		}
		planetIDs[planet.UUID] = conn.LastInsertRowID()
	}

	for i, deposit := range deposits.Deposits {
		planetID, ok := planetIDs[deposit.PlanetUUID]
		if !ok {
			return fmt.Errorf("deposits[%d] references unknown planet uuid %q", i, deposit.PlanetUUID)
		}
		if err := sqlitex.ExecuteTransient(conn, "INSERT INTO deposit (planet_id, sequence, resource, quality, initial_qty, current_qty) VALUES (?, ?, ?, ?, ?, ?);", &sqlitex.ExecOptions{Args: []any{planetID, deposit.Sequence, deposit.Resource, deposit.Quality, deposit.Quantity, deposit.Quantity}}); err != nil {
			return fmt.Errorf("insert deposits[%d]: %w", i, err)
		}
	}
	return nil
}
