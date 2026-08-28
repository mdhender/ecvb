// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"testing"

	"github.com/mdhender/ecvb/internal/cadre"
	"github.com/mdhender/ecvb/internal/labour"
	"github.com/mdhender/ecvb/internal/testdb"
	"github.com/mdhender/ecvb/internal/transport"
	"github.com/mdhender/ecvb/internal/units"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// The drive, the sensors, the fuel, and the enclosed volume are not loaded
// separately: they are what the inventory adds up to. This is that sum.
func TestLoadDerivesEverythingTheInventoryAddsUpTo(t *testing.T) {
	loaded := loadWorld(t, openInventoryTestDatabase(t))
	ship := loaded.Entity(40)
	if ship.Drive.Units != 10 || ship.Drive.TechLevel != 1 {
		t.Errorf("drive = %+v; want 10 units running at level 1", ship.Drive)
	}
	if ship.Sensors.Probes != 4 {
		t.Errorf("probes = %d; want 4", ship.Sensors.Probes)
	}
	if got := ship.Fuel(); got != 60 {
		t.Errorf("fuel = %d; want 60, counted across every section", got)
	}
	if ship.EnclosedVolume != 400 {
		t.Errorf("enclosed volume = %d; want 400, four STRC-10 at 100 VU each", ship.EnclosedVolume)
	}
	if got := ship.Held(units.SectionCargo, "GOLD", 0); got != 1000 {
		t.Errorf("cargo GOLD = %d; want 1000", got)
	}
	if got := ship.Population["SKW"]; got != 30 {
		t.Errorf("SKW = %d; want 30", got)
	}
	if got := ship.ConstructionWorkers(); got != 4 {
		t.Errorf("CWKR = %d; want 4", got)
	}
}

// The fuel goes operational first, then unassembled, then cargo, so a hold of
// spare fuel survives until the working supply is gone. Both the database and
// the loaded copy have to agree afterwards.
func TestBurnFuelEmptiesOperationalThenUnassembledThenCargo(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	before := ship.Mass
	// 45 FUEL empties the 10 operational and 15 unassembled units and takes 20
	// of the 35 held as cargo.
	if err := loaded.BurnFuel(ship, 45); err != nil {
		t.Fatal(err)
	}
	if got := fuelStacks(t, conn, 40); got != "cargo=15" {
		t.Errorf("stored inventory = %q; want only 15 units of cargo left", got)
	}
	if got := ship.Fuel(); got != 15 {
		t.Errorf("loaded fuel = %d; want 15", got)
	}
	if got := ship.Mass; got != before-45 {
		t.Errorf("loaded mass = %d; want %d", got, before-45)
	}
	if got := storedMass(t, conn, 40); got != before-45 {
		t.Errorf("stored mass = %d; want %d", got, before-45)
	}
}

func TestBurnFuelLeavesTheEntityUntouchedWhenItCannotPay(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	if err := loaded.BurnFuel(ship, 61); err == nil {
		t.Fatal("BurnFuel succeeded on 61 of 60 FUEL; want a shortfall error")
	}
	if got := ship.Fuel(); got != 60 {
		t.Errorf("fuel = %d after a refused burn; want 60", got)
	}
	if got := fuelStacks(t, conn, 40); got != "cargo=35 operational=10 unassembled=15" {
		t.Errorf("stored inventory = %q; want every stack as it was", got)
	}
}

// Assembling moves a unit between sections. Nothing is created or destroyed,
// so the mass does not change; what changes is the room it takes and, for the
// six units that only work as components, what the entity can do.
func TestShiftKeepsMassAndCorrectsEverythingDerived(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	before, occupied := ship.Mass, ship.OccupiedVolume()
	if err := loaded.ShiftAll(ship, []Shift{{
		From: units.SectionUnassembled, To: units.SectionComponent,
		Unit: "SNSR", TechLevel: 1, Quantity: 6,
	}}); err != nil {
		t.Fatal(err)
	}
	if ship.Mass != before {
		t.Errorf("mass = %d; want %d unchanged: nothing left the ship", ship.Mass, before)
	}
	// Six more sensors at one probe each, on top of the two SNSR-2 it had.
	if got := ship.Sensors.Probes; got != 10 {
		t.Errorf("probes = %d; want 10", got)
	}
	// SNSR-1 takes 2 VU unassembled and 8 VU as a component.
	if got := ship.OccupiedVolume(); got != occupied+6*(8-2) {
		t.Errorf("occupied = %d; want %d", got, occupied+6*(8-2))
	}
	if got := storedQuantity(t, conn, 40, units.SectionComponent, "SNSR", 1); got != 6 {
		t.Errorf("stored component SNSR-1 = %d; want 6", got)
	}
	if got := storedQuantity(t, conn, 40, units.SectionUnassembled, "SNSR", 1); got != 0 {
		t.Errorf("stored unassembled SNSR-1 = %d; want the emptied row gone", got)
	}
}

