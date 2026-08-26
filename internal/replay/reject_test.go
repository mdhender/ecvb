// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package replay

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/orders"
	"github.com/mdhender/ecvb/internal/testdb"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// TestSubmitRejects covers the failures that never reach the engine.
//
// The two ways an order can fail are different in kind, and the golden replay
// only sees one of them. A shortfall of fuel is a warning at submission and a
// failed order at resolution, because fuel may still reach the ship; that path
// is in the replay. Everything else here -- a missing drive, a ship too heavy
// for the drive it has, a destination that does not exist, a probe budget
// already spent -- cannot change between submission and resolution, so the
// whole file is rejected and nothing is stored.
func TestSubmitRejects(t *testing.T) {
	scenario, err := os.ReadFile("testdata/scenario.sql")
	if err != nil {
		t.Fatalf("read scenario: %v", err)
	}
	for _, tc := range []struct {
		name  string
		order string
		want  string
	}{
		{
			name:  "no drive",
			order: "move ship 102 to orbit 6",
			want:  "ship 102 has no assembled HDRV and cannot move",
		},
		{
			name:  "ship outweighs its drive",
			order: "move ship 103 to orbit 6",
			want:  "ship 103 masses 9000 MU and its drive propels 1045 MU",
		},
		{
			name:  "jump beyond the drive's range",
			order: "jump ship 100 to (3,4,0)",
			want:  "jump of 5 units exceeds ship 100 jump range of 3 units",
		},
		{
			name:  "jump to empty space",
			order: "jump ship 100 to (9,9,9)",
			want:  `game "GOLD-01" has no stellium at (9,9,9)`,
		},
		{
			name:  "no planet in that orbit",
			order: "move ship 100 to orbit 9",
			want:  "system current has no planet in orbit 9",
		},
		{
			name:  "no such system in this stellium",
			order: "probe ship 100 system E orbit 1",
			want:  "current stellium has no system E",
		},
		{
			name:  "more probes than the sensors launch",
			order: "probe ship 100 orbit 4 6 4 6 4",
			want:  "ship 100 has only 4 probes this turn",
		},
		{
			name:  "a ship is not a colony",
			order: "probe colony 100 orbit 4",
			want:  "entity 100 is a ship, not a colony",
		},
		{
			name:  "another faction's ship",
			order: "move ship 200 to orbit 6",
			want:  "ship 200 does not belong to faction 1",
		},
		{
			name:  "the stellium orbit belongs to no system",
			order: "move ship 100 to system A orbit 11",
			want:  "orbit 11 is the stellium orbit and belongs to no system",
		},
		{
			name:  "a colony cannot move",
			order: "move ship 101 to orbit 6",
			want:  "entity 101 is a COPN, not a ship",
		},
		{
			name:  "a name longer than a name may be",
			order: `name ship 100 "twenty five characters!!!"`,
			want:  `a name may be 24 characters and "twenty five characters!!!" is 25`,
		},
		{
			name:  "a name that begins with a space",
			order: `name ship 100 " Bellerophon"`,
			want:  "a name may not begin or end with a space",
		},
		{
			name:  "a name with a gap in it",
			order: `name ship 100 "Bell  erophon"`,
			want:  "a name may not hold two spaces in a row",
		},
		{
			name:  "a name with nothing in it",
			order: `name ship 100 ""`,
			want:  "a name cannot be empty",
		},
		{
			name:  "naming a stellium that is not there",
			order: `name (9,9,9) "Nowhere"`,
			want:  `game "GOLD-01" has no stellium at (9,9,9)`,
		},
		{
			name:  "naming a system the stellium does not hold",
			order: `name (0,0,0) system E "Elsewhere"`,
			want:  "the stellium at (0,0,0) has no system E",
		},
		{
			name:  "naming a planet that is not in that orbit",
			order: `name (0,0,0) system A orbit 9 "Empty"`,
			want:  "system A has no planet in orbit 9",
		},
		{
			name:  "naming another faction's ship",
			order: `name ship 200 "Easy Target"`,
			want:  "ship 200 does not belong to faction 1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := testdb.New(t)
			testdb.Exec(t, conn, string(scenario))
			file := strings.NewReader("game \"GOLD-01\" turn 0\nid faction 1\n\n" + tc.order + "\n")
			_, err := orders.Submit(context.Background(), conn, file)
			if err == nil {
				t.Fatalf("Submit(%q) succeeded; want an error", tc.order)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Submit(%q) error = %v; want containing %q", tc.order, err, tc.want)
			}
			// A rejected file must leave nothing behind.
			assertNoStoredOrders(t, conn)
		})
	}
}

// assertNoStoredOrders checks that a rejected submission stored nothing. An
// invalid file must leave the faction's existing order set exactly as it was.
func assertNoStoredOrders(t *testing.T, conn *sqlite.Conn) {
	t.Helper()
	var rows int
	if err := sqlitex.ExecuteTransient(conn, "SELECT COUNT(*) FROM game_order;", &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			rows = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("count game_order: %v", err)
	}
	if rows != 0 {
		t.Errorf("game_order holds %d rows after a rejected submission; want none", rows)
	}
}
