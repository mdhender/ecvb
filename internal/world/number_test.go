// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"testing"

	"github.com/mdhender/ecvb/internal/entityid"
	"github.com/mdhender/ecvb/internal/prng"
	"github.com/mdhender/ecvb/internal/testdb"
)

// The bug the numbers exist to fix: a row id is drawn from one sequence shared
// by every game in the database, so a second game's entity ids depended on how
// many rows the first game had already written. A number is drawn from the
// game's own counter and the game's own seeds, so the nth entity of a game gets
// the nth number whatever any other game has done.
func TestAGamesNumbersDoNotDependOnAnotherGame(t *testing.T) {
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
		INSERT INTO game (id, code, seed_high, seed_low) VALUES
			(1, 'ONE', 19, 12),
			(2, 'TWO', 19, 12);`)

	// Game ONE gets a head start, which is what used to shift game TWO's ids.
	for range 7 {
		if _, err := NextEntityNumber(conn, 1); err != nil {
			t.Fatal(err)
		}
	}
	seeds := prng.New(19, 12)
	for ordinal := int64(0); ordinal < 5; ordinal++ {
		want, err := entityid.Number(seeds, ordinal)
		if err != nil {
			t.Fatal(err)
		}
		got, err := NextEntityNumber(conn, 2)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("entity %d of game TWO got number %d; want %d", ordinal, got, want)
		}
		// And the two games are not handing out one sequence between them.
		other, err := NextEntityNumber(conn, 1)
		if err != nil {
			t.Fatal(err)
		}
		if other == got {
			t.Errorf("both games gave out number %d", got)
		}
	}
}

// A different pair of seeds gives a different game, which is what keeps two
// live games from reading like copies of each other.
func TestTheSeedsDecideTheNumbers(t *testing.T) {
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
		INSERT INTO game (id, code, seed_high, seed_low) VALUES
			(1, 'ONE', 19, 12),
			(2, 'TWO', 20, 12);`)
	same := 0
	for range 20 {
		a, err := NextEntityNumber(conn, 1)
		if err != nil {
			t.Fatal(err)
		}
		b, err := NextEntityNumber(conn, 2)
		if err != nil {
			t.Fatal(err)
		}
		if a == b {
			same++
		}
	}
	if same > 2 {
		t.Errorf("%d of 20 ordinals gave one number under two different seeds", same)
	}
}

// Faction numbers need no cover -- a player already knows how many factions a
// game has -- so they are the counter itself, and they are per game too.
func TestFactionNumbersCountFromOneInEachGame(t *testing.T) {
	conn := testdb.New(t)
	testdb.Exec(t, conn, "INSERT INTO game (id, code) VALUES (1, 'ONE'), (2, 'TWO');")
	for _, want := range []int64{1, 2, 3} {
		for _, gameID := range []int64{1, 2} {
			got, err := NextFactionNumber(conn, gameID)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("game %d handed out faction number %d; want %d", gameID, got, want)
			}
		}
	}
}

func TestTakingANumberFromAGameThatIsNotThereFails(t *testing.T) {
	conn := testdb.New(t)
	if _, err := NextEntityNumber(conn, 99); err == nil {
		t.Error("took an entity number from a game that does not exist; want an error")
	}
	if _, err := NextFactionNumber(conn, 99); err == nil {
		t.Error("took a faction number from a game that does not exist; want an error")
	}
}