// Only assembled STRC and STRL enclose anything, so unassembling structure
// takes the room away and the entity column has to follow.
func TestShiftingStructureMovesTheEnclosedVolumeWithIt(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	if err := loaded.ShiftAll(ship, []Shift{{
		From: units.SectionComponent, To: units.SectionUnassembled,
		Unit: "STRC", TechLevel: 10, Quantity: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if ship.EnclosedVolume != 300 {
		t.Errorf("enclosed volume = %d; want 300", ship.EnclosedVolume)
	}
	if got := storedEnclosedVolume(t, conn, 40); got != 300 {
		t.Errorf("stored enclosed volume = %d; want 300", got)
	}
	// A SHIP uses a tenth of what it encloses.
	usable, err := ship.UsableEnclosedSpace()
	if err != nil {
		t.Fatal(err)
	}
	if usable != 30 {
		t.Errorf("usable enclosed space = %d; want 30", usable)
	}
}

// RoomAfter is arithmetic on what the entity holds rather than a trial and a
// rollback, so an order can ask before it does anything.
func TestRoomAfterAnswersWithoutTouchingAnything(t *testing.T) {
	loaded := loadWorld(t, openInventoryTestDatabase(t))
	ship := loaded.Entity(40)
	before := ship.OccupiedVolume()
	shifts := []Shift{{From: units.SectionComponent, To: units.SectionUnassembled,
		Unit: "STRC", TechLevel: 10, Quantity: 4}}
	occupied, usable, err := ship.RoomAfter(shifts)
	if err != nil {
		t.Fatal(err)
	}
	// Unassembling all four takes every VU of enclosed space away, and the
	// units themselves then need somewhere to go: 20 VU each.
	if usable != 0 {
		t.Errorf("usable after = %d; want 0", usable)
	}
	if occupied != before+4*20 {
		t.Errorf("occupied after = %d; want %d", occupied, before+4*20)
	}
	if ship.OccupiedVolume() != before || ship.EnclosedVolume != 400 {
		t.Error("RoomAfter changed the entity; it is meant only to answer")
	}
}

// Handing units over is the one mutation that changes what an entity masses,
// because it is the only one where units leave the entity altogether.
func TestHandMovesMassBetweenTwoEntities(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	loaded := loadWorld(t, conn)
	from, to := loaded.Entity(40), loaded.Entity(41)
	fromMass, toMass := from.Mass, to.Mass
	if err := loaded.Hand(from, to, "GOLD", 0, 400); err != nil {
		t.Fatal(err)
	}
	if from.Mass != fromMass-400 || to.Mass != toMass+400 {
		t.Errorf("masses = %d and %d; want %d and %d", from.Mass, to.Mass, fromMass-400, toMass+400)
	}
	if got := storedMass(t, conn, 41); got != toMass+400 {
		t.Errorf("stored recipient mass = %d; want %d", got, toMass+400)
	}
	if got := to.Held(units.SectionCargo, "GOLD", 0); got != 400 {
		t.Errorf("recipient GOLD = %d; want 400", got)
	}
	if err := loaded.Hand(from, to, "GOLD", 0, 601); err == nil {
		t.Error("Hand moved 601 of the 600 left; want a shortfall error")
	}
}

func TestHandPopulationMovesPeopleAndTheirMass(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	loaded := loadWorld(t, conn)
	from, to := loaded.Entity(40), loaded.Entity(41)
	fromMass := from.Mass
	if err := loaded.HandPopulation(from, to, "SKW", 30); err != nil {
		t.Fatal(err)
	}
	if got := from.Population["SKW"]; got != 0 {
		t.Errorf("sender SKW = %d; want the emptied row gone", got)
	}
	if got := to.Population["SKW"]; got != 30 {
		t.Errorf("recipient SKW = %d; want 30", got)
	}
	if from.Mass != fromMass-30*units.PopulationMetrics.Mass {
		t.Errorf("sender mass = %d; want %d", from.Mass, fromMass-60)
	}
	if got := storedPopulation(t, conn, 41, "SKW"); got != 30 {
		t.Errorf("stored recipient SKW = %d; want 30", got)
	}
}

// A worker does one task per turn, so the pool is drawn down across every
// order of the turn rather than refilled for each.
func TestWorkPoolsAreDrawnDownAcrossTheTurnAndNeverPoolWithEachOther(t *testing.T) {
	loaded := loadWorld(t, openInventoryTestDatabase(t))
	ship := loaded.Entity(40)
	if got := loaded.WorkAllowed(cadre.Assembly, ship); got != 4*cadre.WorkPerUnit {
		t.Errorf("allowed = %d MU; want %d, four workers' worth", got, 4*cadre.WorkPerUnit)
	}
	loaded.RecordWork(cadre.Assembly, ship.ID, 1200)
	if got := loaded.WorkAllowed(cadre.Assembly, ship); got != 800 {
		t.Errorf("allowed after 1,200 MU = %d MU; want 800", got)
	}
	// One MU of unassembly takes a whole worker away from assembly, because
	// the two pools round up on their own.
	loaded.RecordWork(cadre.Unassembly, ship.ID, 1)
	if got := loaded.WorkAllowed(cadre.Assembly, ship); got != 300 {
		t.Errorf("allowed = %d MU; want 300, one worker having gone to unassembly", got)
	}
	// Three of the four workers are spoken for by the 1,200 MU of assembly, so
	// unassembly has one left, less the single MU it has already done.
	if got := loaded.WorkAllowed(cadre.Unassembly, ship); got != cadre.WorkPerUnit-1 {
		t.Errorf("unassembly allowed = %d MU; want %d", got, cadre.WorkPerUnit-1)
	}
	// An entity with no cadre does no work at all.
	if got := loaded.WorkAllowed(cadre.Assembly, loaded.Entity(41)); got != 0 {
		t.Errorf("allowed with no cadre = %d MU; want 0", got)
	}
}

// Production labour is a second pair of pools and a different pool of workers:
// an entity's unassigned USK plus t for every assembled AUTO. Stowing and
// unstowing round up on their own the way assembly and unassembly do, and
// neither takes anything from the construction workers.
func TestProductionLabourIsItsOwnPairOfPoolsAndItsOwnPeople(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	testdb.Exec(t, conn, `INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
		(40, 'operational', 'AUTO', 3, 5),
		(40, 'unassembled', 'AUTO', 3, 100);`)
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	// 30 USK less the 4 in the CWKR cadre is 26, and five assembled AUTO-3 add
	// fifteen more. The hundred unassembled ones are freight and add nothing.
	if got := ship.ProductionLabour(); got != 41 {
		t.Errorf("production labour = %d; want 41", got)
	}
	if got := loaded.WorkAllowed(labour.Stowing, ship); got != 41*labour.PerUnit {
		t.Errorf("stowing allowed = %d MU; want %d", got, 41*labour.PerUnit)
	}
	// One MU stowed takes a whole unit of labour away from unstowing, and the
	// construction workers are untouched by either.
	loaded.RecordWork(labour.Stowing, ship.ID, 1)
	if got := loaded.WorkAllowed(labour.Unstowing, ship); got != 40*labour.PerUnit {
		t.Errorf("unstowing allowed = %d MU; want %d", got, 40*labour.PerUnit)
	}
	if got := loaded.WorkAllowed(cadre.Assembly, ship); got != 4*cadre.WorkPerUnit {
		t.Errorf("assembly allowed = %d MU; want %d: freight is not construction", got, 4*cadre.WorkPerUnit)
	}
	// An entity with nobody to move freight moves none.
	if got := loaded.WorkAllowed(labour.Stowing, loaded.Entity(41)); got != 0 {
		t.Errorf("stowing allowed with no people = %d MU; want 0", got)
	}
}

// A transport goes there and comes back, so a turn is the whole of what one
// hull is worth, and the fuel is reckoned over every hull the turn used.
func TestTransportsAreCommittedForTheTurnAndTheirFuelPoolsOverIt(t *testing.T) {
	loaded := loadWorld(t, openInventoryTestDatabase(t))
	ship := loaded.Entity(40)
	free := loaded.TransportsFree(ship)
	if got := transport.Capacity(free); got.Mass != 400 || got.Volume != 1200 {
		t.Fatalf("free capacity = %+v; want 400 MU and 1,200 VU from 20 TRAN-1", got)
	}
	// Fifteen hulls cost two FUEL; five more cost nothing extra, because the
	// twenty are reckoned at once and still round to two.
	if got := loaded.CommitTransports(ship, []transport.Hulls{{TechLevel: 1, Count: 15}}); got != 2 {
		t.Errorf("first commitment = %d FUEL; want 2", got)
	}
	if got := loaded.CommitTransports(ship, []transport.Hulls{{TechLevel: 1, Count: 5}}); got != 0 {
		t.Errorf("second commitment = %d FUEL; want 0, the turn already paying for twenty", got)
	}
	if got := loaded.TransportsFree(ship); len(got) != 0 {
		t.Errorf("free after committing every hull = %+v; want none", got)
	}
}

// One SKW unit operates up to ten transports in a turn -- one that is free to,
// which is four fewer than the ship carries, its CWKR cadre having spoken for
// those.
func TestTheCrewCapsTheTransportsAnEntityCanRun(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	testdb.Exec(t, conn, "UPDATE entity_population SET quantity = 5 WHERE entity_id = 40 AND class = 'SKW';")
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	free := loaded.TransportsFree(ship)
	if len(free) != 1 || free[0].Count != 10 {
		t.Fatalf("free = %+v; want ten hulls, which is all one SKW crews", free)
	}
	loaded.CommitTransports(ship, []transport.Hulls{{TechLevel: 1, Count: 10}})
	if got := loaded.TransportsFree(ship); len(got) != 0 {
		t.Errorf("free after the crew is out = %+v; want none", got)
	}
}

// A cadre is an assignment of real people, so the people in it are not
// available to be given a second job.
func TestACadresPeopleAreSpokenForAndNotAvailableTwice(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	testdb.Exec(t, conn, "UPDATE entity_cadre SET quantity = 30 WHERE entity_id = 40;")
	loaded := loadWorld(t, conn)
	ship := loaded.Entity(40)
	// Thirty skilled workers and thirty construction workers: none spare.
	if got := ship.Unassigned(units.ClassSkilled); got != 0 {
		t.Errorf("unassigned SKW = %d; want 0, the cadre having taken all thirty", got)
	}
	if got := ship.Assigned(units.ClassUnskilled); got != 30 {
		t.Errorf("assigned USK = %d; want 30: one CWKR is one SKW plus one USK", got)
	}
	if got := loaded.TransportsFree(ship); len(got) != 0 {
		t.Errorf("free transports = %+v; want none, there being nobody to crew them", got)
	}
}

// A cadre cannot outlive the people in it. Losing skilled workers below what
// the cadre has spoken for takes the cadre down with them, and the unskilled
// workers it was pairing them with go back to being unassigned.
func TestACadreFallsWithThePopulationItAssigned(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	testdb.Exec(t, conn, "UPDATE entity_cadre SET quantity = 30 WHERE entity_id = 40;")
	loaded := loadWorld(t, conn)
	from, to := loaded.Entity(40), loaded.Entity(41)
	if err := loaded.HandPopulation(from, to, units.ClassSkilled, 3); err != nil {
		t.Fatal(err)
	}
	if got := from.Population[units.ClassSkilled]; got != 27 {
		t.Errorf("SKW = %d; want 27", got)
	}
	if got := from.Population[units.ClassUnskilled]; got != 30 {
		t.Errorf("USK = %d; want 30: the unskilled workers went nowhere", got)
	}
	if got := from.ConstructionWorkers(); got != 27 {
		t.Errorf("CWKR = %d; want 27, the cadre having fallen with its skilled workers", got)
	}
	if got := storedCadre(t, conn, 40, "CWKR"); got != 27 {
		t.Errorf("stored CWKR = %d; want 27", got)
	}
	// The three unskilled workers the cadre released are unassigned again.
	if got := from.Unassigned(units.ClassUnskilled); got != 3 {
		t.Errorf("unassigned USK = %d; want 3", got)
	}
}

// A cadre with skilled workers to spare loses nothing but the workers.
func TestACadreWithPopulationToSpareIsUntouched(t *testing.T) {
	conn := openInventoryTestDatabase(t)
	testdb.Exec(t, conn, "UPDATE entity_cadre SET quantity = 15 WHERE entity_id = 40;")
	loaded := loadWorld(t, conn)
	from, to := loaded.Entity(40), loaded.Entity(41)
	if err := loaded.HandPopulation(from, to, units.ClassSkilled, 3); err != nil {
		t.Fatal(err)
	}
	if got := from.ConstructionWorkers(); got != 15 {
		t.Errorf("CWKR = %d; want 15 untouched: there were fifteen skilled workers spare", got)
	}
	if got := from.Unassigned(units.ClassSkilled); got != 12 {
		t.Errorf("unassigned SKW = %d; want 12", got)
	}
}

func TestTheUncontrolledFactionIsFoundOnLoad(t *testing.T) {
	loaded := loadWorld(t, openInventoryTestDatabase(t))
	if got := loaded.Game().Uncontrolled; got != 2 {
		t.Errorf("uncontrolled faction = %d; want 2", got)
	}
}

func loadWorld(t *testing.T, conn *sqlite.Conn) *World {
	t.Helper()
	loaded, found, err := Load(conn, "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("game TEST does not exist")
	}
	return loaded
}

// Entity 40 is a ship with something in every section and a cadre; 41 is a
// bare colony beside it, so that a transfer has somewhere to go. Both are
// written under their row ids here and under 100040 and 100041 -- the numbers a
// player would write -- wherever a test speaks as a player would.
func openInventoryTestDatabase(t *testing.T) *sqlite.Conn {
	t.Helper()
	conn := testdb.New(t)
	testdb.Exec(t, conn, `
		INSERT INTO users (id, email, role) VALUES (1, 'player@example.com', 'non-administrator');
		INSERT INTO agent (id, code, description) VALUES (1, 'uncontrolled', 'Uncontrolled');
		INSERT INTO game (id, code, turn) VALUES (1, 'TEST', 0);
		INSERT INTO faction (id, game_id, number, user_id) VALUES (1, 1, 1, 1);
		INSERT INTO faction (id, game_id, number, agent_id) VALUES (2, 1, 2, 1);
		INSERT INTO stellium (id, game_id, x, y, z) VALUES (10, 1, 0, 0, 0);
		INSERT INTO system (id, stellium_id, sequence) VALUES (20, 10, 'A');
		INSERT INTO planet (id, system_id, orbit, kind, habitability) VALUES (30, 20, 4, 'rocky', 10);
		INSERT INTO entity (id, game_id, number, unit, tech_level, stellium_id, system_id, planet_id, planet_ring,
			faction_id, enclosed_volume, mass) VALUES
			(40, 1, 100040, 'SHIP', 1, 10, 20, 30, 64, 1, 400, 5000),
			(41, 1, 100041, 'COPN', 1, 10, 20, 30, 0, 1, 400, 500);
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES
			(40, 'component', 'HDRV', 1, 10),
			(40, 'component', 'SNSR', 2, 2),
			(40, 'component', 'STRC', 10, 4),
			(40, 'operational', 'TRAN', 1, 20),
			(40, 'unassembled', 'SNSR', 1, 6),
			(40, 'unassembled', 'FUEL', 0, 15),
			(40, 'operational', 'FUEL', 0, 10),
			(40, 'cargo', 'FUEL', 0, 35),
			(40, 'cargo', 'GOLD', 0, 1000);
		INSERT INTO entity_population (entity_id, class, quantity) VALUES (40, 'SKW', 30), (40, 'USK', 30);
		INSERT INTO entity_cadre (entity_id, cadre, quantity) VALUES (40, 'CWKR', 4);
	`)
	return conn
}

// fuelStacks renders an entity's remaining fuel stacks, so a test can assert
// which sections were emptied.
func fuelStacks(t *testing.T, conn *sqlite.Conn, entityID int64) string {
	t.Helper()
	var text string
	if err := sqlitex.ExecuteTransient(conn, `
		SELECT coalesce(group_concat(printf('%s=%d', section, quantity), ' '), '')
		FROM (SELECT section, quantity FROM inventory
			WHERE entity_id = ? AND unit = 'FUEL' ORDER BY section);`, &sqlitex.ExecOptions{
		Args: []any{entityID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			text = stmt.ColumnText(0)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return text
}

func storedQuantity(t *testing.T, conn *sqlite.Conn, entityID int64, section, unit string, techLevel int) int64 {
	t.Helper()
	return scalar(t, conn, `SELECT coalesce((SELECT quantity FROM inventory
		WHERE entity_id = ? AND section = ? AND unit = ? AND tech_level = ?), 0);`,
		entityID, section, unit, techLevel)
}

func storedPopulation(t *testing.T, conn *sqlite.Conn, entityID int64, class string) int64 {
	t.Helper()
	return scalar(t, conn, `SELECT coalesce((SELECT quantity FROM entity_population
		WHERE entity_id = ? AND class = ?), 0);`, entityID, class)
}

func storedCadre(t *testing.T, conn *sqlite.Conn, entityID int64, name string) int64 {
	t.Helper()
	return scalar(t, conn, `SELECT coalesce((SELECT quantity FROM entity_cadre
		WHERE entity_id = ? AND cadre = ?), 0);`, entityID, name)
}

func storedMass(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	return scalar(t, conn, "SELECT mass FROM entity WHERE id = ?;", entityID)
}

func storedEnclosedVolume(t *testing.T, conn *sqlite.Conn, entityID int64) int64 {
	t.Helper()
	return scalar(t, conn, "SELECT enclosed_volume FROM entity WHERE id = ?;", entityID)
}

func scalar(t *testing.T, conn *sqlite.Conn, query string, args ...any) int64 {
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
