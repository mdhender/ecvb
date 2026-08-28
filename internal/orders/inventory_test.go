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

// A quantity over 999 separates every three digits with a comma, and the same
// comma separates the items of a list. Nothing is ambiguous, because a
// quantity is always followed by a unit code and never by another quantity.
func TestQuantitiesReadTheirOwnSeparatorsApartFromTheListsOwn(t *testing.T) {
	for _, item := range []struct {
		line string
		want string
	}{
		{"colony 100050 assemble 6 SNSR-1", "6 SNSR-1"},
		{"colony 100050 assemble 6,000 SNSR-1", "6,000 SNSR-1"},
		{"colony 100050 assemble 1,234,567 SNSR-1", "1,234,567 SNSR-1"},
		{"colony 100050 assemble 1,000 SNSR-1, 500 STRC-2", "1,000 SNSR-1, 500 STRC-2"},
		{"colony 100050 assemble 500 SNSR-1, 1,000 STRC-2", "500 SNSR-1, 1,000 STRC-2"},
		{"colony 100050 assemble 5 SNSR-1, 600 STRC-2", "5 SNSR-1, 600 STRC-2"},
		{"colony 100050 assemble 6 snsr-1", "6 SNSR-1"},
	} {
		order, err := parseOne(item.line)
		if err != nil {
			t.Errorf("%s: %v", item.line, err)
			continue
		}
		if got := order.Params.Input(); got != item.want {
			t.Errorf("%s read back as %q; want %q", item.line, got, item.want)
		}
	}
}

func TestQuantitiesRefuseWhatTheGrammarDoesNotAllow(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"colony 100050 assemble 5000 SNSR-1", "a quantity over 999 separates every three digits with a comma, as in 5,000"},
		{"colony 100050 assemble 0 SNSR-1", "a quantity is greater than zero"},
		{"colony 100050 assemble 012 SNSR-1", "carries no leading zero"},
		{"colony 100050 assemble six SNSR-1", `invalid quantity "six"`},
		{"colony 100050 assemble 6 SNSR-99", `invalid unit tag "SNSR-99"`},
		{"colony 100050 assemble 6 snsr!", `invalid unit code "SNSR!"`},
		// A line that ran out before its order did is told what was missing,
		// and shown that order's forms after it.
		{"colony 100050 assemble 6", "expected a unit code, found the end of the order"},
		{"colony 100050 assemble 6", "  colony COLONY-ID assemble QUANTITY UNIT, QUANTITY UNIT, ..."},
	} {
		_, err := parseOne(item.line)
		if err == nil {
			t.Errorf("%s parsed; want an error", item.line)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.line, err, item.want)
		}
	}
}

// Which section a unit works in is a property of the unit code, so it is
// settled at Bind and a file that names something never assembled is refused
// whole.
func TestSubmitRefusesAssemblingWhatIsNeverAssembled(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"colony 100050 assemble 100 GOLD", "GOLD is a resource"},
		{"colony 100050 assemble 100 SOL", "SOL is population"},
		{"colony 100050 assemble 100 CWKR", "CWKR is a cadre"},
		{"colony 100050 assemble 100 SNSR-1, 5 SNSR-1", "SNSR-1 is named twice"},
	} {
		_, err := Check(context.Background(), openInventoryOrderDatabase(t), strings.NewReader(header+item.line+"\n"))
		if err == nil {
			t.Errorf("%s was accepted; want the file refused", item.line)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.line, err, item.want)
		}
	}
}

// Nothing in the order says where a unit goes; the unit code does.
func TestAssembleSendsTheSixToComponentsAndTheRestToOperational(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 assemble 20 SNSR-1, 10 FARM-1\n")
	if got := storedQuantity(t, conn, 50, "component", "SNSR", 1); got != 20 {
		t.Errorf("component SNSR-1 = %d; want 20", got)
	}
	if got := storedQuantity(t, conn, 50, "operational", "FARM", 1); got != 10 {
		t.Errorf("operational FARM-1 = %d; want 10", got)
	}
	if got := storedQuantity(t, conn, 50, "unassembled", "SNSR", 1); got != 80 {
		t.Errorf("unassembled SNSR-1 = %d; want 80 left", got)
	}
}

