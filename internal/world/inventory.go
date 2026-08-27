// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world

import (
	"fmt"
	"slices"

	"github.com/mdhender/ecvb/internal/cadre"
	"github.com/mdhender/ecvb/internal/fuel"
	"github.com/mdhender/ecvb/internal/jumpdrive"
	"github.com/mdhender/ecvb/internal/sensors"
	"github.com/mdhender/ecvb/internal/transport"
	"github.com/mdhender/ecvb/internal/units"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// This file is the only thing in the game that writes the inventory table. It
// exists so that "what an entity holds" and "what an entity masses, encloses,
// and can propel" cannot drift apart: every mutation writes the row, corrects
// the loaded copy, and re-derives the drive and the sensors from the component
// section it just changed.

// Stack is one inventory row: a unit at a technology level, in one section of
// one entity.
type Stack struct {
	Section   string
	Unit      string
	TechLevel int
}

// Inventory is what an entity holds, one quantity per stack. An emptied stack
// loses its row rather than keeping a zero, here as in the database, so a zero
// and an absence are the same thing on both sides.
type Inventory map[Stack]int64

// Tag renders a stack's unit the way a player writes it.
func (s Stack) Tag() string {
	if !units.StoredHasTechLevel(s.TechLevel) {
		return s.Unit
	}
	return fmt.Sprintf("%s-%d", s.Unit, s.TechLevel)
}

// Held is how many of a unit an entity holds in a section.
func (e *Entity) Held(section, unit string, techLevel int) int64 {
	return e.Inventory[Stack{Section: section, Unit: unit, TechLevel: techLevel}]
}

// HeldEverywhere is how many of a unit an entity holds across every section.
// Only fuel reads it: fuel is burned rather than assembled, so it counts
// wherever it sits.
func (e *Entity) HeldEverywhere(unit string) int64 {
	total := int64(0)
	for stack, quantity := range e.Inventory {
		if stack.Unit == unit {
			total += quantity
		}
	}
	return total
}

// Fuel is the FUEL the entity holds, in every section. It is derived rather
// than stored so that no mutation can leave it stale.
func (e *Entity) Fuel() int64 { return e.HeldEverywhere(fuel.Unit) }

// stacksIn lists the stacks of one unit in one section, ordered by technology
// level, so a draw that walks them does the same thing every time it is run.
func (e *Entity) stacksIn(section, unit string) []Stack {
	var found []Stack
	for stack, quantity := range e.Inventory {
		if stack.Section == section && stack.Unit == unit && quantity > 0 {
			found = append(found, stack)
		}
	}
	slices.SortFunc(found, func(a, b Stack) int { return a.TechLevel - b.TechLevel })
	return found
}

// OccupiedVolume is the enclosed space the entity's inventory and population
// take up. Assembled structure creates space rather than filling it, and bulk
// resources in the cargo of a COPN or a CORB sit in external depots, so
// neither counts.
func (e *Entity) OccupiedVolume() int64 {
	total := int64(0)
	for stack, quantity := range e.Inventory {
		total += quantity * units.OccupiedVolumePerUnit(e.Unit, stack.Section, stack.Unit,
			stack.TechLevel, units.StoredHasTechLevel(stack.TechLevel))
	}
	for _, quantity := range e.Population {
		total += quantity * units.PopulationMetrics.CargoVolume
	}
	return total
}

// UsableEnclosedSpace is how much room the entity's assembled structure
// actually gives it, after the efficiency of its kind.
func (e *Entity) UsableEnclosedSpace() (int64, error) {
	return units.UsableEnclosedSpace(e.Unit, e.EnclosedVolume)
}

// Assigned is how many of a population class the entity's cadres have spoken
// for.
func (e *Entity) Assigned(class string) int64 {
	total := int64(0)
	for name, quantity := range e.Cadre {
		for _, part := range cadre.Composition(name) {
			if part.Name == class {
				total += quantity * part.PerUnit
			}
		}
	}
	return total
}

// Unassigned is the population of a class that is free to be given work.
//
// A cadre is an assignment of real people, so the people in it are not
// available for a second job: a ship with 100 SKW and 100 CWKR has no skilled
// worker free to crew a transport, and one with 50 CWKR has fifty. The count
// never reads as negative; a database whose cadre outruns its population reads
// as having nobody spare rather than as owing people.
func (e *Entity) Unassigned(class string) int64 {
	return max(e.Population[class]-e.Assigned(class), 0)
}

// ConstructionWorkers is the CWKR cadre the entity has assigned.
func (e *Entity) ConstructionWorkers() int64 { return e.Cadre[cadre.Unit] }

// Transports is the assembled TRAN the entity carries, highest technology
// level first, because a better hull carries more for the same crew and the
// same fuel.
func (e *Entity) Transports() []transport.Hulls {
	var hulls []transport.Hulls
	for _, stack := range e.stacksIn(units.SectionOperational, transport.Unit) {
		hulls = append(hulls, transport.Hulls{TechLevel: stack.TechLevel, Count: e.Inventory[stack]})
	}
	slices.Reverse(hulls)
	return hulls
}

// WorkDone is how much work of one kind an entity's construction workers have
// got through this turn. It is turn state, like the probe budget: a World is
// loaded for one operation and thrown away, so the pools start empty every
// turn without anything having to clear them.
func (w *World) WorkDone(kind string, entityID int64) int64 {
	return w.work[workKey{kind: kind, entityID: entityID}]
}

// WorkAllowed is how much more work of a kind an entity can still do this
// turn, given the cadre it has and what both its pools have already done.
func (w *World) WorkAllowed(kind string, entity *Entity) int64 {
	other := cadre.Assembly
	if kind == cadre.Assembly {
		other = cadre.Unassembly
	}
	return cadre.WorkAllowed(entity.ConstructionWorkers(),
		w.WorkDone(kind, entity.ID), w.WorkDone(other, entity.ID))
}

// RecordWork charges work against an entity's pool for the turn.
func (w *World) RecordWork(kind string, entityID int64, work int64) {
	w.work[workKey{kind: kind, entityID: entityID}] += work
}

// TransportsFree is the hulls an entity has left this turn: the transports it
// carries, less the ones already committed, and no more than its skilled
// workers can crew. Highest technology level first.
func (w *World) TransportsFree(entity *Entity) []transport.Hulls {
	var free []transport.Hulls
	for _, hull := range entity.Transports() {
		left := hull.Count - w.hulls[hullKey{entityID: entity.ID, techLevel: hull.TechLevel}]
		if left > 0 {
			free = append(free, transport.Hulls{TechLevel: hull.TechLevel, Count: left})
		}
	}
	// The crew is reckoned over the turn, not over the order: hulls already
	// committed are already crewed, so what is left is the entity's whole
	// complement less what is out.
	return transport.Limit(free,
		transport.CrewedHulls(entity.Unassigned(units.ClassSkilled))-w.hullsCommitted(entity.ID))
}

// Squares is the sum of t squared over the transports an entity has committed
// this turn, which is what its fuel bill is reckoned from.
func (w *World) Squares(entityID int64) int64 { return w.squares[entityID] }

// CommitTransports takes hulls out of an entity's pool for the turn and
// reports the extra FUEL that using them costs.
//
// The fuel is reckoned over every hull the entity used this turn rather than
// one hull at a time, so a second transfer that shares the round trip pays
// only what it adds to the total. That is the same shape as the work pools:
// the rounding happens once, over the turn.
func (w *World) CommitTransports(entity *Entity, used []transport.Hulls) int64 {
	before := transport.Fuel(w.squares[entity.ID])
	for _, hull := range used {
		w.hulls[hullKey{entityID: entity.ID, techLevel: hull.TechLevel}] += hull.Count
		w.squares[entity.ID] += hull.Count * int64(hull.TechLevel) * int64(hull.TechLevel)
	}
	return transport.Fuel(w.squares[entity.ID]) - before
}

// hullsCommitted is how many of an entity's transports are already out this
// turn.
func (w *World) hullsCommitted(entityID int64) int64 {
	committed := int64(0)
	for key, count := range w.hulls {
		if key.entityID == entityID {
			committed += count
		}
	}
	return committed
}

// Shift is one lot of units going from one section of an entity to another:
// assembling takes them out of unassembled inventory and puts them to work,
// unassembling brings them back, and stowing puts them in cargo.
type Shift struct {
	From      string
	To        string
	Unit      string
	TechLevel int
	Quantity  int64
}

// Tag renders a shift's unit the way a player writes it.
func (s Shift) Tag() string {
	return Stack{Unit: s.Unit, TechLevel: s.TechLevel}.Tag()
}

// RoomAfter is the space an entity would occupy and the space it would have if
// a set of shifts went through. Both are needed together, because a shift can
// change either: assembling a unit usually costs volume, and assembling
// structure creates it.
//
// It is arithmetic on what the entity holds rather than a trial and a rollback,
// so an order can ask before it does anything.
func (e *Entity) RoomAfter(shifts []Shift) (occupied, usable int64, err error) {
	occupied, enclosed := e.OccupiedVolume(), e.EnclosedVolume
	for _, shift := range shifts {
		has := units.StoredHasTechLevel(shift.TechLevel)
		occupied += shift.Quantity * (units.OccupiedVolumePerUnit(e.Unit, shift.To, shift.Unit, shift.TechLevel, has) -
			units.OccupiedVolumePerUnit(e.Unit, shift.From, shift.Unit, shift.TechLevel, has))
		enclosed += shift.Quantity * (units.EnclosedVolumePerUnit(shift.To, shift.Unit, shift.TechLevel) -
			units.EnclosedVolumePerUnit(shift.From, shift.Unit, shift.TechLevel))
	}
	usable, err = units.UsableEnclosedSpace(e.Unit, enclosed)
	return occupied, usable, err
}

// ShiftAll moves units between the sections of one entity.
//
// Nothing is created or destroyed, so the entity's mass does not change. What
// does change is the room it takes -- a unit costs more assembled than it does
// as freight -- and, for the six units that only work as components, what the
// entity can now do.
func (w *World) ShiftAll(entity *Entity, shifts []Shift) error {
	for _, shift := range shifts {
		if shift.Quantity < 0 {
			return fmt.Errorf("move %s on entity %d: quantity must be nonnegative", shift.Unit, entity.ID)
		}
		if shift.Quantity == 0 {
			continue
		}
		source := Stack{Section: shift.From, Unit: shift.Unit, TechLevel: shift.TechLevel}
		held := entity.Inventory[source]
		if held < shift.Quantity {
			return fmt.Errorf("move %s on entity %d: holds %d %s and needs %d",
				shift.Unit, entity.ID, held, shift.From, shift.Quantity)
		}
		if err := w.setStack(entity, source, held-shift.Quantity); err != nil {
			return err
		}
		target := Stack{Section: shift.To, Unit: shift.Unit, TechLevel: shift.TechLevel}
		if err := w.setStack(entity, target, entity.Inventory[target]+shift.Quantity); err != nil {
			return err
		}
	}
	return nil
}

// Hand moves units from one entity's cargo to another's. It is the only
// mutation that changes what an entity masses, because it is the only one
// where units leave the entity altogether.
func (w *World) Hand(from, to *Entity, unit string, techLevel int, quantity int64) error {
	if quantity < 0 {
		return fmt.Errorf("hand %s from entity %d: quantity must be nonnegative", unit, from.ID)
	}
	if quantity == 0 {
		return nil
	}
	stack := Stack{Section: units.SectionCargo, Unit: unit, TechLevel: techLevel}
	held := from.Inventory[stack]
	if held < quantity {
		return fmt.Errorf("hand %s from entity %d: holds %d in cargo and needs %d",
			unit, from.ID, held, quantity)
	}
	if err := w.setStack(from, stack, held-quantity); err != nil {
		return err
	}
	if err := w.setStack(to, stack, to.Inventory[stack]+quantity); err != nil {
		return err
	}
	mass := quantity * units.MetricsForStored(unit, techLevel).Mass
	if err := w.setMass(from, from.Mass-mass); err != nil {
		return err
	}
	return w.setMass(to, to.Mass+mass)
}

// HandPopulation moves population from one entity to another. Population is
// not inventory -- it has a table of its own and is never assembled -- but it
// rides the same transports and carries the same mass, so a transfer moves it
// the same way.
func (w *World) HandPopulation(from, to *Entity, class string, quantity int64) error {
	if quantity < 0 {
		return fmt.Errorf("hand %s from entity %d: quantity must be nonnegative", class, from.ID)
	}
	if quantity == 0 {
		return nil
	}
	held := from.Population[class]
	if held < quantity {
		return fmt.Errorf("hand %s from entity %d: holds %d and needs %d", class, from.ID, held, quantity)
	}
	if err := w.setPopulation(from, class, held-quantity); err != nil {
		return err
	}
	if err := w.setPopulation(to, class, to.Population[class]+quantity); err != nil {
		return err
	}
	mass := quantity * units.PopulationMetrics.Mass
	if err := w.setMass(from, from.Mass-mass); err != nil {
		return err
	}
	return w.setMass(to, to.Mass+mass)
}

// burn draws quantity of a unit out of an entity, section by section in the
// order given, and destroys it. It is what spending fuel is: the fuel leaves
// the entity, so the entity's mass falls with it.
func (w *World) burn(entity *Entity, unit string, quantity int64, sections []string) error {
	if quantity < 0 {
		return fmt.Errorf("burn %s on entity %d: quantity must be nonnegative", unit, entity.ID)
	}
	if quantity == 0 {
		return nil
	}
	// Count it all before drawing any, so a shortfall leaves the entity
	// untouched rather than half-drained.
	if held := entity.HeldEverywhere(unit); held < quantity {
		return fmt.Errorf("burn %s on entity %d: needs %d and holds %d", unit, entity.ID, quantity, held)
	}
	remaining, drawnMass := quantity, int64(0)
	for _, section := range sections {
		for _, stack := range entity.stacksIn(section, unit) {
			if remaining == 0 {
				break
			}
			drawn := min(entity.Inventory[stack], remaining)
			if err := w.setStack(entity, stack, entity.Inventory[stack]-drawn); err != nil {
				return err
			}
			// The mass is summed stack by stack rather than reckoned once over
			// the total, because two stacks of one unit may be at different
			// technology levels and mass differently.
			drawnMass += drawn * units.MetricsForStored(unit, stack.TechLevel).Mass
			remaining -= drawn
		}
	}
	if remaining != 0 {
		return fmt.Errorf("burn %s on entity %d: drew %d of %d", unit, entity.ID, quantity-remaining, quantity)
	}
	return w.setMass(entity, entity.Mass-drawnMass)
}

// setStack leaves one stack holding exactly quantity, in the database and in
// the loaded copy. An emptied stack loses its row rather than keeping a zero,
// so a zero and an absence are the same thing on both sides.
//
// Everything derived from what an entity holds is corrected here: the volume
// its assembled structure encloses, and the drive and sensors its component
// section adds up to. Nothing else recomputes them, which is what keeps a
// second order of a turn measuring the entity as the first order left it.
func (w *World) setStack(entity *Entity, stack Stack, quantity int64) error {
	if quantity < 0 {
		return fmt.Errorf("set entity %d %s %s: quantity must be nonnegative", entity.ID, stack.Section, stack.Unit)
	}
	statement := `
		INSERT INTO inventory (entity_id, section, unit, tech_level, quantity) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (entity_id, section, unit, tech_level) DO UPDATE SET quantity = excluded.quantity;`
	args := []any{entity.ID, stack.Section, stack.Unit, stack.TechLevel, quantity}
	if quantity == 0 {
		statement = "DELETE FROM inventory WHERE entity_id = ? AND section = ? AND unit = ? AND tech_level = ?;"
		args = args[:4]
	}
	if err := sqlitex.ExecuteTransient(w.conn, statement, &sqlitex.ExecOptions{Args: args}); err != nil {
		return fmt.Errorf("set entity %d %s %s: %w", entity.ID, stack.Section, stack.Tag(), err)
	}
	before := entity.Inventory[stack]
	if quantity == 0 {
		delete(entity.Inventory, stack)
	} else {
		entity.Inventory[stack] = quantity
	}
	if enclosed := units.EnclosedVolumePerUnit(stack.Section, stack.Unit, stack.TechLevel); enclosed != 0 {
		if err := w.setEnclosedVolume(entity, entity.EnclosedVolume+(quantity-before)*enclosed); err != nil {
			return err
		}
	}
	if stack.Section == units.SectionComponent {
		entity.recomputeAssemblies()
	}
	return nil
}

func (w *World) setMass(entity *Entity, mass int64) error {
	if mass < 0 {
		return fmt.Errorf("set entity %d mass: mass must be nonnegative", entity.ID)
	}
	if err := sqlitex.ExecuteTransient(w.conn, "UPDATE entity SET mass = ? WHERE id = ?;", &sqlitex.ExecOptions{
		Args: []any{mass, entity.ID},
	}); err != nil {
		return fmt.Errorf("set entity %d mass: %w", entity.ID, err)
	}
	if w.conn.Changes() != 1 {
		return fmt.Errorf("set entity %d mass: entity does not exist", entity.ID)
	}
	entity.Mass = mass
	return nil
}

func (w *World) setEnclosedVolume(entity *Entity, volume int64) error {
	if volume < 0 {
		return fmt.Errorf("set entity %d enclosed volume: volume must be nonnegative", entity.ID)
	}
	if err := sqlitex.ExecuteTransient(w.conn, "UPDATE entity SET enclosed_volume = ? WHERE id = ?;", &sqlitex.ExecOptions{
		Args: []any{volume, entity.ID},
	}); err != nil {
		return fmt.Errorf("set entity %d enclosed volume: %w", entity.ID, err)
	}
	if w.conn.Changes() != 1 {
		return fmt.Errorf("set entity %d enclosed volume: entity does not exist", entity.ID)
	}
	entity.EnclosedVolume = volume
	return nil
}

func (w *World) setPopulation(entity *Entity, class string, quantity int64) error {
	if quantity < 0 {
		return fmt.Errorf("set entity %d %s: quantity must be nonnegative", entity.ID, class)
	}
	statement := `
		INSERT INTO entity_population (entity_id, class, quantity) VALUES (?, ?, ?)
		ON CONFLICT (entity_id, class) DO UPDATE SET quantity = excluded.quantity;`
	args := []any{entity.ID, class, quantity}
	if quantity == 0 {
		statement = "DELETE FROM entity_population WHERE entity_id = ? AND class = ?;"
		args = args[:2]
	}
	if err := sqlitex.ExecuteTransient(w.conn, statement, &sqlitex.ExecOptions{Args: args}); err != nil {
		return fmt.Errorf("set entity %d %s: %w", entity.ID, class, err)
	}
	if quantity == 0 {
		delete(entity.Population, class)
	} else {
		entity.Population[class] = quantity
	}
	return w.settleCadres(entity)
}

// settleCadres cuts an entity's cadres back to the population left to fill
// them.
//
// A cadre is an assignment of real people and cannot outlive them. A ship with
// 100 SKW, 100 USK, and 100 CWKR that loses three skilled workers is left with
// 97 SKW, 100 USK, and 97 CWKR: the cadre falls with the population, and the
// three unskilled workers it was pairing them with go back to being
// unassigned. A ship with only 50 CWKR loses nothing but the workers, because
// it had fifty skilled workers spare to begin with.
//
// The other direction -- taking the cadre and the people in it together, which
// is what capturing or disbanding one does -- is combat's and the disband
// order's, and neither is written. It is the same rule read the other way
// round.
func (w *World) settleCadres(entity *Entity) error {
	for name, quantity := range entity.Cadre {
		allowed := quantity
		for _, part := range cadre.Composition(name) {
			if part.PerUnit <= 0 {
				continue
			}
			allowed = min(allowed, entity.Population[part.Name]/part.PerUnit)
		}
		if allowed >= quantity {
			continue
		}
		if err := w.setCadre(entity, name, allowed); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) setCadre(entity *Entity, name string, quantity int64) error {
	statement := `
		INSERT INTO entity_cadre (entity_id, cadre, quantity) VALUES (?, ?, ?)
		ON CONFLICT (entity_id, cadre) DO UPDATE SET quantity = excluded.quantity;`
	args := []any{entity.ID, name, quantity}
	if quantity == 0 {
		statement = "DELETE FROM entity_cadre WHERE entity_id = ? AND cadre = ?;"
		args = args[:2]
	}
	if err := sqlitex.ExecuteTransient(w.conn, statement, &sqlitex.ExecOptions{Args: args}); err != nil {
		return fmt.Errorf("set entity %d %s cadre: %w", entity.ID, name, err)
	}
	if quantity == 0 {
		delete(entity.Cadre, name)
	} else {
		entity.Cadre[name] = quantity
	}
	return nil
}

// recomputeAssemblies rebuilds what the entity's component section adds up to.
// A drive and a sensor array are the sum over their assembled units, so an
// order that assembles one changes what the entity can do in the same turn.
func (e *Entity) recomputeAssemblies() {
	e.Drive, e.Sensors = jumpdrive.Drive{}, sensors.Array{}
	for stack, quantity := range e.Inventory {
		if stack.Section != units.SectionComponent {
			continue
		}
		switch stack.Unit {
		case jumpdrive.Unit:
			e.Drive = e.Drive.Add(stack.TechLevel, quantity)
		case sensors.Unit:
			e.Sensors = e.Sensors.Add(stack.TechLevel, quantity)
		}
	}
}

func (w *World) loadInventory() error {
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT i.entity_id, i.section, i.unit, i.tech_level, i.quantity
		FROM inventory AS i
		JOIN entity AS e ON e.id = i.entity_id
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ? AND i.quantity > 0;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if entity, ok := w.entities[stmt.ColumnInt64(0)]; ok {
				entity.Inventory[Stack{Section: stmt.ColumnText(1), Unit: stmt.ColumnText(2),
					TechLevel: stmt.ColumnInt(3)}] = stmt.ColumnInt64(4)
			}
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load inventory: %w", err)
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT ep.entity_id, ep.class, ep.quantity
		FROM entity_population AS ep
		JOIN entity AS e ON e.id = ep.entity_id
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if entity, ok := w.entities[stmt.ColumnInt64(0)]; ok {
				entity.Population[stmt.ColumnText(1)] = stmt.ColumnInt64(2)
			}
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load population: %w", err)
	}
	if err := sqlitex.ExecuteTransient(w.conn, `
		SELECT ec.entity_id, ec.cadre, ec.quantity
		FROM entity_cadre AS ec
		JOIN entity AS e ON e.id = ec.entity_id
		JOIN faction AS f ON f.id = e.faction_id
		WHERE f.game_id = ?;`, &sqlitex.ExecOptions{
		Args: []any{w.game.ID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			if entity, ok := w.entities[stmt.ColumnInt64(0)]; ok {
				entity.Cadre[stmt.ColumnText(1)] = stmt.ColumnInt64(2)
			}
			return nil
		},
	}); err != nil {
		return fmt.Errorf("load cadres: %w", err)
	}
	for _, entity := range w.entities {
		entity.recomputeAssemblies()
	}
	return nil
}
