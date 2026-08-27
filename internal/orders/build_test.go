// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/testdb"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// A create is the one order that may run over several lines, so it is
// terminated by `end` and line breaks inside it mean nothing.
func TestCreateReadsOneOrderOutOfSeveralLines(t *testing.T) {
	spread := `colony 50 create ship
  using 60 STRC-8,
        2 HDRV-1
        , 1 SNSR-1
  transfering 25 FOOD, 5 SKW
  with 500 CWKR
end
`
	one := "colony 50 create ship using 60 STRC-8, 2 HDRV-1, 1 SNSR-1 " +
		"transfering 25 FOOD, 5 SKW with 500 CWKR end\n"
	spreadOrder, err := parseOne(strings.TrimSuffix(spread, "\n"))
	if err != nil {
		t.Fatalf("the order spread over lines was refused: %v", err)
	}
	oneOrder, err := parseOne(strings.TrimSuffix(one, "\n"))
	if err != nil {
		t.Fatalf("the order on one line was refused: %v", err)
	}
	if spreadOrder.Params.Input() != oneOrder.Params.Input() {
		t.Errorf("spread = %q; one line = %q; want them the same",
			spreadOrder.Params.Input(), oneOrder.Params.Input())
	}
	// The order is reported against the line it began on, not the line it
	// ended on.
	if spreadOrder.Line != 4 {
		t.Errorf("line = %d; want 4, where the order begins", spreadOrder.Line)
	}
}

// An order that runs to a terminator has to find one.
func TestCreateWithNoEndIsRefused(t *testing.T) {
	_, err := Parse(strings.NewReader(header + "colony 50 create ship using 60 STRC-8\n"))
	if err == nil {
		t.Fatal("an unterminated create was accepted; want it refused")
	}
	if !strings.Contains(err.Error(), "runs until `end`, and the file ended first") {
		t.Errorf("error = %q; want it to say the file ended first", err)
	}
}