// A transport sets its load down in cargo and nowhere else, and stage 10
// assembles what stage 9 delivered, so an assemble looks in cargo as well --
// after unassembled inventory, which is the section units are kept in to be
// worked on.
func TestAssembleDrawsFromUnassembledFirstAndThenFromCargo(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// 30 sensors in cargo beside the 100 unassembled. Asking for 110 empties
	// the unassembled hundred and takes ten out of cargo.
	testdb.Exec(t, conn, `
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
			VALUES (50, 'cargo', 'SNSR', 1, 30);
		UPDATE entity_cadre SET quantity = 20 WHERE entity_id = 50;`)
	apply(t, conn, "colony 100050 assemble 110 SNSR-1\n")
	if got := storedQuantity(t, conn, 50, "component", "SNSR", 1); got != 110 {
		t.Errorf("component SNSR-1 = %d; want 110", got)
	}
	if got := storedQuantity(t, conn, 50, "unassembled", "SNSR", 1); got != 0 {
		t.Errorf("unassembled SNSR-1 = %d; want none: it is drawn on first", got)
	}
	if got := storedQuantity(t, conn, 50, "cargo", "SNSR", 1); got != 20 {
		t.Errorf("cargo SNSR-1 = %d; want 20: only the ten it still needed came out of cargo", got)
	}
}

// The two sections are one pool for the cadre, which does not care where a
// unit was kept.
func TestTheCadreRationsAnAssembleAcrossBothItsSections(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	testdb.Exec(t, conn, `
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
			VALUES (50, 'cargo', 'SNSR', 1, 30);`)
	// Five workers do 2,500 MU and a sensor masses 40 MU, so 62 are assembled
	// however they are split between the sections, and cargo is untouched.
	result := check(t, conn, "colony 100050 assemble 110 SNSR-1\n")
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0].Message, "assembled 62 of 110 SNSR-1") {
		t.Fatalf("warnings = %+v; want the order stopped at 62", result.Warnings)
	}
}

// A shortage is a rate rather than a failure: the order does what the cadre
// paid for, says how much that was, and is not refused.
func TestAssembleDoesWhatTheCadrePaysForAndSaysSo(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// Five workers do 2,500 MU a turn, and a SNSR-1 masses 40 MU, so 62 of the
	// 100 are assembled and the sixty-third is not.
	result := check(t, conn, "colony 100050 assemble 100 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	want := "colony 100050 assembled 62 of 100 SNSR-1; its 5 CWKR had 2,500 MU of work left this turn"
	if result.Warnings[0].Message != want {
		t.Errorf("warning = %q; want %q", result.Warnings[0].Message, want)
	}
	// A shortage is a rate, not a failure: the order still succeeds.
	if got := apply(t, conn, "colony 100050 assemble 100 SNSR-1\n"); got.Orders != 1 {
		t.Fatalf("orders = %d; want the order kept", got.Orders)
	}
	if got := storedQuantity(t, conn, 50, "component", "SNSR", 1); got != 62 {
		t.Errorf("component SNSR-1 = %d; want 62", got)
	}
}

// Work of the same kind is pooled across an entity, so the workers it needs
// are reckoned from one total rather than order by order.
func TestTheWorkPoolIsDrawnDownAcrossEveryOrderOfTheTurn(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	result := check(t, conn, "colony 100050 assemble 30 SNSR-1\ncolony 100050 assemble 30 SNSR-1\ncolony 100050 assemble 30 SNSR-1\n")
	// 1,200 MU, then 1,200 MU, and the 2,500 MU pool has 100 MU left, which is
	// two more sensors.
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one, on the third order", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "assembled 2 of 30 SNSR-1") {
		t.Errorf("warning = %q; want the third order stopped at 2", result.Warnings[0].Message)
	}
}

