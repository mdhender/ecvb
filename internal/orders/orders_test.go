// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package orders

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/mdhender/ecvb/internal/testdb"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

const validOrders = `game "TEST" turn 3
id player " PLAYER@EXAMPLE.COM "

ship 40 move to system b orbit 4
ship 40 move to orbit 11
ship 40 jump to (1,2,3)
`

func TestCheckValidatesSequentialOrdersWithoutWriting(t *testing.T) {
	conn := openOrderTestDatabase(t)
	result, err := Check(context.Background(), conn, strings.NewReader(validOrders))
	if err != nil {
		t.Fatal(err)
	}
	if result.GameCode != "TEST" || result.Turn != 3 || result.FactionID != 1 || result.Orders != 3 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %+v; want none", result.Warnings)
	}
	if got := orderCount(t, conn); got != 1 {
		t.Fatalf("order count after check = %d; want 1", got)
	}
}

// Checking a file runs the turn against the database and rolls it back, so the
// thing to get wrong is the rolling back. Nothing a turn would change may
// survive a check: not where a ship is, not the fuel it holds, not the mass
// that fuel was part of, not what a probe read, and not what a phase's sweep
// wrote when nobody ordered it.
func TestCheckPutsTheWorldBackTheWayItFoundIt(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

ship 40 probe orbit 4
ship 40 move to orbit 6
ship 40 move to orbit 11
ship 40 jump to (1,2,3)
`
	before := worldSnapshot(t, conn)
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if after := worldSnapshot(t, conn); after != before {
		t.Errorf("check changed the world:\n before %s\n  after %s", before, after)
	}
}

// worldSnapshot is everything an order may touch, in one string.
func worldSnapshot(t *testing.T, conn *sqlite.Conn) string {
	t.Helper()
	var parts []string
	for _, query := range []string{
		`SELECT group_concat(printf('%d@%d/%s/%s/%s:%d', id, stellium_id, coalesce(system_id, '-'),
			coalesce(planet_id, '-'), coalesce(planet_ring, '-'), mass), ' ')
			FROM (SELECT * FROM entity ORDER BY id)`,
		`SELECT group_concat(printf('%d/%s/%d', entity_id, section, quantity), ' ')
			FROM (SELECT * FROM inventory ORDER BY entity_id, section, unit, tech_level)`,
		`SELECT (SELECT count(*) FROM probe_contact) || '/' || (SELECT count(*) FROM probe_deposit)
			|| '/' || (SELECT count(*) FROM sensor_survey) || '/' || (SELECT count(*) FROM sensor_contact)
			|| '/' || (SELECT count(*) FROM in_transit)`,
	} {
		if err := sqlitex.ExecuteTransient(conn, query+";", &sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				parts = append(parts, stmt.ColumnText(0))
				return nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return strings.Join(parts, " | ")
}

// A jump begins from the stellium orbit and ship 40 starts at a planet, so
// this file only binds if the move on the second line resolved before the jump
// on the first. The phase order is proved by a rule rather than by a count.
func TestCheckValidatesMovesBeforeJumpsRegardlessOfFileOrder(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1
ship 40 jump to (1,2,3)
ship 40 move to orbit 11
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckReportsInvalidTargets(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 4
id faction 1
ship 999 jump to (1,2,3)
ship 41 move to orbit 6
ship 42 jump to (1,2,3)
ship 40 jump to (9,9,9)
`
	_, err := Check(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatal("Check succeeded; want errors")
	}
	message := err.Error()
	for _, want := range []string{
		"line 1: game \"TEST\" is on turn 3, not turn 4",
		"line 3: ship 999 does not exist",
		"line 4: entity 41 is a COPN, not a ship",
		"line 5: ship 42 does not belong to faction 1",
		"line 6: game \"TEST\" has no stellium at (9,9,9)",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}

func TestSubmitAtomicallyReplacesOrders(t *testing.T) {
	conn := openOrderTestDatabase(t)
	result, err := Submit(context.Background(), conn, strings.NewReader(validOrders))
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 3 {
		t.Fatalf("submitted orders = %d; want 3", result.Orders)
	}

	var rows []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%d|%s|%s', sequence, actor_entity_id, verb, input)
		FROM game_order WHERE faction_id = 1 AND turn = 3 ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			rows = append(rows, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1|40|move|system B orbit 4",
		"2|40|move|orbit 11",
		"3|40|jump|(1,2,3)",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("orders = %q; want %q", rows, want)
	}

	invalid := strings.Replace(validOrders, "(1,2,3)", "(9,9,9)", 1)
	if _, err := Submit(context.Background(), conn, strings.NewReader(invalid)); err == nil {
		t.Fatal("invalid Submit succeeded")
	}
	if got := orderCount(t, conn); got != 3 {
		t.Fatalf("order count after rejected submit = %d; want 3", got)
	}
}

func TestSubmitRejectsResolvedTurn(t *testing.T) {
	conn := openOrderTestDatabase(t)
	if err := sqlitex.ExecuteTransient(conn, "UPDATE game SET turn_state = 'resolved' WHERE id = 1;", nil); err != nil {
		t.Fatal(err)
	}
	_, err := Submit(context.Background(), conn, strings.NewReader(validOrders))
	if err == nil || !strings.Contains(err.Error(), "resolved and not accepting orders") {
		t.Fatalf("Submit error = %v; want resolved turn error", err)
	}
	if got := orderCount(t, conn); got != 1 {
		t.Fatalf("order count after rejected submit = %d; want 1", got)
	}
}

// The order table enforces the two ids that are still ids: the faction plays
// this game, and the actor belongs to that faction. Everything else an order
// names is stored as the words the player wrote and resolved again when the
// turn runs, so there is no third id here to get wrong.
func TestTheOrderTableEnforcesTheIdsItStillHolds(t *testing.T) {
	conn := openOrderTestDatabase(t)
	for name, script := range map[string]string{
		// Ship 42 belongs to faction 2.
		"an actor the faction does not own": `
			INSERT INTO game_order (
				game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
			) VALUES (1, 3, 1, 2, 4, 'jump', 42, '(1,2,3)', '{"x":1,"y":2,"z":3}');`,
		// Faction 2 plays game 1, so it does not play game 2.
		"a faction that does not play this game": `
			INSERT INTO game_order (
				game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
			) VALUES (2, 3, 2, 2, 4, 'jump', NULL, '(1,2,3)', '{"x":1,"y":2,"z":3}');`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := sqlitex.ExecuteTransient(conn, script, nil); err == nil {
				t.Fatal("insert succeeded; want foreign key error")
			}
		})
	}
}

