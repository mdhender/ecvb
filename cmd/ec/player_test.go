// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/database"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestRunAddPlayer(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	writeTestKit(t, directory)
	conn := openPlayerTestDatabase(t, directory)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (email, role) VALUES ('player@example.com', 'non-administrator');
		INSERT INTO game (code) VALUES ('TEST');
		INSERT INTO stellium (game_id, x, y, z) VALUES (1, 4, 5, 6);
		INSERT INTO system (stellium_id, sequence) VALUES (1, 'A');
		INSERT INTO planet (system_id, orbit, kind, habitability) VALUES
			(1, 3, 'rocky', 8),
			(1, 4, 'rocky', 25);
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{
		"--db-path", directory, "add", "player",
		"--game", "TEST", "--email", "  PLAYER@EXAMPLE.COM ",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run add player: %v", err)
	}
	if got := stdout.String(); got != "1\n" {
		t.Fatalf("stdout = %q; want %q", got, "1\n")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q; want empty", got)
	}

	conn = openPlayerTestDatabase(t, directory)
	defer conn.Close()
	var factionUserID, planetFactionID int64
	var orbit, habitability int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT f.user_id, p.faction_id, p.orbit, p.habitability
		FROM faction AS f
		JOIN planet AS p ON p.faction_id = f.id;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			factionUserID = stmt.ColumnInt64(0)
			planetFactionID = stmt.ColumnInt64(1)
			orbit = stmt.ColumnInt(2)
			habitability = stmt.ColumnInt(3)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if factionUserID != 1 || planetFactionID != 1 || orbit != 4 || habitability != 25 {
		t.Fatalf("home assignment = (user %d, faction %d, orbit %d, habitability %d); want (1, 1, 4, 25)", factionUserID, planetFactionID, orbit, habitability)
	}
}