// A missing `end` stops at the next order rather than eating the rest of the
// file. A player who left one out has one mistake, and should be told about it
// and about everything else wrong in the same pass -- not fix it and find three
// more waiting.
func TestAMissingEndDoesNotSwallowTheRestOfTheFile(t *testing.T) {
	body := `colony 50 create ship
  using 60 STRC-8
  transfering 25 FOOD
  with 5 CWKR
ship 51 fly to orbit 4
colony 50 assemble 6 SNSR-99
`
	_, err := Parse(strings.NewReader(header + body))
	if err == nil {
		t.Fatal("the file was accepted; want it refused")
	}
	// Three problems, and the create's names the line that gave it away.
	for _, want := range []string{
		"line 4: CREATE runs until `end`, and line 8 begins another order",
		`line 8: unknown order "fly"`,
		`line 9: invalid unit tag "SNSR-99"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q;\n  want it to hold %q", err, want)
		}
	}
}

// A create whose id the player mistyped is still a create, so it is still read
// to its `end`. The file scanner has to decide that a line runs to a terminator
// before anything has checked what the line says, and an id it cannot read is
// no reason to stop: reading the order whole is what leaves the player with the
// one thing they got wrong instead of that plus a complaint about every line of
// the order's body.
func TestACreateWithAMistypedIdIsStillReadWhole(t *testing.T) {
	body := `colony fifty create ship
  using 60 STRC-8
  transfering 25 FOOD
  with 5 CWKR
end
`
	_, err := Parse(strings.NewReader(header + body))
	if err == nil {
		t.Fatal("a create with a mistyped id was accepted; want it refused")
	}
	want := `line 4: invalid colony id: "fifty" is not a number`
	if err.Error() != want {
		t.Errorf("error = %q;\n  want exactly %q", err, want)
	}
}

// A blank line or a comment inside a multi-line order is part of it, not the
// end of it.
func TestBlankLinesAndCommentsInsideACreateAreNotTheEndOfIt(t *testing.T) {
	body := `colony 50 create ship
  using 60 STRC-8

  # the food is for the crew
  transfering 25 FOOD
  with 5 CWKR
end
`
	submission, err := Parse(strings.NewReader(header + body))
	if err != nil {
		t.Fatalf("a create with a blank line and a comment in it was refused: %v", err)
	}
	if len(submission.Orders) != 1 {
		t.Fatalf("orders = %d; want 1", len(submission.Orders))
	}
	want := "ship using 60 STRC-8 transfering 25 FOOD with 5 CWKR"
	if got := submission.Orders[0].Params.Input(); got != want {
		t.Errorf("read back as %q; want %q", got, want)
	}
}

// The order after a multi-line one is still numbered by its own physical line.
func TestALineAfterAMultiLineOrderKeepsItsOwnNumber(t *testing.T) {
	body := `colony 50 create ship
  using 60 STRC-8
  transfering 25 FOOD
  with 500 CWKR
end
colony 50 assemble 20 SNSR-1
`
	submission, err := Parse(strings.NewReader(header + body))
	if err != nil {
		t.Fatal(err)
	}
	if len(submission.Orders) != 2 {
		t.Fatalf("orders = %d; want 2", len(submission.Orders))
	}
	if got := submission.Orders[1].Line; got != 9 {
		t.Errorf("the assemble is on line %d; want 9", got)
	}
}

func TestCreateRefusesWhatCannotChange(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		// A colony is built at a planet, so the builder has to be at one.
		{"ship 55 create enclosed colony using 60 STRC-8 transfering 25 FOOD with 5 CWKR end",
			"a colony is created at a planet"},
		// An open-air colony breathes the air outside.
		{"colony 56 create open-air colony using 60 STRC-8 transfering 25 FOOD with 5 CWKR end",
			"habitability is above 0"},
		// A using line names what the entity is made of.
		{"colony 50 create ship using 100 GOLD transfering 25 FOOD with 5 CWKR end",
			"GOLD is not built into an entity"},
		{"colony 50 create ship using 60 STRC-8 transfering 5 CWKR with 5 CWKR end",
			"CWKR is a cadre"},
		{"colony 50 create ship using 60 STRC-8, 5 STRC-8 transfering 25 FOOD with 5 CWKR end",
			"STRC-8 is named twice in the using list"},
	} {
		_, err := Check(context.Background(), openBuildOrderDatabase(t), strings.NewReader(header+item.line+"\n"))
		if err == nil {
			t.Errorf("%s was accepted; want the file refused", item.line)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.line, err, item.want)
		}
	}
}

// A create succeeds the moment it is given: the entity exists, unfinished, and
// everything after that is rate rather than failure.
func TestCreatePutsAnUnfinishedEntityOnTheBoardAtOnce(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	result := apply(t, conn, "colony 50 create orbital colony as trade-station"+
		" using 60 STRC-8 transfering 25 FOOD with 5 CWKR end\n")
	if result.Orders != 1 {
		t.Fatalf("orders = %d; want 1", result.Orders)
	}
	id := onlyBuild(t, conn)
	if got := buildScalar(t, conn, "SELECT unit FROM entity WHERE id = ?;", id); got != "CORB" {
		t.Errorf("kind = %v; want CORB", got)
	}
	// An orbital colony sits in ring 1, and takes the technology level of the
	// entity that created it.
	if got := buildScalar(t, conn, "SELECT planet_ring FROM entity WHERE id = ?;", id); got != int64(1) {
		t.Errorf("ring = %v; want 1", got)
	}
	if got := buildScalar(t, conn, "SELECT tech_level FROM entity WHERE id = ?;", id); got != int64(1) {
		t.Errorf("tech level = %v; want 1, the builder's", got)
	}
	if got := buildScalar(t, conn, "SELECT trade_station FROM entity WHERE id = ?;", id); got != int64(1) {
		t.Errorf("trade station = %v; want 1", got)
	}
	if got := buildScalar(t, conn, "SELECT faction_id FROM entity WHERE id = ?;", id); got != int64(1) {
		t.Errorf("faction = %v; want 1, the faction building it", got)
	}
}

// An unfinished entity exists and is visible, but it is not yet a thing that
// acts.
func TestAnUnfinishedEntityCanBeGivenNoOrders(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	apply(t, conn, "colony 50 create orbital colony using 60 STRC-8 transfering 25 FOOD with 5 CWKR end\n")
	id := onlyBuild(t, conn)
	for _, item := range []struct{ line, want string }{
		{"colony ID assemble 10 STRC-8", "under construction and can be given no orders"},
		{"colony 50 transfer 100 GOLD to colony ID", "under construction; only the build that began it"},
	} {
		line := strings.ReplaceAll(item.line, "ID", formatQuantity(id))
		_, err := Check(context.Background(), conn, strings.NewReader(header+line+"\n"))
		if err == nil || !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: err = %v; want it to mention %q", line, err, item.want)
		}
	}
}

// The first turn of a build: it claims at stage 5, delivers on the builder's
// transports at stage 9, and its workers assemble what arrived at stage 10.
// Only the structural lines are eligible until the structure is done, because
// everything else needs enclosed space and structure is what makes some.
func TestABuildClaimsDeliversAndAssemblesInOneTurn(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	haulage(t, conn)
	apply(t, conn, "colony 50 create orbital colony using 20 STRC-10, 2 SNSR-1"+
		" transfering 25 FOOD with 5 CWKR end\n")
	id := onlyBuild(t, conn)
	// 20 STRC-10 mass 20 MU each, and five CWKR do 2,500 MU of work between
	// them, so the whole structure is claimed, carried, and assembled in the
	// turn the build was ordered.
	if got := storedQuantity(t, conn, id, "component", "STRC", 10); got != 20 {
		t.Errorf("assembled STRC-10 = %d; want all 20", got)
	}
	if got := storedEnclosedVolume(t, conn, id); got != 20*100 {
		t.Errorf("enclosed volume = %d; want %d", got, 20*100)
	}
	// The sensors and the food were not eligible: the structure was not done
	// when the claim was made.
	if got := buildScalar(t, conn, `SELECT sum(claimed + delivered + completed)
		FROM construction_item WHERE entity_id = ? AND unit <> 'STRC';`, id); got != int64(0) {
		t.Errorf("non-structural progress = %v; want none in the first turn", got)
	}
	if got := buildScalar(t, conn, "SELECT structure_complete FROM under_construction WHERE entity_id = ?;", id); got != int64(1) {
		t.Errorf("structure_complete = %v; want 1", got)
	}
	// The workers commute. They rode out, worked the shift, and came home, so
	// no population row moved.
	if got := storedPopulation(t, conn, 50, "SKW"); got != 50 {
		t.Errorf("the builder's SKW = %d; want all 50: the workers sleep at home", got)
	}
	if got := storedPopulation(t, conn, id, "SKW"); got != 0 {
		t.Errorf("SKW at the site = %d; want none", got)
	}
}

// The structure gates everything else, so the rest of a build is claimed on the
// turn after the structure finishes. It is not waiting on anything: the lines
// were skipped rather than blocked, and they take priority back the moment they
// are eligible again.
func TestTheRestOfABuildFollowsTheStructure(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	haulage(t, conn)
	apply(t, conn, "colony 50 create orbital colony using 20 STRC-10, 2 SNSR-1"+
		" transfering 25 FOOD with 5 CWKR end\n")
	id := onlyBuild(t, conn)
	runTurn(t, conn)
	if got := storedQuantity(t, conn, id, "component", "SNSR", 1); got != 2 {
		t.Errorf("assembled SNSR-1 = %d; want 2, the turn after the structure finished", got)
	}
	// A transfering line is finished the moment it arrives: cargo is where it
	// was going.
	if got := storedQuantity(t, conn, id, "cargo", "FOOD", 0); got != 25 {
		t.Errorf("FOOD in cargo = %d; want 25", got)
	}
	var open int64
	if err := scanOne(conn, "SELECT count(*) FROM under_construction;", &open); err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Errorf("builds still open = %d; want none: every line finished", open)
	}
}

// A build never fails for want -- it only slows. A builder with nothing to
// carry the load delivers nothing, and that is a correct outcome to see rather
// than a defect to guard against.
func TestABuildWithNoTransportsMakesNoProgressAndDoesNotFail(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	testdb.Exec(t, conn, "DELETE FROM inventory WHERE entity_id = 50 AND unit = 'TRAN';")
	result := apply(t, conn, "colony 50 create orbital colony using 20 STRC-10"+
		" transfering 25 FOOD with 5 CWKR end\n")
	if result.Orders != 1 {
		t.Fatalf("orders = %d; want the create kept", result.Orders)
	}
	id := onlyBuild(t, conn)
	if got := buildScalar(t, conn, `SELECT sum(delivered + completed)
		FROM construction_item WHERE entity_id = ?;`, id); got != int64(0) {
		t.Errorf("progress = %v; want none: nothing could be carried", got)
	}
	// A claim lives for one turn and is never banked, so nothing is owed to
	// next turn either.
	if got := buildScalar(t, conn, "SELECT sum(claimed) FROM construction_item WHERE entity_id = ?;", id); got != int64(0) {
		t.Errorf("claims = %v; want none held over", got)
	}
}

// Unsupported people never leave the entity handing them over: only assembled
// LFSU supports anyone, and this colony has none.
func TestABuildWillNotDeliverPeopleItCannotKeepAlive(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	haulage(t, conn)
	apply(t, conn, "colony 50 create enclosed colony using 1 STRC-10"+
		" transfering 5 USK with 5 CWKR end\n")
	id := onlyBuild(t, conn)
	// The first turn finishes the structure, so the population line is
	// eligible from the second -- and still does not move.
	runTurn(t, conn)
	if got := storedPopulation(t, conn, id, "USK"); got != 0 {
		t.Errorf("USK aboard = %d; want none: nothing supports them", got)
	}
	if got := storedPopulation(t, conn, 50, "USK"); got != 50 {
		t.Errorf("USK at the builder = %d; want all 50 still there", got)
	}
}

// Where two builds of one entity compete, the older is served first and each
// takes up to its own cap. Seniority is the entity id, which rises
// monotonically and is never reused.
func TestTheOlderBuildIsServedFirst(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	// Only 20 STRC-10 in cargo between them, and each build wants all 20.
	apply(t, conn, `colony 50 create orbital colony using 20 STRC-10 transfering 25 FOOD with 5 CWKR end
colony 50 create enclosed colony using 20 STRC-10 transfering 25 FOOD with 5 CWKR end
`)
	first, second := twoBuilds(t, conn)
	if got := buildScalar(t, conn, `SELECT delivered + completed FROM construction_item
		WHERE entity_id = ? AND unit = 'STRC';`, first); got != int64(20) {
		t.Errorf("the senior build got %v STRC-10; want all 20", got)
	}
	if got := buildScalar(t, conn, `SELECT claimed + delivered + completed FROM construction_item
		WHERE entity_id = ? AND unit = 'STRC';`, second); got != int64(0) {
		t.Errorf("the junior build got %v STRC-10; want none: the senior one took the stock", got)
	}
}

// Explicitly ordered work outranks a standing commitment: a transfer order is
// served before a build's claim, because the sweep runs after the phase's
// orders.
func TestATransferOrderIsServedBeforeABuildsDelivery(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	// The twenty TRAN-1 carry 400 MU a turn. A transfer of 400 GOLD uses all
	// of it, so the build's structure waits for next turn.
	apply(t, conn, `colony 50 create orbital colony using 20 STRC-10 transfering 25 FOOD with 5 CWKR end
colony 50 transfer 400 GOLD to ship 51
`)
	id := onlyBuild(t, conn)
	if got := storedQuantity(t, conn, 51, "cargo", "GOLD", 0); got != 400 {
		t.Errorf("the transfer moved %d GOLD; want all 400: it was asked for by name", got)
	}
	if got := buildScalar(t, conn, `SELECT sum(delivered + completed)
		FROM construction_item WHERE entity_id = ?;`, id); got != int64(0) {
		t.Errorf("the build delivered %v; want none: the transfer took the hulls", got)
	}
}

// Colony 50 is a colony with transports, stock in cargo to build from, and a
// small cadre. Ship 51 is beside it; 55 is in the stellium orbit, where no
// colony can be built; 56 is at a planet nobody can breathe on.
func openBuildOrderDatabase(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn := openInventoryOrderDatabase(t)
	testdb.Exec(t, conn, `
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES (32, 20, 8, 'asteroid', 0);
		INSERT INTO entity (id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
			faction_id, enclosed_volume, mass) VALUES
			(55, 'SHIP', 1, 10, NULL, NULL, NULL, 1, 50000, 10000),
			(56, 'COPN', 1, 10, 20, 32, 0, 1, 50000, 10000);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(50, 'cargo', 'STRC', 10, 20),
			(50, 'cargo', 'SNSR', 1, 20),
			(50, 'cargo', 'FARM', 1, 20),
			(50, 'cargo', 'FOOD', 0, 100);
	`)
	return conn
}

// haulage gives the builder transports enough that the load and the workers
// both fit, which is what a build needs to move at all: twenty TRAN-1 carry 400
// MU a turn, and 20 STRC-10 are exactly that with nothing left for a commute.
func haulage(t *testing.T, conn *sqlite.Conn) {
	t.Helper()
	testdb.Exec(t, conn, `UPDATE inventory SET quantity = 200
		WHERE entity_id = 50 AND section = 'operational' AND unit = 'TRAN';`)
}

// runTurn runs the phases against the database with nobody ordering anything,
// which is what a build's next turn looks like: the create order departed and
// succeeded, and the three sweeps carry on without it.
func runTurn(t *testing.T, conn *sqlite.Conn) {
	t.Helper()
	apply(t, conn, "")
}

// onlyBuild is the entity the file being tested began building.
func onlyBuild(t *testing.T, conn *sqlite.Conn) int64 {
	t.Helper()
	var id int64
	if err := scanOne(conn, "SELECT coalesce(min(entity_id), 0) FROM under_construction;", &id); err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("no build was recorded")
	}
	return id
}

func twoBuilds(t *testing.T, conn *sqlite.Conn) (first, second int64) {
	t.Helper()
	var ids []int64
	if err := forEachRow(conn, "SELECT entity_id FROM under_construction ORDER BY entity_id;",
		func(stmt *sqlite.Stmt) { ids = append(ids, stmt.ColumnInt64(0)) }); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("builds = %d; want 2", len(ids))
	}
	return ids[0], ids[1]
}

// buildScalar reads one value out of the database, as whatever type SQLite
// holds it in, so a test can assert on a column without a helper per column.
func buildScalar(t *testing.T, conn *sqlite.Conn, query string, args ...any) any {
	t.Helper()
	var value any
	if err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if stmt.ColumnType(0) == sqlite.TypeText {
				value = stmt.ColumnText(0)
			} else {
				value = stmt.ColumnInt64(0)
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return value
}

func scanOne(conn *sqlite.Conn, query string, into *int64) error {
	return sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			*into = stmt.ColumnInt64(0)
			return nil
		},
	})
}

func forEachRow(conn *sqlite.Conn, query string, row func(*sqlite.Stmt)) error {
	return sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			row(stmt)
			return nil
		},
	})
}

// An entity cannot hold more than it encloses, and that bounds what may be
// delivered to it as well as what it may assemble. It is the other half of
// structure-first: structure delivered to an unfinished entity consumes no
// enclosed space, and nothing else is exempt.
func TestABuildDeliversOnlyWhatTheNewEntityHasRoomFor(t *testing.T) {
	conn := openBuildOrderDatabase(t)
	haulage(t, conn)
	// One STRC-10 encloses 100 VU and an orbital colony uses a tenth of it, so
	// the new colony has 10 VU to receive things into. A FARM-1 takes 2 VU in
	// cargo, so five of the twenty asked for fit and the rest wait for
	// structure that is never coming.
	apply(t, conn, "colony 50 create orbital colony using 1 STRC-10"+
		" transfering 20 FARM-1 with 5 CWKR end\n")
	id := onlyBuild(t, conn)
	runTurn(t, conn)
	if got := storedQuantity(t, conn, id, "cargo", "FARM", 1); got != 5 {
		t.Errorf("delivered FARM-1 = %d; want 5, which is all the colony encloses room for", got)
	}
	// The build is still open, and is not owed anything: a claim lives for one
	// turn and what could not be carried was released.
	if got := buildScalar(t, conn, "SELECT sum(claimed) FROM construction_item WHERE entity_id = ?;", id); got != int64(0) {
		t.Errorf("claims held over = %v; want none", got)
	}
}