// What a status means is one block on one table now, instead of the same block
// copied onto every kind of order. A failed order says why it failed and
// burned nothing getting there, and it went nowhere.
func TestTheOrderTableEnforcesWhatAStatusMeans(t *testing.T) {
	for name, tc := range map[string]struct {
		script string
		want   string
	}{
		"a failed order says why": {
			script: `
				INSERT INTO game_order (
					game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, status
				) VALUES (1, 3, 1, 2, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}', 'failed');`,
			want: "CHECK constraint failed",
		},
		"a failed order burned nothing": {
			script: `
				INSERT INTO game_order (
					game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params,
					status, error_message, fuel_spent
				) VALUES (1, 3, 1, 2, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}', 'failed', 'no fuel', 160);`,
			want: "CHECK constraint failed",
		},
		"a pending order has not failed": {
			script: `
				INSERT INTO game_order (
					game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params, error_message
				) VALUES (1, 3, 1, 2, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}', 'no fuel');`,
			want: "CHECK constraint failed",
		},
		"a failed order goes nowhere": {
			script: `
				INSERT INTO game_order (
					game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params,
					status, error_message
				) VALUES (1, 3, 1, 2, 4, 'jump', 40, '(1,2,3)', '{"x":1,"y":2,"z":3}', 'failed', 'no fuel');
				INSERT INTO order_movement (
					game_id, turn, faction_id, sequence, start_stellium_id, final_stellium_id
				) VALUES (1, 3, 1, 2, 10, 11);`,
			want: "a failed order goes nowhere",
		},
	} {
		t.Run(name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			err := sqlitex.ExecuteScript(conn, tc.script, nil)
			if err == nil {
				t.Fatal("insert succeeded; want the row refused")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v; want it to mention %q", err, tc.want)
			}
		})
	}
}

