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
			order: "ship 327307 move to orbit 6",
			want:  "ship 327307 has no assembled HDRV and cannot move",
		},
		{
			name:  "ship outweighs its drive",
			order: "ship 896680 move to orbit 6",
			want:  "ship 896680 masses 9000 MU and its drive propels 1045 MU",
		},
		{
			// Ship 985070 starts at a planet, and a jump begins from the stellium
			// orbit. (3,4,0) is 5 light years away, which the drive's old
			// range of 3 refused and nothing refuses now.
			name:  "jump from a planet",
			order: "ship 985070 jump to (3,4,0)",
			want:  "ship 985070 is at a planet and a jump begins from the stellium orbit; move it to orbit 11 first",
		},
		{
			name:  "jump to empty space",
			order: "ship 985070 jump to (9,9,9)",
			want:  `game "GOLD-01" has no stellium at (9,9,9)`,
		},
		{
			name:  "no planet in that orbit",
			order: "ship 985070 move to orbit 9",
			want:  "system current has no planet in orbit 9",
		},
		{
			name:  "no such system in this stellium",
			order: "ship 985070 probe system E orbit 1",
			want:  "current stellium has no system E",
		},
		{
			name:  "more probes than the sensors launch",
			order: "ship 985070 probe orbit 4 6 4 6 4",
			want:  "ship 985070 has only 4 probes this turn",
		},
		{
			name:  "a ship is not a colony",
			order: "colony 985070 probe orbit 4",
			want:  "entity 985070 is a ship, not a colony",
		},
		{
			name:  "another faction's ship",
			order: "ship 560344 move to orbit 6",
			want:  "ship 560344 does not belong to faction 1",
		},
		{
			name:  "the stellium orbit belongs to no system",
			order: "ship 985070 move to system A orbit 11",
			want:  "orbit 11 is the stellium orbit and belongs to no system",
		},
		{
			name:  "a colony cannot move",
			order: "ship 503683 move to orbit 6",
			want:  "entity 503683 is a COPN, not a ship",
		},
		{
			// Written as a colony, the line never reaches MOVE's parser: the
			// subject decides which orders a line may name.
			name:  "an order given to a subject that cannot be given it",
			order: "colony 503683 move to orbit 6",
			want:  "MOVE is given to a ship, not to a colony",
		},
		{
			name:  "a name longer than a name may be",
			order: `ship 985070 name "twenty five characters!!!"`,
			want:  `a name may be 24 characters and "twenty five characters!!!" is 25`,
		},
		{
			name:  "a name that begins with a space",
			order: `ship 985070 name " Bellerophon"`,
			want:  "a name may not begin or end with a space",
		},
		{
			name:  "a name with a gap in it",
			order: `ship 985070 name "Bell  erophon"`,
			want:  "a name may not hold two spaces in a row",
		},
		{
			name:  "a name with nothing in it",
			order: `ship 985070 name ""`,
			want:  "a name cannot be empty",
		},
		{
			name:  "naming a stellium that is not there",
			order: `we name (9,9,9) "Nowhere"`,
			want:  `game "GOLD-01" has no stellium at (9,9,9)`,
		},
		{
			name:  "naming a system the stellium does not hold",
			order: `we name (0,0,0) system E "Elsewhere"`,
			want:  "the stellium at (0,0,0) has no system E",
		},
		{
			name:  "naming a planet that is not in that orbit",
			order: `we name (0,0,0) system A orbit 9 "Empty"`,
			want:  "system A has no planet in orbit 9",
		},
		{
			name:  "assembling a resource, which is measured rather than made",
			order: "colony 802784 assemble 100 GOLD",
			want:  "GOLD is a resource; it is measured rather than made, and is never assembled",
		},
		{
			name:  "assembling people",
			order: "colony 802784 assemble 100 SOL",
			want:  "SOL is population; people are carried and fed, not assembled",
		},
		{
			name:  "assembling a cadre, which is an assignment rather than a thing",
			order: "colony 802784 assemble 5 CWKR",
			want:  "CWKR is a cadre, an assignment of people rather than a unit, and is never assembled",
		},
		{
			name:  "one order naming the same unit twice",
			order: "colony 802784 unassemble 10 STRC-10, 5 STRC-10",
			want:  "STRC-10 is named twice",
		},
		{
			// A quantity over 999 separates every three digits with a comma.
			name:  "a quantity written without its separators",
			order: "colony 802784 assemble 1000 SNSR-1",
			want:  "a quantity over 999 separates every three digits with a comma, as in 1,000",
		},
		{
			// A faction may hand things to its own entities and to the
			// derelicts nobody holds, and to nobody else. Refusing another
			// faction's is in internal/orders, which has a second player to
			// refuse; this scenario's only other faction is the uncontrolled
			// one, and handing things to that is allowed.
			name:  "transferring to itself",
			order: "colony 802784 transfer 100 GOLD to colony 802784",
			want:  "colony 802784 cannot transfer to itself",
		},
		{
			name:  "transferring a cadre rather than the people in it",
			order: "colony 802784 transfer 5 CWKR to colony 503683",
			want:  "CWKR is a cadre, an assignment of people rather than a thing to carry",
		},
		{
			name:  "naming another faction's ship",
			order: `ship 560344 name "Easy Target"`,
			want:  "ship 560344 does not belong to faction 1",
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