// Assembly and unassembly draw on the same cadre and never pool with each
// other: one MU of unassembly costs a whole worker.
func TestUnassemblyTakesAWholeWorkerFromAssembly(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// One STRC-10 unassembled is 20 MU of work and one whole worker, leaving
	// four for assembly: 2,000 MU, which is 50 sensors and not 62.
	result := check(t, conn, "colony 100050 unassemble 1 STRC-10\ncolony 100050 assemble 100 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "assembled 50 of 100 SNSR-1") {
		t.Errorf("warning = %q; want the assembly held to 50", result.Warnings[0].Message)
	}
}

// An entity cannot hold more than it encloses, and doing less of the order
// would not fix it, so this is the one thing that fails an assemble or an
// unassemble outright.
func TestUnassemblingStructureFailsWhenItWouldLeaveTheEntityOverpacked(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	result := check(t, conn, "colony 100050 unassemble 100 STRC-10\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "colony 100050 would hold") {
		t.Errorf("warning = %q; want it to say the colony would be overpacked", result.Warnings[0].Message)
	}
	apply(t, conn, "colony 100050 unassemble 100 STRC-10\n")
	if got := storedQuantity(t, conn, 50, "component", "STRC", 10); got != 100 {
		t.Errorf("component STRC-10 = %d; want all 100 still there: a failed order moves nothing", got)
	}
}

// Unassembly is lossless, and `and stow` puts the units down in cargo, which
// is where a transfer needs them.
func TestUnassembleIsLosslessAndStowsToCargoWhenAsked(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 unassemble and stow 10 STRC-10\n")
	if got := storedQuantity(t, conn, 50, "cargo", "STRC", 10); got != 10 {
		t.Errorf("cargo STRC-10 = %d; want all 10, unassembly being lossless", got)
	}
	if got := storedQuantity(t, conn, 50, "unassembled", "STRC", 10); got != 0 {
		t.Errorf("unassembled STRC-10 = %d; want none: `and stow` puts them in cargo", got)
	}
	if got := storedEnclosedVolume(t, conn, 50); got != 90*100 {
		t.Errorf("enclosed volume = %d; want %d, the ten units having stopped enclosing", got, 90*100)
	}
}

func TestTransferHandsUnitsOverAndChargesItsTransportsFuel(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 transfer 300 GOLD to ship 100051\n")
	if got := storedQuantity(t, conn, 51, "cargo", "GOLD", 0); got != 300 {
		t.Errorf("recipient GOLD = %d; want 300", got)
	}
	if got := storedQuantity(t, conn, 50, "cargo", "GOLD", 0); got != 700 {
		t.Errorf("sender GOLD = %d; want 700 left", got)
	}
	// 300 MU needs fifteen TRAN-1, and fifteen hulls burn ceil(15/10) = 2 FUEL.
	if got := storedQuantity(t, conn, 50, "cargo", "FUEL", 0); got != 1998 {
		t.Errorf("sender FUEL = %d; want 1,998, two having gone on the transports", got)
	}
}

// The fuel is reckoned over every transport the entity used in the turn at
// once, so a second transfer that shares the round trip pays only what it adds.
func TestTransferFuelIsReckonedOverTheWholeTurn(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 transfer 300 GOLD to ship 100051\ncolony 100050 transfer 100 GOLD to ship 100051\n")
	if got := storedQuantity(t, conn, 51, "cargo", "GOLD", 0); got != 400 {
		t.Errorf("recipient GOLD = %d; want all 400 across the two orders", got)
	}
	// Fifteen hulls cost 2 FUEL and five more cost nothing: twenty reckoned at
	// once still round to 2. Charged one order at a time it would have been 3.
	if got := storedQuantity(t, conn, 50, "cargo", "FUEL", 0); got != 1998 {
		t.Errorf("sender FUEL = %d; want 1,998", got)
	}
}