func openOrderTestDatabase(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
		INSERT INTO users (id, email, role) VALUES (1, 'player@example.com', 'non-administrator');
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 3), (2, 'OTHER', 3);
		INSERT INTO agent (id, code, description) VALUES (1, 'uncontrolled', 'Uncontrolled');
		INSERT INTO faction (id, game_id, user_id) VALUES (1, 1, 1);
		INSERT INTO faction (id, game_id, agent_id) VALUES (2, 1, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES
			(10, 1, 0, 0, 0), (11, 1, 1, 2, 3), (12, 2, 1, 2, 3), (13, 1, 2, 4, 6);
		INSERT INTO system (id, stellium_id, sequence) VALUES
			(20, 10, 'A'), (21, 11, 'A'), (22, 11, 'B'), (23, 10, 'B');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES
			(30, 20, 4, 'rocky', 10), (31, 20, 6, 'rocky', 10),
			(32, 22, 4, 'rocky', 10), (33, 23, 4, 'rocky', 10);
		INSERT INTO entity (id, unit, tech_level, stellium_id, system_id, planet_id, planet_ring, faction_id, enclosed_volume) VALUES
			(40, 'SHIP', 1, 10, 20, 30, 64, 1, 100),
			(41, 'COPN', 1, 10, 20, 30, 0, 1, 100),
			(42, 'SHIP', 1, 10, 20, 30, 64, 2, 100);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(40, 'component', 'HDRV', 4, 1), (42, 'component', 'HDRV', 4, 1),
			(40, 'component', 'SNSR', 2, 1), (41, 'component', 'SNSR', 1, 1),
			(40, 'cargo', 'FUEL', 0, 500), (42, 'cargo', 'FUEL', 0, 500);
		INSERT INTO game_order (
			game_id, turn, faction_id, sequence, source_line, verb, actor_entity_id, input, params
		) VALUES (1, 3, 1, 1, 3, 'jump', 40, '(0,0,0)', '{"x":0,"y":0,"z":0}');
		-- Checking a file runs the orders for real and rolls them back, so a
		-- ship has to mass at least the fuel it is about to burn. 500 of each
		-- ship's 3,000 MU is the fuel it carries, at 1 MU each.
		UPDATE entity SET mass = 3000 WHERE id IN (40, 42);
		UPDATE entity SET mass = 1234 WHERE id = 41;
	`)
	return conn
}

func orderCount(t *testing.T, conn *sqlite.Conn) int {
	t.Helper()
	count := 0
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT count(*) FROM game_order WHERE faction_id = 1 AND turn = 3;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			count = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCheckRejectsJumpsTheDriveCannotMake(t *testing.T) {
	// Stellium 11 is (1,2,3) from stellium 10, a distance of 4. Ship 40 is put
	// in the stellium orbit first, because a drive that cannot jump cannot
	// move either and the move would fail before the jump was reached.
	const inTheStelliumOrbit = `
		UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`
	input := `game "TEST" turn 3
id faction 1

ship 40 jump to (1,2,3)
`
	for _, tc := range []struct {
		name    string
		setup   string
		problem string
	}{
		{
			name:    "too massive",
			setup:   `UPDATE entity SET mass = 4181 WHERE id = 40;`,
			problem: "ship 40 masses 4181 MU and its jump drive propels 4180 MU",
		},
		{
			name:    "no drive",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'HDRV';`,
			problem: "ship 40 has no assembled HDRV and cannot jump",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			if err := sqlitex.ExecuteScript(conn, inTheStelliumOrbit+tc.setup, nil); err != nil {
				t.Fatal(err)
			}
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
			if !strings.Contains(err.Error(), "line 4") {
				t.Errorf("error = %v; want it to name the source line", err)
			}
		})
	}
}