func TestHomeCandidatesRequireOrbitFourAndHabitability25(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	conn := openPlayerTestDatabase(t, directory)
	defer conn.Close()
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO game (code) VALUES ('TEST');
		INSERT INTO stellium (game_id, x, y, z) VALUES
			(1, 1, 0, 0), (1, 2, 0, 0), (1, 3, 0, 0);
		INSERT INTO system (stellium_id, sequence) VALUES
			(1, 'A'), (2, 'A'), (3, 'A');
		INSERT INTO planet (system_id, orbit, kind, habitability) VALUES
			(1, 3, 'rocky', 25),
			(2, 4, 'rocky', 8),
			(3, 4, 'rocky', 25);
	`, nil); err != nil {
		t.Fatal(err)
	}

	candidates, err := homeCandidates(conn, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].planetID != 3 {
		t.Fatalf("candidates = %+v; want only orbit-4, habitability-25 planet 3", candidates)
	}
}

func TestAddPlayerUsesRoundedUpDistance(t *testing.T) {
	tests := []struct {
		name      string
		x         int
		y         int
		wantError bool
	}{
		{name: "distance six", x: 6, wantError: true},
		{name: "distance rounds up to seven", x: 6, y: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "database")
			createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
			writeTestKit(t, directory)
			conn := openPlayerTestDatabase(t, directory)
			script := fmt.Sprintf(`
				INSERT INTO users (email, role) VALUES
					('first@example.com', 'non-administrator'),
					('second@example.com', 'non-administrator');
				INSERT INTO game (code) VALUES ('TEST');
				INSERT INTO stellium (game_id, x, y, z) VALUES (1, 0, 0, 0), (1, %d, %d, 0);
				INSERT INTO system (stellium_id, sequence) VALUES (1, 'A'), (2, 'A');
				INSERT INTO planet (system_id, orbit, kind, habitability) VALUES
					(1, 4, 'rocky', 25), (2, 4, 'rocky', 25);
				INSERT INTO faction (game_id, user_id) VALUES (1, 1);
				UPDATE planet SET faction_id = 1 WHERE system_id = 1 AND orbit = 4;
			`, tt.x, tt.y)
			if err := sqlitex.ExecuteScript(conn, script, nil); err != nil {
				t.Fatal(err)
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}

			_, err := addPlayer(context.Background(), directory, "TEST", "second@example.com")
			if tt.wantError && (err == nil || !strings.Contains(err.Error(), "no eligible home planet")) {
				t.Fatalf("addPlayer error = %v; want no eligible home planet", err)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("addPlayer: %v", err)
			}

			conn = openPlayerTestDatabase(t, directory)
			defer conn.Close()
			var factions int
			if err := sqlitex.ExecuteTransient(conn, "SELECT count(*) FROM faction;", &sqlitex.ExecOptions{ResultFunc: func(stmt *sqlite.Stmt) error {
				factions = stmt.ColumnInt(0)
				return nil
			}}); err != nil {
				t.Fatal(err)
			}
			wantFactions := 3
			if tt.wantError {
				wantFactions = 1
			}
			if factions != wantFactions {
				t.Fatalf("factions = %d; want %d", factions, wantFactions)
			}
		})
	}
}

func TestAddPlayerRejectsInvalidGameState(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	conn := openPlayerTestDatabase(t, directory)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (email, role) VALUES ('player@example.com', 'non-administrator');
		INSERT INTO game (code, turn) VALUES ('STARTED', 1);
		INSERT INTO game (code) VALUES ('READY');
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		game  string
		email string
		want  string
	}{
		{name: "missing game", game: "MISSING", email: "player@example.com", want: "does not exist"},
		{name: "started game", game: "STARTED", email: "player@example.com", want: "turn 1"},
		{name: "missing user", game: "READY", email: "missing@example.com", want: "does not exist"},
		{name: "invalid email", game: "STARTED", email: "not an email", want: "invalid email"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := addPlayer(context.Background(), directory, tt.game, tt.email)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("addPlayer error = %v; want containing %q", err, tt.want)
			}
		})
	}
}

func TestAddPlayerRejectsDuplicateUser(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "database")
	createTestDatabase(t, directory, database.ApplicationID, database.SchemaVersion)
	conn := openPlayerTestDatabase(t, directory)
	if err := sqlitex.ExecuteScript(conn, `
		INSERT INTO users (email, role) VALUES ('player@example.com', 'non-administrator');
		INSERT INTO game (code) VALUES ('TEST');
		INSERT INTO faction (game_id, user_id) VALUES (1, 1);
	`, nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := addPlayer(context.Background(), directory, "TEST", "player@example.com")
	if err == nil || !strings.Contains(err.Error(), "already in game") {
		t.Fatalf("addPlayer error = %v; want duplicate error", err)
	}
}

func TestSeparatedFromHomes(t *testing.T) {
	homes := []homeCandidate{{x: 0, y: 0, z: 0}}
	if separatedFromHomes(homeCandidate{x: 6}, homes) {
		t.Error("squared distance 36 accepted; want rejected")
	}
	if !separatedFromHomes(homeCandidate{x: 6, y: 1}, homes) {
		t.Error("squared distance 37 rejected; want accepted")
	}
}

func TestSelectHomeCandidateIsDeterministicAcrossInputOrder(t *testing.T) {
	ascending := []homeCandidate{
		{planetID: 1, x: -12, y: 4, z: 8},
		{planetID: 2, x: -2, y: 7, z: 1},
		{planetID: 3, x: 5, y: -3, z: 9},
		{planetID: 4, x: 13, y: 2, z: -6},
	}
	reversed := []homeCandidate{ascending[3], ascending[2], ascending[1], ascending[0]}
	first, found := selectHomeCandidate(ascending, nil, 19, 12)
	if !found {
		t.Fatal("selectHomeCandidate found no candidate")
	}
	second, found := selectHomeCandidate(reversed, nil, 19, 12)
	if !found {
		t.Fatal("selectHomeCandidate found no candidate for reversed input")
	}
	if first != second {
		t.Fatalf("selection differs by input order: %+v and %+v", first, second)
	}
}

func openPlayerTestDatabase(t *testing.T, directory string) *sqlite.Conn {
	t.Helper()
	conn, err := sqlite.OpenConn(filepath.Join(directory, database.Filename), sqlite.OpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return conn
}