// A shortage of transports fills the order partway rather than failing it.
func TestTransferIsFilledPartwayWhenTheTransportsRunOut(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	result := check(t, conn, "colony 100050 transfer 1,000 GOLD to ship 100051\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	want := "colony 100050 transferred 400 of 1,000 GOLD; it had 400 MU and 1,200 VU of transport left this turn"
	if result.Warnings[0].Message != want {
		t.Errorf("warning = %q; want %q", result.Warnings[0].Message, want)
	}
}

func TestTransferMovesPopulation(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 transfer 20 SOL to ship 100051\n")
	if got := storedPopulation(t, conn, 51, "SOL"); got != 20 {
		t.Errorf("recipient SOL = %d; want 20", got)
	}
	if got := storedPopulation(t, conn, 50, "SOL"); got != 80 {
		t.Errorf("sender SOL = %d; want 80 left", got)
	}
}

// A transfer fails if the two entities are not at the same place when the
// order runs. There is no partial answer to being in the wrong place.
func TestTransferFailsWhenTheTwoAreNotAtTheSamePlace(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	result := check(t, conn, "colony 100050 transfer 100 GOLD to ship 100052\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "are not at the same place") {
		t.Errorf("warning = %q; want it to say they are not co-located", result.Warnings[0].Message)
	}
}

// A faction may hand things to its own entities and to the derelicts nobody
// holds, but not to another faction's.
func TestTransferReachesOwnEntitiesAndDerelictsAndNoOneElse(t *testing.T) {
	if _, err := Check(context.Background(), openInventoryOrderDatabase(t),
		strings.NewReader(header+"colony 100050 transfer 100 GOLD to ship 100053\n")); err != nil {
		t.Errorf("transfer to an uncontrolled entity was refused: %v", err)
	}
	_, err := Check(context.Background(), openInventoryOrderDatabase(t),
		strings.NewReader(header+"colony 100050 transfer 100 GOLD to ship 100054\n"))
	if err == nil || !strings.Contains(err.Error(), "belongs to another faction") {
		t.Errorf("transfer to another faction's ship: err = %v; want it refused", err)
	}
	_, err = Check(context.Background(), openInventoryOrderDatabase(t),
		strings.NewReader(header+"colony 100050 transfer 100 GOLD to colony 100050\n"))
	if err == nil || !strings.Contains(err.Error(), "cannot transfer to itself") {
		t.Errorf("transfer to itself: err = %v; want it refused", err)
	}
}

// Assembling a sensor changes what the entity can do, but a later phase has to
// be the one that uses it: assembly resolves before probes.
func TestAssembledSensorsAreReadyForTheSameTurnsProbes(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// Colony 100050 has no assembled sensors to start with, so the probe only
	// binds because the assemble in the same file ran first.
	if _, err := Check(context.Background(), conn,
		strings.NewReader(header+"colony 100050 probe orbit 4\n")); err == nil {
		t.Fatal("a probe bound with no assembled SNSR; want it refused")
	}
	if _, err := Check(context.Background(), conn,
		strings.NewReader(header+"colony 100050 probe orbit 4\ncolony 100050 assemble 20 SNSR-1\n")); err != nil {
		t.Errorf("a probe after an assemble in the same file was refused: %v", err)
	}
}

// Check and Submit measure a file by doing the turn and then putting the
// database back, so the inventory a turn moved has to go back too: what was
// assembled, what was handed to somebody else, the mass and the enclosed
// volume that followed it, and the population.
func TestCheckPutsTheInventoryBackTheWayItFoundIt(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	before := worldSnapshot(t, conn)
	body := `colony 100050 unassemble and stow 10 STRC-10
colony 100050 transfer 300 GOLD, 20 SOL to ship 100051
colony 100050 assemble 20 SNSR-1
`
	if _, err := Check(context.Background(), conn, strings.NewReader(header+body)); err != nil {
		t.Fatal(err)
	}
	if after := worldSnapshot(t, conn); after != before {
		t.Errorf("check changed the world:\n before %s\n  after %s", before, after)
	}
	// The same file run without the rollback has to move something, or the
	// assertion above is measuring nothing.
	moved := openInventoryOrderDatabase(t)
	apply(t, moved, body)
	if after := worldSnapshot(t, moved); after == before {
		t.Error("the file changes nothing even when it is not rolled back; this test proves nothing")
	}
}

const header = `game "TEST" turn 0
id faction 1

`

func parseOne(line string) (Order, error) {
	submission, err := Parse(strings.NewReader(header + line + "\n"))
	if err != nil {
		return Order{}, err
	}
	return submission.Orders[0], nil
}

func check(t *testing.T, conn *sqlite.Conn, body string) Result {
	t.Helper()
	result, err := Check(context.Background(), conn, strings.NewReader(header+body))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// apply runs a file against the database for real -- bind and apply, phase by
// phase -- without the savepoint that Check and Submit always roll back, so
// that a test can look at what the orders actually did. It is the same code
// path either of those takes; only the rollback is missing.
func apply(t *testing.T, conn *sqlite.Conn, body string) Result {
	t.Helper()
	submission, err := Parse(strings.NewReader(header + body))
	if err != nil {
		t.Fatal(err)
	}
	validated, err := simulate(context.Background(), conn, submission)
	if err != nil {
		t.Fatal(err)
	}
	return validated.result
}

// Colony 100050 is an open-air colony with a cadre, transports, stock in every
// section, and room to spare. Ship 100051 is beside it, 52 is at another planet,
// 53 is a derelict, and 54 belongs to somebody else.
func openInventoryOrderDatabase(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
		INSERT INTO users (id, email, role) VALUES
			(1, 'player@example.com', 'non-administrator'),
			(2, 'rival@example.com', 'non-administrator');
		INSERT INTO agent (id, code, description) VALUES (1, 'uncontrolled', 'Uncontrolled');
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 0);
		INSERT INTO faction (id, game_id, number, user_id) VALUES (1, 1, 1, 1), (3, 1, 3, 2);
		INSERT INTO faction (id, game_id, number, agent_id) VALUES (2, 1, 2, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (10, 1, 0, 0, 0);
		INSERT INTO system (id, stellium_id, sequence) VALUES (20, 10, 'A');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES
			(30, 20, 4, 'rocky', 10), (31, 20, 6, 'rocky', 10);
		INSERT INTO entity (id, game_id, number, unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
			faction_id, enclosed_volume, mass) VALUES
			(50, 1, 100050, 'COPN', 1, 10, 20, 30, 0, 1, 10000, 100000),
			(51, 1, 100051, 'SHIP', 1, 10, 20, 30, 64, 1, 50000, 10000),
			(52, 1, 100052, 'SHIP', 1, 10, 20, 31, 64, 1, 50000, 10000),
			(53, 1, 100053, 'SHIP', 1, 10, 20, 30, 64, 2, 50000, 10000),
			(54, 1, 100054, 'SHIP', 1, 10, 20, 30, 64, 3, 50000, 10000);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(50, 'component', 'STRC', 10, 100),
			(50, 'operational', 'TRAN', 1, 20),
			(50, 'unassembled', 'SNSR', 1, 100),
			(50, 'unassembled', 'FARM', 1, 10),
			(50, 'cargo', 'GOLD', 0, 1000),
			(50, 'cargo', 'FUEL', 0, 2000),
			(51, 'component', 'STRC', 10, 500);
		INSERT INTO entity_population (entity_id, class, quantity) VALUES
			(50, 'SKW', 50), (50, 'USK', 50), (50, 'SOL', 100);
		INSERT INTO entity_cadre (entity_id, cadre, quantity) VALUES (50, 'CWKR', 5);
	`)
	return conn
}

func storedQuantity(t *testing.T, conn *sqlite.Conn, entityID int64, section, unit string, techLevel int) int64 {
	t.Helper()
	return inventoryScalar(t, conn, `SELECT coalesce((SELECT quantity FROM inventory
		WHERE entity_id = ? AND section = ? AND unit = ? AND tech_level = ?), 0);`,
		entityID, section, unit, techLevel)
}

func storedPopulation(t *testing.T, conn *sqlite.Conn, entityID int64, class string) int64 {
	t.Helper()
	return inventoryScalar(t, conn, `SELECT coalesce((SELECT quantity FROM entity_population
		WHERE entity_id = ? AND class = ?), 0);`, entityID, class)
}

func storedEnclosedVolume(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	return inventoryScalar(t, conn, "SELECT enclosed_volume FROM entity WHERE id = ?;", entityID)
}

func inventoryScalar(t *testing.T, conn *sqlite.Conn, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := sqlitex.ExecuteTransient(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			value = stmt.ColumnInt64(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return value
}

// STOW and UNSTOW -------------------------------------------------------

// Stowing moves units out of unassembled inventory into cargo, which is where
// a transport picks a load up; unstowing moves them back.
func TestStowAndUnstowMoveUnitsBetweenUnassembledInventoryAndCargo(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 stow 40 SNSR-1\n")
	if got := storedQuantity(t, conn, 50, "cargo", "SNSR", 1); got != 40 {
		t.Errorf("cargo SNSR-1 = %d; want 40", got)
	}
	if got := storedQuantity(t, conn, 50, "unassembled", "SNSR", 1); got != 60 {
		t.Errorf("unassembled SNSR-1 = %d; want 60 left", got)
	}
	apply(t, conn, "colony 100050 unstow 25 SNSR-1\n")
	if got := storedQuantity(t, conn, 50, "cargo", "SNSR", 1); got != 15 {
		t.Errorf("cargo SNSR-1 = %d; want 15 left", got)
	}
	if got := storedQuantity(t, conn, 50, "unassembled", "SNSR", 1); got != 85 {
		t.Errorf("unassembled SNSR-1 = %d; want 85", got)
	}
}

// Neither order takes a unit apart or puts one together, so what may be named
// is wider than what an assemble may name: a resource is freight like anything
// else, and unstowing one is what readies it for the market.
func TestStowReachesTheResourcesAnAssembleNeverCan(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	apply(t, conn, "colony 100050 unstow 400 GOLD\n")
	if got := storedQuantity(t, conn, 50, "unassembled", "GOLD", 0); got != 400 {
		t.Errorf("unassembled GOLD = %d; want 400", got)
	}
	if got := storedQuantity(t, conn, 50, "cargo", "GOLD", 0); got != 600 {
		t.Errorf("cargo GOLD = %d; want 600 left", got)
	}
}

// Only two things are never stowed, and neither is inventory at all.
func TestSubmitRefusesStowingWhatIsNotFreight(t *testing.T) {
	for _, item := range []struct{ line, want string }{
		{"colony 100050 stow 100 SOL", "SOL is population; people are carried rather than stowed"},
		{"colony 100050 stow 100 CWKR", "CWKR is a cadre"},
		{"colony 100050 unstow 100 SOL", "SOL is population; people are carried rather than stowed"},
		{"colony 100050 stow 100 SNSR-1, 5 SNSR-1", "SNSR-1 is named twice"},
	} {
		_, err := Check(context.Background(), openInventoryOrderDatabase(t), strings.NewReader(header+item.line+"\n"))
		if err == nil {
			t.Errorf("%s was accepted; want the file refused", item.line)
			continue
		}
		if !strings.Contains(err.Error(), item.want) {
			t.Errorf("%s: error = %q; want it to mention %q", item.line, err, item.want)
		}
	}
}

// Production labour is the entity's unassigned USK -- a worker already in a
// cadre has been spoken for -- plus t for every assembled AUTO it carries.
func TestStowIsRationedByProductionLabourAndNotByTheCadre(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// The colony holds a hundred sensors and nothing else to stow, so the
	// stock is what stops this one.
	result := check(t, conn, "colony 100050 stow 1,000 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	want := "colony 100050 stowed 100 of 1,000 SNSR-1; it holds no more"
	if result.Warnings[0].Message != want {
		t.Errorf("warning = %q; want %q", result.Warnings[0].Message, want)
	}
	// With a thousand in cargo to draw on, the labour is what stops it. 50 USK
	// less the 5 in the CWKR cadre is 45, which moves 22,500 MU a turn; a
	// SNSR-1 masses 40 MU, so 562 of them come out of cargo and the 563rd does
	// not.
	testdb.Exec(t, conn, `INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
		VALUES (50, 'cargo', 'SNSR', 1, 1000);`)
	result = check(t, conn, "colony 100050 unstow 1,000 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	want = "colony 100050 unstowed 562 of 1,000 SNSR-1; " +
		"its 45 units of production labour had 22,500 MU of freight left this turn"
	if result.Warnings[0].Message != want {
		t.Errorf("warning = %q; want %q", result.Warnings[0].Message, want)
	}
}

// An assembled AUTO stands in for t unskilled workers wherever unskilled work
// is done, and moving freight is unskilled work. It is a pool and never a
// member: nothing is assigned into it and nothing is drafted for it.
func TestAutomationStandsInForUnskilledWorkersWhenItIsAssembled(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	testdb.Exec(t, conn, `INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
		(50, 'cargo', 'SNSR', 1, 1000),
		(50, 'operational', 'AUTO', 2, 10),
		(50, 'unassembled', 'AUTO', 2, 100);`)
	// Ten assembled AUTO-2 add 20 to the 45 unassigned USK: 65 units of labour
	// move 32,500 MU a turn, which is 812 sensors. The hundred unassembled
	// ones add nothing, being freight until somebody puts them to work.
	result := check(t, conn, "colony 100050 unstow 1,000 SNSR-1\n")
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0].Message,
		"unstowed 812 of 1,000 SNSR-1; its 65 units of production labour") {
		t.Fatalf("warnings = %+v; want the order stopped at 812", result.Warnings)
	}
}

// One unit of production labour does one task a turn, so labour that stowed
// cannot also unstow: the two pools round up on their own.
func TestStowingTakesAWholeWorkerFromUnstowing(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	testdb.Exec(t, conn, `INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
		VALUES (50, 'cargo', 'SNSR', 1, 1000);`)
	// One sensor stowed is 40 MU of freight and one whole worker, leaving 44
	// to unstow with: 22,000 MU, which is 550 sensors rather than 562.
	result := check(t, conn, "colony 100050 stow 1 SNSR-1\ncolony 100050 unstow 1,000 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	want := "colony 100050 unstowed 550 of 1,000 SNSR-1; " +
		"its 45 units of production labour had 22,000 MU of freight left this turn"
	if result.Warnings[0].Message != want {
		t.Errorf("warning = %q; want %q", result.Warnings[0].Message, want)
	}
}

// The note says what the workers had left when the order ran, not what a whole
// turn of them is worth. An order that came second is answered by whatever the
// first one left it, and quoting the full rate at a player whose second order
// stopped short of it explains nothing.
func TestTheShortfallNoteSaysWhatWasLeftRatherThanTheTurnsWholeRate(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// Two orders of one pool: the first takes 1,200 MU of the cadre's 2,500
	// and the second is answered by the 1,300 that is left, not by the 2,500
	// the five workers are worth over a turn.
	result := check(t, conn, "colony 100050 assemble 30 SNSR-1\ncolony 100050 assemble 100 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one, on the second order", result.Warnings)
	}
	want := "colony 100050 assembled 32 of 100 SNSR-1; its 5 CWKR had 1,300 MU of work left this turn"
	if result.Warnings[0].Message != want {
		t.Errorf("warning = %q; want %q", result.Warnings[0].Message, want)
	}
}

// A construction worker and production labour are two different pools of
// people, so an assemble and a stow in one turn do not take work from each
// other.
func TestStowingAndAssemblingDrawOnDifferentPeople(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// The cadre still assembles its full 62 sensors even though the stow moved
	// 400 MU of gold in the same turn.
	result := check(t, conn, "colony 100050 unstow 400 GOLD\ncolony 100050 assemble 100 SNSR-1\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one, on the assemble", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "assembled 62 of 100 SNSR-1") {
		t.Errorf("warning = %q; want the assemble untouched by the unstow", result.Warnings[0].Message)
	}
}

// A stow at stage 6b feeds a transfer at stage 9, and an unstow at stage 10a
// takes what the transfer set down back out of cargo -- all in one turn,
// whichever way round the player wrote the lines.
func TestStowFeedsATransferAndUnstowTakesTheLoadBackOutInOneTurn(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// The ship needs people of its own: production labour is the entity's, and
	// nothing rides out with the freight to unload it at the far end.
	testdb.Exec(t, conn, "INSERT INTO entity_population (entity_id, class, quantity) VALUES (51, 'USK', 10);")
	apply(t, conn, `ship 100051 unstow 10 SNSR-1
colony 100050 transfer 10 SNSR-1 to ship 100051
colony 100050 stow 10 SNSR-1
`)
	if got := storedQuantity(t, conn, 51, "unassembled", "SNSR", 1); got != 10 {
		t.Errorf("ship unassembled SNSR-1 = %d; want 10, the unstow having run last", got)
	}
	if got := storedQuantity(t, conn, 51, "cargo", "SNSR", 1); got != 0 {
		t.Errorf("ship cargo SNSR-1 = %d; want none: the unstow emptied it", got)
	}
	if got := storedQuantity(t, conn, 50, "cargo", "SNSR", 1); got != 0 {
		t.Errorf("colony cargo SNSR-1 = %d; want none: the transfer took what the stow put there", got)
	}
	if got := storedQuantity(t, conn, 50, "unassembled", "SNSR", 1); got != 90 {
		t.Errorf("colony unassembled SNSR-1 = %d; want 90", got)
	}
}

// Automation is the one unit that takes more room unassembled than it does in
// cargo, so it is the one unit an unstow can be refused for want of space.
func TestUnstowingAutomationFailsWhenItWouldLeaveTheEntityOverpacked(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	// The colony encloses 10,000 VU and holds 700 of it. An AUTO-10 takes 20
	// VU in cargo and 40 unassembled, so 400 of them sit in cargo comfortably
	// and unstowing them all would need 16,000 VU.
	testdb.Exec(t, conn, `INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
		VALUES (50, 'cargo', 'AUTO', 10, 400);`)
	result := check(t, conn, "colony 100050 unstow 400 AUTO-10\n")
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %+v; want one", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0].Message, "colony 100050 would hold") {
		t.Errorf("warning = %q; want it to say the colony would be overpacked", result.Warnings[0].Message)
	}
	apply(t, conn, "colony 100050 unstow 400 AUTO-10\n")
	if got := storedQuantity(t, conn, 50, "cargo", "AUTO", 10); got != 400 {
		t.Errorf("cargo AUTO-10 = %d; want all 400 still there: a failed order moves nothing", got)
	}
}

// Stowing automation frees room rather than costing it, which is the same rule
// read the other way round.
func TestStowingAutomationFreesTheRoomItTakesUnassembled(t *testing.T) {
	conn := openInventoryOrderDatabase(t)
	testdb.Exec(t, conn, `INSERT INTO inventory (entity_id, section, unit, tech_level, quantity)
		VALUES (50, 'unassembled', 'AUTO', 2, 100);`)
	apply(t, conn, "colony 100050 stow 100 AUTO-2\n")
	if got := storedQuantity(t, conn, 50, "cargo", "AUTO", 2); got != 100 {
		t.Errorf("cargo AUTO-2 = %d; want all 100", got)
	}
}