// A jump begins from the stellium orbit. A ship still at a planet when the
// jump phase reaches it cannot go, and because that cannot change between the
// file and the turn, it rejects the file rather than failing one order.
func TestCheckRejectsAJumpFromAPlanet(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 starts at planet 30, and orbit 6 is another planet: a move that
	// leaves the ship at a planet is no help.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 6
ship 40 jump to (1,2,3)
`
	_, err := Check(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatal("Check succeeded; want the jump rejected")
	}
	want := "line 5: ship 40 is at a planet and a jump begins from the stellium orbit; move it to orbit 11 first"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v; want %q", err, want)
	}
}

// Technology level used to cap how far a drive jumped. It no longer caps
// anything: this jump is 8 light years with a technology level 4 drive, which
// the old rule refused, and the only thing that grows with the distance is the
// fuel. See TestSubmitPricesASecondJumpFromTheFirstDestination for that.
func TestCheckAcceptsAJumpBeyondTheOldDriveRange(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 11
ship 40 jump to (2,4,6)
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want 2", result.Orders)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("warnings = %+v; want none", result.Warnings)
	}
}

// A ship makes one crossing at a time. The first jump takes it off the board
// for the rest of the turn -- arrivals resolve after every order -- so the
// second is given to a ship that is not there to receive it. That is a Bind
// failure, so the whole file is refused and nothing is stored.
//
// This is the rule that stopped a ship chaining jumps to cross the map in a
// single turn: the shortest crossing there is still takes the turn it began in.
func TestSubmitRefusesASecondJumpWhileTheShipIsCrossing(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 11
ship 40 jump to (1,2,3)
ship 40 jump to (2,4,6)
`
	_, err := Submit(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatalf("Submit accepted two jumps for one ship in one turn")
	}
	const want = "line 6: ship 40 is in transit and arrives on turn 3; it can be given no orders until then"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Submit error = %v; want it to contain %q", err, want)
	}
	// A refused file stores nothing: the fixture's one pre-existing order is
	// all that is left.
	if got := orderCount(t, conn); got != 1 {
		t.Fatalf("order count after a refused submit = %d; want 1", got)
	}
}

// A crossing of one turn is due in the turn it departed, so the arrival step
// lands the ship before the turn is out and nothing is left in transit. That is
// what every jump did before a crossing could span turns, and it is why the
// short case needed no special path.
//
// Check rolls everything back, so what this proves is that the dry run reached
// the end with the ship landed rather than stranded: a crossing left standing
// would have failed the move on the following line.
func TestCheckLandsAOneTurnCrossingWithinTheTurn(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Stellium 11 at (1,2,3) is 4 light years from the origin and ship 40's
	// drive is an HDRV-4, so the crossing is 4/4 = one turn.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 11
ship 40 jump to (1,2,3)
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want 2", result.Orders)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("warnings = %+v; want none", result.Warnings)
	}
}

func TestCheckAcceptsProbesWithinTheSensorBudget(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 carries one SNSR-2, so it launches two probes. System 20 has
	// planets in orbits 4 and 6.
	input := `game "TEST" turn 3
id faction 1

ship 40 probe orbit 4 6
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want one order per probed orbit", result.Orders)
	}
}

func TestCheckRejectsProbesTheSensorsCannotSupport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   string
		input   string
		problem string
	}{
		{
			name:    "over budget",
			input:   "ship 40 probe orbit 4 6 4",
			problem: "ship 40 has only 2 probes this turn",
		},
		{
			name:    "empty orbit",
			input:   "ship 40 probe orbit 5",
			problem: "system current has no planet in orbit 5",
		},
		{
			name:    "no sensors",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'SNSR';`,
			input:   "ship 40 probe orbit 4",
			problem: "ship 40 has no assembled SNSR and cannot probe",
		},
		{
			name:    "no system",
			setup:   `UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`,
			input:   "ship 40 probe orbit 4",
			problem: "ship 40 is orbiting the stellium; name a system to probe",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			if tc.setup != "" {
				if err := sqlitex.ExecuteScript(conn, tc.setup, nil); err != nil {
					t.Fatal(err)
				}
			}
			input := "game \"TEST\" turn 3\nid faction 1\n\n" + tc.input + "\n"
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
		})
	}
}

func TestCheckProbesTheSystemTheShipStartsIn(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 starts in system 20, which has planets in orbits 4 and 6.
	// Moving to system B puts it in system 23, which has no orbit 6. Probes
	// resolve before moves, so the probe reads system 20 and succeeds.
	input := `game "TEST" turn 3
id faction 1

ship 40 probe orbit 6
ship 40 move to system b orbit 4
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 2 {
		t.Errorf("orders = %d; want 2", result.Orders)
	}
}

func TestCheckOrdersProbesAheadOfMovesAndJumps(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// File order is move, jump, probe. Resolution order is probe, move, jump,
	// and the stored sequence has to record the resolution order.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 11
ship 40 jump to (1,2,3)
ship 40 probe orbit 4
`
	if _, err := Submit(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var order []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT verb, sequence FROM game_order
		WHERE faction_id = 1 AND turn = 3 ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			order = append(order, fmt.Sprintf("%s=%d", stmt.ColumnText(0), stmt.ColumnInt(1)))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"probe=1", "move=2", "jump=3"}; !slices.Equal(order, want) {
		t.Errorf("sequences = %v; want %v", order, want)
	}
}

func TestSubmitStoresOneProbeOrderForEachOrbit(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

ship 40 probe orbit 4 6
`
	if _, err := Submit(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var orbits []int
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT json_extract(params, '$.orbits[0]') FROM game_order
		WHERE faction_id = 1 AND turn = 3 AND verb = 'probe' ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			orbits = append(orbits, stmt.ColumnInt(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []int{4, 6}; !slices.Equal(orbits, want) {
		t.Errorf("stored orbits = %v; want %v", orbits, want)
	}
}

func TestCheckProbesANamedSystemOfTheCurrentStellium(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 is in system 20 (A) of stellium 10, which also holds system 23
	// (B). Naming B probes a system the ship is not in.
	input := `game "TEST" turn 3
id faction 1

ship 40 probe system B orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckProbesANamedSystemFromStelliumOrbit(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// A ship orbiting the stellium has no current system, so a bare probe
	// fails. Naming a system of that stellium works.
	if err := sqlitex.ExecuteScript(conn,
		`UPDATE entity SET system_id = NULL, planet_id = NULL, planet_ring = NULL WHERE id = 40;`, nil); err != nil {
		t.Fatal(err)
	}
	input := `game "TEST" turn 3
id faction 1

ship 40 probe system A orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckRejectsAProbeOfASystemTheStelliumDoesNotHold(t *testing.T) {
	conn := openOrderTestDatabase(t)
	input := `game "TEST" turn 3
id faction 1

ship 40 probe system C orbit 4
`
	_, err := Check(context.Background(), conn, strings.NewReader(input))
	if err == nil {
		t.Fatal("Check succeeded; want a problem")
	}
	if want := "current stellium has no system C"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v; want %q", err, want)
	}
}

func TestCheckAcceptsAColonyProbe(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Entity 41 is a COPN at planet 30 in system 20, with one SNSR-1.
	input := `game "TEST" turn 3
id faction 1

colony 41 probe orbit 4
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Orders != 1 {
		t.Errorf("orders = %d; want 1", result.Orders)
	}
}

func TestCheckRejectsAProbeThatNamesTheWrongKindOfEntity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		problem string
	}{
		{name: "colony named as a ship", input: "ship 41 probe orbit 4", problem: "entity 41 is a COPN, not a ship"},
		{name: "ship named as a colony", input: "colony 40 probe orbit 4", problem: "entity 40 is a ship, not a colony"},
		// A colony is not a subject MOVE takes, so the line is refused before
		// MOVE's parser ever sees it, and the error says who may be given one.
		{name: "colony ordered to move", input: "colony 41 move to orbit 6", problem: "MOVE is given to a ship, not to a colony"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			input := "game \"TEST\" turn 3\nid faction 1\n\n" + tc.input + "\n"
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
		})
	}
}

func TestSubmitStoresMoveFuelAndTheStelliumOrbit(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// Ship 40 starts at planet 30 in system A of stellium 10. Orbit 6 is
	// another planet of system A, system B orbit 4 is planet 33 of the same
	// stellium, and orbit 11 is the stellium orbit.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 6
ship 40 move to system B orbit 4
ship 40 move to orbit 11
`
	if _, err := Submit(context.Background(), conn, strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var rows []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%s|%d', sequence, input, fuel_spent)
		FROM game_order WHERE faction_id = 1 AND turn = 3 AND verb = 'move' ORDER BY sequence;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				rows = append(rows, stmt.ColumnText(0))
				return nil
			},
		}); err != nil {
		t.Fatal(err)
	}
	// The cost of each move is measured from where the one before it left the
	// ship, which is why the second one costs twice the first.
	want := []string{
		"1|orbit 6|4",          // one hop: planet to planet inside system A
		"2|system B orbit 4|8", // two hops: planet to planet across systems
		"3|orbit 11|4",         // one hop: planet to the stellium orbit
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("move orders = %q; want %q", rows, want)
	}
}

func TestCheckRejectsMovesTheDriveCannotMake(t *testing.T) {
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 6
`
	for _, tc := range []struct {
		name    string
		setup   string
		problem string
	}{
		{
			name:    "too massive",
			setup:   `UPDATE entity SET mass = 4181 WHERE id = 40;`,
			problem: "ship 40 masses 4181 MU and its drive propels 4180 MU",
		},
		{
			name:    "no drive",
			setup:   `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'HDRV';`,
			problem: "ship 40 has no assembled HDRV and cannot move",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := openOrderTestDatabase(t)
			if err := sqlitex.ExecuteScript(conn, tc.setup, nil); err != nil {
				t.Fatal(err)
			}
			_, err := Check(context.Background(), conn, strings.NewReader(input))
			if err == nil {
				t.Fatal("Check succeeded; want a problem")
			}
			if !strings.Contains(err.Error(), tc.problem) {
				t.Errorf("error = %v; want it to report %q", err, tc.problem)
			}
			if !strings.Contains(err.Error(), "line 4") {
				t.Errorf("error = %v; want it to name the source line", err)
			}
		})
	}
}

func TestCheckMovesFromTheStelliumOrbitAndRejectsQualifyingIt(t *testing.T) {
	conn := openOrderTestDatabase(t)
	// A ship in the stellium orbit has no current system, so it has to name
	// one; the stellium orbit itself belongs to no system, so naming one for
	// orbit 11 is an error.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 11
ship 40 move to orbit 4
`
	if _, err := Check(context.Background(), conn, strings.NewReader(input)); err == nil ||
		!strings.Contains(err.Error(), "line 5: ship has no current system") {
		t.Fatalf("Check error = %v; want the ship to have left its system", err)
	}
	qualified := `game "TEST" turn 3
id faction 1

ship 40 move to system A orbit 11
`
	if _, err := Check(context.Background(), conn, strings.NewReader(qualified)); err == nil ||
		!strings.Contains(err.Error(), "orbit 11 is the stellium orbit and belongs to no system") {
		t.Fatalf("Check error = %v; want the stellium orbit to reject a system", err)
	}
}

func TestCheckWarnsWhenTheShipCannotPayForItsOrders(t *testing.T) {
	conn := openOrderTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `
		UPDATE inventory SET quantity = 200 WHERE entity_id = 40 AND unit = 'FUEL';`, nil); err != nil {
		t.Fatal(err)
	}
	// Ship 40 has one HDRV-4 and 200 FUEL. The move out to the stellium orbit
	// burns 1 * 0.1 * 40, leaving 196, and the 4-light-year jump wants
	// 1 * 4 * 40. The whole jump is billed on departure, so a tank that would
	// have covered part of the crossing does not send the ship on any of it.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 11
ship 40 jump to (2,4,6)
`
	result, err := Check(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if result.Orders != 2 {
		t.Fatalf("orders = %d; want 2", result.Orders)
	}
	var lines []string
	for _, warning := range result.Warnings {
		lines = append(lines, fmt.Sprintf("%d: %s", warning.Line, warning.Message))
	}
	// (2,4,6) is 8 light years from the origin: 320 FUEL, twice what is left.
	want := []string{
		"5: ship 40 needs 320 FUEL to jump and holds 196; the order is kept in case that changes before the turn resolves",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("warnings = %q; want %q", lines, want)
	}
}

func TestCheckWarnsOnceTheTankIsEmptyAndSubmitKeepsTheOrders(t *testing.T) {
	conn := openOrderTestDatabase(t)
	if err := sqlitex.ExecuteScript(conn, `DELETE FROM inventory WHERE entity_id = 40 AND unit = 'FUEL';`, nil); err != nil {
		t.Fatal(err)
	}
	// A dry ship warns on every order but still submits them: fuel may reach
	// the ship before the turn resolves.
	input := `game "TEST" turn 3
id faction 1

ship 40 move to orbit 6
ship 40 move to orbit 4
`
	result, err := Submit(context.Background(), conn, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("warnings = %+v; want one per order", result.Warnings)
	}
	for _, warning := range result.Warnings {
		if !strings.Contains(warning.Message, "needs 4 FUEL to move and holds 0") {
			t.Errorf("warning = %q; want it to report an empty tank", warning.Message)
		}
	}
	var stored []string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT printf('%d|%d', sequence, fuel_spent)
		FROM game_order WHERE faction_id = 1 AND turn = 3 AND verb = 'move' ORDER BY sequence;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			stored = append(stored, stmt.ColumnText(0))
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"1|4", "2|4"}; strings.Join(stored, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stored moves = %q; want %q", stored, want)
	}
}
